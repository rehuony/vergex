package account

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"vergex/internal/database/testdb"
	"vergex/internal/market/instrument"
	"vergex/internal/user"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var (
	traderOwner = Owner{Source: SourceTrader, SourceID: "t1"}
	ledgerStart = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
)

type ledgerFixture struct {
	t       *testing.T
	s       *Ledger
	db      *gorm.DB
	account string
	btc     int64
	eth     int64
	next    int
}

func newLedgerFixture(t *testing.T) *ledgerFixture {
	t.Helper()
	db := testdb.Open(t)
	if err := user.NewStore(db).Create(&user.User{ID: testdb.UserID, Email: "me@example.com", PasswordHash: "x"}); err != nil {
		t.Fatal(err)
	}
	account, err := NewStore(db).Create(testdb.UserID, "binance", "Main", "key", "secret", "", false)
	if err != nil {
		t.Fatal(err)
	}
	btc, err := instrument.Ensure(db, "binance", instrument.KindPerpetual, "BTCUSDT", "BTCUSDT")
	if err != nil {
		t.Fatal(err)
	}
	eth, err := instrument.Ensure(db, "binance", instrument.KindPerpetual, "ETHUSDT", "ETHUSDT")
	if err != nil {
		t.Fatal(err)
	}
	return &ledgerFixture{t: t, s: NewLedger(db), db: db, account: account, btc: btc, eth: eth}
}

func dec(value string) decimal.Decimal { return decimal.RequireFromString(value) }

// trade builds a trade on BTC with a fresh trade ID, a minute after the last.
func (f *ledgerFixture) trade(side, positionSide, quantity, price string) Trade {
	f.next++
	return Trade{
		AccountID:       f.account,
		InstrumentID:    f.btc,
		ExchangeTradeID: fmt.Sprintf("trade-%d", f.next),
		ExchangeOrderID: fmt.Sprintf("order-%d", f.next),
		Side:            side,
		PositionSide:    positionSide,
		Price:           dec(price),
		Quantity:        dec(quantity),
		QuoteQuantity:   dec(price).Mul(dec(quantity)),
		Commission:      dec("0.5"),
		CommissionAsset: "USDT",
		ExecutedAt:      ledgerStart.Add(time.Duration(f.next) * time.Minute),
	}
}

func (f *ledgerFixture) record(trade Trade, fallback Owner) {
	f.t.Helper()
	if _, err := f.s.RecordTrade(trade, fallback); err != nil {
		f.t.Fatalf("record %s: %v", trade.ExchangeTradeID, err)
	}
}

func (f *ledgerFixture) positions() []LedgerPosition {
	f.t.Helper()
	var positions []LedgerPosition
	if err := f.db.Order("id").Find(&positions).Error; err != nil {
		f.t.Fatal(err)
	}
	return positions
}

func (f *ledgerFixture) fill(tradeID string) LedgerFill {
	f.t.Helper()
	var fill LedgerFill
	if err := f.db.Where("exchange_trade_id = ?", tradeID).First(&fill).Error; err != nil {
		f.t.Fatal(err)
	}
	return fill
}

func assertDecimal(t *testing.T, name string, got decimal.Decimal, want string) {
	t.Helper()
	if !got.Equal(dec(want)) {
		t.Errorf("%s = %s, want %s", name, got, want)
	}
}

func count(t *testing.T, db *gorm.DB, model any) int64 {
	t.Helper()
	var n int64
	if err := db.Model(model).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func TestReportingATradeAgainChangesNothing(t *testing.T) {
	f := newLedgerFixture(t)
	trade := f.trade(SideBuy, SideLong, "2", "100")
	if recorded, err := f.s.RecordTrade(trade, traderOwner); err != nil || !recorded {
		t.Fatalf("first report: %v, err %v", recorded, err)
	}
	before := f.positions()[0]
	if recorded, err := f.s.RecordTrade(trade, traderOwner); err != nil || recorded {
		t.Fatalf("second report: %v, err %v", recorded, err)
	}
	after := f.positions()[0]
	assertDecimal(t, "quantity", after.Quantity, "2")
	if !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Error("a repeat touched the position")
	}
	if n := count(t, f.db, &LedgerFill{}); n != 1 {
		t.Errorf("fills = %d", n)
	}
	if n := count(t, f.db, &LedgerOrder{}); n != 1 {
		t.Errorf("orders = %d", n)
	}
}

