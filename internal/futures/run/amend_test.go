package run

import (
	"context"
	"errors"
	"testing"

	"vergex/internal/account"
	"vergex/internal/futures/plan"
)

// openPosition is a run holding a filled entry with its stop and target working.
func openPosition(t *testing.T) (*Controller, *testBroker, *testMarket, View) {
	t.Helper()
	c, b, m, p := setup(t, "limit")
	c.steppedByHand = true
	v := started(t, c, p)
	protected(t, c, b, v.ID)
	c.step(context.Background(), testUser, v.ID)
	got, err := c.Get(testUser, v.ID)
	if err != nil || got.Status != "position_open" || len(got.Orders) != 3 {
		t.Fatalf("run = %s, orders %+v, err %v", got.Status, got.Orders, err)
	}
	return c, b, m, got
}

func orderOf(t *testing.T, v View, role, status string) Order {
	t.Helper()
	found := []Order{}
	for _, order := range v.Orders {
		if order.Role == role && order.Status == status {
			found = append(found, order)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%s %s orders = %+v", role, status, v.Orders)
	}
	return found[0]
}

func TestMovingAStopPlacesTheNewOneBeforeCancellingTheOld(t *testing.T) {
	c, b, _, v := openPosition(t)
	oldStop := v.Orders[1]
	placedBefore := len(b.placed)
	var cancelledWhenPlaced int
	b.onPlace = func(Order) { cancelledWhenPlaced = len(b.cancelled) }

	got, err := c.AmendExit(context.Background(), testUser, v.ID, "sl", "99500")
	if err != nil {
		t.Fatal(err)
	}
	if len(b.placed) != placedBefore+1 || cancelledWhenPlaced != 0 || len(b.cancelled) != 1 || b.cancelled[0] != oldStop.ClientID {
		t.Fatalf("placed %d, cancelled %v, %d cancelled when placing", len(b.placed)-placedBefore, b.cancelled, cancelledWhenPlaced)
	}
	fresh := orderOf(t, got, "sl", "NEW")
	if fresh.Price != "99500" || fresh.Replaces != oldStop.ClientID || fresh.Quantity != oldStop.Quantity || fresh.ParentID != oldStop.ParentID || fresh.ClientID == oldStop.ClientID {
		t.Fatalf("new stop = %+v", fresh)
	}
	if got.Status != "position_open" || got.Plan.Idea.Stop != "99000" {
		t.Fatalf("run = %s, plan stop %s", got.Status, got.Plan.Idea.Stop)
	}

	// It reads back as written, and the worker sees a protected position with nothing to add.
	placed := len(b.placed)
	c.step(context.Background(), testUser, v.ID)
	after, err := c.Get(testUser, v.ID)
	if err != nil || after.Status != "position_open" || len(b.placed) != placed {
		t.Fatalf("after a step: %s, %d placed, err %v", after.Status, len(b.placed)-placed, err)
	}

	// The ledger has the new order and the old one's end.
	c.ledgerWG.Wait()
	orders := ledgerOrders(t, c, v.ID, 4)
	if orders[1].Status != account.OrderCanceled || orders[3].Status != account.OrderNew || orders[3].Role != account.RoleStop {
		t.Fatalf("ledger = %+v", orders)
	}
}

// After a restart nothing is watching a run until someone asks.
func TestMovingAnExitPutsAWorkerOnTheRun(t *testing.T) {
	c, _, _, v := openPosition(t)
	c.steppedByHand = false
	if _, err := c.AmendExit(context.Background(), testUser, v.ID, "sl", "99500"); err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	_, watched := c.workers[v.ID]
	c.mu.Unlock()
	if !watched {
		t.Fatal("no worker to finish the change or notice a stop")
	}
}

func TestMovingAnExitCanBeRepeatedOrAskedTwice(t *testing.T) {
	c, b, _, v := openPosition(t)
	if _, err := c.AmendExit(context.Background(), testUser, v.ID, "tp", "105000"); err != nil {
		t.Fatal(err)
	}
	placed := len(b.placed)
	if _, err := c.AmendExit(context.Background(), testUser, v.ID, "tp", "105000"); err != nil {
		t.Fatal(err)
	}
	if len(b.placed) != placed {
		t.Fatal("the same price sent another order")
	}
	got, err := c.AmendExit(context.Background(), testUser, v.ID, "tp", "106000")
	if err != nil {
		t.Fatal(err)
	}
	if orderOf(t, got, "tp", "NEW").Price != "106000" || got.Status != "position_open" {
		t.Fatalf("after moving twice: %s, %+v", got.Status, got.Orders)
	}
	c.step(context.Background(), testUser, v.ID)
	if after, err := c.Get(testUser, v.ID); err != nil || after.Status != "position_open" {
		t.Fatalf("after a step: %+v, err %v", after.Status, err)
	}
}

func TestAnExitMovesOnlyWhereItCannotFireAtOnce(t *testing.T) {
	c, b, _, v := openPosition(t)
	placed := len(b.placed)
	for _, tc := range []struct {
		role  string
		price plan.Decimal
		key   string
	}{
		{"sl", "100000", "exitSide"},
		{"tp", "99000", "exitSide"},
		{"sl", "99500.5", "tick"},
		{"sl", "0", "exitPrice"},
	} {
		_, err := c.AmendExit(context.Background(), testUser, v.ID, tc.role, tc.price)
		var rejection plan.Rejection
		if !errors.As(err, &rejection) || rejection.Key != tc.key {
			t.Errorf("%s at %s = %v", tc.role, tc.price, err)
		}
	}
	if _, err := c.AmendExit(context.Background(), testUser, v.ID, "entry", "99000"); err == nil {
		t.Error("moved an entry")
	}
	if len(b.placed) != placed {
		t.Fatalf("sent %d orders", len(b.placed)-placed)
	}
}

func TestOnlyAnOpenProtectedPositionHasItsExitMoved(t *testing.T) {
	c, b, _, p := setup(t, "limit")
	v := started(t, c, p)
	c.step(context.Background(), testUser, v.ID)
	placed := len(b.placed)
	_, err := c.AmendExit(context.Background(), testUser, v.ID, "sl", "99500")
	var rejection plan.Rejection
	if !errors.As(err, &rejection) || rejection.Key != "exitNotOpen" || len(b.placed) != placed {
		t.Fatalf("err = %v, %d sent", err, len(b.placed)-placed)
	}
}

func TestARefusedMoveLeavesTheOldStopAlone(t *testing.T) {
	c, b, _, v := openPosition(t)
	b.errorPlace = account.RefusedOrder{Reason: "would trigger immediately"}
	got, err := c.AmendExit(context.Background(), testUser, v.ID, "sl", "99500")
	var rejection plan.Rejection
	if !errors.As(err, &rejection) || rejection.Key != "exitRefused" {
		t.Fatalf("err = %v", err)
	}
	if len(b.cancelled) != 0 || got.Status != "position_open" || orderOf(t, got, "sl", "NEW").ClientID != v.Orders[1].ClientID {
		t.Fatalf("run = %s, cancelled %v, orders %+v", got.Status, b.cancelled, got.Orders)
	}
	if refused := orderOf(t, got, "sl", "REJECTED"); refused.Error == "" {
		t.Fatalf("refused = %+v", refused)
	}

	// The worker carries on with the old one, and a later move still works.
	b.errorPlace = nil
	c.step(context.Background(), testUser, v.ID)
	if after, err := c.Get(testUser, v.ID); err != nil || after.Status != "position_open" {
		t.Fatalf("after a step: %s, err %v", after.Status, err)
	}
	if got, err = c.AmendExit(context.Background(), testUser, v.ID, "sl", "99600"); err != nil {
		t.Fatal(err)
	}
	if orderOf(t, got, "sl", "NEW").Price != "99600" {
		t.Fatalf("orders = %+v", got.Orders)
	}
}

// The exchange took it, but the answer never came back.
func TestALostAnswerIsReconciledAndNeverSentAgain(t *testing.T) {
	c, b, _, v := openPosition(t)
	b.errorPlace = errors.New("connection reset")
	got, err := c.AmendExit(context.Background(), testUser, v.ID, "sl", "99500")
	if err == nil || got.Status != "needs_attention" || len(b.cancelled) != 0 {
		t.Fatalf("err = %v, run = %s, cancelled %v", err, got.Status, b.cancelled)
	}
	orderOf(t, got, "sl", "UNKNOWN")
	b.errorPlace = nil
	placed := len(b.placed)

	// While it is unresolved, nothing else may be moved.
	if _, err := c.AmendExit(context.Background(), testUser, v.ID, "tp", "105000"); err == nil || len(b.placed) != placed {
		t.Fatalf("moved during attention: %v", err)
	}
	c.step(context.Background(), testUser, v.ID)
	after, err := c.Get(testUser, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.placed) != placed {
		t.Fatal("the worker sent it again")
	}
	if orderOf(t, after, "sl", "NEW").Price != "99500" || len(b.cancelled) != 1 || b.cancelled[0] != v.Orders[1].ClientID {
		t.Fatalf("orders = %+v, cancelled %v", after.Orders, b.cancelled)
	}
	c.step(context.Background(), testUser, v.ID)
	if after, err = c.Get(testUser, v.ID); err != nil || after.Status != "position_open" {
		t.Fatalf("after reconciling: %s, err %v", after.Status, err)
	}
}

// A crash between recording the new order and sending it.
func TestANewOrderThatNeverLeftIsDroppedAndTheOldStays(t *testing.T) {
	c, b, _, v := openPosition(t)
	old := v.Orders[1]
	fresh := Order{ClientID: cid(v.ID, len(v.Orders), "sl", "replace/99500"), Role: "sl", Symbol: old.Symbol, Side: old.Side, Price: "99500", Quantity: old.Quantity, Status: "submitting", Filled: "0", ParentID: old.ParentID, Replaces: old.ClientID}
	v.Orders = append(v.Orders, fresh)
	if err := c.save(&v); err != nil {
		t.Fatal(err)
	}
	placed := len(b.placed)

	// While the exchange's answer is unclear, the run waits.
	c.step(context.Background(), testUser, v.ID)
	got, err := c.Get(testUser, v.ID)
	if err != nil || got.Status != "needs_attention" || len(b.placed) != placed {
		t.Fatalf("run = %s, err %v, sent %d", got.Status, err, len(b.placed)-placed)
	}

	// Once it says it has no such order, the old stop is the stop again.
	b.missingIsKnown = true
	c.step(context.Background(), testUser, v.ID)
	c.step(context.Background(), testUser, v.ID)
	got, err = c.Get(testUser, v.ID)
	if err != nil || got.Status != "position_open" || len(b.placed) != placed || len(b.cancelled) != 0 {
		t.Fatalf("run = %s (%s), err %v, sent %d, cancelled %v", got.Status, got.Error, err, len(b.placed)-placed, b.cancelled)
	}
	if orderOf(t, got, "sl", "NEW").ClientID != old.ClientID || orderOf(t, got, "sl", "REJECTED").ClientID != fresh.ClientID {
		t.Fatalf("orders = %+v", got.Orders)
	}
}

func TestAnOldStopThatWouldNotCancelIsCancelledLater(t *testing.T) {
	c, b, _, v := openPosition(t)
	b.cancelErr = errors.New("timeout")
	got, err := c.AmendExit(context.Background(), testUser, v.ID, "sl", "99500")
	if err == nil || got.Status != "needs_attention" {
		t.Fatalf("err = %v, run = %s", err, got.Status)
	}
	c.step(context.Background(), testUser, v.ID)
	if still, _ := c.Get(testUser, v.ID); still.Status != "needs_attention" {
		t.Fatalf("run = %s with the cancel still failing", still.Status)
	}
	b.cancelErr = nil
	placed := len(b.placed)
	c.step(context.Background(), testUser, v.ID)
	c.step(context.Background(), testUser, v.ID)
	after, err := c.Get(testUser, v.ID)
	if err != nil || after.Status != "position_open" || len(b.placed) != placed || after.Orders[1].Status != "CANCELED" {
		t.Fatalf("run = %s, err %v, sent %d, old %s", after.Status, err, len(b.placed)-placed, after.Orders[1].Status)
	}
}

func TestAMovedStopThatFiresFinishesTheRun(t *testing.T) {
	c, b, _, v := openPosition(t)
	got, err := c.AmendExit(context.Background(), testUser, v.ID, "sl", "99500")
	if err != nil {
		t.Fatal(err)
	}
	stop := b.orders[orderOf(t, got, "sl", "NEW").ClientID]
	stop.Status, stop.Filled = "FILLED", stop.Quantity
	b.orders[stop.ClientID] = stop
	b.account.Position = "0"
	if !c.step(context.Background(), testUser, v.ID) {
		t.Fatal("the run did not finish")
	}
	out, err := c.Get(testUser, v.ID)
	if err != nil || out.Status != "completed" {
		t.Fatalf("run = %s (%s), err %v", out.Status, out.Error, err)
	}
	if orderOf(t, out, "tp", "CANCELED").Role != "tp" {
		t.Fatalf("orders = %+v", out.Orders)
	}
}

// The old stop fires just as it is being replaced: the cancel fails, the position is gone,
// and what is left on the exchange has to go too.
func TestAStopThatFiresWhileBeingMovedLeavesNothingBehind(t *testing.T) {
	c, b, _, v := openPosition(t)
	oldStop := v.Orders[1]
	b.cancelErr = errors.New("already triggered")
	got, err := c.AmendExit(context.Background(), testUser, v.ID, "sl", "99500")
	if err == nil || got.Status != "needs_attention" {
		t.Fatalf("err = %v, run = %s", err, got.Status)
	}
	fired := b.orders[oldStop.ClientID]
	fired.Status, fired.Filled = "FILLED", fired.Quantity
	b.orders[oldStop.ClientID] = fired
	b.account.Position = "0"
	b.cancelErr = nil
	fresh, target := got.Orders[len(got.Orders)-1], v.Orders[2]
	for range 3 {
		if c.step(context.Background(), testUser, v.ID) {
			break
		}
	}
	out, err := c.Get(testUser, v.ID)
	if err != nil || out.Status != "completed" {
		t.Fatalf("run = %s (%s), err %v, orders %+v", out.Status, out.Error, err, out.Orders)
	}
	if b.orders[fresh.ClientID].Status != "CANCELED" || b.orders[target.ClientID].Status != "CANCELED" {
		t.Fatalf("left on the exchange: %+v", b.orders)
	}
}

// Nothing stored can claim a move that does not line up with what it replaces.
func TestAReplacementMustMatchWhatItReplaces(t *testing.T) {
	_, _, _, v := openPosition(t)
	stop, target := v.Orders[1], v.Orders[2]
	for name, replacement := range map[string]Order{
		"nothing":      {Replaces: "nothing", Role: "sl", Quantity: stop.Quantity, ParentID: stop.ParentID},
		"another role": {Replaces: target.ClientID, Role: "sl", Quantity: stop.Quantity, ParentID: stop.ParentID},
		"more of it":   {Replaces: stop.ClientID, Role: "sl", Quantity: "1", ParentID: stop.ParentID},
	} {
		replacement.ClientID, replacement.Symbol, replacement.Side, replacement.Price, replacement.Status, replacement.Filled = cid(v.ID, 9, name, ""), stop.Symbol, stop.Side, "99500", "NEW", "0"
		tampered := v
		tampered.Orders = append(append([]Order{}, v.Orders...), replacement)
		if err := validateIntent(tampered); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}
