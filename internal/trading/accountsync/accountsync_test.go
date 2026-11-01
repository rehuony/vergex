package accountsync

import (
	"context"
	"errors"
	"testing"
	"time"
	"vergex/internal/market/instrument"
	"vergex/internal/store"
	"vergex/internal/store/storetest"
	"vergex/internal/trading"

	"github.com/shopspring/decimal"
)

type fakeSource struct {
	fills   []trading.Fill
	err     error
	cursors []trading.FillCursor

	open     map[string][]trading.OpenOrder // per symbol
	statuses map[string]string              // per order ID
	asked    []string                       // what was asked of the exchange, in order
}

func (f *fakeSource) GetOpenOrders(symbol string) ([]trading.OpenOrder, error) {
	f.asked = append(f.asked, "open "+symbol)
	return f.open[symbol], nil
}

func (f *fakeSource) GetOrderStatus(symbol, orderID string) (map[string]interface{}, error) {
	f.asked = append(f.asked, "status "+orderID)
	status, ok := f.statuses[orderID]
	if !ok {
		return nil, errors.New("no such order")
	}
	return map[string]interface{}{"status": status}, nil
}

func (f *fakeSource) FillsSince(cursor trading.FillCursor) ([]trading.Fill, error) {
	f.cursors = append(f.cursors, cursor)
	return f.fills, f.err
}

func fill(id, side, positionSide, quantity string, at time.Time) trading.Fill {
	return trading.Fill{
		TradeID: id, OrderID: "order-" + id, Symbol: "BTCUSDT", NativeSymbol: "BTCUSDT", Side: side, PositionSide: positionSide,
		Price: decimal.NewFromInt(100), Quantity: decimal.RequireFromString(quantity), QuoteQuantity: decimal.NewFromInt(100),
		Commission: decimal.RequireFromString("0.1"), CommissionAsset: "USDT", Time: at,
	}
}

func newFixture(t *testing.T) (*Service, *store.Store, *store.Exchange, *fakeSource) {
	t.Helper()
	st := storetest.New(t)
	t.Cleanup(func() { _ = st.Close() })
	user := storetest.Account(t, st)
	id, err := st.Exchange().Create(user, "binance", "Main", "key", "secret", "")
	if err != nil {
		t.Fatal(err)
	}
	ex, err := st.Exchange().Get(id)
	if err != nil {
		t.Fatal(err)
	}
	source := &fakeSource{}
	s := New(st)
	s.open = func(context.Context, *store.Exchange) (Source, error) { return source, nil }
	return s, st, ex, source
}

