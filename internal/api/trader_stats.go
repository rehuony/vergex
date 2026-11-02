package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleStatisticsFull returns the full set of computed performance metrics for
// a single trader: win rate, profit factor, Sharpe ratio, max drawdown, and the
// average win/loss amounts. These are derived from the trader's CLOSED positions
// via Positions.GetFullStatsByTraderFilters — the same computation the
// strategy engine feeds to the agent, so the dashboard and the model see identical
// numbers.
//
// The existing GET /statistics endpoint only returns cycle/position counts; this
// endpoint exposes the richer trade-quality metrics the terminal dashboard needs.
func (s *Server) handleStatisticsFull(c *gin.Context) {
	trader, err := s.ownedTrader(c)
	if err != nil {
		SafeBadRequest(c, "Invalid trader ID")
		return
	}

	stats, err := s.stores.Positions.GetFullStats(trader.ID, s.initialBalance(trader))
	if err != nil {
		SafeInternalError(c, "Get full statistics", err)
		return
	}

	c.JSON(http.StatusOK, stats)
}

// handleTraderMetrics summarises every trader of the account from stored data
// only, so listing them never calls an exchange however many there are.
func (s *Server) handleTraderMetrics(c *gin.Context) {
	traders, err := s.stores.Traders.List(c.GetString("user_id"))
	if err != nil {
		SafeInternalError(c, "Get traders", err)
		return
	}
	latest, err := s.stores.Equity.GetAllTradersLatest()
	if err != nil {
		SafeInternalError(c, "Get latest equity", err)
		return
	}

	metrics := make(map[string]gin.H, len(traders))
	for _, trader := range traders {
		initial := s.initialBalance(trader)
		row := gin.H{}
		if snapshot := latest[trader.ID]; snapshot != nil {
			row["equity"] = snapshot.TotalEquity
			row["equity_at"] = snapshot.Timestamp.Unix()
			if initial > 0 {
				row["return_pct"] = (snapshot.TotalEquity - initial) / initial * 100
			}
		}
		stats, err := s.stores.Positions.GetFullStats(trader.ID, initial)
		if err != nil {
			SafeInternalError(c, "Get trader statistics", err)
			return
		}
		if stats.TotalTrades > 0 {
			row["total_trades"] = stats.TotalTrades
			row["win_rate"] = stats.WinRate
			row["max_drawdown_pct"] = stats.MaxDrawdownPct
		}
		metrics[trader.ID] = row
	}
	c.JSON(http.StatusOK, metrics)
}
