package planning

import (
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
	"vergex/internal/planning/core"
	"vergex/internal/store"
)

// Where a plan came from.
const (
	OriginAssistant = "assistant"
	OriginManual    = "manual"
)

type Draft struct {
	UserID         string    `gorm:"column:user_id;primaryKey" json:"-"`
	ID             string    `gorm:"column:id;primaryKey" json:"id"`
	ExchangeID     string    `gorm:"column:account_id;serializer:nulltext" json:"exchange_id"`
	Venue          string    `gorm:"column:venue" json:"venue"`
	Symbol         string    `gorm:"column:symbol" json:"symbol"`
	Status         string    `gorm:"column:status" json:"status"`
	Origin         string    `gorm:"column:origin;default:manual" json:"-"`
	ConversationID string    `gorm:"column:conversation_id;serializer:nulltext" json:"-"`
	RevisesPlanID  string    `gorm:"column:revises_plan_id;serializer:nulltext" json:"-"`
	Body           string    `gorm:"column:body" json:"-"`
	CreatedAt      time.Time `gorm:"column:created_at" json:"-"`
	UpdatedAt      time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (Draft) TableName() string { return "frontier.plans" }

type DraftStore struct {
	DB                        *gorm.DB
	UserID, ExchangeID, Venue string
	// Origin and ConversationID say where the plans added here come from.
	Origin, ConversationID string
}

// NewDraftStore returns a user's plans, with new ones going to an account and venue.
func NewDraftStore(st *store.Store, user, exchangeID, venue string) *DraftStore {
	return &DraftStore{DB: st.GormDB(), UserID: user, ExchangeID: exchangeID, Venue: venue}
}

func (d *DraftStore) Records() map[string]core.Record {
	out := map[string]core.Record{}
	var rows []Draft
	if d.DB.Where("user_id = ?", d.UserID).Order("updated_at desc").Limit(200).Find(&rows).Error != nil {
		return out
	}
	for _, row := range rows {
		var p core.Plan
		if json.Unmarshal([]byte(row.Body), &p) == nil {
			out[p.ID] = core.Record{Plan: p, State: row.Status, Updated: row.UpdatedAt}
		}
	}
	return out
}

// Accounts says which account each of the user's plans was drafted on.
func (d *DraftStore) Accounts() map[string]string {
	var rows []Draft
	d.DB.Select("id", "account_id").Where("user_id = ?", d.UserID).Find(&rows)
	accounts := make(map[string]string, len(rows))
	for _, row := range rows {
		accounts[row.ID] = row.ExchangeID
	}
	return accounts
}

// Find returns one of the user's plans.
func (d *DraftStore) Find(id string) (*Draft, error) {
	var row Draft
	if e := d.DB.Where("user_id = ? AND id = ?", d.UserID, id).First(&row).Error; e != nil {
		return nil, e
	}
	return &row, nil
}

// HasDraft reports whether a plan is still an unapproved draft on this store's account.
func (d *DraftStore) HasDraft(id string) bool {
	row, e := d.Find(id)
	return e == nil && row.ExchangeID == d.ExchangeID && row.Status == "draft"
}

func (d *DraftStore) Add(plans []core.Plan) error {
	return d.DB.Transaction(func(tx *gorm.DB) error { return d.add(tx, plans, "") })
}
func (d *DraftStore) add(tx *gorm.DB, plans []core.Plan, revises string) error {
	for _, p := range plans {
		if e := p.Verify(time.Now().UTC()); e != nil {
			return e
		}
		if p.Snapshot.Venue != d.Venue || p.Snapshot.Kind != "perpetual" {
			return errors.New("plan must match the connected perpetual account")
		}
		raw, e := json.Marshal(p)
		if e != nil {
			return e
		}
		now := time.Now().UTC()
		r := Draft{UserID: d.UserID, ID: p.ID, ExchangeID: d.ExchangeID, Venue: d.Venue, Symbol: p.Snapshot.Symbol, Status: "draft", Origin: d.Origin, ConversationID: d.ConversationID, RevisesPlanID: revises, Body: string(raw), CreatedAt: now, UpdatedAt: now}
		if e := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&r).Error; e != nil {
			return e
		}
	}
	return nil
}
func (d *DraftStore) Revise(old string, plans []core.Plan, now time.Time) error {
	return d.DB.Transaction(func(tx *gorm.DB) error {
		var row Draft
		if e := tx.Where("user_id = ? AND id = ?", d.UserID, old).First(&row).Error; e != nil {
			return e
		}
		if row.Status != "draft" {
			return errors.New("locked live plan cannot be revised; cancel approval first")
		}
		if e := d.add(tx, plans, old); e != nil {
			return e
		}
		res := tx.Model(&Draft{}).Where("user_id = ? AND id = ? AND status = ?", d.UserID, old, "draft").Updates(map[string]any{"status": "cancelled", "updated_at": now})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errors.New("plan changed concurrently")
		}
		return nil
	})
}
