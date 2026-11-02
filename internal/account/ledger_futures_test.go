package account_test

import (
	"testing"
	"time"

	"vergex/internal/account"
	"vergex/internal/account/accounttest"
	"vergex/internal/database/testdb"
	"vergex/internal/market/instrument"

	"github.com/shopspring/decimal"
)

func TestFuturesResultsAddUpARunsClosedPositions(t *testing.T) {
	db := testdb.Open(t)
	ledger := accounttest.NewLedger(t, db)
	book := account.NewLedger(db)
	btc, err := instrument.Ensure(db, "binance", instrument.KindPerpetual, "BTCUSDT", "BTCUSDT")
	if err != nil {
		t.Fatal(err)
	}
	eth, err := instrument.Ensure(db, "binance", instrument.KindPerpetual, "ETHUSDT", "ETHUSDT")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	trade := func(instrumentID int64, id, side string, realized string, owner string) {
		t.Helper()
		record := account.Trade{
			AccountID: ledger.Account, InstrumentID: instrumentID, ExchangeTradeID: id, ExchangeOrderID: "order-" + id, Side: side, PositionSide: account.SideLong,
			Price: decimal.NewFromInt(100), Quantity: decimal.NewFromInt(1), QuoteQuantity: decimal.NewFromInt(100), CommissionAsset: "USDT", ExecutedAt: now,
		}
		if realized != "" {
			record.RealizedPnL = decimal.NewNullDecimal(decimal.RequireFromString(realized))
		}
		if _, err := book.RecordTrade(record, account.Owner{Source: account.SourceFutures, SourceID: owner}); err != nil {
			t.Fatal(err)
		}
	}
	trade(btc, "1", account.SideBuy, "", "finished")
	trade(btc, "2", account.SideSell, "12.5", "finished")
	trade(eth, "3", account.SideBuy, "", "still-open")
	ledger.Open("a-trader", "SOLUSDT", account.SideLong, 1, 100, now)
	ledger.Close("a-trader", "SOLUSDT", account.SideLong, 1, 110, 10, now)

	results, err := book.FuturesResults(ledger.Account)
	if err != nil || len(results) != 1 {
		t.Fatalf("results = %+v, err %v", results, err)
	}
	got := results[0]
	if got.SourceID != "finished" || got.RealizedPnL != 12.5 || got.Closes != 1 || got.ClosedAt == nil {
		t.Fatalf("result = %+v", got)
	}
}
