package agent

import (
	"time"
	"vergex/internal/database"

	"gorm.io/gorm"
)

// The statuses of a call.
const (
	CallOK     = "ok"
	CallFailed = "failed"
)

// AgentCall records one request to a model, failed ones included. Costs are
// estimated from models.dev prices; calls whose model could not be priced keep
// their token counts with Unpriced=true and CostUSD=0.
type Call struct {
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

func (Call) TableName() string { return "agent.calls" }

// AgentCallStore handles the records of calls to models.
type CallStore struct {
	db *gorm.DB
}

// NewAgentCallStore creates a new AgentCallStore
func NewCallStore(db *gorm.DB) *CallStore {
	return &CallStore{db: db}
}

// Record stores one call.
func (s *CallStore) Record(call *Call) error {
	if call.Unpriced {
		call.CostUSD = 0
	}
	return s.db.Create(call).Error
}

// CursorAt returns where to read on from with After to see the calls from a time on.
func (s *CallStore) CursorAt(at time.Time) (int64, error) {
	return database.CursorAt(s.db, "agent.calls", "created_at", at)
}

// After returns the calls past an ID and no older than since, oldest first.
func (s *CallStore) After(id int64, since time.Time, limit int) ([]Call, error) {
	var calls []Call
	err := s.db.Where("id > ? AND created_at >= ?", id, since).Order("id ASC").Limit(limit).Find(&calls).Error
	return calls, err
}

// ForTrader returns a trader's calls within a period, plus their total cost.
func (s *CallStore) ForTrader(traderID string, period string) ([]Call, float64, error) {
	var calls []Call
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

// CallTotals adds up what a piece of work cost. Failed calls are counted, and hold no tokens
// because the provider's answer never came: a stopped call may still have been charged.
type CallTotals struct {
	Model            string  `json:"model"`
	Calls            int     `json:"calls"`
	Failed           int     `json:"failed"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	CostUSD          float64 `json:"cost_usd"`
	Unpriced         bool    `json:"unpriced"`
	DurationMs       int     `json:"duration_ms"`
	// FinishReason is why the last call's reply ended.
	FinishReason string `json:"finish_reason"`
}

// ForConversation adds up the calls of a conversation made since a moment. The zero time
// means all of them.
func (s *CallStore) ForConversation(conversationID string, since time.Time) (CallTotals, error) {
	var totals CallTotals
	if conversationID == "" {
		return totals, nil
	}
	query := s.db.Where("conversation_id = ?", conversationID)
	if !since.IsZero() {
		query = query.Where("created_at >= ?", since)
	}
	var calls []Call
	if err := query.Order("id ASC").Find(&calls).Error; err != nil {
		return totals, err
	}
	for _, call := range calls {
		totals.Calls++
		if call.Status == CallFailed {
			totals.Failed++
		}
		totals.Model = call.Model
		totals.PromptTokens += call.PromptTokens
		totals.CompletionTokens += call.CompletionTokens
		totals.CostUSD += call.CostUSD
		totals.Unpriced = totals.Unpriced || call.Unpriced
		totals.DurationMs += call.DurationMs
		totals.FinishReason = call.FinishReason
	}
	return totals, nil
}

// AgentCallSummary aggregates agent usage for a period.
type AgentCallSummary struct {
	TotalUSD         float64                      `json:"total"`
	Count            int64                        `json:"count"`
	FailedCount      int64                        `json:"failed_count"`
	UnpricedCount    int64                        `json:"unpriced_count"`
	PromptTokens     int64                        `json:"prompt_tokens"`
	CompletionTokens int64                        `json:"completion_tokens"`
	ByModel          map[string]AgentCallModelSum `json:"by_model"`
	// ByFeature says what made the calls: traders, Futures, Telegram, the assistant or a test.
	ByFeature map[string]AgentCallModelSum `json:"by_feature"`
	// RecentErrors are the last calls that failed, newest first.
	RecentErrors []CallError `json:"recent_errors"`
}

// AgentCallModelSum aggregates agent usage for one model or feature.
type AgentCallModelSum struct {
	TotalUSD         float64 `json:"total"`
	Count            int64   `json:"count"`
	FailedCount      int64   `json:"failed_count"`
	UnpricedCount    int64   `json:"unpriced_count"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
}

func (a AgentCallModelSum) add(b AgentCallModelSum) AgentCallModelSum {
	return AgentCallModelSum{
		TotalUSD:         a.TotalUSD + b.TotalUSD,
		Count:            a.Count + b.Count,
		FailedCount:      a.FailedCount + b.FailedCount,
		UnpricedCount:    a.UnpricedCount + b.UnpricedCount,
		PromptTokens:     a.PromptTokens + b.PromptTokens,
		CompletionTokens: a.CompletionTokens + b.CompletionTokens,
	}
}

// CallError is a call that failed, as it can be shown.
type CallError struct {
	ID        int64     `json:"id"`
	Feature   string    `json:"feature"`
	Model     string    `json:"model"`
	Error     string    `json:"error"`
	CreatedAt time.Time `json:"created_at"`
}

// recentCallErrors is how many failed calls a summary lists.
const recentCallErrors = 5

// GetSummary returns summary stats for a period, across everything or for one trader when
// traderID is set.
func (s *CallStore) GetSummary(traderID, period string) (AgentCallSummary, error) {
	summary := AgentCallSummary{
		ByModel: make(map[string]AgentCallModelSum), ByFeature: make(map[string]AgentCallModelSum), RecentErrors: []CallError{},
	}

	type modelRow struct {
		Model            string  `gorm:"column:model"`
		Feature          string  `gorm:"column:feature"`
		Total            float64 `gorm:"column:total"`
		Count            int64   `gorm:"column:cnt"`
		Failed           int64   `gorm:"column:failed"`
		Unpriced         int64   `gorm:"column:unpriced"`
		PromptTokens     int64   `gorm:"column:prompt_tokens"`
		CompletionTokens int64   `gorm:"column:completion_tokens"`
	}
	scoped := func() *gorm.DB {
		query := s.db.Model(&Call{})
		if traderID != "" {
			query = query.Where("feature = ? AND source_id = ?", FeatureTrader, traderID)
		}
		return applyPeriodFilter(query, period)
	}
	var rows []modelRow
	query := scoped().Select(`model, feature,
		COALESCE(SUM(cost_usd), 0) AS total,
		COUNT(*) AS cnt,
		COALESCE(SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END), 0) AS failed,
		COALESCE(SUM(CASE WHEN unpriced THEN 1 ELSE 0 END), 0) AS unpriced,
		COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens,
		COALESCE(SUM(completion_tokens), 0) AS completion_tokens`).Group("model, feature")
	if err := query.Find(&rows).Error; err != nil {
		return summary, err
	}
	for _, r := range rows {
		row := AgentCallModelSum{
			TotalUSD:         r.Total,
			Count:            r.Count,
			FailedCount:      r.Failed,
			UnpricedCount:    r.Unpriced,
			PromptTokens:     r.PromptTokens,
			CompletionTokens: r.CompletionTokens,
		}
		summary.ByModel[r.Model] = summary.ByModel[r.Model].add(row)
		summary.ByFeature[r.Feature] = summary.ByFeature[r.Feature].add(row)
		summary.TotalUSD += r.Total
		summary.Count += r.Count
		summary.FailedCount += r.Failed
		summary.UnpricedCount += r.Unpriced
		summary.PromptTokens += r.PromptTokens
		summary.CompletionTokens += r.CompletionTokens
	}
	if summary.FailedCount > 0 {
		var failed []Call
		if err := scoped().Where("status = ?", CallFailed).Order("id DESC").Limit(recentCallErrors).Find(&failed).Error; err != nil {
			return summary, err
		}
		for _, call := range failed {
			summary.RecentErrors = append(summary.RecentErrors, CallError{ID: call.ID, Feature: call.Feature, Model: call.Model, Error: call.Error, CreatedAt: call.CreatedAt})
		}
	}
	return summary, nil
}

// applyPeriodFilter limits a query to a period. Days are UTC days, as in the daily report,
// so a day's cost reads the same in both.
func applyPeriodFilter(query *gorm.DB, period string) *gorm.DB {
	now := time.Now().UTC()
	switch period {
	case "today":
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
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
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		return query.Where("created_at >= ?", start)
	}
}
