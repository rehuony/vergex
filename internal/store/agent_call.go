package store

import (
	"time"

	"gorm.io/gorm"
)

// The features a call or conversation can belong to.
const (
	FeatureTrader   = "trader"
	FeatureFrontier = "frontier"
	FeatureTelegram = "telegram"
	FeatureTest     = "test"
)

// The statuses of a call.
const (
	CallOK     = "ok"
	CallFailed = "failed"
)

// AgentCall records one request to a model, failed ones included. Costs are
// estimated from models.dev prices; calls whose model could not be priced keep
// their token counts with Unpriced=true and CostUSD=0.
type AgentCall struct {
	ID                 int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Feature            string    `gorm:"column:feature" json:"feature"`
	SourceID           string    `gorm:"column:source_id;serializer:nulltext" json:"source_id"`
	ConversationID     string    `gorm:"column:conversation_id;serializer:nulltext" json:"conversation_id"`
	Model              string    `gorm:"column:model" json:"model"`
	Provider           string    `gorm:"column:provider" json:"provider"`
	Host               string    `gorm:"column:host" json:"host"`
	PromptTokens       int       `gorm:"column:prompt_tokens" json:"prompt_tokens"`
	CachedPromptTokens int       `gorm:"column:cached_prompt_tokens" json:"cached_prompt_tokens"`
	CompletionTokens   int       `gorm:"column:completion_tokens" json:"completion_tokens"`
	CostUSD            float64   `gorm:"column:cost_usd" json:"cost_usd"`
	Unpriced           bool      `gorm:"column:unpriced" json:"unpriced"`
	DurationMs         int       `gorm:"column:duration_ms" json:"duration_ms"`
	Status             string    `gorm:"column:status;default:ok" json:"status"`
	Error              string    `gorm:"column:error;serializer:nulltext" json:"error"`
	FinishReason       string    `gorm:"column:finish_reason;serializer:nulltext" json:"finish_reason"`
	CreatedAt          time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (AgentCall) TableName() string { return "agent.calls" }

// AgentCallStore handles the records of calls to models.
type AgentCallStore struct {
	db *gorm.DB
}

// NewAgentCallStore creates a new AgentCallStore
func NewAgentCallStore(db *gorm.DB) *AgentCallStore {
	return &AgentCallStore{db: db}
}

// Record stores one call.
func (s *AgentCallStore) Record(call *AgentCall) error {
	if call.Unpriced {
		call.CostUSD = 0
	}
	return s.db.Create(call).Error
}

// After returns the calls past an ID and no older than since, oldest first.
func (s *AgentCallStore) After(id int64, since time.Time, limit int) ([]AgentCall, error) {
	var calls []AgentCall
	err := s.db.Where("id > ? AND created_at >= ?", id, since).Order("id ASC").Limit(limit).Find(&calls).Error
	return calls, err
}

// ForTrader returns a trader's calls within a period, plus their total cost.
func (s *AgentCallStore) ForTrader(traderID string, period string) ([]AgentCall, float64, error) {
	var calls []AgentCall
	query := s.db.Where("feature = ? AND source_id = ?", FeatureTrader, traderID)
	query = applyPeriodFilter(query, period)
	if err := query.Order("created_at DESC").Find(&calls).Error; err != nil {
		return nil, 0, err
	}

	var total float64
	for _, call := range calls {
		total += call.CostUSD
	}
	return calls, total, nil
}

// AgentCallSummary aggregates agent usage for a period.
type AgentCallSummary struct {
	TotalUSD         float64                      `json:"total"`
	Count            int64                        `json:"count"`
	UnpricedCount    int64                        `json:"unpriced_count"`
	PromptTokens     int64                        `json:"prompt_tokens"`
	CompletionTokens int64                        `json:"completion_tokens"`
	ByModel          map[string]AgentCallModelSum `json:"by_model"`
}

// AgentCallModelSum aggregates agent usage for one model.
type AgentCallModelSum struct {
	TotalUSD         float64 `json:"total"`
	Count            int64   `json:"count"`
	UnpricedCount    int64   `json:"unpriced_count"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
}

// GetSummary returns summary stats for a period, across all traders or for
// one when traderID is set.
func (s *AgentCallStore) GetSummary(traderID, period string) (AgentCallSummary, error) {
	summary := AgentCallSummary{ByModel: make(map[string]AgentCallModelSum)}

	type modelRow struct {
		Model            string  `gorm:"column:model"`
		Total            float64 `gorm:"column:total"`
		Count            int64   `gorm:"column:cnt"`
		Unpriced         int64   `gorm:"column:unpriced"`
		PromptTokens     int64   `gorm:"column:prompt_tokens"`
		CompletionTokens int64   `gorm:"column:completion_tokens"`
	}
	var rows []modelRow
	query := s.db.Model(&AgentCall{}).Select(`model,
		COALESCE(SUM(cost_usd), 0) AS total,
		COUNT(*) AS cnt,
		COALESCE(SUM(CASE WHEN unpriced THEN 1 ELSE 0 END), 0) AS unpriced,
		COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens,
		COALESCE(SUM(completion_tokens), 0) AS completion_tokens`).Group("model")
	if traderID != "" {
		query = query.Where("feature = ? AND source_id = ?", FeatureTrader, traderID)
	}
	query = applyPeriodFilter(query, period)
	if err := query.Find(&rows).Error; err != nil {
		return summary, err
	}
	for _, r := range rows {
		summary.ByModel[r.Model] = AgentCallModelSum{
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
