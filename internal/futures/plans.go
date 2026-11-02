package futures

import (
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
	"vergex/internal/futures/plan"
)

// Where a plan came from.
const (
	OriginAssistant = "assistant"
	OriginManual    = "manual"
)

type Plan struct {
	UserID         string    `gorm:"column:user_id" json:"-"`
	ID             string    `gorm:"column:id;primaryKey" json:"id"`
	AccountID      string    `gorm:"column:account_id;serializer:nulltext" json:"account_id"`
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

func (Plan) TableName() string { return "futures.plans" }

type PlanStore struct {
	DB                       *gorm.DB
	UserID, AccountID, Venue string
	// Origin and ConversationID say where the plans added here come from.
	Origin, ConversationID string
}

// NewPlanStore returns a user's plans, with new ones going to an account and venue.
func NewPlanStore(db *gorm.DB, user, accountID, venue string) *PlanStore {
	return &PlanStore{DB: db, UserID: user, AccountID: accountID, Venue: venue}
}

func (d *PlanStore) Records() map[string]plan.Record {
	out := map[string]plan.Record{}
	var rows []Plan
	if d.DB.Where("user_id = ?", d.UserID).Order("updated_at desc").Limit(200).Find(&rows).Error != nil {
		return out
	}
	for _, row := range rows {
		var p plan.Plan
		if json.Unmarshal([]byte(row.Body), &p) == nil {
			out[p.ID] = plan.Record{Plan: p, State: row.Status, Updated: row.UpdatedAt}
		}
	}
	return out
}

// Accounts says which account each of the user's plans was drafted on.
func (d *PlanStore) Accounts() map[string]string {
	var rows []Plan
	d.DB.Select("id", "account_id").Where("user_id = ?", d.UserID).Find(&rows)
	accounts := make(map[string]string, len(rows))
	for _, row := range rows {
		accounts[row.ID] = row.AccountID
	}
	return accounts
}

// OnAccount lists the plans drafted on this store's account.
func (d *PlanStore) OnAccount() ([]string, error) {
	var ids []string
	e := d.DB.Model(&Plan{}).Where("user_id = ? AND account_id = ?", d.UserID, d.AccountID).Pluck("id", &ids).Error
	return ids, e
}

// Find returns one of the user's plans.
func (d *PlanStore) Find(id string) (*Plan, error) {
	var row Plan
	if e := d.DB.Where("user_id = ? AND id = ?", d.UserID, id).First(&row).Error; e != nil {
		return nil, e
	}
	return &row, nil
}

// HasDraft reports whether a plan is still an unapproved draft on this store's account.
func (d *PlanStore) HasDraft(id string) bool {
	row, e := d.Find(id)
	return e == nil && row.AccountID == d.AccountID && row.Status == "draft"
}

// ErrPlanRan refuses to delete a plan that went to the exchange: its run is the record of
// what was sent.
var ErrPlanRan = errors.New("a plan that ran is kept as the record of its orders")

// Delete removes a plan nothing ever came of. A plan that revised it stays, without the link.
func (d *PlanStore) Delete(id string) error {
	return d.DB.Transaction(func(tx *gorm.DB) error {
		if _, e := (&PlanStore{DB: tx, UserID: d.UserID}).Find(id); e != nil {
			return e
		}
		var runs int64
		if e := tx.Table("futures.runs").Where("plan_id = ?", id).Count(&runs).Error; e != nil {
			return e
		}
		if runs > 0 {
			return ErrPlanRan
		}
		if e := tx.Exec("UPDATE futures.plans SET revises_plan_id = NULL WHERE revises_plan_id = ?", id).Error; e != nil {
			return e
		}
		return tx.Where("user_id = ? AND id = ?", d.UserID, id).Delete(&Plan{}).Error
	})
}

func (d *PlanStore) Add(plans []plan.Plan) error {
	return d.DB.Transaction(func(tx *gorm.DB) error { return d.add(tx, plans, "") })
}
func (d *PlanStore) add(tx *gorm.DB, plans []plan.Plan, revises string) error {
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
		r := Plan{UserID: d.UserID, ID: p.ID, AccountID: d.AccountID, Venue: d.Venue, Symbol: p.Snapshot.Symbol, Status: "draft", Origin: d.Origin, ConversationID: d.ConversationID, RevisesPlanID: revises, Body: string(raw), CreatedAt: now, UpdatedAt: now}
		if e := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&r).Error; e != nil {
			return e
		}
	}
	return nil
}
func (d *PlanStore) Revise(old string, plans []plan.Plan, now time.Time) error {
	return d.DB.Transaction(func(tx *gorm.DB) error {
		var row Plan
		if e := tx.Where("user_id = ? AND id = ?", d.UserID, old).First(&row).Error; e != nil {
			return e
		}
		if row.Status != "draft" {
			return errors.New("locked live plan cannot be revised; cancel approval first")
		}
		if e := d.add(tx, plans, old); e != nil {
			return e
		}
		res := tx.Model(&Plan{}).Where("user_id = ? AND id = ? AND status = ?", d.UserID, old, "draft").Updates(map[string]any{"status": "cancelled", "updated_at": now})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errors.New("plan changed concurrently")
		}
		return nil
	})
}
