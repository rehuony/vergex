package run

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// RefusedOrder is an order refused before it reached the exchange, so the ledger never
// hears of it. UpdatedAt is when its row last changed, which a run's own time is not:
// a restart moves every unfinished run to needs_attention.
type RefusedOrder struct {
	RunID     string    `gorm:"column:run_id"`
	PlanID    string    `gorm:"column:plan_id"`
	AccountID string    `gorm:"column:account_id"`
	ClientID  string    `gorm:"column:client_id"`
	Role      string    `gorm:"column:role"`
	Symbol    string    `gorm:"column:symbol"`
	Quantity  string    `gorm:"column:quantity"`
	Error     string    `gorm:"column:error"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

// RefusedSince returns the refused orders changed at or after a time, oldest first. It
// only reads the database, so it never checks a run with the exchange.
func RefusedSince(db *gorm.DB, since time.Time, limit int) ([]RefusedOrder, error) {
	var orders []RefusedOrder
	err := db.Raw(`SELECT o.run_id, r.plan_id, r.account_id, o.client_id, o.role, o.symbol, o.quantity, o.error, o.updated_at
		FROM futures.run_orders o JOIN futures.runs r ON r.id = o.run_id
		WHERE o.exchange_order_id = '' AND o.error <> '' AND o.updated_at >= ?
		ORDER BY o.updated_at LIMIT ?`, since, limit).Scan(&orders).Error
	return orders, err
}

// OrderEnded reports whether an order's status is one it cannot leave.
func OrderEnded(status string) bool {
	return status == "FILLED" || status == "CANCELED" || status == "REJECTED" || status == "EXPIRED"
}

// CheckCloseEntries refuses a manual close while a run on the same account and symbol
// could still open more of the position.
func CheckCloseEntries(db *gorm.DB, accountKey, symbol string) error {
	var runs []Run
	err := db.Raw(`SELECT r.* FROM futures.runs r
		JOIN futures.plans p ON p.user_id = r.user_id AND p.id = r.plan_id
		WHERE r.account_identity = ? AND p.symbol = ? AND r.state NOT IN ?`,
		accountKey, symbol, []string{"completed", "cancelled", "expired"}).Scan(&runs).Error
	if err != nil {
		return errors.New("Could not verify remaining Futures entries")
	}
	for _, run := range runs {
		if run.State == "approved" || run.State == "starting" || run.State == "running" {
			return errors.New("Stop remaining Futures entries before closing this position")
		}
		var statuses []string
		if err := db.Model(&runOrder{}).Where("run_id = ? AND role = ?", run.ID, "entry").Pluck("status", &statuses).Error; err != nil {
			return errors.New("Could not verify remaining Futures entries")
		}
		for _, status := range statuses {
			if !OrderEnded(status) {
				return errors.New("An entry is still open or unconfirmed. Stop and reconcile it before closing")
			}
		}
	}
	return nil
}
