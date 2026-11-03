package syncer

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"testing"
	"time"
	"vergex/internal/account"
	"vergex/internal/account/accounttest"
	"vergex/internal/database/testdb"
	"vergex/internal/market/instrument"
	"vergex/internal/trader"

	"github.com/shopspring/decimal"
)

type fakeSource struct {
	fills   []account.Fill
	err     error
	cursors []account.FillCursor

	open     map[string][]account.OpenOrder // per symbol
	statuses map[string]string              // per order ID
	asked    []string                       // what was asked of the exchange, in order
}

func (f *fakeSource) GetOpenOrders(symbol string) ([]account.OpenOrder, error) {
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

func (f *fakeSource) FillsSince(cursor account.FillCursor) ([]account.Fill, error) {
	f.cursors = append(f.cursors, cursor)
	return f.fills, f.err
}

func fill(id, side, positionSide, quantity string, at time.Time) account.Fill {
	return account.Fill{
		TradeID: id, OrderID: "order-" + id, Symbol: "BTCUSDT", NativeSymbol: "BTCUSDT", Side: side, PositionSide: positionSide,
		Price: decimal.NewFromInt(100), Quantity: decimal.RequireFromString(quantity), QuoteQuantity: decimal.NewFromInt(100),
		Commission: decimal.RequireFromString("0.1"), CommissionAsset: "USDT", Time: at,
	}
}

// testStores is what these tests read and write with.
type testStores struct {
	DB       *gorm.DB
	Accounts *account.Store
	Ledger   *account.Ledger
}

func newTestStores(db *gorm.DB) *testStores {
	return &testStores{DB: db, Accounts: account.NewStore(db), Ledger: account.NewLedger(db)}
}

// testFeatures is what cmd/vergex tells the sync about traders.
func testFeatures(db *gorm.DB) Features {
	traders := trader.NewStore(db)
	return Features{
		ActiveAccounts: traders.RunningAccounts,
		Owner: func(accountID string) (account.Owner, error) {
			id, err := traders.RunningOn(accountID)
			if err != nil || id == "" {
				return account.Owner{}, err
			}
			return account.Owner{Source: account.SourceTrader, SourceID: id}, nil
		},
	}
}

func newFixture(t *testing.T) (*Service, *testStores, *account.Account, *fakeSource) {
	t.Helper()
	st := newTestStores(testdb.Open(t))
	user := testdb.AddUser(t, st.DB)
	id, err := st.Accounts.Create(user, "binance", "Main", "key", "secret", "", false)
	if err != nil {
		t.Fatal(err)
	}
	ex, err := st.Accounts.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	source := &fakeSource{}
	s := New(st.DB, testFeatures(st.DB))
	s.open = func(context.Context, *account.Account) (Source, error) { return source, nil }
	return s, st, ex, source
}

func positions(t *testing.T, st *testStores) []account.LedgerPosition {
	t.Helper()
	var rows []account.LedgerPosition
	if err := st.DB.Order("id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestSyncBooksAnAccountsTradesOnceOldestFirst(t *testing.T) {
	s, st, ex, source := newFixture(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	// The exchange can list them in any order, and the close must come after the open.
	source.fills = []account.Fill{
		fill("102", account.SideSell, account.SideLong, "1", now),
		fill("101", account.SideBuy, account.SideLong, "1", now.Add(-time.Minute)),
	}
	for range 2 {
		if err := s.Sync(ex.ID); err != nil {
			t.Fatal(err)
		}
	}
	rows := positions(t, st)
	if len(rows) != 1 || rows[0].Status != account.PositionClosed || rows[0].Source != account.SourceExternal {
		t.Fatalf("positions = %+v", rows)
	}
	var fills int64
	st.DB.Model(&account.LedgerFill{}).Count(&fills)
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
	ledger := account.NewLedger(st.DB)
	btc, err := instrument.Ensure(st.DB, "binance", instrument.KindPerpetual, "BTCUSDT")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.RecordTrade(account.Trade{
		AccountID: ex.ID, InstrumentID: btc, VenueTradeID: "9", VenueOrderID: "9", Side: account.SideBuy, PositionSide: account.SideLong,
		Price: decimal.NewFromInt(100), Quantity: decimal.NewFromInt(1), QuoteQuantity: decimal.NewFromInt(100), ExecutedAt: last,
	}, account.Owner{Source: account.SourceTrader, SourceID: "bot"}); err != nil {
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
	source.fills = []account.Fill{fill("1", account.SideBuy, account.SideLong, "1", now)}
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

	running := testdb.AddTrader(t, st.DB, &testdb.Trader{UserID: ex.UserID, Name: "Bot", AccountID: ex.ID, Running: true})
	source.fills = []account.Fill{fill("1", account.SideBuy, account.SideLong, "1", now)}
	if err := s.Sync(ex.ID); err != nil {
		t.Fatal(err)
	}

	// A reservation outranks a running trader.
	if err := st.DB.Create(&account.Lease{AccountID: ex.ID, Source: account.SourceFutures, SourceID: "run"}).Error; err != nil {
		t.Fatal(err)
	}
	source.fills = []account.Fill{fill("2", account.SideSell, account.SideShort, "1", now)}
	if err := s.Sync(ex.ID); err != nil {
		t.Fatal(err)
	}

	rows := positions(t, st)
	if len(rows) != 2 {
		t.Fatalf("positions = %+v", rows)
	}
	if rows[0].Source != account.SourceTrader || rows[0].SourceID != running.ID {
		t.Errorf("first = %+v", rows[0])
	}
	if rows[1].Source != account.SourceFutures || rows[1].SourceID != "run" {
		t.Errorf("second = %+v", rows[1])
	}
}

func TestSyncStillBooksWhatItReadWhenASymbolFails(t *testing.T) {
	s, st, ex, source := newFixture(t)
	source.fills = []account.Fill{fill("1", account.SideBuy, account.SideLong, "1", time.Now().UTC())}
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
	s.open = func(context.Context, *account.Account) (Source, error) {
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
	if err := st.Accounts.Update(ex.UserID, ex.ID, "new-key", "", ""); err != nil {
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
		ids, err := s.activeAccounts()
		if err != nil {
			t.Fatal(err)
		}
		return ids
	}
	if ids := active(); len(ids) != 0 {
		t.Fatalf("idle account is active: %v", ids)
	}

	trader := testdb.AddTrader(t, st.DB, &testdb.Trader{UserID: ex.UserID, Name: "Bot", AccountID: ex.ID, Running: true})
	if ids := active(); len(ids) != 1 || ids[0] != ex.ID {
		t.Fatalf("running trader: %v", ids)
	}
	if err := st.DB.Exec(`UPDATE trader.traders SET running = false WHERE id = ?`, trader.ID).Error; err != nil {
		t.Fatal(err)
	}
	if ids := active(); len(ids) != 0 {
		t.Fatalf("stopped trader: %v", ids)
	}

	lease := account.Lease{AccountID: ex.ID, Source: account.SourceFutures, SourceID: "run"}
	if err := st.DB.Create(&lease).Error; err != nil {
		t.Fatal(err)
	}
	if ids := active(); len(ids) != 1 {
		t.Fatalf("reservation: %v", ids)
	}
	if err := st.DB.Delete(&lease).Error; err != nil {
		t.Fatal(err)
	}

	// A stopped trader's position can still hit its stop on the exchange.
	ledger := accounttest.NewLedger(t, st.DB)
	ledger.Open("bot", "BTCUSDT", account.SideLong, 1, 100, time.Now())
	if ids := active(); len(ids) != 1 || ids[0] != ledger.Account {
		t.Fatalf("open position: %v", ids)
	}
	ledger.Close("bot", "BTCUSDT", account.SideLong, 1, 110, 10, time.Now())
	if ids := active(); len(ids) != 0 {
		t.Fatalf("closed position: %v", ids)
	}

	// A loop stays a while after its account goes quiet, then ends.
	now := time.Now()
	s.now = func() time.Time { return now }
	if err := st.DB.Table("trader.traders").Where("id = ?", trader.ID).Update("running", true).Error; err != nil {
		t.Fatal(err)
	}
	s.scan()
	state := s.accounts[ex.ID]
	if state == nil || state.stop == nil {
		t.Fatal("no loop started")
	}
	if err := st.DB.Exec(`UPDATE trader.traders SET running = false WHERE id = ?`, trader.ID).Error; err != nil {
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

	btc, err := instrument.Ensure(st.DB, "binance", instrument.KindPerpetual, "BTCUSDT")
	if err != nil {
		t.Fatal(err)
	}
	place := func(id string, owner account.Owner) int64 {
		order := account.LedgerOrder{
			AccountID: ex.ID, InstrumentID: btc, Source: owner.Source, SourceID: owner.SourceID, Role: account.RoleGrid,
			VenueOrderID: id, Side: account.SideBuy, Type: "LIMIT", Status: account.OrderNew, Quantity: decimal.NewFromInt(1),
		}
		if err := st.Ledger.RecordOrder(&order); err != nil {
			t.Fatal(err)
		}
		return order.ID
	}
	trader := account.Owner{Source: account.SourceTrader, SourceID: "bot"}
	listed := place("listed", trader)
	canceled := place("canceled", trader)
	futures := place("futures", account.Owner{Source: account.SourceFutures, SourceID: "run"})
	unknown := place("unknown", trader)

	source.open = map[string][]account.OpenOrder{"BTCUSDT": {{OrderID: "listed"}}}
	source.statuses = map[string]string{"canceled": account.OrderCanceled}
	if err := s.Sync(ex.ID); err == nil {
		t.Fatal("an order that could not be read should be reported")
	}

	status := func(id int64) (string, bool) {
		t.Helper()
		var order account.LedgerOrder
		if err := st.DB.First(&order, id).Error; err != nil {
			t.Fatal(err)
		}
		return order.Status, order.ClosedAt != nil
	}
	if got, closed := status(listed); got != account.OrderNew || closed {
		t.Errorf("listed = %s, closed %v", got, closed)
	}
	if got, closed := status(canceled); got != account.OrderCanceled || !closed {
		t.Errorf("canceled = %s, closed %v", got, closed)
	}
	if got, closed := status(unknown); got != account.OrderNew || closed {
		t.Errorf("unknown = %s, closed %v", got, closed)
	}
	// Futures reconciles its own orders.
	if got, _ := status(futures); got != account.OrderNew {
		t.Errorf("futures = %s", got)
	}
	for _, asked := range source.asked {
		if asked == "status futures" || asked == "status listed" {
			t.Errorf("asked %q", asked)
		}
	}

	// A settled order is not asked about again, and a late fill does not reopen it.
	source.asked = nil
	source.fills = []account.Fill{fill("7", account.SideBuy, account.SideLong, "1", time.Now().UTC())}
	source.fills[0].OrderID = "canceled"
	source.statuses["unknown"] = account.OrderFilled
	if err := s.Sync(ex.ID); err != nil {
		t.Fatal(err)
	}
	for _, asked := range source.asked {
		if asked == "status canceled" {
			t.Errorf("asked %q", asked)
		}
	}
	if got, _ := status(canceled); got != account.OrderCanceled {
		t.Errorf("canceled after a late fill = %s", got)
	}
}

func TestATradeThatCannotBeBookedHoldsNothingUpAndIsKept(t *testing.T) {
	s, st, ex, source := newFixture(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	unbookable := account.Fill{
		TradeID: "t1", OrderID: "o1", Symbol: "OLDUSDT", NativeSymbol: "OLD-USDT-SWAP", Side: account.SideSell, Time: now.Add(-time.Minute),
		Unbookable: "sell 7 contracts at 3: OKX no longer describes this contract",
	}
	source.fills = []account.Fill{unbookable, fill("2", account.SideBuy, account.SideLong, "1", now)}
	if err := s.Sync(ex.ID); err != nil {
		t.Fatal(err)
	}
	// The other trade is in the ledger, and this one is not, at any size.
	rows := positions(t, st)
	if len(rows) != 1 {
		t.Fatalf("positions = %+v", rows)
	}
	var fills int64
	st.DB.Model(&account.LedgerFill{}).Count(&fills)
	if fills != 1 {
		t.Fatalf("fills = %d", fills)
	}
	var instruments int64
	st.DB.Model(&instrument.Instrument{}).Where("symbol = ?", "OLDUSDT").Count(&instruments)
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
	skipped, err := st.Ledger.SkippedFillsSince(ex.UserID, now.Add(-time.Hour), 10)
	if err != nil || len(skipped) != 1 {
		t.Fatalf("skipped = %+v, err %v", skipped, err)
	}
	if skipped[0].NativeSymbol != "OLD-USDT-SWAP" || skipped[0].VenueTradeID != "t1" || skipped[0].Detail != unbookable.Unbookable {
		t.Fatalf("skipped = %+v", skipped[0])
	}
	if others, err := st.Ledger.SkippedFillsSince(testdb.OtherUserID, now.Add(-time.Hour), 10); err != nil || len(others) != 0 {
		t.Fatalf("another user saw %+v, err %v", others, err)
	}

	// If the exchange can say after all, the trade is booked and no longer missing.
	source.fills = []account.Fill{fill("t1", account.SideBuy, account.SideLong, "1", now.Add(-time.Minute))}
	source.fills[0].NativeSymbol = "OLD-USDT-SWAP"
	if err := s.Sync(ex.ID); err != nil {
		t.Fatal(err)
	}
	if skipped, err := st.Ledger.SkippedFillsSince(ex.UserID, now.Add(-time.Hour), 10); err != nil || len(skipped) != 0 {
		t.Fatalf("still skipped: %+v, err %v", skipped, err)
	}
}
