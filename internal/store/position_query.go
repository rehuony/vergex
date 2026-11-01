package store

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// TraderStats trading statistics metrics
type TraderStats struct {
	TotalTrades    int     `json:"total_trades"`
	WinTrades      int     `json:"win_trades"`
	LossTrades     int     `json:"loss_trades"`
	WinRate        float64 `json:"win_rate"`
	ProfitFactor   float64 `json:"profit_factor"`
	SharpeRatio    float64 `json:"sharpe_ratio"`
	TotalPnL       float64 `json:"total_pnl"`
	TotalFee       float64 `json:"total_fee"`
	AvgWin         float64 `json:"avg_win"`
	AvgLoss        float64 `json:"avg_loss"`
	MaxDrawdownPct float64 `json:"max_drawdown_pct"`
}

// RealizedOnAccounts sums the realized P/L of positions closed on the accounts since
// the given time (Unix milliseconds), whoever traded them.
func (s *PositionStore) RealizedOnAccounts(accountIDs []string, sinceMs int64) (float64, error) {
	if len(accountIDs) == 0 {
		return 0, nil
	}
	var total float64
	err := s.db.Table("trading.positions").
		Select("COALESCE(SUM(realized_pnl), 0)").
		Where("status = ? AND account_id IN ? AND closed_at >= ?", PositionClosed, accountIDs, time.UnixMilli(sinceMs).UTC()).
		Scan(&total).Error
	if err != nil {
		return 0, fmt.Errorf("failed to sum realized P/L: %w", err)
	}
	return total, nil
}

// UpdatedSince returns the positions updated at or after a time, oldest change first.
func (s *PositionStore) UpdatedSince(sinceMs int64, limit int) ([]*TraderPosition, error) {
	var positions []*TraderPosition
	err := s.positions().Where("p.updated_at >= ?", time.UnixMilli(sinceMs).UTC()).Order("p.updated_at ASC").Limit(limit).Find(&positions).Error
	return positions, err
}

// afterMilli is the first moment past a time in Unix milliseconds. Callers remember
// the time of the last row they read, cut to milliseconds, and a stored time is finer,
// so a plain comparison would hand that row back.
func afterMilli(sinceMs int64) time.Time {
	return time.UnixMilli(sinceMs + 1).UTC()
}

// ClosedSince returns positions closed after sinceMs, oldest first.
func (s *PositionStore) ClosedSince(traderIDs []string, sinceMs int64, limit int) ([]*TraderPosition, error) {
	if len(traderIDs) == 0 {
		return nil, nil
	}
	var positions []*TraderPosition
	err := s.positions().Where("p.status = ? AND p.source = ? AND p.source_id IN ? AND p.closed_at >= ?", PositionClosed, SourceTrader, traderIDs, afterMilli(sinceMs)).
		Order("p.closed_at ASC").
		Limit(limit).
		Find(&positions).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query closed positions: %w", err)
	}
	return positions, nil
}

// GetFullStats gets complete trading statistics. startingEquity is the real
// account baseline used for the drawdown equity curve; pass 0 when unknown.
func (s *PositionStore) GetFullStats(traderID string, startingEquity float64) (*TraderStats, error) {
	stats := &TraderStats{}

	var positions []TraderPosition
	err := s.closedPositions(traderID).
		Order("p.closed_at ASC").
		Find(&positions).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query position statistics: %w", err)
	}

	var pnls []float64
	var totalWin, totalLoss float64

	for _, pos := range positions {
		stats.TotalTrades++
		stats.TotalPnL += pos.RealizedPnL
		stats.TotalFee += pos.Fee
		pnls = append(pnls, pos.RealizedPnL)

		if pos.RealizedPnL > 0 {
			stats.WinTrades++
			totalWin += pos.RealizedPnL
		} else if pos.RealizedPnL < 0 {
			stats.LossTrades++
			totalLoss += -pos.RealizedPnL
		}
	}

	if stats.TotalTrades > 0 {
		stats.WinRate = float64(stats.WinTrades) / float64(stats.TotalTrades) * 100
	}
	if totalLoss > 0 {
		stats.ProfitFactor = totalWin / totalLoss
	}
	if stats.WinTrades > 0 {
		stats.AvgWin = totalWin / float64(stats.WinTrades)
	}
	if stats.LossTrades > 0 {
		stats.AvgLoss = totalLoss / float64(stats.LossTrades)
	}
	if len(pnls) > 1 {
		stats.SharpeRatio = calculateSharpeRatioFromPnls(pnls)
	}
	if len(pnls) > 0 {
		stats.MaxDrawdownPct = calculateMaxDrawdownFromPnls(pnls, startingEquity)
	}

	return stats, nil
}

