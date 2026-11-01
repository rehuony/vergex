package store_test

import (
	"testing"
	"time"

	"vergex/internal/market/instrument"
	"vergex/internal/store"
	"vergex/internal/store/storetest"

	"github.com/shopspring/decimal"
)

func TestFrontierResultsAddUpARunsClosedPositions(t *testing.T) {
	st := storetest.New(t)
	t.Cleanup(func() { _ = st.Close() })
	ledger := storetest.NewLedger(t, st)
	btc, err := instrument.Ensure(st.GormDB(), "binance", instrument.KindPerpetual, "BTCUSDT", "BTCUSDT")
	if err != nil {
		t.Fatal(err)
	}
	eth, err := instrument.Ensure(st.GormDB(), "binance", instrument.KindPerpetual, "ETHUSDT", "ETHUSDT")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	trade := func(instrumentID int64, id, side string, realized string, owner string) {
		t.Helper()
		record := store.Trade{
			AccountID: ledger.Account, InstrumentID: instrumentID, ExchangeTradeID: id, ExchangeOrderID: "order-" + id, Side: side, PositionSide: store.SideLong,
			Price: decimal.NewFromInt(100), Quantity: decimal.NewFromInt(1), QuoteQuantity: decimal.NewFromInt(100), CommissionAsset: "USDT", ExecutedAt: now,
		}
		if realized != "" {
			record.RealizedPnL = decimal.NewNullDecimal(decimal.RequireFromString(realized))
		}
		if _, err := st.Ledger().RecordTrade(record, store.Owner{Source: store.SourceFrontier, SourceID: owner}); err != nil {
			t.Fatal(err)
		}
	}
	trade(btc, "1", store.SideBuy, "", "finished")
	trade(btc, "2", store.SideSell, "12.5", "finished")
	trade(eth, "3", store.SideBuy, "", "still-open")
	ledger.Open("a-trader", "SOLUSDT", store.SideLong, 1, 100, now)
	ledger.Close("a-trader", "SOLUSDT", store.SideLong, 1, 110, 10, now)

	results, err := st.Ledger().FrontierResults(ledger.Account)
	if err != nil || len(results) != 1 {
		t.Fatalf("results = %+v, err %v", results, err)
	}
	got := results[0]
	if got.SourceID != "finished" || got.RealizedPnL != 12.5 || got.Closes != 1 || got.ClosedAt == nil {
		t.Fatalf("result = %+v", got)
	}
}
