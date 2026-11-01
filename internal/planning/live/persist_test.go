package live

import (
	"context"
	"errors"
	"testing"
	"time"

	"vergex/internal/market/instrument"
	"vergex/internal/planning"
	"vergex/internal/store"

	"github.com/google/uuid"
	exactdecimal "github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func TestAnOrderReadsBackExactlyAsWritten(t *testing.T) {
	c, _, _, p := setup(t, "limit")
	v := approved(t, c, p)
	written := Order{
		Type: "market", ReduceOnly: true, PostOnly: true, VenueClientID: "vergex-1", ParentID: "parent", ClientID: "client", OrderID: "9007199254740993",
		Role: "sl", Symbol: "BTC-USDT-SWAP", Side: "short", Price: "0.001000000000", Quantity: "12257.100", Status: "PARTIALLY_FILLED",
		Filled: "0.100000000000", Average: "0.1287878419678551", Error: "a reason",
	}
	row := toRunOrder(v.ID, 3, written)
	if err := c.DB.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	var read runOrder
	if err := c.DB.Where("run_id = ? AND seq = ?", v.ID, 3).First(&read).Error; err != nil {
		t.Fatal(err)
	}
	if got := read.order(); got != written {
		t.Fatalf("read back\n%+v\nwrote\n%+v", got, written)
	}
}

