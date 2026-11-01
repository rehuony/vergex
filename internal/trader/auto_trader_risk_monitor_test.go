package trader

import (
	"context"
	"errors"
	"sync"
	"testing"
	"vergex/internal/market"
	"vergex/internal/trading"
)

// fakeMarks is the live mark prices the monitor sees.
type fakeMarks struct {
	mu      sync.Mutex
	marks   map[string]float64
	watched []string
}

func (f *fakeMarks) Recent(context.Context, string, string, int) ([]market.Kline, error) {
	return nil, errors.New("not used")
}

func (f *fakeMarks) WatchMark(_, symbol string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.watched = append(f.watched, symbol)
}

func (f *fakeMarks) Mark(_, symbol string) (float64, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	price, ok := f.marks[symbol]
	return price, ok
}

func (f *fakeMarks) set(symbol string, price float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.marks[symbol] = price
}

// fakeHolder holds one position on an imaginary exchange. Embedding the interface
// makes any call the monitor should not make panic.
type fakeHolder struct {
	trading.Trader
	mark   float64
	failed bool
	reads  int
	closes int
}

func (f *fakeHolder) GetPositions() ([]map[string]any, error) {
	f.reads++
	if f.failed {
		return nil, errors.New("exchange down")
	}
	if f.closes > 0 {
		return nil, nil
	}
	return []map[string]any{{
		"symbol": "BTCUSDT", "side": "long", "entryPrice": 100.0, "markPrice": f.mark, "positionAmt": 1.0, "leverage": 10.0,
	}}, nil
}

func (f *fakeHolder) CloseLong(string, float64) (map[string]any, error) {
	f.closes++
	return map[string]any{"orderId": 1}, nil
}

func monitorFixture(t *testing.T) (*AutoTrader, *fakeHolder, *fakeMarks) {
	t.Helper()
	marks := &fakeMarks{marks: map[string]float64{}}
	market.SetLiveSource(marks)
	t.Cleanup(func() { market.SetLiveSource(noLiveSource{}) })
	holder := &fakeHolder{mark: 102}
	return &AutoTrader{exchange: "binance", trader: holder, peakPnLCache: map[string]float64{}}, holder, marks
}

// noLiveSource leaves other tests with no live data.
type noLiveSource struct{}

func (noLiveSource) Recent(context.Context, string, string, int) ([]market.Kline, error) {
	return nil, errors.New("no live data")
}
func (noLiveSource) WatchMark(string, string)            {}
func (noLiveSource) Mark(string, string) (float64, bool) { return 0, false }

func TestLiveMarksRaiseThePeakAndCallTheCheckOfRecord(t *testing.T) {
	at, holder, marks := monitorFixture(t)
	held := at.checkPositionDrawdown(nil)
	if len(held) != 1 || len(marks.watched) != 1 || marks.watched[0] != "BTCUSDT" {
		t.Fatalf("held = %+v, watched = %v", held, marks.watched)
	}

	// Between reads the price runs to +10%, which the minute check never saw.
	marks.set("BTCUSDT", 110)
	if at.marksCallForCheck(held) {
		t.Fatal("a new high is no reason to check")
	}
	if peak := at.GetPeakPnLCache()["BTCUSDT_long"]; peak != 100 {
		t.Fatalf("peak = %v, want the stream's high at 10x", peak)
	}

	// It gives back 40% of that, still above +5%.
	marks.set("BTCUSDT", 105.5)
	if !at.marksCallForCheck(held) {
		t.Fatal("the giveback should call the check")
	}
	if holder.closes != 0 {
		t.Fatal("the stream must not close anything itself")
	}

	// The exchange agrees, so the existing close runs and the position is forgotten.
	holder.mark = 105.5
	if held = at.checkPositionDrawdown(held); len(held) != 0 {
		t.Fatalf("held = %+v after closing", held)
	}
	if holder.closes != 1 {
		t.Fatalf("closes = %d", holder.closes)
	}
	if _, kept := at.GetPeakPnLCache()["BTCUSDT_long"]; kept {
		t.Fatal("the peak should be cleared after closing")
	}
}

func TestTheExchangeHasTheLastWordOnClosing(t *testing.T) {
	at, holder, marks := monitorFixture(t)
	held := at.checkPositionDrawdown(nil)
	marks.set("BTCUSDT", 110)
	at.marksCallForCheck(held)
	marks.set("BTCUSDT", 105.5)
	if !at.marksCallForCheck(held) {
		t.Fatal("the giveback should call the check")
	}

	// By the time the exchange answers, the price is back up.
	holder.mark = 109
	if held = at.checkPositionDrawdown(held); len(held) != 1 {
		t.Fatalf("held = %+v", held)
	}
	if holder.closes != 0 {
		t.Fatal("closed on a mark the exchange does not confirm")
	}
}

func TestMonitorWithoutAStreamBehavesAsBefore(t *testing.T) {
	at, holder, _ := monitorFixture(t)
	held := at.checkPositionDrawdown(nil)
	// No marks arrive at all.
	if at.marksCallForCheck(held) {
		t.Fatal("no marks, nothing to call a check for")
	}
	holder.mark = 110
	held = at.checkPositionDrawdown(held)
	holder.mark = 105.5
	at.checkPositionDrawdown(held)
	if holder.closes != 1 {
		t.Fatalf("closes = %d, the minute check must still work alone", holder.closes)
	}
}

func TestAFailedReadKeepsWatchingWhatWasHeld(t *testing.T) {
	at, holder, _ := monitorFixture(t)
	held := at.checkPositionDrawdown(nil)
	holder.failed = true
	if again := at.checkPositionDrawdown(held); len(again) != 1 {
		t.Fatalf("held = %+v after a failed read", again)
	}
}

func TestSmallMovesNeverArm(t *testing.T) {
	at, holder, marks := monitorFixture(t)
	held := at.checkPositionDrawdown(nil)
	marks.set("BTCUSDT", 103)
	at.marksCallForCheck(held)
	marks.set("BTCUSDT", 100.5)
	if at.marksCallForCheck(held) {
		t.Fatal("under +5% the monitor is not armed, however large the giveback")
	}
	if holder.closes != 0 {
		t.Fatal("closed an unarmed position")
	}
}

func TestShortPositionsTrackTheOtherWay(t *testing.T) {
	at, _, marks := monitorFixture(t)
	held := []heldPosition{{symbol: "ETHUSDT", side: "short", entryPrice: 100, leverage: 5}}
	marks.set("ETHUSDT", 90)
	at.marksCallForCheck(held)
	if peak := at.GetPeakPnLCache()["ETHUSDT_short"]; peak != 50 {
		t.Fatalf("peak = %v", peak)
	}
	marks.set("ETHUSDT", 94.5)
	if !at.marksCallForCheck(held) {
		t.Fatal("a short giving back 45% at +5.5% should call the check")
	}
}
