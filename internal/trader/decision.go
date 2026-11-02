package trader

import (
	"context"
	"fmt"
	"time"

	"vergex/internal/account"
	"vergex/internal/agent"
	"vergex/internal/database"

	"gorm.io/gorm"
)

// How a decision turned out.
const (
	OutcomeExecuted = "executed"
	OutcomeFailed   = "failed"
	OutcomeBlocked  = "blocked"
	OutcomeHold     = "hold"
)

// DecisionStore decision log storage
type DecisionStore struct {
	db *gorm.DB
}

// cycleRow is one run of a trader's loop.
type cycleRow struct {
	ID               int64              `gorm:"primaryKey;autoIncrement"`
	TraderID         string             `gorm:"column:trader_id"`
	CycleNumber      int                `gorm:"column:cycle_number"`
	ConversationID   string             `gorm:"column:conversation_id;serializer:nulltext"`
	CandidateSymbols database.TextArray `gorm:"column:candidate_symbols"`
	ExecutionLog     database.TextArray `gorm:"column:execution_log"`
	AccountState     *AccountSnapshot   `gorm:"column:account_state;serializer:json"`
	Limits           any                `gorm:"column:limits;serializer:json"`
	Success          bool               `gorm:"column:success"`
	Error            string             `gorm:"column:error;serializer:nulltext"`
	AgentDurationMs  int64              `gorm:"column:agent_duration_ms"`
	FinishedAt       time.Time          `gorm:"column:finished_at"`
}

func (cycleRow) TableName() string { return "trader.cycles" }

// decisionRow is one thing the model decided in a cycle.
type decisionRow struct {
	ID         int64     `gorm:"primaryKey;autoIncrement"`
	CycleID    int64     `gorm:"column:cycle_id"`
	Seq        int       `gorm:"column:seq"`
	Action     string    `gorm:"column:action"`
	Symbol     string    `gorm:"column:symbol"`
	Quantity   float64   `gorm:"column:quantity"`
	Leverage   int       `gorm:"column:leverage"`
	Price      float64   `gorm:"column:price"`
	StopLoss   float64   `gorm:"column:stop_loss"`
	TakeProfit float64   `gorm:"column:take_profit"`
	Confidence int       `gorm:"column:confidence"`
	Reasoning  string    `gorm:"column:reasoning"`
	Outcome    string    `gorm:"column:outcome"`
	Error      string    `gorm:"column:error;serializer:nulltext"`
	OrderID    *int64    `gorm:"column:order_id"`
	DecidedAt  time.Time `gorm:"column:decided_at"`
}

func (decisionRow) TableName() string { return "trader.decisions" }

// DecisionRecord decision record (external API struct). ConversationID is where the
// prompts, reply and thinking are kept; a full record reads them from there.
type DecisionRecord struct {
	ID              int64            `json:"id"`
	TraderID        string           `json:"trader_id"`
	CycleNumber     int              `json:"cycle_number"`
	Timestamp       time.Time        `json:"timestamp"`
	ConversationID  string           `json:"-"`
	SystemPrompt    string           `json:"system_prompt"`
	InputPrompt     string           `json:"input_prompt"`
	CoTTrace        string           `json:"cot_trace"`
	RawResponse     string           `json:"raw_response"` // Raw model response for debugging
	CandidateCoins  []string         `json:"candidate_coins"`
	ExecutionLog    []string         `json:"execution_log"`
	Success         bool             `json:"success"`
	ErrorMessage    string           `json:"error_message"`
	AgentDurationMs int64            `json:"agent_duration_ms"`
	AccountState    *AccountSnapshot `json:"account_state"`
	Limits          any              `json:"-"` // The limits this cycle ran under, kept for reading it later
	Decisions       []DecisionAction `json:"decisions"`
}

// AccountSnapshot account state snapshot
type AccountSnapshot struct {
	TotalBalance          float64 `json:"total_balance"`
	AvailableBalance      float64 `json:"available_balance"`
	TotalUnrealizedProfit float64 `json:"total_unrealized_profit"`
	PositionCount         int     `json:"position_count"`
	MarginUsedPct         float64 `json:"margin_used_pct"`
	InitialBalance        float64 `json:"initial_balance"`
}

