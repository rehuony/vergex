package forecast

import (
	"math"
	"testing"
)

func projectionResult(direction string) Result {
	return Result{Close: 100, Direction: direction, ExpectedMovePct: 4, Horizon: 4}
}

func TestProjectGoesTheWayOfTheCallBySizeOfTheScore(t *testing.T) {
	rows := Project(projectionResult(DirectionUp), 0.75, 3_600_000, 3_600_000)
	if len(rows) != 4 {
		t.Fatalf("rows = %d, want the horizon", len(rows))
	}
	if rows[0].Time != 7200 || rows[3].Time != 18000 {
		t.Fatalf("times = %d..%d, want the bars after the forecast", rows[0].Time, rows[3].Time)
	}
	// Score 75 is half way to certainty, so half the 4% expected move.
	if end := rows[3].Close; math.Abs(end-102) > 1e-9 {
		t.Fatalf("end = %v, want 102", end)
	}
	for i, row := range rows {
		if row.Low > math.Min(row.Open, row.Close) || row.High < math.Max(row.Open, row.Close) {
			t.Fatalf("row %d is not a candle: %+v", i, row)
		}
		// One typical bar's range: 4% over sqrt(4) bars.
		if span := row.High - row.Low; math.Abs(span-2) > 1e-9 {
			t.Fatalf("row %d spans %v, want 2", i, span)
		}
		if i > 0 && row.Open != rows[i-1].Close {
			t.Fatalf("row %d does not continue the last", i)
		}
	}
	if rows[0].Open != 100 {
		t.Fatalf("first open = %v, want the forecast's close", rows[0].Open)
	}

	down := Project(projectionResult(DirectionDown), 0.25, 0, 3_600_000)
	if math.Abs(down[3].Close-98) > 1e-9 {
		t.Fatalf("down end = %v, want 98", down[3].Close)
	}
}

func TestProjectStaysFlatOnANeutralCall(t *testing.T) {
	rows := Project(projectionResult(DirectionNeutral), 0.52, 0, 3_600_000)
	for _, row := range rows {
		if row.Open != 100 || row.Close != 100 {
			t.Fatalf("row = %+v, a neutral call should stay flat", row)
		}
	}
}

func TestProjectIsTheSameEveryTime(t *testing.T) {
	first := Project(projectionResult(DirectionUp), 0.7, 0, 60_000)
	second := Project(projectionResult(DirectionUp), 0.7, 0, 60_000)
	for i := range first {
		if first[i] != second[i] {
			t.Fatal("the projection should not vary")
		}
	}
}

func TestProjectNeedsAHorizonAndAPrice(t *testing.T) {
	if rows := Project(Result{Close: 100}, 0.7, 0, 60_000); rows != nil {
		t.Fatalf("rows = %+v without a horizon", rows)
	}
	if rows := Project(Result{Horizon: 4}, 0.7, 0, 60_000); rows != nil {
		t.Fatalf("rows = %+v without a price", rows)
	}
}
