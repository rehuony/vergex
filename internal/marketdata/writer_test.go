package marketdata

import (
	"context"
	"testing"
	"time"

	"vergex/internal/provider/binance"
	"vergex/internal/provider/wsfeed"
	"vergex/internal/testdb"
)

func writerFixture(t *testing.T) (*Writer, *Store, *Series, *int) {
	t.Helper()
	store, err := NewStore(testdb.Open(t))
	if err != nil {
		t.Fatal(err)
	}
	now := base() + 100*hour + 1000
	series, err := store.EnsureSeries("BTCUSDT", "1h", TierWatch, now)
	if err != nil {
		t.Fatal(err)
	}
	series.FirstOpenTime = base()
	series.LastClosedTime = base() + 97*hour
	if err := store.WriteCandlesForTest(series, []binance.Candle{closedBar(base() + 97*hour)}); err != nil {
		t.Fatal(err)
	}
	holes := 0
	writer := NewWriter(store, nil, func() { holes++ }, nil)
	writer.now = func() time.Time { return time.UnixMilli(now) }
	return writer, store, series, &holes
}

func closedBar(openTime int64) binance.Candle {
	return binance.Candle{OpenTime: openTime, Open: 100, High: 110, Low: 90, Close: 105, Volume: 5}
}

func streamBar(openTime int64) wsfeed.Candle {
	return wsfeed.Candle{Symbol: "BTCUSDT", Interval: "1h", OpenTime: openTime, Open: 100, High: 110, Low: 90, Close: 105, Volume: 5, Closed: true}
}

func TestWriterExtendsASeriesAndNeedsNoREST(t *testing.T) {
	writer, store, series, holes := writerFixture(t)
	wrote := 0
	writer.written = func() { wrote++ }

	writer.flush([]wsfeed.Candle{streamBar(base() + 99*hour), streamBar(base() + 98*hour)})

	stored, err := store.FindSeries("BTCUSDT", "1h")
	if err != nil {
		t.Fatal(err)
	}
	if stored.LastClosedTime != base()+99*hour {
		t.Fatalf("water mark = %d, want it past both candles", stored.LastClosedTime)
	}
	if want := base() + 101*hour + settleDelay.Milliseconds(); stored.NextDueAt != want {
		t.Fatalf("next due = %d, want %d: the hydrator has nothing to do until the next close", stored.NextDueAt, want)
	}
	if count, _ := store.CandleCount(series.ID); count != 3 {
		t.Fatalf("candles = %d, want 3", count)
	}
	if *holes != 0 || wrote != 1 {
		t.Fatalf("holes = %d, wrote = %d", *holes, wrote)
	}
}

func TestWriterLeavesAHoleToTheHydrator(t *testing.T) {
	writer, store, series, holes := writerFixture(t)

	// The candle after the water mark never arrived, as across a reconnect.
	writer.flush([]wsfeed.Candle{streamBar(base() + 99*hour)})

	stored, _ := store.FindSeries("BTCUSDT", "1h")
	if stored.LastClosedTime != base()+97*hour {
		t.Fatalf("water mark moved to %d across a hole", stored.LastClosedTime)
	}
	if stored.NextDueAt != writer.now().UnixMilli() {
		t.Fatal("the series should fall due at once")
	}
	if count, _ := store.CandleCount(series.ID); count != 1 {
		t.Fatalf("candles = %d, nothing past the hole may be stored", count)
	}
	if *holes != 1 {
		t.Fatalf("holes = %d, the hydrator should be told", *holes)
	}
}

func TestWriterRefusesImpossibleCandles(t *testing.T) {
	writer, store, series, holes := writerFixture(t)
	bad := streamBar(base() + 98*hour)
	bad.High = 1

	writer.flush([]wsfeed.Candle{bad})

	stored, _ := store.FindSeries("BTCUSDT", "1h")
	if stored.LastClosedTime != base()+97*hour {
		t.Fatal("a bad candle moved the water mark")
	}
	if count, _ := store.CandleCount(series.ID); count != 1 || *holes != 1 {
		t.Fatalf("candles = %d, holes = %d", count, *holes)
	}
}

func TestWriterIgnoresWhatItHasAndWhatNobodyTracks(t *testing.T) {
	writer, store, series, holes := writerFixture(t)
	wrote := 0
	writer.written = func() { wrote++ }
	before, _ := store.FindSeries("BTCUSDT", "1h")

	other := streamBar(base() + 98*hour)
	other.Symbol = "ETHUSDT"
	writer.flush([]wsfeed.Candle{streamBar(base() + 97*hour), other})

	after, _ := store.FindSeries("BTCUSDT", "1h")
	if after.NextDueAt != before.NextDueAt || after.LastClosedTime != before.LastClosedTime {
		t.Fatal("a repeated candle should change nothing")
	}
	if count, _ := store.CandleCount(series.ID); count != 1 || *holes != 0 || wrote != 0 {
		t.Fatalf("candles = %d, holes = %d, wrote = %d", count, *holes, wrote)
	}
	if found, _ := store.FindSeries("ETHUSDT", "1h"); found != nil {
		t.Fatal("the writer should not create series")
	}
}

func TestWriterDoesNotStartASeriesBeforeItsHistory(t *testing.T) {
	writer, store, _, holes := writerFixture(t)
	fresh, _ := store.EnsureSeries("ETHUSDT", "1h", TierOnDemand, base())
	bar := streamBar(base() + 99*hour)
	bar.Symbol = "ETHUSDT"

	writer.flush([]wsfeed.Candle{bar})

	if count, _ := store.CandleCount(fresh.ID); count != 0 || *holes != 0 {
		t.Fatalf("candles = %d, holes = %d", count, *holes)
	}
}

func TestAnOlderViewOfASeriesCannotUndoTheWaterMark(t *testing.T) {
	writer, store, _, _ := writerFixture(t)
	stale, _ := store.FindSeries("BTCUSDT", "1h")

	writer.flush([]wsfeed.Candle{streamBar(base() + 98*hour)})

	// What the hydrator does with the copy it read before the writer ran.
	stale.FailCount = 1
	if err := store.saveSeries(stale); err != nil {
		t.Fatal(err)
	}
	stored, _ := store.FindSeries("BTCUSDT", "1h")
	if stored.LastClosedTime != base()+98*hour {
		t.Fatalf("water mark went back to %d", stored.LastClosedTime)
	}
	if stored.FailCount != 1 {
		t.Fatal("the other fields should still be saved")
	}
}

func TestWriterWritesWhatIsLeftWhenStopped(t *testing.T) {
	writer, store, _, _ := writerFixture(t)
	in := make(chan wsfeed.Candle, 1)
	writer.in = in
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		writer.Run(ctx)
		close(done)
	}()
	in <- streamBar(base() + 98*hour)
	for len(in) > 0 {
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("writer did not stop")
	}
	stored, _ := store.FindSeries("BTCUSDT", "1h")
	if stored.LastClosedTime != base()+98*hour {
		t.Fatalf("water mark = %d", stored.LastClosedTime)
	}
}
