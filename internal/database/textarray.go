package database

import (
	"database/sql/driver"
	"sync"

	"github.com/jackc/pgx/v5/pgtype"
)

// TextArray is a Go string slice in a text[] column.
type TextArray []string

var (
	// pgtype.Map caches its plans and isn't safe for concurrent use.
	textArrayMu  sync.Mutex
	textArrayMap = pgtype.NewMap()
)

// Value implements driver.Valuer. A nil slice is stored as an empty array.
func (a TextArray) Value() (driver.Value, error) {
	values := []string(a)
	if values == nil {
		values = []string{}
	}
	textArrayMu.Lock()
	defer textArrayMu.Unlock()
	encoded, err := textArrayMap.Encode(pgtype.TextArrayOID, pgtype.TextFormatCode, values, nil)
	if err != nil {
		return nil, err
	}
	return string(encoded), nil
}

// Scan implements sql.Scanner.
func (a *TextArray) Scan(src any) error {
	textArrayMu.Lock()
	defer textArrayMu.Unlock()
	return textArrayMap.SQLScanner((*[]string)(a)).Scan(src)
}