// RecentTrade recent trade record
type RecentTrade struct {
	Symbol       string  `json:"symbol"`
	Side         string  `json:"side"`
	EntryPrice   float64 `json:"entry_price"`
	ExitPrice    float64 `json:"exit_price"`
	RealizedPnL  float64 `json:"realized_pnl"`
	PnLPct       float64 `json:"pnl_pct"`
	EntryTime    int64   `json:"entry_time"`
	ExitTime     int64   `json:"exit_time"`
	HoldDuration string  `json:"hold_duration"`
}

// GetRecentTrades gets recent closed trades
func (s *PositionStore) GetRecentTrades(traderID string, limit int) ([]RecentTrade, error) {
	var positions []TraderPosition
	err := s.closedPositions(traderID).
		Order("p.closed_at DESC").
		Limit(limit).
		Find(&positions).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query recent trades: %w", err)
	}

	var trades []RecentTrade
	for _, pos := range positions {
		t := RecentTrade{
			Symbol:      pos.Symbol,
			Side:        strings.ToLower(pos.Side),
			EntryPrice:  pos.EntryPrice,
			ExitPrice:   pos.ExitPrice,
			RealizedPnL: pos.RealizedPnL,
			EntryTime:   pos.EntryTime / 1000, // Convert ms to seconds for API compatibility
		}

		if pos.ExitTime > 0 {
			t.ExitTime = pos.ExitTime / 1000 // Convert ms to seconds
			durationMs := pos.ExitTime - pos.EntryTime
			t.HoldDuration = formatDurationMs(durationMs)
		}

		if pos.EntryPrice > 0 {
			// A position known only through its fills has no leverage on record.
			leverage := float64(max(pos.Leverage, 1))
			if t.Side == "long" {
				t.PnLPct = (pos.ExitPrice - pos.EntryPrice) / pos.EntryPrice * 100 * leverage
			} else {
				t.PnLPct = (pos.EntryPrice - pos.ExitPrice) / pos.EntryPrice * 100 * leverage
			}
		}

		trades = append(trades, t)
	}

	return trades, nil
}

// calculateSharpeRatioFromPnls calculates Sharpe ratio
func calculateSharpeRatioFromPnls(pnls []float64) float64 {
	if len(pnls) < 2 {
		return 0
	}

	var sum float64
	for _, pnl := range pnls {
		sum += pnl
	}
	mean := sum / float64(len(pnls))

	var variance float64
	for _, pnl := range pnls {
		variance += (pnl - mean) * (pnl - mean)
	}
	stdDev := math.Sqrt(variance / float64(len(pnls)-1))

	if stdDev == 0 {
		return 0
	}

	return mean / stdDev
}

// calculateMaxDrawdownFromPnls reconstructs an equity curve from the realized
// PnL sequence on top of startingEquity and returns the max peak-to-trough
// drawdown as a PERCENT (e.g. 18.5 = -18.5%). The baseline matters: the same
// $87 dip is 0.9% of a $10k account but 18% of a $480 one, so callers should
// pass the trader's real initial balance. A non-positive baseline falls back
// to a neutral $10k so the metric stays defined (but understated) when the
// account baseline is unknown.
func calculateMaxDrawdownFromPnls(pnls []float64, startingEquity float64) float64 {
	if len(pnls) == 0 {
		return 0
	}

	if startingEquity <= 0 {
		startingEquity = 10000.0
	}
	equity := startingEquity
	peak := startingEquity
	var maxDD float64

	for _, pnl := range pnls {
		equity += pnl
		if equity > peak {
			peak = equity
		}
		if peak > 0 {
			dd := (peak - equity) / peak * 100
			if dd > maxDD {
				maxDD = dd
			}
		}
	}

	return maxDD
}