func TestBookingTheSameBatchAtOnceDoesNotDoubleThePosition(t *testing.T) {
	f := newLedgerFixture(t)
	var batch []Trade
	for range 10 {
		batch = append(batch, f.trade(SideBuy, SideLong, "0.1", "100"))
	}
	var wg sync.WaitGroup
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, trade := range batch {
				if _, err := f.s.RecordTrade(trade, traderOwner); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
	positions := f.positions()
	if len(positions) != 1 {
		t.Fatalf("positions = %d", len(positions))
	}
	assertDecimal(t, "quantity", positions[0].Quantity, "1")
	assertDecimal(t, "entry quantity", positions[0].EntryQuantity, "1")
	assertDecimal(t, "fee", positions[0].Fee, "5")
	if n := count(t, f.db, &LedgerFill{}); n != 10 {
		t.Errorf("fills = %d", n)
	}
}

func TestTheSameTradeIDOnAnotherSymbolIsAnotherTrade(t *testing.T) {
	f := newLedgerFixture(t)
	btc := f.trade(SideBuy, SideLong, "1", "100")
	eth := btc
	eth.InstrumentID = f.eth
	f.record(btc, traderOwner)
	if recorded, err := f.s.RecordTrade(eth, traderOwner); err != nil || !recorded {
		t.Fatalf("ETH trade with BTC's ID: %v, err %v", recorded, err)
	}
	if positions := f.positions(); len(positions) != 2 {
		t.Fatalf("positions = %d", len(positions))
	}
}

func TestHedgeModeAddsAndClosesInSteps(t *testing.T) {
	f := newLedgerFixture(t)
	f.record(f.trade(SideBuy, SideLong, "1", "100"), traderOwner)
	f.record(f.trade(SideBuy, SideLong, "2", "130"), traderOwner)

	partial := f.trade(SideSell, SideLong, "1", "150")
	partial.RealizedPnL = decimal.NewNullDecimal(dec("25"))
	f.record(partial, traderOwner)
	position := f.positions()[0]
	if position.Status != PositionOpen {
		t.Fatalf("status = %s", position.Status)
	}
	assertDecimal(t, "entry price", position.EntryPrice, "120")
	assertDecimal(t, "entry quantity", position.EntryQuantity, "3")
	assertDecimal(t, "quantity", position.Quantity, "2")
	assertDecimal(t, "realized, as reported", position.RealizedPnL, "25")

	// Nothing reported here, so it is worked out from the entry price.
	rest := f.trade(SideSell, SideLong, "2", "180")
	f.record(rest, traderOwner)
	position = f.positions()[0]
	if position.Status != PositionClosed || position.ClosedAt == nil || !position.ClosedAt.Equal(rest.ExecutedAt) {
		t.Fatalf("position = %+v", position)
	}
	assertDecimal(t, "quantity", position.Quantity, "0")
	assertDecimal(t, "entry quantity", position.EntryQuantity, "3")
	assertDecimal(t, "exit price", position.ExitPrice, "170")
	assertDecimal(t, "realized", position.RealizedPnL, "145")
	assertDecimal(t, "fee", position.Fee, "2")

	// The next open starts a new position.
	f.record(f.trade(SideBuy, SideLong, "1", "200"), traderOwner)
	if positions := f.positions(); len(positions) != 2 || positions[1].Status != PositionOpen {
		t.Fatalf("positions = %+v", positions)
	}
}

func TestAFeeInAnotherAssetStaysOnTheFill(t *testing.T) {
	f := newLedgerFixture(t)
	trade := f.trade(SideBuy, SideLong, "1", "100")
	trade.Commission, trade.CommissionAsset = dec("0.002"), "BNB"
	f.record(trade, traderOwner)
	assertDecimal(t, "position fee", f.positions()[0].Fee, "0")
	fill := f.fill(trade.ExchangeTradeID)
	assertDecimal(t, "fill commission", fill.Commission, "0.002")
	if fill.CommissionAsset != "BNB" {
		t.Errorf("asset = %s", fill.CommissionAsset)
	}
}

func TestSmallNumbersStayExact(t *testing.T) {
	f := newLedgerFixture(t)
	f.record(f.trade(SideBuy, SideLong, "0.00000001", "0.000000541"), traderOwner)
	f.record(f.trade(SideSell, SideLong, "0.00000001", "0.000000641"), traderOwner)
	position := f.positions()[0]
	if position.Status != PositionClosed {
		t.Fatalf("status = %s, quantity %s", position.Status, position.Quantity)
	}
	assertDecimal(t, "entry", position.EntryPrice, "0.000000541")
	assertDecimal(t, "exit", position.ExitPrice, "0.000000641")
	assertDecimal(t, "realized", position.RealizedPnL, "0.000000000000001")
}

func TestOneWayModeReducesThenReverses(t *testing.T) {
	f := newLedgerFixture(t)
	f.record(f.trade(SideBuy, "", "2", "100"), traderOwner)
	partial := f.trade(SideSell, "", "1", "110")
	f.record(partial, traderOwner)
	position := f.positions()[0]
	assertDecimal(t, "after a partial close", position.Quantity, "1")
	assertDecimal(t, "realized", position.RealizedPnL, "10")

	reverse := f.trade(SideSell, "", "4", "120")
	f.record(reverse, traderOwner)
	positions := f.positions()
	if len(positions) != 2 {
		t.Fatalf("positions = %+v", positions)
	}
	long, short := positions[0], positions[1]
	if long.Side != SideLong || long.Status != PositionClosed || short.Side != SideShort || short.Status != PositionOpen {
		t.Fatalf("long %+v, short %+v", long, short)
	}
	assertDecimal(t, "long realized", long.RealizedPnL, "30")
	assertDecimal(t, "short quantity", short.Quantity, "3")
	assertDecimal(t, "short entry", short.EntryPrice, "120")
	// The fee is split by how much of the trade each side took.
	assertDecimal(t, "long fee", long.Fee, "1.125")
	assertDecimal(t, "short fee", short.Fee, "0.375")
	if fill := f.fill(reverse.ExchangeTradeID); fill.PositionID == nil || *fill.PositionID != long.ID {
		t.Errorf("fill points at %v", fill.PositionID)
	}
}

func TestAOneWayCloseFromBeforeWeWereWatchingOpensNothing(t *testing.T) {
	f := newLedgerFixture(t)
	trade := f.trade(SideSell, "", "1", "100")
	trade.RealizedPnL = decimal.NewNullDecimal(dec("5"))
	f.record(trade, traderOwner)
	if positions := f.positions(); len(positions) != 0 {
		t.Fatalf("opened %+v", positions)
	}
	assertDecimal(t, "unmatched", f.fill(trade.ExchangeTradeID).UnmatchedQuantity, "1")
}

func TestACloseThatArrivesBeforeItsOpenStillClosesIt(t *testing.T) {
	f := newLedgerFixture(t)
	open := f.trade(SideBuy, SideLong, "2", "100")
	closing := f.trade(SideSell, SideLong, "2", "130")
	closing.RealizedPnL = decimal.NewNullDecimal(dec("60"))

	f.record(closing, traderOwner)
	if positions := f.positions(); len(positions) != 0 {
		t.Fatalf("a close opened %+v", positions)
	}
	assertDecimal(t, "waiting", f.fill(closing.ExchangeTradeID).UnmatchedQuantity, "2")

	f.record(open, traderOwner)
	positions := f.positions()
	if len(positions) != 1 || positions[0].Status != PositionClosed {
		t.Fatalf("positions = %+v", positions)
	}
	assertDecimal(t, "quantity", positions[0].Quantity, "0")
	assertDecimal(t, "exit", positions[0].ExitPrice, "130")
	assertDecimal(t, "realized", positions[0].RealizedPnL, "60")
	assertDecimal(t, "fee", positions[0].Fee, "1")
	fill := f.fill(closing.ExchangeTradeID)
	assertDecimal(t, "taken", fill.UnmatchedQuantity, "0")
	if fill.PositionID == nil || *fill.PositionID != positions[0].ID {
		t.Errorf("fill points at %v", fill.PositionID)
	}
}

func TestAnOldCloseDoesNotEatANewerOpen(t *testing.T) {
	f := newLedgerFixture(t)
	old := f.trade(SideSell, SideLong, "1", "90")
	f.record(old, traderOwner)
	f.record(f.trade(SideBuy, SideLong, "2", "100"), traderOwner)
	position := f.positions()[0]
	if position.Status != PositionOpen {
		t.Fatalf("status = %s", position.Status)
	}
	assertDecimal(t, "quantity", position.Quantity, "2")
	assertDecimal(t, "still waiting", f.fill(old.ExchangeTradeID).UnmatchedQuantity, "1")
}

func TestAnOverCloseKeepsTheRestForLater(t *testing.T) {
	f := newLedgerFixture(t)
	f.record(f.trade(SideBuy, SideLong, "1", "100"), traderOwner)
	over := f.trade(SideSell, SideLong, "3", "110")
	f.record(over, traderOwner)
	if position := f.positions()[0]; position.Status != PositionClosed {
		t.Fatalf("status = %s", position.Status)
	}
	assertDecimal(t, "unmatched", f.fill(over.ExchangeTradeID).UnmatchedQuantity, "2")
}

func TestAStopNoOneRecordedGoesToWhoeverHoldsThePosition(t *testing.T) {
	f := newLedgerFixture(t)
	f.record(f.trade(SideBuy, SideLong, "1", "100"), traderOwner)

	// The trader has stopped, so nothing claims this order but the position.
	stop := f.trade(SideSell, SideLong, "1", "90")
	f.record(stop, Owner{})

	var order LedgerOrder
	if err := f.db.Where("exchange_order_id = ?", stop.ExchangeOrderID).First(&order).Error; err != nil {
		t.Fatal(err)
	}
	if order.Source != SourceTrader || order.SourceID != "t1" || order.Role != RoleClose || order.Status != OrderFilled {
		t.Fatalf("order = %+v", order)
	}
	assertDecimal(t, "order quantity", order.Quantity, "1")
	if position := f.positions()[0]; position.Status != PositionClosed {
		t.Fatalf("status = %s", position.Status)
	}
}

func TestAnUnknownOpenGoesToTheFallbackOrIsExternal(t *testing.T) {
	f := newLedgerFixture(t)
	f.record(f.trade(SideBuy, SideLong, "1", "100"), traderOwner)
	f.record(f.trade(SideSell, SideShort, "1", "100"), Owner{})
	positions := f.positions()
	if positions[0].Source != SourceTrader || positions[0].SourceID != "t1" {
		t.Errorf("fallback owner: %+v", positions[0])
	}
	if positions[1].Source != SourceExternal || positions[1].SourceID != "" {
		t.Errorf("no owner: %+v", positions[1])
	}
}

func TestACloseTakesItsOwnPositionFirstThenTheOldest(t *testing.T) {
	f := newLedgerFixture(t)
	f.record(f.trade(SideBuy, SideLong, "1", "100"), Owner{})
	f.record(f.trade(SideBuy, SideLong, "1", "100"), Owner{Source: SourceFutures, SourceID: "run"})
	f.record(f.trade(SideBuy, SideLong, "1", "100"), traderOwner)

	placed := f.trade(SideSell, SideLong, "2", "110")
	if err := f.s.RecordOrder(&LedgerOrder{
		AccountID: f.account, InstrumentID: f.btc, Source: SourceTrader, SourceID: "t1", Role: RoleClose,
		ExchangeOrderID: placed.ExchangeOrderID, Side: SideSell, PositionSide: SideLong, Type: "MARKET", Status: OrderNew, Quantity: dec("2"),
	}); err != nil {
		t.Fatal(err)
	}
	f.record(placed, Owner{})

	positions := f.positions()
	external, futures, trader := positions[0], positions[1], positions[2]
	if trader.Status != PositionClosed {
		t.Errorf("the order's own position stayed open: %+v", trader)
	}
	if external.Status != PositionClosed {
		t.Errorf("the oldest stayed open: %+v", external)
	}
	if futures.Status != PositionOpen {
		t.Errorf("the newer one was closed: %+v", futures)
	}
}

func TestAnOrderRecordedFirstIsCompletedByItsFills(t *testing.T) {
	f := newLedgerFixture(t)
	leverage := 5
	first := f.trade(SideBuy, SideLong, "1", "100")
	order := LedgerOrder{
		AccountID: f.account, InstrumentID: f.btc, Source: SourceTrader, SourceID: "t1", Role: RoleEntry,
		ExchangeOrderID: first.ExchangeOrderID, Side: SideBuy, PositionSide: SideLong, Type: "MARKET",
		Status: OrderNew, Quantity: dec("3"), Leverage: &leverage, MarginMode: "cross",
	}
	if err := f.s.RecordOrder(&order); err != nil {
		t.Fatal(err)
	}
	f.record(first, Owner{})
	var read LedgerOrder
	if err := f.db.First(&read, order.ID).Error; err != nil {
		t.Fatal(err)
	}
	if read.Status != OrderPartiallyFilled || read.ClosedAt != nil {
		t.Fatalf("order = %+v", read)
	}
	assertDecimal(t, "quantity", read.Quantity, "3")

	second := f.trade(SideBuy, SideLong, "2", "130")
	second.ExchangeOrderID = first.ExchangeOrderID
	f.record(second, Owner{})
	if err := f.db.First(&read, order.ID).Error; err != nil {
		t.Fatal(err)
	}
	if read.Status != OrderFilled || read.ClosedAt == nil || !read.ClosedAt.Equal(second.ExecutedAt) {
		t.Fatalf("order = %+v", read)
	}
	assertDecimal(t, "filled", read.FilledQuantity, "3")
	assertDecimal(t, "average", read.AvgFillPrice.Decimal, "120")

	position := f.positions()[0]
	if position.Leverage == nil || *position.Leverage != 5 || position.MarginMode != "cross" {
		t.Errorf("position = %+v", position)
	}
}

func TestAFillThatBeatsItsOrderIsCompletedByIt(t *testing.T) {
	f := newLedgerFixture(t)
	trade := f.trade(SideBuy, SideLong, "1", "100")
	f.record(trade, traderOwner)
	if n := count(t, f.db, &LedgerOrder{}); n != 1 {
		t.Fatalf("orders = %d", n)
	}

	leverage := 3
	order := LedgerOrder{
		AccountID: f.account, InstrumentID: f.btc, Source: SourceTrader, SourceID: "t1", Role: RoleEntry,
		ExchangeOrderID: trade.ExchangeOrderID, ClientOrderID: "client-1", Side: SideBuy, PositionSide: SideLong, Type: "MARKET",
		Status: OrderNew, Quantity: dec("2"), Leverage: &leverage, MarginMode: "isolated",
	}
	if err := f.s.RecordOrder(&order); err != nil {
		t.Fatal(err)
	}
	if n := count(t, f.db, &LedgerOrder{}); n != 1 {
		t.Fatalf("orders = %d", n)
	}
	var read LedgerOrder
	if err := f.db.First(&read, order.ID).Error; err != nil {
		t.Fatal(err)
	}
	if read.ClientOrderID != "client-1" || read.Type != "MARKET" || read.Status != OrderPartiallyFilled {
		t.Fatalf("order = %+v", read)
	}
	assertDecimal(t, "filled", read.FilledQuantity, "1")
	assertDecimal(t, "quantity", read.Quantity, "2")
	if position := f.positions()[0]; position.Leverage == nil || *position.Leverage != 3 {
		t.Errorf("position = %+v", position)
	}
}

func TestATriggeredOrderFindsItsFills(t *testing.T) {
	f := newLedgerFixture(t)
	f.record(f.trade(SideBuy, SideLong, "1", "100"), traderOwner)
	trade := f.trade(SideSell, SideLong, "1", "90")
	order := LedgerOrder{
		AccountID: f.account, InstrumentID: f.btc, Source: SourceFutures, SourceID: "run", Role: RoleStop,
		ExchangeOrderID: "algo-1", TriggeredOrderID: trade.ExchangeOrderID, Side: SideSell, PositionSide: SideLong,
		Type: "STOP_MARKET", Status: OrderNew, Quantity: dec("1"),
	}
	if err := f.s.RecordOrder(&order); err != nil {
		t.Fatal(err)
	}
	f.record(trade, Owner{})
	if n := count(t, f.db, &LedgerOrder{}); n != 2 {
		t.Fatalf("orders = %d", n)
	}
	if fill := f.fill(trade.ExchangeTradeID); fill.OrderID != order.ID {
		t.Errorf("fill went to order %d", fill.OrderID)
	}
	if position := f.positions()[0]; position.CloseReason != "stop_loss" {
		t.Errorf("reason = %s", position.CloseReason)
	}
}