// DecisionAction decision action
type DecisionAction struct {
	Action     string    `json:"action"`
	Symbol     string    `json:"symbol"`
	Quantity   float64   `json:"quantity"`
	Leverage   int       `json:"leverage"`
	Price      float64   `json:"price"`
	StopLoss   float64   `json:"stop_loss,omitempty"`   // Stop loss price
	TakeProfit float64   `json:"take_profit,omitempty"` // Take profit price
	Confidence int       `json:"confidence,omitempty"`  // Agent confidence (0-100)
	Reasoning  string    `json:"reasoning,omitempty"`   // Brief reasoning
	OrderID    int64     `json:"order_id"`              // The order in the ledger, 0 without one
	Timestamp  time.Time `json:"timestamp"`
	Success    bool      `json:"success"`
	Error      string    `json:"error"`
	Blocked    bool      `json:"blocked,omitempty"` // A risk guard stopped it; nothing reached the exchange.
}

// outcome is how the action turned out, for the table.
func (a DecisionAction) outcome() string {
	switch {
	case a.Blocked:
		return OutcomeBlocked
	case a.Action == "hold" || a.Action == "wait":
		return OutcomeHold
	case a.Success:
		return OutcomeExecuted
	}
	return OutcomeFailed
}

// Statistics statistics information
type Statistics struct {
	TotalCycles         int `json:"total_cycles"`
	SuccessfulCycles    int `json:"successful_cycles"`
	FailedCycles        int `json:"failed_cycles"`
	TotalOpenPositions  int `json:"total_open_positions"`
	TotalClosePositions int `json:"total_close_positions"`
}

// NewDecisionStore creates a new DecisionStore
func NewDecisionStore(db *gorm.DB) *DecisionStore {
	return &DecisionStore{db: db}
}

// WithContext returns the store with its queries bound to ctx, for a caller that must
// not wait on the database for long.
func (s *DecisionStore) WithContext(ctx context.Context) *DecisionStore {
	return &DecisionStore{db: s.db.WithContext(ctx)}
}

// LogDecision saves a cycle and what it decided.
func (s *DecisionStore) LogDecision(record *DecisionRecord) error {
	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now().UTC()
	} else {
		record.Timestamp = record.Timestamp.UTC()
	}
	cycle := &cycleRow{
		TraderID:         record.TraderID,
		CycleNumber:      record.CycleNumber,
		ConversationID:   record.ConversationID,
		CandidateSymbols: record.CandidateCoins,
		ExecutionLog:     storableLines(record.ExecutionLog),
		AccountState:     record.AccountState,
		Limits:           record.Limits,
		Success:          record.Success,
		Error:            database.StorableText(record.ErrorMessage),
		AgentDurationMs:  record.AgentDurationMs,
		FinishedAt:       record.Timestamp,
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(cycle).Error; err != nil {
			return err
		}
		if len(record.Decisions) == 0 {
			return nil
		}
		rows := make([]decisionRow, len(record.Decisions))
		for i, action := range record.Decisions {
			rows[i] = decisionRow{
				CycleID:    cycle.ID,
				Seq:        i + 1,
				Action:     action.Action,
				Symbol:     action.Symbol,
				Quantity:   action.Quantity,
				Leverage:   action.Leverage,
				Price:      action.Price,
				StopLoss:   action.StopLoss,
				TakeProfit: action.TakeProfit,
				Confidence: action.Confidence,
				Reasoning:  database.StorableText(action.Reasoning),
				Outcome:    action.outcome(),
				Error:      database.StorableText(action.Error),
				DecidedAt:  action.Timestamp,
			}
			if action.OrderID != 0 {
				orderID := action.OrderID
				rows[i].OrderID = &orderID
			}
			if rows[i].DecidedAt.IsZero() {
				rows[i].DecidedAt = record.Timestamp
			}
		}
		return tx.Create(&rows).Error
	})
	if err != nil {
		return fmt.Errorf("failed to insert decision record: %w", err)
	}
	record.ID = cycle.ID
	return nil
}

func storableLines(lines []string) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = database.StorableText(line)
	}
	return out
}

// afterMillis is the first time after one in Unix milliseconds. Callers pass the time
// of the last row they read, cut to milliseconds, and a stored time is finer, so a plain
// comparison would hand that row back.
func afterMillis(sinceMs int64) time.Time {
	return time.UnixMilli(sinceMs + 1).UTC()
}

