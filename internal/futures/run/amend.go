package run

import (
	"context"
	"errors"
	"fmt"
	"time"

	"vergex/internal/account"
	"vergex/internal/futures/plan"
)

// currentOrders leaves out what moving a stop or target made history: an order replaced by
// one the exchange took, and a replacement it refused. Whatever filled still counts in
// filledExposure, which reads every order.
func currentOrders(orders []Order) []Order {
	replaced := map[string]bool{}
	for _, order := range orders {
		if order.Replaces != "" && order.Status != "REJECTED" {
			replaced[order.Replaces] = true
		}
	}
	current := make([]Order, 0, len(orders))
	for _, order := range orders {
		if replaced[order.ClientID] || (order.Replaces != "" && order.Status == "REJECTED") {
			continue
		}
		current = append(current, order)
	}
	return current
}

const neverTakenReason = "The exchange did not take this order."

// neverTaken says a replacement was recorded but the exchange clearly never got it, as after
// a crash just before sending. Only a replacement can be dropped like this, since what it
// was to replace still stands.
func neverTaken(order Order, lookup error) bool {
	return order.Replaces != "" && order.OrderID == "" && errors.Is(lookup, account.ErrOrderNotFound)
}

// AmendExit moves a run's stop or target to a new price. Only a position with nothing more
// to come can be changed, so no later fill needs protecting at the old price.
//
// Each working order is replaced in turn, the new one placed before the old one goes, so the
// position is never without its stop: both reduce a position and neither can open one. The
// new order is durable before it is sent. If the exchange refuses it, the old order simply
// stays. If nobody can tell what happened, the run needs attention and the worker sorts it
// out, as it does after a restart: it never sends the order again.
func (c *Controller) AmendExit(ctx context.Context, user, id, role string, price plan.Decimal) (View, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if role != "sl" && role != "tp" {
		return View{}, errors.New("only a stop or a target can be moved")
	}
	lock := c.runLock(id)
	if err := lock.LockContext(ctx); err != nil {
		return View{}, err
	}
	defer lock.Unlock()
	v, err := c.Get(user, id)
	if err != nil {
		return View{}, err
	}
	if v.Status != "position_open" {
		return v, plan.Rejection{Key: "exitNotOpen", Text: "only an open, fully protected position can have its stop or target moved"}
	}
	broker, key, err := c.Factory(ctx, user, v.AccountID)
	if err != nil || key != v.AccountKey || !brokerFits(broker, v.Plan.Snapshot) {
		return v, errors.New("Could not verify the account; no order was sent")
	}
	book, err := c.book(ctx, v.Plan.Snapshot)
	if err != nil {
		return v, errors.New("Fresh market data is unavailable; no order was sent")
	}
	if err := plan.CheckExit(v.Plan, role, price, book, time.Now().UTC()); err != nil {
		return v, err
	}

	replaced := map[string]bool{}
	for _, order := range currentOrders(v.Orders) {
		replaced[order.ClientID] = true
	}
	working := []int{}
	for i, order := range v.Orders {
		if order.Role == role && order.Status == "NEW" && order.OrderID != "" && replaced[order.ClientID] {
			working = append(working, i)
		}
	}
	if len(working) == 0 {
		return v, plan.Rejection{Key: "exitNotOpen", Text: "there is no working order to move"}
	}
	// However this goes, a worker has to be there to finish it. After a restart there is none.
	c.launch(id, user)
	for _, i := range working {
		old := v.Orders[i]
		// Asking again for the price it already has does nothing.
		if cmp(old.Price, price) == 0 {
			continue
		}
		fresh := Order{ClientID: cid(v.ID, len(v.Orders), role, "replace/"+string(price)), Role: role, Symbol: old.Symbol, Side: old.Side, Price: price, Quantity: old.Quantity, Status: "submitting", Filled: "0", ParentID: old.ParentID, Replaces: old.ClientID}
		if fresh, err = routedOrder(broker, fresh); err != nil {
			return v, errors.New("Could not route the new order; none was sent")
		}
		v.Orders = append(v.Orders, fresh)
		if err := c.save(&v); err != nil {
			return v, errors.New("Could not record the change; no order was sent")
		}
		last := len(v.Orders) - 1
		receipt, err := broker.Place(ctx, fresh)
		if err == nil && !validReceipt(fresh, receipt) {
			err = errors.New("invalid protection acknowledgement")
		}
		var refused account.RefusedOrder
		if errors.As(err, &refused) {
			v.Orders[last].Status, v.Orders[last].Error = "REJECTED", refused.Reason
			_ = c.save(&v)
			return v, plan.Rejection{Key: "exitRefused", Params: map[string]string{"reason": refused.Reason}, Text: "The exchange refused the new order, and the old one stays: " + refused.Reason}
		}
		if err != nil {
			v.Orders[last].Status = "UNKNOWN"
			v.Status = "needs_attention"
			v.Error = "The new " + orderRoleName(role) + " order is unconfirmed, and the old one stays. No order will be sent again until it is reconciled. " + err.Error()
			_ = c.save(&v)
			return v, errors.New(v.Error)
		}
		v.Orders[last] = receipt
		if err := c.save(&v); err != nil {
			return v, errors.New("The new order was placed but could not be recorded; reconcile before anything else")
		}
		if err := broker.Cancel(ctx, old); err != nil {
			v.Status = "needs_attention"
			v.Error = fmt.Sprintf("The new %s order is working, but cancelling the old one is unconfirmed. Vergex keeps trying.", orderRoleName(role))
			_ = c.save(&v)
			return v, errors.New(v.Error)
		}
		v.Orders[i].Status = "CANCELED"
		if err := c.save(&v); err != nil {
			return v, err
		}
	}
	return v, nil
}

// finishReplacements cancels old orders still working once the order replacing them is. An
// interrupted change gets here after a restart.
func finishReplacements(ctx context.Context, broker Broker, v *View, attention func(string)) {
	taken := map[string]bool{}
	for _, order := range v.Orders {
		if order.Replaces != "" && order.OrderID != "" && order.Status == "NEW" {
			taken[order.Replaces] = true
		}
	}
	for i := range v.Orders {
		old := &v.Orders[i]
		if !taken[old.ClientID] || old.Status != "NEW" {
			continue
		}
		if err := broker.Cancel(ctx, *old); err != nil {
			attention("Cancelling a replaced " + orderRoleName(old.Role) + " order is unconfirmed; Vergex keeps trying.")
			continue
		}
		old.Status = "CANCELED"
	}
}
