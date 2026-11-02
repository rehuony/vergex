package run

import (
	"errors"
	"slices"
	"time"

	"vergex/internal/account"
	"vergex/internal/database"
	"vergex/internal/futures"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// runOrder is an Order as the controller keeps it, in its place in the run's list. The
// columns are the Order's fields and nothing is converted: the controller's checks read
// back exactly what they wrote.
type runOrder struct {
	RunID           string          `gorm:"column:run_id;primaryKey"`
	Seq             int             `gorm:"column:seq;primaryKey;autoIncrement:false"`
	Role            string          `gorm:"column:role"`
	Symbol          string          `gorm:"column:symbol"`
	Side            string          `gorm:"column:side"`
	Type            string          `gorm:"column:type"`
	ClientID        string          `gorm:"column:client_id"`
	VenueClientID   string          `gorm:"column:venue_client_id"`
	ParentClientID  string          `gorm:"column:parent_client_id"`
	ExchangeOrderID string          `gorm:"column:exchange_order_id"`
	Price           account.Decimal `gorm:"column:price"`
	Quantity        account.Decimal `gorm:"column:quantity"`
	Status          string          `gorm:"column:status"`
	Filled          account.Decimal `gorm:"column:filled"`
	Average         account.Decimal `gorm:"column:average"`
	ReduceOnly      bool            `gorm:"column:reduce_only"`
	PostOnly        bool            `gorm:"column:post_only"`
	Error           string          `gorm:"column:error"`
	UpdatedAt       time.Time       `gorm:"column:updated_at;autoUpdateTime:false"`
}

func (runOrder) TableName() string { return "futures.run_orders" }

func toRunOrder(runID string, seq int, o Order) runOrder {
	return runOrder{
		RunID: runID, Seq: seq, Role: o.Role, Symbol: o.Symbol, Side: o.Side, Type: o.Type, ClientID: o.ClientID,
		VenueClientID: o.VenueClientID, ParentClientID: o.ParentID, ExchangeOrderID: o.OrderID, Price: o.Price,
		Quantity: o.Quantity, Status: o.Status, Filled: o.Filled, Average: o.Average, ReduceOnly: o.ReduceOnly,
		PostOnly: o.PostOnly, Error: o.Error,
	}
}

func (r runOrder) order() Order {
	return Order{
		Type: r.Type, ReduceOnly: r.ReduceOnly, PostOnly: r.PostOnly, VenueClientID: r.VenueClientID, ParentID: r.ParentClientID,
		ClientID: r.ClientID, OrderID: r.ExchangeOrderID, Role: r.Role, Symbol: r.Symbol, Side: r.Side, Price: r.Price,
		Quantity: r.Quantity, Status: r.Status, Filled: r.Filled, Average: r.Average, Error: r.Error,
	}
}

// storedRun is what the database holds of a run's changing parts.
type storedRun struct {
	state, err string
	started    *time.Time
	deadline   *time.Time
	orders     []Order
}

func (v *View) remember() {
	v.stored = storedRun{state: v.State, err: v.Error, started: v.StartedAt, deadline: v.Deadline, orders: slices.Clone(v.Orders)}
}

func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

func (v *View) changed() bool {
	s := &v.stored
	return v.State != s.state || v.Error != s.err || !sameTime(v.StartedAt, s.started) || !sameTime(v.Deadline, s.deadline) || !slices.Equal(v.Orders, s.orders)
}

// writeRun writes the run and whichever of its orders changed, on the caller's
// transaction, so a run is never durable without the intent it depends on.
func writeRun(tx *gorm.DB, v *View) error {
	v.UpdatedAt = time.Now().UTC()
	result := tx.Model(&Run{}).Where("id = ? AND user_id = ?", v.ID, v.UserID).Updates(map[string]any{
		"state": v.State, "error": database.NullText(v.Error), "started_at": v.StartedAt, "deadline": v.Deadline, "updated_at": v.UpdatedAt,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("execution not found or not owned")
	}
	var rows []runOrder
	for i, order := range v.Orders {
		if i >= len(v.stored.orders) || order != v.stored.orders[i] {
			row := toRunOrder(v.ID, i, order)
			row.UpdatedAt = v.UpdatedAt
			rows = append(rows, row)
		}
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&rows).Error
}

// loadedRun is a run read back, or why it could not be trusted.
type loadedRun struct {
	view View
	err  error
}

// loadRuns reads each run's plan and orders and verifies them against its approval.
func loadRuns(db *gorm.DB, rows []Run) ([]loadedRun, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	runIDs := make([]string, len(rows))
	planIDs := make([]string, len(rows))
	for i, row := range rows {
		runIDs[i], planIDs[i] = row.ID, row.PlanID
	}
	var plans []futures.Plan
	if err := db.Select("user_id", "id", "body").Where("id IN ?", planIDs).Find(&plans).Error; err != nil {
		return nil, err
	}
	bodies := make(map[[2]string]string, len(plans))
	for _, plan := range plans {
		bodies[[2]string{plan.UserID, plan.ID}] = plan.Body
	}
	var stored []runOrder
	if err := db.Where("run_id IN ?", runIDs).Order("run_id, seq").Find(&stored).Error; err != nil {
		return nil, err
	}
	orders := make(map[string][]Order, len(rows))
	for _, row := range stored {
		orders[row.RunID] = append(orders[row.RunID], row.order())
	}
	out := make([]loadedRun, len(rows))
	for i, row := range rows {
		list := orders[row.ID]
		if list == nil {
			list = []Order{}
		}
		out[i].view, out[i].err = decode(row, bodies[[2]string{row.UserID, row.PlanID}], list)
	}
	return out, nil
}

func loadRun(db *gorm.DB, row Run) (View, error) {
	loaded, err := loadRuns(db, []Run{row})
	if err != nil {
		return View{}, err
	}
	return loaded[0].view, loaded[0].err
}
