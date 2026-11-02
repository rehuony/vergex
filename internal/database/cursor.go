package database

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// CursorAt is where to start reading a table in ID order to see every row from a time on:
// just before the first such row, or past the last row when there is none. IDs and times
// need not run together, so a row with a late ID and an old time does not hide the rows
// before it.
func CursorAt(db *gorm.DB, table, timeColumn string, at time.Time) (int64, error) {
	var id int64
	query := fmt.Sprintf(`SELECT COALESCE((SELECT MIN(id) - 1 FROM %[1]s WHERE %[2]s >= ?), (SELECT MAX(id) FROM %[1]s), 0)`, table, timeColumn)
	err := db.Raw(query, at).Scan(&id).Error
	return id, err
}