// Every order is durable before it can reach the exchange, entries and protection alike.
func TestAnOrderIsDurableBeforeItIsSent(t *testing.T) {
	c, b, _, p := setup(t, "limit")
	v := started(t, c, p)
	sent := 0
	b.onPlace = func(o Order) {
		sent++
		var status string
		if err := c.DB.Model(&runOrder{}).Where("run_id = ? AND client_id = ?", v.ID, o.ClientID).Pluck("status", &status).Error; err != nil {
			t.Error(err)
		}
		if status != "submitting" {
			t.Errorf("%s order sent while its row says %q", o.Role, status)
		}
	}
	c.step(context.Background(), testUser, v.ID)
	got, err := c.Get(testUser, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	entry := b.orders[got.Orders[0].ClientID]
	entry.Status, entry.Filled = "FILLED", entry.Quantity
	b.orders[entry.ClientID] = entry
	b.account.Position = entry.Quantity
	c.step(context.Background(), testUser, v.ID)
	if sent != 3 {
		t.Fatalf("sent %d orders, want the entry and its protection pair", sent)
	}
}

func TestNoOrderIsSentWhenItsIntentCannotBeSaved(t *testing.T) {
	c, b, _, p := setup(t, "limit")
	v := started(t, c, p)
	if err := c.DB.Exec(`ALTER TABLE frontier.run_orders RENAME TO run_orders_gone`).Error; err != nil {
		t.Fatal(err)
	}
	c.step(context.Background(), testUser, v.ID)
	if len(b.placed) != 0 {
		t.Fatalf("sent with no durable intent: %+v", b.placed)
	}
}

func TestASaveWithNothingNewLeavesTheRunUntouched(t *testing.T) {
	c, _, _, p := setup(t, "limit")
	v := started(t, c, p)
	c.step(context.Background(), testUser, v.ID)
	updatedAt := func() time.Time {
		var run Run
		if err := c.DB.Where("id = ?", v.ID).First(&run).Error; err != nil {
			t.Fatal(err)
		}
		return run.UpdatedAt
	}
	before := updatedAt()

	// Watching an order that has not moved changes nothing.
	c.step(context.Background(), testUser, v.ID)
	if after := updatedAt(); !after.Equal(before) {
		t.Fatalf("updated_at moved from %v to %v with nothing new", before, after)
	}

	got, err := c.Get(testUser, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	got.Error = "something new"
	if err := c.save(&got); err != nil {
		t.Fatal(err)
	}
	if after := updatedAt(); !after.After(before) {
		t.Fatal("a real change did not move updated_at")
	}
}

// ledgerOrders waits for the background copy.
func ledgerOrders(t *testing.T, c *Controller, runID string, want int) []store.LedgerOrder {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var orders []store.LedgerOrder
		if err := c.DB.Where("source = ? AND source_id = ?", store.SourceFrontier, runID).Order("seq").Find(&orders).Error; err != nil {
			t.Fatal(err)
		}
		if len(orders) >= want {
			return orders
		}
		if time.Now().After(deadline) {
			t.Fatalf("ledger orders = %d, want %d", len(orders), want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func protected(t *testing.T, c *Controller, b *testBroker, runID string) View {
	t.Helper()
	c.step(context.Background(), testUser, runID)
	got, err := c.Get(testUser, runID)
	if err != nil {
		t.Fatal(err)
	}
	entry := b.orders[got.Orders[0].ClientID]
	entry.Status, entry.Filled = "FILLED", entry.Quantity
	b.orders[entry.ClientID] = entry
	b.account.Position = entry.Quantity
	c.step(context.Background(), testUser, runID)
	got, err = c.Get(testUser, runID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestARunsOrdersReachTheLedgerAsTheTradesTheyAre(t *testing.T) {
	c, b, _, p := setup(t, "limit")
	v := started(t, c, p)
	got := protected(t, c, b, v.ID)
	orders := ledgerOrders(t, c, v.ID, 3)
	if len(orders) != 3 {
		t.Fatalf("orders = %+v", orders)
	}

	entry, stop, target := orders[0], orders[1], orders[2]
	if entry.Role != store.RoleEntry || entry.Side != store.SideBuy || entry.PositionSide != store.SideLong || entry.Type != "LIMIT" || !entry.Price.Valid || entry.ExchangeOrderID != got.Orders[0].OrderID || entry.ClientOrderID != got.Orders[0].ClientID {
		t.Fatalf("entry = %+v", entry)
	}
	if entry.Leverage == nil || *entry.Leverage != p.Constraints.EffectiveLeverage() || entry.Seq == nil || *entry.Seq != 0 {
		t.Fatalf("entry leverage or place: %+v", entry)
	}
	if stop.Role != store.RoleStop || stop.Side != store.SideSell || stop.Type != "STOP_MARKET" || !stop.StopPrice.Valid || stop.Price.Valid || stop.ParentClientOrderID != entry.ClientOrderID {
		t.Fatalf("stop = %+v", stop)
	}
	if target.Role != store.RoleTake || target.Side != store.SideSell || target.Type != "TAKE_PROFIT_MARKET" {
		t.Fatalf("target = %+v", target)
	}
}

func TestAnOrderThatEndedUnfilledIsSettledInTheLedger(t *testing.T) {
	c, b, _, p := setup(t, "limit")
	v := started(t, c, p)
	c.step(context.Background(), testUser, v.ID)
	got, err := c.Get(testUser, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	ledgerOrders(t, c, v.ID, 1)
	entry := b.orders[got.Orders[0].ClientID]
	entry.Status = "CANCELED"
	b.orders[entry.ClientID] = entry
	c.step(context.Background(), testUser, v.ID)

	deadline := time.Now().Add(10 * time.Second)
	for {
		orders := ledgerOrders(t, c, v.ID, 1)
		if orders[0].Status == store.OrderCanceled && orders[0].ClosedAt != nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("order = %+v", orders[0])
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// The ledger counts fills by adding them up, and the controller trusts only the
// exchange's receipts, so neither may write what the other reads.
func TestALedgerFillLeavesTheRunsOwnStateAlone(t *testing.T) {
	c, _, _, p := setup(t, "limit")
	v := started(t, c, p)
	c.step(context.Background(), testUser, v.ID)
	orders := ledgerOrders(t, c, v.ID, 1)
	before, err := c.Get(testUser, v.ID)
	if err != nil {
		t.Fatal(err)
	}

	btc, err := instrument.Ensure(c.DB, "binance", instrument.KindPerpetual, "BTCUSDT", "BTCUSDT")
	if err != nil {
		t.Fatal(err)
	}
	ledger := store.NewLedgerStore(c.DB)
	half := orders[0].Quantity.Div(exactdecimal.NewFromInt(2))
	if _, err := ledger.RecordTrade(store.Trade{
		AccountID: testAccount, InstrumentID: btc, ExchangeTradeID: "t-1", ExchangeOrderID: orders[0].ExchangeOrderID, Side: store.SideBuy, PositionSide: store.SideLong,
		Price: exactdecimal.NewFromInt(100000), Quantity: half, QuoteQuantity: half.Mul(exactdecimal.NewFromInt(100000)), ExecutedAt: time.Now(),
	}, store.Owner{}); err != nil {
		t.Fatal(err)
	}

	after, err := c.Get(testUser, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Orders[0] != before.Orders[0] || after.Orders[0].Status != "NEW" {
		t.Fatalf("the fill reached the run's state: %+v", after.Orders[0])
	}
	// And the ledger has it under the run, for what the plan went on to realize.
	var position store.LedgerPosition
	if err := c.DB.Where("source = ? AND source_id = ?", store.SourceFrontier, v.ID).First(&position).Error; err != nil {
		t.Fatalf("position: %v", err)
	}
	if !position.Quantity.Equal(half) {
		t.Fatalf("position = %+v", position)
	}
}

func TestALedgerThatCannotBeWrittenDoesNotStopARun(t *testing.T) {
	c, b, _, p := setup(t, "limit")
	v := started(t, c, p)
	if err := c.DB.Exec(`ALTER TABLE trading.orders RENAME TO orders_gone`).Error; err != nil {
		t.Fatal(err)
	}
	got := protected(t, c, b, v.ID)
	if len(b.placed) != 3 {
		t.Fatalf("the run stopped at %d orders: %+v", len(b.placed), got)
	}
	if got.State != "position_open" && got.State != "running" && got.State != "stopping" {
		t.Fatalf("state = %q, %s", got.State, got.Error)
	}
}

func TestAnAccountWithOnlyAnUnusedApprovalCanStillBeDeleted(t *testing.T) {
	c, _, _, p := setup(t, "limit")
	exchanges := store.NewExchangeStore(c.DB)

	v := approved(t, c, p)
	if err := exchanges.Delete(testUser, testAccount); !errors.Is(err, gorm.ErrForeignKeyViolated) {
		t.Fatalf("an account a run holds: %v", err)
	}
	if _, err := c.Stop(testUser, v.ID, v.ApprovalHash); err != nil {
		t.Fatal(err)
	}
	if err := exchanges.Delete(testUser, testAccount); err != nil {
		t.Fatalf("nothing happened on this account: %v", err)
	}
	var runs, plans int64
	c.DB.Model(&Run{}).Count(&runs)
	c.DB.Model(&planning.Draft{}).Where("account_id IS NOT NULL").Count(&plans)
	if runs != 0 || plans != 0 {
		t.Fatalf("runs = %d, plans still on it = %d", runs, plans)
	}
}

func TestAnIDThatIsNotARunFindsNothing(t *testing.T) {
	c, _, _, _ := setup(t, "limit")
	for _, id := range []string{"run", uuid.NewString()} {
		if _, err := c.Get(testUser, id); err == nil || err.Error() != "execution not found or not owned" {
			t.Fatalf("%q: %v", id, err)
		}
	}
}

// A restart moves every unfinished run, but it says nothing new about their orders.
func TestARefusedOrderKeepsItsOwnTimeAcrossARestart(t *testing.T) {
	c, _, _, p := setup(t, "limit")
	v := started(t, c, p)
	got, err := c.Get(testUser, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	got.Orders[0].Error = "refused"
	if err := c.save(&got); err != nil {
		t.Fatal(err)
	}
	st, err := store.NewMigrated(c.DB)
	if err != nil {
		t.Fatal(err)
	}
	since := time.Now().Add(-time.Hour)
	before, err := RefusedSince(st, since, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 1 || before[0].RunID != v.ID || before[0].PlanID != v.PlanID || before[0].AccountID != testAccount || before[0].Error != "refused" {
		t.Fatalf("refused = %+v", before)
	}

	if _, err := New(st, c.Factory, c.Market); err != nil {
		t.Fatal(err)
	}
	var run Run
	if err := c.DB.Where("id = ?", v.ID).First(&run).Error; err != nil {
		t.Fatal(err)
	}
	if run.State != "needs_attention" || !run.UpdatedAt.After(before[0].UpdatedAt) {
		t.Fatalf("the restart left the run at %s, %v", run.State, run.UpdatedAt)
	}
	after, err := RefusedSince(st, since, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 || !after[0].UpdatedAt.Equal(before[0].UpdatedAt) {
		t.Fatalf("refused order moved from %v to %+v", before[0].UpdatedAt, after)
	}
}
