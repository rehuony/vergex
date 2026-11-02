package api

import (
	"net/http"
	"vergex/internal/account"

	"github.com/gin-gonic/gin"
)

// planRunResult is what a finished plan realized, as the ledger has it.
type planRunResult struct {
	RealizedPnL float64 `json:"realized_pnl"`
	Closes      int     `json:"closes"`
	ClosedAt    int64   `json:"closed_at"` // Unix milliseconds of the last close.
}

// handleFuturesResults reads what an account's finished plans realized from the ledger.
// The account's fills get there through its sync, so this asks no exchange.
func (s *Server) handleFuturesResults(c *gin.Context) {
	user := c.GetString("user_id")
	p, err := s.futuresService()
	if err != nil {
		futuresError(c, err)
		return
	}
	account, err := s.accountReadConfig(user, "", c.Query("account_id"))
	if err != nil {
		futuresError(c, err)
		return
	}
	views, err := p.controller.ListContext(c.Request.Context(), user)
	if err != nil {
		futuresError(c, err)
		return
	}
	settled := map[string]bool{}
	for _, view := range views {
		if view.AccountID == account.ID && settledRunStates[view.State] {
			settled[view.ID] = true
		}
	}
	rows, err := s.stores.Ledger.FuturesResults(account.ID)
	if err != nil {
		SafeInternalError(c, "Get plan results", err)
		return
	}
	c.JSON(http.StatusOK, planResults(rows, settled))
}

// planResults keeps the results of runs with nothing left to watch.
func planResults(rows []account.RunResult, settled map[string]bool) map[string]planRunResult {
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