// SymbolStats per-symbol trading statistics
type SymbolStats struct {
	Symbol      string  `json:"symbol"`
	TotalTrades int     `json:"total_trades"`
	WinTrades   int     `json:"win_trades"`
	WinRate     float64 `json:"win_rate"`
	TotalPnL    float64 `json:"total_pnl"`
	AvgPnL      float64 `json:"avg_pnl"`
	AvgHoldMins float64 `json:"avg_hold_mins"`
}

// GetSymbolStats gets per-symbol trading statistics
func (s *PositionStore) GetSymbolStats(traderID string, limit int) ([]SymbolStats, error) {
	var positions []TraderPosition
	err := s.closedPositions(traderID).Find(&positions).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query symbol stats: %w", err)
	}

	// Group by symbol
	symbolMap := make(map[string]*SymbolStats)
	symbolHoldMins := make(map[string][]float64)

	for _, pos := range positions {
		if _, ok := symbolMap[pos.Symbol]; !ok {
			symbolMap[pos.Symbol] = &SymbolStats{Symbol: pos.Symbol}
			symbolHoldMins[pos.Symbol] = []float64{}
		}
		s := symbolMap[pos.Symbol]
		s.TotalTrades++
		s.TotalPnL += pos.RealizedPnL
		if pos.RealizedPnL > 0 {
			s.WinTrades++
		}

		if pos.ExitTime > 0 {
			holdMins := float64(pos.ExitTime-pos.EntryTime) / 60000.0 // ms to minutes
			symbolHoldMins[pos.Symbol] = append(symbolHoldMins[pos.Symbol], holdMins)
		}
	}

	var stats []SymbolStats
	for symbol, s := range symbolMap {
		if s.TotalTrades > 0 {
			s.WinRate = float64(s.WinTrades) / float64(s.TotalTrades) * 100
			s.AvgPnL = s.TotalPnL / float64(s.TotalTrades)
		}
		if len(symbolHoldMins[symbol]) > 0 {
			var totalMins float64
			for _, m := range symbolHoldMins[symbol] {
				totalMins += m
			}
			s.AvgHoldMins = totalMins / float64(len(symbolHoldMins[symbol]))
		}
		stats = append(stats, *s)
	}

	// Sort by TotalPnL descending and limit
	for i := 0; i < len(stats)-1; i++ {
		for j := i + 1; j < len(stats); j++ {
			if stats[j].TotalPnL > stats[i].TotalPnL {
				stats[i], stats[j] = stats[j], stats[i]
			}
		}
	}

	if limit > 0 && len(stats) > limit {
		stats = stats[:limit]
	}

	return stats, nil
}

// DirectionStats long/short performance comparison
type DirectionStats struct {
	Side       string  `json:"side"`
	TradeCount int     `json:"trade_count"`
	WinRate    float64 `json:"win_rate"`
	TotalPnL   float64 `json:"total_pnl"`
	AvgPnL     float64 `json:"avg_pnl"`
}

// GetDirectionStats analyzes long vs short performance
func (s *PositionStore) GetDirectionStats(traderID string) ([]DirectionStats, error) {
	var positions []TraderPosition
	err := s.closedPositions(traderID).Find(&positions).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query direction stats: %w", err)
	}

	sideStats := make(map[string]*DirectionStats)
	for _, pos := range positions {
		if _, ok := sideStats[pos.Side]; !ok {
			sideStats[pos.Side] = &DirectionStats{Side: pos.Side}
		}
		s := sideStats[pos.Side]
		s.TradeCount++
		s.TotalPnL += pos.RealizedPnL
		if pos.RealizedPnL > 0 {
			s.WinRate++
		}
	}

	var stats []DirectionStats
	for _, s := range sideStats {
		if s.TradeCount > 0 {
			s.AvgPnL = s.TotalPnL / float64(s.TradeCount)
			s.WinRate = s.WinRate / float64(s.TradeCount) * 100
		}
		stats = append(stats, *s)
	}

	return stats, nil
}
