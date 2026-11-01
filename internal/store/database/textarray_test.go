package database_test

import (
	"slices"
	"testing"

	"vergex/internal/store/database"
	"vergex/internal/store/database/testdb"
)

type arrayRow struct {
	ID     int64 `gorm:"primaryKey;autoIncrement:false"`
	Values database.TextArray
}

func (arrayRow) TableName() string { return "array_rows" }

func TestTextArrayRoundTrips(t *testing.T) {
	db := testdb.Blank(t)
	if err := db.Exec(`CREATE TABLE array_rows (id bigint PRIMARY KEY, values text[] NOT NULL DEFAULT '{}')`).Error; err != nil {
		t.Fatal(err)
	}
	for id, values := range map[int64][]string{
		1: {"BTCUSDT", "ETHUSDT"},
		2: {`has "quotes" and \ backslash`, "has, comma", "{braces}", "", "NULL", "多字节"},
		3: {},
	} {
		if err := db.Create(&arrayRow{ID: id, Values: values}).Error; err != nil {
			t.Fatalf("create %d: %v", id, err)
		}
		var read arrayRow
		if err := db.First(&read, id).Error; err != nil {
			t.Fatalf("read %d: %v", id, err)
		}
		if !slices.Equal([]string(read.Values), values) {
			t.Fatalf("row %d = %q, want %q", id, read.Values, values)
		}
	}
	if err := db.Create(&arrayRow{ID: 4}).Error; err != nil {
		t.Fatalf("nil slice: %v", err)
	}
	var read arrayRow
	if err := db.First(&read, 4).Error; err != nil {
		t.Fatal(err)
	}
	if len(read.Values) != 0 {
		t.Fatalf("nil stored as %q", read.Values)
	}
}