func positions(t *testing.T, st *store.Store) []store.LedgerPosition {
	t.Helper()
	var rows []store.LedgerPosition
	if err := st.GormDB().Order("id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestSyncBooksAnAccountsTradesOnceOldestFirst(t *testing.T) {
	s, st, ex, source := newFixture(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	// The exchange can list them in any order, and the close must come after the open.
	source.fills = []trading.Fill{
		fill("102", store.SideSell, store.SideLong, "1", now),
		fill("101", store.SideBuy, store.SideLong, "1", now.Add(-time.Minute)),
	}
	for range 2 {
		if err := s.Sync(ex.ID); err != nil {
			t.Fatal(err)
		}
	}
	rows := positions(t, st)
	if len(rows) != 1 || rows[0].Status != store.PositionClosed || rows[0].Source != store.SourceExternal {
		t.Fatalf("positions = %+v", rows)
	}
	var fills int64
	st.GormDB().Model(&store.LedgerFill{}).Count(&fills)
	if fills != 2 {
		t.Fatalf("fills = %d", fills)
	}

	// The first read starts a day back, and the next from where that one got to.
	first, second := source.cursors[0], source.cursors[1]
	if time.Since(first.Since) < 23*time.Hour {
		t.Errorf("first cursor = %+v", first)
	}
	if time.Since(second.Since) > 2*readOverlap {
		t.Errorf("second cursor = %+v", second)
	}
}

func TestAfterAnyDowntimeReadingPicksUpFromTheLastTrade(t *testing.T) {
	s, st, ex, source := newFixture(t)
	last := time.Now().UTC().Add(-72 * time.Hour).Truncate(time.Millisecond)
	ledger := store.NewLedgerStore(st.GormDB())
	btc, err := instrument.Ensure(st.GormDB(), "binance", instrument.KindPerpetual, "BTCUSDT", "BTCUSDT")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.RecordTrade(store.Trade{
		AccountID: ex.ID, InstrumentID: btc, ExchangeTradeID: "9", ExchangeOrderID: "9", Side: store.SideBuy, PositionSide: store.SideLong,
		Price: decimal.NewFromInt(100), Quantity: decimal.NewFromInt(1), QuoteQuantity: decimal.NewFromInt(100), ExecutedAt: last,
	}, store.Owner{Source: store.SourceTrader, SourceID: "bot"}); err != nil {
		t.Fatal(err)
	}

	// A new service is what a restart makes.
	if err := s.Sync(ex.ID); err != nil {
		t.Fatal(err)
	}
	first := source.cursors[0]
	if !first.Since.Equal(last.Add(-readOverlap)) {
		t.Errorf("read from %s, want %s", first.Since, last.Add(-readOverlap))
	}
	// The position is still open here, so its symbol is asked about even if nothing else
	// says it traded: it may have closed while this was not running.
	if len(first.OpenSymbols) != 1 || first.OpenSymbols[0] != "BTCUSDT" {
		t.Errorf("cursor = %+v", first)
	}

	// Having read that far, the next read does not go back there again.
	if err := s.Sync(ex.ID); err != nil {
		t.Fatal(err)
	}
	if since := source.cursors[1].Since; time.Since(since) > 2*readOverlap {
		t.Errorf("second read from %s", since)
	}
}

func TestAFailedReadDoesNotMoveWhereTheNextOneStarts(t *testing.T) {
	s, _, ex, source := newFixture(t)
	now := time.Now().UTC()
	// One symbol fails, and another's newer trade is booked.
	source.fills = []trading.Fill{fill("1", store.SideBuy, store.SideLong, "1", now)}
	source.err = errors.New("read trades of ETHUSDT: down")
	if err := s.Sync(ex.ID); err == nil {
		t.Fatal("the failure should be reported")
	}
	start := source.cursors[0].Since

	source.fills, source.err = nil, nil
	if err := s.Sync(ex.ID); err != nil {
		t.Fatal(err)
	}
	// Any older trade of the symbol that failed is still inside this read.
	if !source.cursors[1].Since.Equal(start) {
		t.Errorf("second read from %s, want %s", source.cursors[1].Since, start)
	}
	if err := s.Sync(ex.ID); err != nil {
		t.Fatal(err)
	}
	if !source.cursors[2].Since.After(start) {
		t.Errorf("a whole read did not move the start: %s", source.cursors[2].Since)
	}
}

func TestSyncGivesUnknownOrdersToWhoeverHasTheAccount(t *testing.T) {
	s, st, ex, source := newFixture(t)
	now := time.Now().UTC()

	running := storetest.AddTrader(t, st, &store.Trader{UserID: ex.UserID, Name: "Bot", ExchangeID: ex.ID, IsRunning: true})
	source.fills = []trading.Fill{fill("1", store.SideBuy, store.SideLong, "1", now)}
	if err := s.Sync(ex.ID); err != nil {
		t.Fatal(err)
	}

	// A reservation outranks a running trader.
	if err := st.GormDB().Create(&store.AccountLease{AccountID: ex.ID, Holder: store.LeaseHolderFrontier, HolderID: "run"}).Error; err != nil {
		t.Fatal(err)
	}
	source.fills = []trading.Fill{fill("2", store.SideSell, store.SideShort, "1", now)}
	if err := s.Sync(ex.ID); err != nil {
		t.Fatal(err)
	}

	rows := positions(t, st)
	if len(rows) != 2 {
		t.Fatalf("positions = %+v", rows)
	}
	if rows[0].Source != store.SourceTrader || rows[0].SourceID != running.ID {
		t.Errorf("first = %+v", rows[0])
	}
	if rows[1].Source != store.SourceFrontier || rows[1].SourceID != "run" {
		t.Errorf("second = %+v", rows[1])
	}
}

func TestSyncStillBooksWhatItReadWhenASymbolFails(t *testing.T) {
	s, st, ex, source := newFixture(t)
	source.fills = []trading.Fill{fill("1", store.SideBuy, store.SideLong, "1", time.Now().UTC())}
	source.err = errors.New("read trades of ETHUSDT: down")
	if err := s.Sync(ex.ID); err == nil {
		t.Fatal("the failure should be reported, so the loop backs off")
	}
	if rows := positions(t, st); len(rows) != 1 {
		t.Fatalf("positions = %+v", rows)
	}
}

func TestSyncReconnectsWhenTheAccountChanges(t *testing.T) {
	s, st, ex, source := newFixture(t)
	opened := 0
	s.open = func(context.Context, *store.Exchange) (Source, error) {
		opened++
		return source, nil
	}
	for range 2 {
		if err := s.Sync(ex.ID); err != nil {
			t.Fatal(err)
		}
	}
	if opened != 1 {
		t.Fatalf("opened %d times", opened)
	}
	if err := st.Exchange().Update(ex.UserID, ex.ID, "new-key", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.Sync(ex.ID); err != nil {
		t.Fatal(err)
	}
	if opened != 2 {
		t.Fatalf("opened %d times", opened)
	}
}

func TestAnAccountIsActiveWhileAnythingCanBeTradingOnIt(t *testing.T) {
	s, st, ex, _ := newFixture(t)
	active := func() []string {
		t.Helper()
		ids, err := st.Exchange().ActiveIDs()
		if err != nil {
			t.Fatal(err)
		}
		return ids
	}
	if ids := active(); len(ids) != 0 {
		t.Fatalf("idle account is active: %v", ids)
	}

	trader := storetest.AddTrader(t, st, &store.Trader{UserID: ex.UserID, Name: "Bot", ExchangeID: ex.ID, IsRunning: true})
	if ids := active(); len(ids) != 1 || ids[0] != ex.ID {
		t.Fatalf("running trader: %v", ids)
	}
	if err := st.GormDB().Model(trader).Update("is_running", false).Error; err != nil {
		t.Fatal(err)
	}
	if ids := active(); len(ids) != 0 {
		t.Fatalf("stopped trader: %v", ids)
	}

	lease := store.AccountLease{AccountID: ex.ID, Holder: store.LeaseHolderFrontier, HolderID: "run"}
	if err := st.GormDB().Create(&lease).Error; err != nil {
		t.Fatal(err)
	}
	if ids := active(); len(ids) != 1 {
		t.Fatalf("reservation: %v", ids)
	}
	if err := st.GormDB().Delete(&lease).Error; err != nil {
		t.Fatal(err)
	}

	// A stopped trader's position can still hit its stop on the exchange.
	ledger := storetest.NewLedger(t, st)
	ledger.Open("bot", "BTCUSDT", store.SideLong, 1, 100, time.Now())
	if ids := active(); len(ids) != 1 || ids[0] != ledger.Account {
		t.Fatalf("open position: %v", ids)
	}
	ledger.Close("bot", "BTCUSDT", store.SideLong, 1, 110, 10, time.Now())
	if ids := active(); len(ids) != 0 {
		t.Fatalf("closed position: %v", ids)
	}

	// A loop stays a while after its account goes quiet, then ends.
	now := time.Now()
	s.now = func() time.Time { return now }
	if err := st.GormDB().Model(&trader).Update("is_running", true).Error; err != nil {
		t.Fatal(err)
	}
	s.scan()
	state := s.accounts[ex.ID]
	if state == nil || state.stop == nil {
		t.Fatal("no loop started")
	}
	if err := st.GormDB().Model(trader).Update("is_running", false).Error; err != nil {
		t.Fatal(err)
	}
	now = now.Add(stayFor / 2)
	s.scan()
	if state.stop == nil {
		t.Fatal("the loop ended too soon")
	}
	now = now.Add(stayFor)
	s.scan()
	if state.stop != nil {
		t.Fatal("the loop never ended")
	}
}

func TestSyncAsksTheExchangeOnlyAboutOrdersStillWorking(t *testing.T) {
	s, st, ex, source := newFixture(t)
	if err := s.Sync(ex.ID); err != nil {
		t.Fatal(err)
	}
	if len(source.asked) != 0 {
		t.Fatalf("nothing is working, but it asked %v", source.asked)
	}

	btc, err := instrument.Ensure(st.GormDB(), "binance", instrument.KindPerpetual, "BTCUSDT", "BTCUSDT")
	if err != nil {
		t.Fatal(err)
	}
	place := func(id string, owner store.Owner) int64 {
		order := store.LedgerOrder{
			AccountID: ex.ID, InstrumentID: btc, Source: owner.Source, SourceID: owner.SourceID, Role: store.RoleGrid,
			ExchangeOrderID: id, Side: store.SideBuy, Type: "LIMIT", Status: store.OrderNew, Quantity: decimal.NewFromInt(1),
		}
		if err := st.Ledger().RecordOrder(&order); err != nil {
			t.Fatal(err)
		}
		return order.ID
	}
	trader := store.Owner{Source: store.SourceTrader, SourceID: "bot"}
	listed := place("listed", trader)
	canceled := place("canceled", trader)
	frontier := place("frontier", store.Owner{Source: store.SourceFrontier, SourceID: "run"})
	unknown := place("unknown", trader)

	source.open = map[string][]trading.OpenOrder{"BTCUSDT": {{OrderID: "listed"}}}
	source.statuses = map[string]string{"canceled": store.OrderCanceled}
	if err := s.Sync(ex.ID); err == nil {
		t.Fatal("an order that could not be read should be reported")
	}

	status := func(id int64) (string, bool) {
		t.Helper()
		var order store.LedgerOrder
		if err := st.GormDB().First(&order, id).Error; err != nil {
			t.Fatal(err)
		}
		return order.Status, order.ClosedAt != nil
	}
	if got, closed := status(listed); got != store.OrderNew || closed {
		t.Errorf("listed = %s, closed %v", got, closed)
	}
	if got, closed := status(canceled); got != store.OrderCanceled || !closed {
		t.Errorf("canceled = %s, closed %v", got, closed)
	}
	if got, closed := status(unknown); got != store.OrderNew || closed {
		t.Errorf("unknown = %s, closed %v", got, closed)
	}
	// Frontier reconciles its own orders.
	if got, _ := status(frontier); got != store.OrderNew {
		t.Errorf("frontier = %s", got)
	}
	for _, asked := range source.asked {
		if asked == "status frontier" || asked == "status listed" {
			t.Errorf("asked %q", asked)
		}
	}

	// A settled order is not asked about again, and a late fill does not reopen it.
	source.asked = nil
	source.fills = []trading.Fill{fill("7", store.SideBuy, store.SideLong, "1", time.Now().UTC())}
	source.fills[0].OrderID = "canceled"
	source.statuses["unknown"] = store.OrderFilled
	if err := s.Sync(ex.ID); err != nil {
		t.Fatal(err)
	}
	for _, asked := range source.asked {
		if asked == "status canceled" {
			t.Errorf("asked %q", asked)
		}
	}
	if got, _ := status(canceled); got != store.OrderCanceled {
		t.Errorf("canceled after a late fill = %s", got)
	}
}

func TestATradeThatCannotBeBookedHoldsNothingUpAndIsKept(t *testing.T) {
	s, st, ex, source := newFixture(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	unbookable := trading.Fill{
		TradeID: "t1", OrderID: "o1", Symbol: "OLDUSDT", NativeSymbol: "OLD-USDT-SWAP", Side: store.SideSell, Time: now.Add(-time.Minute),
		Unbookable: "sell 7 contracts at 3: OKX no longer describes this contract",
	}
	source.fills = []trading.Fill{unbookable, fill("2", store.SideBuy, store.SideLong, "1", now)}
	if err := s.Sync(ex.ID); err != nil {
		t.Fatal(err)
	}
	// The other trade is in the ledger, and this one is not, at any size.
	rows := positions(t, st)
	if len(rows) != 1 {
		t.Fatalf("positions = %+v", rows)
	}
	var fills int64
	st.GormDB().Model(&store.LedgerFill{}).Count(&fills)
	if fills != 1 {
		t.Fatalf("fills = %d", fills)
	}
	var instruments int64
	st.GormDB().Model(&instrument.Instrument{}).Where("symbol = ?", "OLDUSDT").Count(&instruments)
	if instruments != 0 {
		t.Fatal("a contract nothing could book was recorded as an instrument")
	}
	// And the read moved on: this is no reason to read it forever.
	start := source.cursors[0].Since
	if err := s.Sync(ex.ID); err != nil {
		t.Fatal(err)
	}
	if !source.cursors[1].Since.After(start) {
		t.Fatalf("the read stayed at %s", source.cursors[1].Since)
	}

	// Seen twice, it is kept once, and the user can be told what it was.
	skipped, err := st.Ledger().SkippedFillsSince(ex.UserID, now.Add(-time.Hour), 10)
	if err != nil || len(skipped) != 1 {
		t.Fatalf("skipped = %+v, err %v", skipped, err)
	}
	if skipped[0].NativeSymbol != "OLD-USDT-SWAP" || skipped[0].ExchangeTradeID != "t1" || skipped[0].Detail != unbookable.Unbookable {
		t.Fatalf("skipped = %+v", skipped[0])
	}
	if others, err := st.Ledger().SkippedFillsSince(storetest.OtherUserID, now.Add(-time.Hour), 10); err != nil || len(others) != 0 {
		t.Fatalf("another user saw %+v, err %v", others, err)
	}

	// If the exchange can say after all, the trade is booked and no longer missing.
	source.fills = []trading.Fill{fill("t1", store.SideBuy, store.SideLong, "1", now.Add(-time.Minute))}
	source.fills[0].NativeSymbol = "OLD-USDT-SWAP"
	if err := s.Sync(ex.ID); err != nil {
		t.Fatal(err)
	}
	if skipped, err := st.Ledger().SkippedFillsSince(ex.UserID, now.Add(-time.Hour), 10); err != nil || len(skipped) != 0 {
		t.Fatalf("still skipped: %+v, err %v", skipped, err)
	}
}
