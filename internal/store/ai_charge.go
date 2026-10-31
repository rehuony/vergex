package store

import (
	"time"

	"gorm.io/gorm"
)

// AICharge records the token usage and estimated cost of a single AI call.
// Costs are estimated from models.dev prices; calls whose model could not be
// priced keep their token counts with Unpriced=true and CostUSD=0.
type AICharge struct {
	ID               int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	TraderID         string    `gorm:"column:trader_id;not null;index:idx_ai_charges_trader" json:"trader_id"`
	Model            string    `gorm:"column:model;not null" json:"model"`
	Provider         string    `gorm:"column:provider;not null" json:"provider"`
	Host             string    `gorm:"column:host;not null;default:''" json:"host"`
	PromptTokens     int       `gorm:"column:prompt_tokens;not null;default:0" json:"prompt_tokens"`
	CompletionTokens int       `gorm:"column:completion_tokens;not null;default:0" json:"completion_tokens"`
	CostUSD          float64   `gorm:"column:cost_usd;not null" json:"cost_usd"`
	Unpriced         bool      `gorm:"column:unpriced;not null;default:false" json:"unpriced"`
	CreatedAt        time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (AICharge) TableName() string { return "ai_charges" }

// AIChargeStore handles AI charge records
type AIChargeStore struct {
	db *gorm.DB
}

// NewAIChargeStore creates a new AIChargeStore
func NewAIChargeStore(db *gorm.DB) *AIChargeStore {
	return &AIChargeStore{db: db}
}

func (s *AIChargeStore) initTables() error {
	return s.db.AutoMigrate(&AICharge{})
}

// Record stores one AI call's usage record.
func (s *AIChargeStore) Record(charge *AICharge) error {
	if charge.Unpriced {
		charge.CostUSD = 0
	}
	return s.db.Create(charge).Error
}

// After returns the charges past an ID and no older than since, oldest first.
func (s *AIChargeStore) After(id int64, since time.Time, limit int) ([]AICharge, error) {
	var charges []AICharge
	err := s.db.Where("id > ? AND created_at >= ?", id, since).Order("id ASC").Limit(limit).Find(&charges).Error
	return charges, err
}

// GetCharges returns charges for a trader within a period, plus total cost
func (s *AIChargeStore) GetCharges(traderID string, period string) ([]AICharge, float64, error) {
	var charges []AICharge
	query := s.db.Where("trader_id = ?", traderID)
	query = applyPeriodFilter(query, period)
	if err := query.Order("created_at DESC").Find(&charges).Error; err != nil {
		return nil, 0, err
	}

	var total float64
	for _, c := range charges {
		total += c.CostUSD
	}
	return charges, total, nil
}

// GetDailyCost returns total cost across all traders for a period
func (s *AIChargeStore) GetDailyCost(period string) float64 {
	var total float64
	query := s.db.Model(&AICharge{}).Select("COALESCE(SUM(cost_usd), 0)")
	query = applyPeriodFilter(query, period)
	query.Scan(&total)
	return total
}

// AIChargeSummary aggregates AI usage for a period.
type AIChargeSummary struct {
	TotalUSD         float64                     `json:"total"`
	Count            int64                       `json:"count"`
	UnpricedCount    int64                       `json:"unpriced_count"`
	PromptTokens     int64                       `json:"prompt_tokens"`
	CompletionTokens int64                       `json:"completion_tokens"`
	ByModel          map[string]AIChargeModelSum `json:"by_model"`
}

// AIChargeModelSum aggregates AI usage for one model.
type AIChargeModelSum struct {
	TotalUSD         float64 `json:"total"`
	Count            int64   `json:"count"`
	UnpricedCount    int64   `json:"unpriced_count"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
}

// GetSummary returns summary stats for a period, across all traders or for
// one when traderID is set.
func (s *AIChargeStore) GetSummary(traderID, period string) (AIChargeSummary, error) {
	summary := AIChargeSummary{ByModel: make(map[string]AIChargeModelSum)}

	type modelRow struct {
		Model            string  `gorm:"column:model"`
		Total            float64 `gorm:"column:total"`
		Count            int64   `gorm:"column:cnt"`
		Unpriced         int64   `gorm:"column:unpriced"`
		PromptTokens     int64   `gorm:"column:prompt_tokens"`
		CompletionTokens int64   `gorm:"column:completion_tokens"`
	}
	var rows []modelRow
	query := s.db.Model(&AICharge{}).Select(`model,
		COALESCE(SUM(cost_usd), 0) AS total,
		COUNT(*) AS cnt,
		COALESCE(SUM(CASE WHEN unpriced THEN 1 ELSE 0 END), 0) AS unpriced,
		COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens,
		COALESCE(SUM(completion_tokens), 0) AS completion_tokens`).Group("model")
	if traderID != "" {
		query = query.Where("trader_id = ?", traderID)
	}
	query = applyPeriodFilter(query, period)
	if err := query.Find(&rows).Error; err != nil {
		return summary, err
	}
	for _, r := range rows {
		summary.ByModel[r.Model] = AIChargeModelSum{
			TotalUSD:         r.Total,
			Count:            r.Count,
			UnpricedCount:    r.Unpriced,
			PromptTokens:     r.PromptTokens,
			CompletionTokens: r.CompletionTokens,
		}
		summary.TotalUSD += r.Total
		summary.Count += r.Count
		summary.UnpricedCount += r.Unpriced
		summary.PromptTokens += r.PromptTokens
		summary.CompletionTokens += r.CompletionTokens
	}
	return summary, nil
}

func applyPeriodFilter(query *gorm.DB, period string) *gorm.DB {
	now := time.Now()
	switch period {
	case "today":
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		return query.Where("created_at >= ?", start)
	case "week":
		return query.Where("created_at >= ?", now.AddDate(0, 0, -7))
	case "month":
		return query.Where("created_at >= ?", now.AddDate(0, -1, 0))
	case "all":
		return query
	default:
		// Try parse as date
		if t, err := time.Parse("2006-01-02", period); err == nil {
			end := t.AddDate(0, 0, 1)
			return query.Where("created_at >= ? AND created_at < ?", t, end)
		}
		// Default to today
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		return query.Where("created_at >= ?", start)
	}
}
