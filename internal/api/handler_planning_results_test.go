package api

import (
	"testing"
	"time"
	"vergex/internal/store"
)

func TestOnlyFinishedPlansShowAResult(t *testing.T) {
	closed := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	results := planResults([]store.RunResult{
		{SourceID: "finished", RealizedPnL: 8, Closes: 2, ClosedAt: &closed},
		{SourceID: "running", RealizedPnL: 99, Closes: 1, ClosedAt: &closed},
	}, map[string]bool{"finished": true})

	if len(results) != 1 {
		t.Fatalf("results = %+v", results)
	}
	if got := results["finished"]; got.RealizedPnL != 8 || got.Closes != 2 || got.ClosedAt != closed.UnixMilli() {
		t.Fatalf("finished = %+v", got)
	}
}
