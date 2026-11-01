package api

import (
	"net/http"
	"vergex/internal/store"

	"github.com/gin-gonic/gin"
)

// planRunResult is what a finished plan realized, as the ledger has it.
type planRunResult struct {
	RealizedPnL float64 `json:"realized_pnl"`
	Closes      int     `json:"closes"`
	ClosedAt    int64   `json:"closed_at"` // Unix milliseconds of the last close.
}

// handlePlanningResults reads what an account's finished plans realized from the ledger.
// The account's fills get there through its sync, so this asks no exchange.
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
	settled := map[string]bool{}
	for _, view := range views {
		if view.ExchangeID == exchange.ID && settledRunStates[view.State] {
			settled[view.ID] = true
		}
	}
	rows, err := s.store.Ledger().FrontierResults(exchange.ID)
	if err != nil {
		SafeInternalError(c, "Get plan results", err)
		return
	}
	c.JSON(http.StatusOK, planResults(rows, settled))
}

// planResults keeps the results of runs with nothing left to watch.
func planResults(rows []store.RunResult, settled map[string]bool) map[string]planRunResult {
	results := map[string]planRunResult{}
	for _, row := range rows {
		if !settled[row.SourceID] {
			continue
		}
		result := planRunResult{RealizedPnL: row.RealizedPnL, Closes: row.Closes}
		if row.ClosedAt != nil {
			result.ClosedAt = row.ClosedAt.UnixMilli()
		}
		results[row.SourceID] = result
	}
	return results
}
