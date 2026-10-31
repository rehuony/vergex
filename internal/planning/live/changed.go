package live

import (
	"time"

	"gorm.io/gorm"
)

// ChangedSince returns the runs updated at or after a time, oldest change first. It
// only reads the database, so unlike List it never checks a run with the exchange.
// A run that fails verification is left out.
func ChangedSince(db *gorm.DB, since time.Time, limit int) ([]View, error) {
	var rows []Run
	if err := db.Where("updated_at >= ?", since).Order("updated_at ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	views := make([]View, 0, len(rows))
	for _, row := range rows {
		if view, err := decode(row); err == nil {
			views = append(views, view)
		}
	}
	return views, nil
}
