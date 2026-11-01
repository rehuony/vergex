package trader

import (
	"testing"
	"time"

	"vergex/internal/market/instrument"
	"vergex/internal/store"
	"vergex/internal/store/storetest"

	"github.com/shopspring/decimal"
)

func ledgerTrader(t *testing.T) (*AutoTrader, *store.Store, *storetest.Ledger) {
	t.Helper()
	st := storetest.New(t)
	t.Cleanup(func() { _ = st.Close() })
	ledger := storetest.NewLedger(t, st)
	return &AutoTrader{store: st, id: "t1", exchange: "binance", exchangeID: ledger.Account, config: AutoTraderConfig{IsCrossMargin: true}}, st, ledger
}

func TestAPlacedOrderIsRecordedAndItsFillsJoinIt(t *testing.T) {
	at, st, ledger := ledgerTrader(t)
	recorded := at.recordOrder(map[string]interface{}{"orderId": int64(77), "clientOrderId": "vergex-1"}, "BTCUSDT", "open_long", 0.5, 10)

	orders, err := st.Order().GetTraderOrders("t1", 10)
	if err != nil || len(orders) != 1 {
		t.Fatalf("orders = %d, err %v", len(orders), err)
	}
	order := orders[0]
	if recorded == 0 || order.ID != recorded {
		t.Fatalf("recorded as %d, order is %d", recorded, order.ID)
	}
	if order.ExchangeOrderID != "77" || order.ClientOrderID != "vergex-1" || order.Status != store.OrderNew || order.OrderAction != "open_long" || order.Leverage != 10 || order.Quantity != 0.5 {
		t.Fatalf("order = %+v", order)
	}

	// Recording it again, as a retry would, names the same order.
	if again := at.recordOrder(map[string]interface{}{"orderId": int64(77), "clientOrderId": "vergex-1"}, "BTCUSDT", "open_long", 0.5, 10); again != recorded {
		t.Fatalf("recorded again as %d, want %d", again, recorded)
	}

	// The account's sync reports the fill, and it joins the order rather than making one.
	btc, err := instrument.Ensure(st.GormDB(), "binance", instrument.KindPerpetual, "BTCUSDT", "BTCUSDT")
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.Ledger().RecordTrade(store.Trade{
		AccountID: ledger.Account, InstrumentID: btc, ExchangeTradeID: "t-1", ExchangeOrderID: "77", Side: store.SideBuy, PositionSide: store.SideLong,
		Price: decimal.NewFromInt(100), Quantity: decimal.RequireFromString("0.5"), QuoteQuantity: decimal.NewFromInt(50), ExecutedAt: time.Now(),
	}, store.Owner{})
	if err != nil {
		t.Fatal(err)
	}
	orders, err = st.Order().GetTraderOrders("t1", 10)
	if err != nil || len(orders) != 1 || orders[0].Status != store.OrderFilled {
		t.Fatalf("orders = %+v, err %v", orders, err)
	}
	// The real leverage reaches the position, and with it what the model sees.
	position, err := st.Position().GetOpenPositionBySymbol("t1", "BTCUSDT", "long")
	if err != nil || position == nil || position.Leverage != 10 {
		t.Fatalf("position = %+v, err %v", position, err)
	}
}

func TestEachActionIsRecordedAsWhatItDoes(t *testing.T) {
	at, st, _ := ledgerTrader(t)
	for i, action := range []string{"open_long", "open_short", "close_long", "close_short"} {
		at.recordOrder(map[string]interface{}{"orderId": float64(i + 1)}, "BTCUSDT", action, 1, 0)
	}
	orders, err := st.Order().GetTraderOrders("t1", 10)
	if err != nil || len(orders) != 4 {
		t.Fatalf("orders = %d, err %v", len(orders), err)
	}
	want := map[string]struct {
		action, side string
		reduceOnly   bool
	}{
		"1": {"open_long", "BUY", false},
		"2": {"open_short", "SELL", false},
		"3": {"close_long", "SELL", true},
		"4": {"close_short", "BUY", true},
	}
	for _, order := range orders {
		expected := want[order.ExchangeOrderID]
		if order.OrderAction != expected.action || order.Side != expected.side || order.ReduceOnly != expected.reduceOnly {
			t.Errorf("order %s = %+v", order.ExchangeOrderID, order)
		}
	}
}

func TestAnOrderWithNoIDIsNotRecorded(t *testing.T) {
	at, st, _ := ledgerTrader(t)
	for _, result := range []map[string]interface{}{{}, {"orderId": ""}, {"orderId": int64(0)}} {
		at.recordOrder(result, "BTCUSDT", "open_long", 1, 5)
	}
	if orders, err := st.Order().GetTraderOrders("t1", 10); err != nil || len(orders) != 0 {
		t.Fatalf("orders = %+v, err %v", orders, err)
	}
}

func TestAGridOrderIsRecordedWithItsPrice(t *testing.T) {
	at, st, _ := ledgerTrader(t)
	at.recordGridOrder(
		&LimitOrderRequest{Symbol: "BTCUSDT", Side: "BUY", Price: 95, Quantity: 0.1, Leverage: 3, PostOnly: true},
		&LimitOrderResult{OrderID: "grid-1", ClientID: "c-1", PositionSide: "BOTH"},
	)
	orders, err := st.Order().GetTraderOrders("t1", 10)
	if err != nil || len(orders) != 1 {
		t.Fatalf("orders = %d, err %v", len(orders), err)
	}
	order := orders[0]
	if order.Role != store.RoleGrid || order.Type != "LIMIT" || order.Price != 95 || order.Side != "BUY" || order.PositionSide != "" || order.Leverage != 3 {
		t.Fatalf("order = %+v", order)
	}
}

// Cooling down reads the ledger, so a stop that fired on the exchange counts as a close.
func TestThrottleSeesRealOrdersAndFiredStops(t *testing.T) {
	at, _, ledger := ledgerTrader(t)
	now := time.Now()
	since := now.Add(-time.Hour)
	if got := at.findRecentCloseOrder("BTCUSDT", since); got != nil {
		t.Fatalf("no close yet, got %+v", got)
	}

	ledger.Open("t1", "BTCUSDT", store.SideLong, 1, 100, now.Add(-10*time.Minute))
	if got := at.findRecentOpenOrder("BTCUSDT", "long", since); got == nil {
		t.Fatal("the open was not seen")
	}
	if got := at.findRecentOpenOrder("BTCUSDT", "short", since); got != nil {
		t.Fatalf("no short was opened, got %+v", got)
	}
	if got := at.findRecentOpenOrder("BTCUSDT", "long", now); got != nil {
		t.Fatalf("too old to count, got %+v", got)
	}

	// Nothing recorded this order: it is known only through its fill.
	ledger.Close("someone-else", "BTCUSDT", store.SideLong, 1, 90, -10, now.Add(-time.Minute))
	if got := at.findRecentCloseOrder("BTCUSDT", since); got == nil {
		t.Fatal("the fired stop was not seen as a close")
	}
}
