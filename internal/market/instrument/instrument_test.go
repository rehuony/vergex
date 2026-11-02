package instrument_test

import (
	"testing"

	"vergex/internal/database/testdb"
	"vergex/internal/market/instrument"
)

func TestEnsureCreatesOnceAndKeepsKindsApart(t *testing.T) {
	db := testdb.Open(t)
	perpetual, err := instrument.Ensure(db, "binance", instrument.KindPerpetual, "BTCUSDT", "BTCUSDT")
	if err != nil {
		t.Fatal(err)
	}
	again, err := instrument.Ensure(db, "binance", instrument.KindPerpetual, "BTCUSDT", "BTCUSDT")
	if err != nil {
		t.Fatal(err)
	}
	if again != perpetual {
		t.Fatalf("second call made a new instrument: %d != %d", again, perpetual)
	}
	spot, err := instrument.Ensure(db, "binance", instrument.KindSpot, "BTCUSDT", "BTCUSDT")
	if err != nil {
		t.Fatal(err)
	}
	if spot == perpetual {
		t.Fatal("spot and perpetual share an instrument")
	}

	var rows []instrument.Instrument
	if err := db.Order("id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if rows[0].BaseAsset != "BTC" || rows[0].QuoteAsset != "USDT" || rows[0].SettleAsset == nil || *rows[0].SettleAsset != "USDT" {
		t.Fatalf("perpetual = %+v", rows[0])
	}
	if rows[1].SettleAsset != nil {
		t.Fatalf("spot settles in %s", *rows[1].SettleAsset)
	}
}
