package run

import (
	"context"
	"strings"
	"time"

	"vergex/internal/account"
	"vergex/internal/logger"
	"vergex/internal/market/instrument"
	"vergex/internal/service"

	exactdecimal "github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// ledgerWait bounds copying a run's orders to the ledger.
const ledgerWait = 10 * time.Second

// ledgerChange is something the ledger has not heard about one of a run's orders.
type ledgerChange struct {
	seq   int
	order Order
	// placed is when the order has just become known to the exchange, ended when it has
	// just stopped there.
	placed, ended bool
}

// ledgerChanges compares the orders with what the database held before this write.
func ledgerChanges(v *View) []ledgerChange {
	var changes []ledgerChange
	for i, order := range v.Orders {
		if order.OrderID == "" {
			continue
		}
		var before Order
		if i < len(v.stored.orders) {
			before = v.stored.orders[i]
		}
		placed := before.OrderID == ""
		// An entry's fills settle it. A stop or target that fires fills under a new order
		// ID, so nothing else would.
		ended := order.Status != before.Status && OrderEnded(order.Status) && (order.Status != "FILLED" || order.Role != "entry")
		if placed || ended {
			changes = append(changes, ledgerChange{seq: i, order: order, placed: placed, ended: ended})
		}
	}
	return changes
}

// keepInLedger copies what is new about a run's orders to the ledger, whose fills and
// positions then follow by themselves. It runs in the background and failure is only
// logged, so nothing about it can touch execution: without it fills still arrive, and
// they find their owner through the position or the account's lease. The controller
// never reads the ledger back.
func (c *Controller) keepInLedger(v *View) {
	changes := ledgerChanges(v)
	if len(changes) == 0 {
		return
	}
	c.mu.Lock()
	if c.stopped {
		c.mu.Unlock()
		return
	}
	c.ledgerWG.Add(1)
	run := v.Run
	// A run's writes go in the order they happened, or an order could be settled before
	// the ledger knows it, and stay open there for good.
	before := c.ledgerLast[run.ID]
	done := make(chan struct{})
	if c.ledgerLast == nil {
		c.ledgerLast = map[string]<-chan struct{}{}
	}
	c.ledgerLast[run.ID] = done
	c.mu.Unlock()
	venue := v.Plan.Snapshot.Venue
	leverage := v.Plan.Constraints.EffectiveLeverage()
	service.Go("futures-ledger", func() {
		defer c.ledgerWG.Done()
		defer func() {
			close(done)
			c.mu.Lock()
			defer c.mu.Unlock()
			if c.ledgerLast[run.ID] == (<-chan struct{})(done) {
				delete(c.ledgerLast, run.ID)
			}
		}()
		if before != nil {
			<-before
		}
		ctx, cancel := context.WithTimeout(context.Background(), ledgerWait)
		defer cancel()
		db := c.DB.WithContext(ctx)
		for _, change := range changes {
			if err := recordChange(db, run, venue, leverage, change); err != nil {
				logger.Warnf("Futures: keep order %s of run %s in the ledger: %v", change.order.OrderID, run.ID, err)
			}
		}
	})
}

func recordChange(db *gorm.DB, run Run, venue string, leverage int, change ledgerChange) error {
	instrumentID, err := ledgerInstrument(db, venue, change.order.Symbol)
	if err != nil {
		return err
	}
	ledger := account.NewLedger(db)
	if change.placed {
		order, err := ledgerOrder(run, instrumentID, leverage, change)
		if err != nil {
			return err
		}
		if err := ledger.RecordOrder(order); err != nil {
			return err
		}
	}
	if change.ended {
		return ledger.SettleVenueOrder(run.AccountID, instrumentID, change.order.OrderID, change.order.Status)
	}
	return nil
}

func ledgerInstrument(db *gorm.DB, venue, symbol string) (int64, error) {
	return instrument.Ensure(db, venue, instrument.KindPerpetual, instrument.Normalize(symbol))
}

// ledgerOrder says in the ledger's terms what an order does. An Order's side is the
// direction of its plan, so a stop or target is the opposite trade.
func ledgerOrder(run Run, instrumentID int64, leverage int, change ledgerChange) (*account.LedgerOrder, error) {
	o := change.order
	quantity, err := exactdecimal.NewFromString(string(o.Quantity))
	if err != nil {
		return nil, err
	}
	price, err := exactdecimal.NewFromString(string(o.Price))
	if err != nil {
		return nil, err
	}
	seq := change.seq
	order := &account.LedgerOrder{
		AccountID: run.AccountID, InstrumentID: instrumentID, Source: account.SourceFutures, SourceID: run.ID, Seq: &seq, Role: o.Role,
		ClientID: o.ClientID, VenueClientID: o.VenueClientID, VenueOrderID: o.OrderID,
		PositionSide: o.Side, Status: account.OrderNew, Quantity: quantity, ReduceOnly: o.ReduceOnly, PostOnly: o.PostOnly,
	}
	if o.Role == "entry" {
		order.Side = tradeSide(o.Side)
		order.Type = strings.ToUpper(o.Type)
		if order.Type == "" {
			order.Type = "LIMIT"
			order.Price = exactdecimal.NewNullDecimal(price)
		}
		order.Leverage = &leverage
		return order, nil
	}
	order.Side = account.SideBuy
	if o.Side == account.SideLong {
		order.Side = account.SideSell
	}
	order.Type = "STOP_MARKET"
	if o.Role == "tp" {
		order.Type = "TAKE_PROFIT_MARKET"
	}
	order.StopPrice = exactdecimal.NewNullDecimal(price)
	return order, nil
}

func tradeSide(positionSide string) string {
	if positionSide == account.SideShort {
		return account.SideSell
	}
	return account.SideBuy
}

// RecordManualClose puts a manual close in the ledger. A failure is only logged: the
// order has been sent, and its fill arrives without this.
func RecordManualClose(db *gorm.DB, venue, accountID, requestID, positionSide string, o Order) {
	if o.OrderID == "" {
		return
	}
	err := func() error {
		instrumentID, err := ledgerInstrument(db, venue, o.Symbol)
		if err != nil {
			return err
		}
		quantity, err := exactdecimal.NewFromString(string(o.Quantity))
		if err != nil {
			return err
		}
		side := account.SideBuy
		if positionSide == account.SideLong {
			side = account.SideSell
		}
		return account.NewLedger(db).RecordOrder(&account.LedgerOrder{
			AccountID: accountID, InstrumentID: instrumentID, Source: account.SourceManual, SourceID: requestID, Role: account.RoleClose,
			ClientID: o.ClientID, VenueClientID: o.VenueClientID, VenueOrderID: o.OrderID, Side: side, PositionSide: positionSide,
			Type: "MARKET", Status: account.OrderNew, Quantity: quantity, ReduceOnly: true,
		})
	}()
	if err != nil {
		logger.Warnf("Futures: keep manual close %s in the ledger: %v", o.OrderID, err)
	}
}