// records assembles API records from cycles, in the order given, with what they decided.
func (s *DecisionStore) records(cycles []*cycleRow) ([]*DecisionRecord, error) {
	records := make([]*DecisionRecord, len(cycles))
	byCycle := make(map[int64]*DecisionRecord, len(cycles))
	ids := make([]int64, len(cycles))
	for i, cycle := range cycles {
		records[i] = &DecisionRecord{
			ID:              cycle.ID,
			TraderID:        cycle.TraderID,
			CycleNumber:     cycle.CycleNumber,
			Timestamp:       cycle.FinishedAt,
			ConversationID:  cycle.ConversationID,
			CandidateCoins:  cycle.CandidateSymbols,
			ExecutionLog:    cycle.ExecutionLog,
			AccountState:    cycle.AccountState,
			Success:         cycle.Success,
			ErrorMessage:    cycle.Error,
			AgentDurationMs: cycle.AgentDurationMs,
		}
		byCycle[cycle.ID] = records[i]
		ids[i] = cycle.ID
	}
	if len(ids) == 0 {
		return records, nil
	}
	var rows []decisionRow
	if err := s.db.Where("cycle_id IN ?", ids).Order("cycle_id, seq").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to query decisions: %w", err)
	}
	for _, row := range rows {
		action := DecisionAction{
			Action:     row.Action,
			Symbol:     row.Symbol,
			Quantity:   row.Quantity,
			Leverage:   row.Leverage,
			Price:      row.Price,
			StopLoss:   row.StopLoss,
			TakeProfit: row.TakeProfit,
			Confidence: row.Confidence,
			Reasoning:  row.Reasoning,
			Timestamp:  row.DecidedAt,
			Success:    row.Outcome == OutcomeExecuted || row.Outcome == OutcomeHold,
			Error:      row.Error,
			Blocked:    row.Outcome == OutcomeBlocked,
		}
		if row.OrderID != nil {
			action.OrderID = *row.OrderID
		}
		record := byCycle[row.CycleID]
		record.Decisions = append(record.Decisions, action)
	}
	return records, nil
}

// withConversations adds what was sent to the model and what it said.
func (s *DecisionStore) withConversations(records []*DecisionRecord) error {
	byConversation := make(map[string]*DecisionRecord, len(records))
	ids := make([]string, 0, len(records))
	for _, record := range records {
		if record.ConversationID != "" {
			byConversation[record.ConversationID] = record
			ids = append(ids, record.ConversationID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var messages []agent.ConversationMessage
	if err := s.db.Where("conversation_id IN ?", ids).Order("conversation_id, seq").Find(&messages).Error; err != nil {
		return fmt.Errorf("failed to query conversations: %w", err)
	}
	for _, message := range messages {
		record := byConversation[message.ConversationID]
		switch message.Role {
		case agent.RoleSystem:
			record.SystemPrompt = message.Content
		case agent.RoleUser:
			record.InputPrompt = message.Content
		case agent.RoleAssistant:
			record.RawResponse = message.Content
			record.CoTTrace = message.Thinking
		}
	}
	return nil
}

// FailedAction is a trade action the exchange or the trader refused.
type FailedAction struct {
	TraderID  string    `gorm:"column:trader_id"`
	Action    string    `gorm:"column:action"`
	Symbol    string    `gorm:"column:symbol"`
	Error     string    `gorm:"column:error"`
	Timestamp time.Time `gorm:"column:finished_at"`
}

// FailedActionsSince returns actions that failed after sinceMs, oldest first. Actions a
// risk guard blocked are left out: they are the guard working.
func (s *DecisionStore) FailedActionsSince(traderIDs []string, sinceMs int64, limit int) ([]FailedAction, error) {
	if len(traderIDs) == 0 {
		return nil, nil
	}
	var out []FailedAction
	err := s.db.Raw(`SELECT c.trader_id, d.action, d.symbol, COALESCE(d.error, '') AS error, c.finished_at
		FROM trader.decisions d
		JOIN trader.cycles c ON c.id = d.cycle_id
		WHERE c.trader_id IN ? AND c.finished_at >= ? AND d.outcome = ?
		ORDER BY c.finished_at ASC, d.id ASC
		LIMIT ?`, traderIDs, afterMillis(sinceMs), OutcomeFailed, limit).Scan(&out).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query failed actions: %w", err)
	}
	return out, nil
}

// CyclesAfter returns the records past an ID and no older than since, oldest first,
// without their prompts and model output.
func (s *DecisionStore) CyclesAfter(id int64, since time.Time, limit int) ([]*DecisionRecord, error) {
	var cycles []*cycleRow
	err := s.db.Where("id > ? AND finished_at >= ?", id, since).
		Order("id ASC").
		Limit(limit).
		Find(&cycles).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query decision cycles: %w", err)
	}
	return s.records(cycles)
}

// GetLatestRecords gets the latest N records for specified trader (sorted by time in ascending order: old to new)
func (s *DecisionStore) GetLatestRecords(traderID string, n int) ([]*DecisionRecord, error) {
	var cycles []*cycleRow
	err := s.db.Where("trader_id = ?", traderID).
		Order("id DESC").
		Limit(n).
		Find(&cycles).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query decision records: %w", err)
	}
	records, err := s.records(cycles)
	if err != nil {
		return nil, err
	}
	if err := s.withConversations(records); err != nil {
		return nil, err
	}

	// Reverse array to sort time from old to new
	for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
		records[i], records[j] = records[j], records[i]
	}

	return records, nil
}

