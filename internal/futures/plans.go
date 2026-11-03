package futures

import (
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
	"vergex/internal/account"
	"vergex/internal/futures/plan"
)

type Plan struct {
	UserID         string    `gorm:"column:user_id" json:"-"`
	ID             string    `gorm:"column:id;primaryKey" json:"id"`
	AccountID      string    `gorm:"column:account_id;serializer:nulltext" json:"account_id"`
	Symbol         string    `gorm:"column:symbol" json:"symbol"`
	Status         string    `gorm:"column:status" json:"status"`
	ConversationID string    `gorm:"column:conversation_id;serializer:nulltext" json:"-"`
	Body           string    `gorm:"column:body" json:"-"`
	CreatedAt      time.Time `gorm:"column:created_at" json:"-"`
	UpdatedAt      time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (Plan) TableName() string { return "futures.plans" }

type PlanStore struct {
	DB                       *gorm.DB
	UserID, AccountID, Venue string
	// ConversationID says where the plans added here are drafted, empty for one drafted by
	// hand.
	ConversationID string
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
			out[p.ID] = plan.Record{Plan: p, Status: row.Status, Updated: row.UpdatedAt}
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

// ErrPlanAtWork refuses to delete a plan the exchange may still hear from: one with a run that
// has not settled, still holds its account or still has a position open.
var ErrPlanAtWork = errors.New("a plan still at work on the exchange can't be deleted")

// SettledRuns are the states a run ends in, when none of its orders can still be live.
var SettledRuns = []string{"completed", "cancelled", "expired"}

// Delete removes a plan with its runs and their orders, once nothing of it is at work. What it
// sent stays in the account's ledger, which keeps its own record of the orders, fills and
// positions. A plan that revised it stays, without the link.
func (d *PlanStore) Delete(id string) error {
	return d.DB.Transaction(func(tx *gorm.DB) error {
		if _, e := (&PlanStore{DB: tx, UserID: d.UserID}).Find(id); e != nil {
			return e
		}
		// The lock keeps a run from moving on while this decides.
		var runs []struct {
			ID     string
			Status string
		}
		if e := tx.Raw(`SELECT id, status FROM futures.runs WHERE plan_id = ? FOR UPDATE`, id).Scan(&runs).Error; e != nil {
			return e
		}
		ids := make([]string, 0, len(runs))
		for _, run := range runs {
			if !RunSettled(run.Status) {
				return ErrPlanAtWork
			}
			ids = append(ids, run.ID)
		}
		if len(ids) > 0 {
			var holding int64
			if e := tx.Raw(`SELECT (SELECT count(*) FROM account.leases WHERE source = ? AND source_id IN ?)
				+ (SELECT count(*) FROM account.positions WHERE source = ? AND source_id IN ? AND status = ?)`,
				account.SourceFutures, ids, account.SourceFutures, ids, account.PositionOpen).Scan(&holding).Error; e != nil {
				return e
			}
			if holding > 0 {
				return ErrPlanAtWork
			}
			// Their orders go with them.
			if e := tx.Exec(`DELETE FROM futures.runs WHERE id IN ?`, ids).Error; e != nil {
				return e
			}
		}
		return tx.Where("user_id = ? AND id = ?", d.UserID, id).Delete(&Plan{}).Error
	})
}

// RunSettled says whether a run has ended, when none of its orders can still be live.
func RunSettled(state string) bool {
	for _, s := range SettledRuns {
		if s == state {
			return true
		}
	}
	return false
}

// DeleteDraftedIn removes the plans drafted in conversations that are going, as long as they
// never ran. One that ran is the record of its orders, and stays without the link.
func (d *PlanStore) DeleteDraftedIn(conversationIDs []string) error {
	if len(conversationIDs) == 0 {
		return nil
	}
	return d.DB.Exec(`DELETE FROM futures.plans p WHERE p.user_id = ? AND p.conversation_id IN ?
		AND NOT EXISTS (SELECT 1 FROM futures.runs r WHERE r.plan_id = p.id)`, d.UserID, conversationIDs).Error
}

func (d *PlanStore) Add(plans []plan.Plan) error {
	return d.DB.Transaction(func(tx *gorm.DB) error { return d.add(tx, plans) })
}
func (d *PlanStore) add(tx *gorm.DB, plans []plan.Plan) error {
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
		r := Plan{UserID: d.UserID, ID: p.ID, AccountID: d.AccountID, Symbol: p.Snapshot.Symbol, Status: "draft", ConversationID: d.ConversationID, Body: string(raw), CreatedAt: now, UpdatedAt: now}
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
		if e := d.add(tx, plans); e != nil {
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
