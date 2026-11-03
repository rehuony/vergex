package instrument_test

import (
	"testing"

	"vergex/internal/database/testdb"
	"vergex/internal/market/instrument"
)

func TestEnsureCreatesOnceAndKeepsKindsApart(t *testing.T) {
	db := testdb.Open(t)
	perpetual, err := instrument.Ensure(db, "binance", instrument.KindPerpetual, "BTCUSDT")
	if err != nil {
		t.Fatal(err)
	}
	again, err := instrument.Ensure(db, "binance", instrument.KindPerpetual, "BTCUSDT")
	if err != nil {
		t.Fatal(err)
	}
	if again != perpetual {
		t.Fatalf("second call made a new instrument: %d != %d", again, perpetual)
	}
	spot, err := instrument.Ensure(db, "binance", instrument.KindSpot, "BTCUSDT")
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
	if len(rows) != 2 || rows[0].Kind != instrument.KindPerpetual || rows[1].Kind != instrument.KindSpot || rows[0].Symbol != rows[1].Symbol {
		t.Fatalf("instruments = %+v", rows)
	}
}