// GetSummaries returns records newest first without the stored prompts and raw
// responses, which can be tens of kilobytes each. beforeID pages to older ones.
func (s *DecisionStore) GetSummaries(traderID string, limit int, beforeID int64) ([]*DecisionRecord, error) {
	query := s.db.Where("trader_id = ?", traderID)
	if beforeID > 0 {
		query = query.Where("id < ?", beforeID)
	}
	var cycles []*cycleRow
	if err := query.Order("id DESC").Limit(limit).Find(&cycles).Error; err != nil {
		return nil, fmt.Errorf("failed to query decision summaries: %w", err)
	}
	return s.records(cycles)
}

// GetRecord returns one full record of the trader, or nil when it has none with
// that id.
func (s *DecisionStore) GetRecord(traderID string, id int64) (*DecisionRecord, error) {
	var cycles []*cycleRow
	if err := s.db.Where("trader_id = ? AND id = ?", traderID, id).Limit(1).Find(&cycles).Error; err != nil {
		return nil, fmt.Errorf("failed to query decision record: %w", err)
	}
	if len(cycles) == 0 {
		return nil, nil
	}
	records, err := s.records(cycles)
	if err != nil {
		return nil, err
	}
	if err := s.withConversations(records); err != nil {
		return nil, err
	}
	return records[0], nil
}

// GetStatistics gets statistics information for specified trader
func (s *DecisionStore) GetStatistics(traderID string) (*Statistics, error) {
	stats := &Statistics{}

	var totalCount, successCount int64
	s.db.Model(&cycleRow{}).Where("trader_id = ?", traderID).Count(&totalCount)
	s.db.Model(&cycleRow{}).Where("trader_id = ? AND success = ?", traderID, true).Count(&successCount)

	stats.TotalCycles = int(totalCount)
	stats.SuccessfulCycles = int(successCount)
	stats.FailedCycles = stats.TotalCycles - stats.SuccessfulCycles

	// Count from the ledger using raw query for cross-table
	s.db.Raw("SELECT COUNT(*) FROM account.positions WHERE source = ? AND source_id = ?", account.SourceTrader, traderID).Scan(&stats.TotalOpenPositions)
	s.db.Raw("SELECT COUNT(*) FROM account.positions WHERE source = ? AND source_id = ? AND status = ?", account.SourceTrader, traderID, account.PositionClosed).Scan(&stats.TotalClosePositions)

	return stats, nil
}

// GetLastCycleNumber gets the last cycle number for specified trader
func (s *DecisionStore) GetLastCycleNumber(traderID string) (int, error) {
	var cycleNumber *int
	err := s.db.Model(&cycleRow{}).
		Where("trader_id = ?", traderID).
		Select("MAX(cycle_number)").
		Scan(&cycleNumber).Error
	if err != nil {
		return 0, err
	}
	if cycleNumber == nil {
		return 0, nil
	}
	return *cycleNumber, nil
}
