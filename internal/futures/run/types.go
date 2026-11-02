// Package run executes only finite, immutable, human-approved order baskets.
// Its broker interface has no autonomous strategy or agent capability.
package run

import (
	"context"
	"time"
	"vergex/internal/account"
	"vergex/internal/futures/plan"
)

// Aliases keep execution's contract in one place while it belongs to Vergex.
type Order = account.Order
type Snapshot = account.Snapshot

// ClientIDRouter supplies an exchange-native route before intent is persisted.
// Empty routes on historical records retain their original raw ID for lookup.
type ClientIDRouter interface{ RouteClientID(string) (string, error) }

func routedOrder(b Broker, o Order) (Order, error) {
	if router, ok := b.(ClientIDRouter); ok {
		id, e := router.RouteClientID(o.ClientID)
		if e != nil {
			return o, e
		}
		o.VenueClientID = id
	}
	return o, nil
}

// Broker preserves exact approved decimals and stable client IDs. Missing orders
// are never assumed filled and never authorize blind resubmission.
type Broker interface {
	Venue() string
	Account(context.Context, string) (Snapshot, error)
	Prepare(context.Context, string) error
	Place(context.Context, Order) (Order, error)
	Lookup(context.Context, Order) (Order, error)
	Cancel(context.Context, Order) error
}
type Run struct {
	ID           string     `gorm:"primaryKey" json:"id"`
	UserID       string     `gorm:"column:user_id" json:"-"`
	AccountID    string     `gorm:"column:account_id" json:"account_id"`
	AccountKey   string     `gorm:"column:account_identity" json:"-"`
	PlanID       string     `gorm:"column:plan_id" json:"plan_id"`
	PlanHash     string     `gorm:"column:plan_hash" json:"plan_hash"`
	ApprovalHash string     `gorm:"column:approval_hash" json:"approval_hash"`
	State        string     `gorm:"column:state" json:"state"`
	ApprovedAt   time.Time  `gorm:"column:approved_at" json:"approved_at"`
	StartedAt    *time.Time `gorm:"column:started_at" json:"started_at,omitempty"`
	Deadline     *time.Time `gorm:"column:deadline" json:"deadline,omitempty"`
	UpdatedAt    time.Time  `gorm:"column:updated_at" json:"updated_at"`
	Error        string     `gorm:"column:error;serializer:nulltext" json:"error,omitempty"`
}

func (Run) TableName() string { return "futures.runs" }

type View struct {
	Run
	Plan   plan.Plan `json:"plan"`
	Orders []Order   `json:"orders"`
	// stored is what the database holds, so a save writes only what changed.
	stored storedRun
}
type Factory func(context.Context, string, string) (Broker, string, error)
