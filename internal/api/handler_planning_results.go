package api

import (
	"net/http"
	"time"

	"vergex/internal/market"
	"vergex/internal/planning/live"
	"vergex/internal/trader/types"

	"github.com/gin-gonic/gin"
)

const (
	// planResultsLookback bounds how far back the exchange is asked. An older plan
	// shows no result rather than a wrong one.
	planResultsLookback = 30 * 24 * time.Hour
	planResultsLimit    = 500
	// A close can land just after the run recorded its last change.
	planResultsGrace = 2 * time.Minute
)

// planRunResult is what a finished plan realized, as the exchange reports it.
type planRunResult struct {
	RealizedPnL float64 `json:"realized_pnl"`
	Closes      int     `json:"closes"`
	ClosedAt    int64   `json:"closed_at"` // Unix milliseconds of the last close.
}

// handlePlanningResults reads an account's closed P/L from the exchange and
// credits each close to the finished plan it belongs to. It only reads.
func (s *Server) handlePlanningResults(c *gin.Context) {
	user := c.GetString("user_id")
	p, err := s.planningService()
	if err != nil {
		planningError(c, err)
		return
	}
	exchange, err := s.accountReadConfig(user, "", c.Query("exchange_id"))
	if err != nil {
		planningError(c, err)
		return
	}
	views, err := p.live.ListContext(c.Request.Context(), user)
	if err != nil {
		planningError(c, err)
		return
	}
	var runs []live.View
	since := time.Now().UTC()
	for _, view := range views {
		if view.ExchangeID != exchange.ID || !settledRunStates[view.State] || !runHasFill(view) {
			continue
		}
		runs = append(runs, view)
		if start := runStart(view); start.Before(since) {
			since = start
		}
	}
	if len(runs) == 0 {
		c.JSON(http.StatusOK, gin.H{})
		return
	}
	if oldest := time.Now().UTC().Add(-planResultsLookback); since.Before(oldest) {
		since = oldest
	}
	reader, err := buildExchangeProbeTrader(exchange, user)
	if err != nil {
		planningError(c, err)
		return
	}
	records, err := reader.GetClosedPnL(since, planResultsLimit)
	if err != nil {
		SafeInternalError(c, "Get closed P/L", err)
		return
	}
	c.JSON(http.StatusOK, attributeClosedPnL(runs, records))
}

func runStart(run live.View) time.Time {
	if run.StartedAt != nil {
		return *run.StartedAt
	}
	return run.ApprovedAt
}

func runHasFill(run live.View) bool {
	for _, order := range run.Orders {
		if decimalValue(string(order.Filled)) > 0 {
			return true
		}
	}
	return false
}

// attributeClosedPnL credits each close to the run on the same symbol whose
// lifetime contains it. Account leases keep one unresolved run per account, so
// the windows of one account's runs do not overlap.
func attributeClosedPnL(runs []live.View, records []types.ClosedPnLRecord) map[string]planRunResult {
	results := map[string]planRunResult{}
	for _, record := range records {
		symbol := market.Normalize(record.Symbol)
		for _, run := range runs {
			if market.Normalize(run.Plan.Snapshot.Symbol) != symbol {
				continue
			}
			if record.ExitTime.Before(runStart(run)) || record.ExitTime.After(run.UpdatedAt.Add(planResultsGrace)) {
				continue
			}
			result := results[run.ID]
			result.RealizedPnL += record.RealizedPnL
			result.Closes++
			if at := record.ExitTime.UnixMilli(); at > result.ClosedAt {
				result.ClosedAt = at
			}
			results[run.ID] = result
			break
		}
	}
	return results
}
