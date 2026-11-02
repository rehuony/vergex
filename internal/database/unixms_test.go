package database_test

import (
	"database/sql"
	"testing"

	"vergex/internal/database"
	"vergex/internal/database/testdb"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type timedRow struct {
	ID        int64 `gorm:"primaryKey;autoIncrement:false"`
	OpenTime  int64 `gorm:"column:open_at;serializer:unixms"`
	UpdatedAt int64 `gorm:"column:updated_at;serializer:unixms;autoUpdateTime:false"`
}

func (timedRow) TableName() string { return "timed_rows" }

func timedTable(t *testing.T) *gorm.DB {
	t.Helper()
	db := testdb.Blank(t)
	err := db.Exec(`CREATE TABLE timed_rows (id bigint PRIMARY KEY, open_at timestamptz, updated_at timestamptz)`).Error
	if err != nil {
		t.Fatal(err)
	}
	return db
}

const (
	firstMillis  = int64(1_700_000_000_123)
	secondMillis = int64(1_700_000_060_456)
)

func storedTime(t *testing.T, db *gorm.DB, id int64) sql.NullTime {
	t.Helper()
	var stored sql.NullTime
	if err := db.Raw("SELECT open_at FROM timed_rows WHERE id = ?", id).Scan(&stored).Error; err != nil {
		t.Fatal(err)
	}
	return stored
}

func readRow(t *testing.T, db *gorm.DB, id int64) timedRow {
	t.Helper()
	var row timedRow
	if err := db.First(&row, id).Error; err != nil {
		t.Fatal(err)
	}
	return row
}

func TestUnixMillisRoundTripsThroughEveryWrite(t *testing.T) {
	db := timedTable(t)

	if err := db.Create(&timedRow{ID: 1, OpenTime: firstMillis}).Error; err != nil {
		t.Fatalf("Create: %v", err)
	}
	if stored := storedTime(t, db, 1); !stored.Valid || stored.Time.UnixMilli() != firstMillis {
		t.Fatalf("stored = %v", stored)
	}
	if row := readRow(t, db, 1); row.OpenTime != firstMillis {
		t.Fatalf("read = %d", row.OpenTime)
	}

	batch := []timedRow{{ID: 2, OpenTime: firstMillis}, {ID: 3, OpenTime: secondMillis}}
	if err := db.CreateInBatches(batch, 1).Error; err != nil {
		t.Fatalf("CreateInBatches: %v", err)
	}
	if row := readRow(t, db, 3); row.OpenTime != secondMillis {
		t.Fatalf("batch read = %d", row.OpenTime)
	}

	conflict := timedRow{ID: 3, OpenTime: firstMillis}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&conflict).Error; err != nil {
		t.Fatalf("OnConflict: %v", err)
	}
	if row := readRow(t, db, 3); row.OpenTime != secondMillis {
		t.Fatalf("conflict changed the row: %d", row.OpenTime)
	}

	saved := timedRow{ID: 1, OpenTime: secondMillis}
	if err := db.Save(&saved).Error; err != nil {
		t.Fatalf("Save: %v", err)
	}
	if row := readRow(t, db, 1); row.OpenTime != secondMillis {
		t.Fatalf("saved read = %d", row.OpenTime)
	}
}

func TestUnixMillisZeroIsNull(t *testing.T) {
	db := timedTable(t)
	if err := db.Create(&timedRow{ID: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if stored := storedTime(t, db, 1); stored.Valid {
		t.Fatalf("zero stored as %v", stored.Time)
	}
	if row := readRow(t, db, 1); row.OpenTime != 0 || row.UpdatedAt != 0 {
		t.Fatalf("NULL read as %+v", row)
	}
}

func TestUpdatedAtIsNotFilledInSeconds(t *testing.T) {
	db := timedTable(t)
	if err := db.Create(&timedRow{ID: 1, UpdatedAt: firstMillis}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&timedRow{}).Where("id = ?", 1).Updates(map[string]any{"open_at": database.FromMillis(secondMillis)}).Error; err != nil {
		t.Fatal(err)
	}
	if row := readRow(t, db, 1); row.UpdatedAt != firstMillis {
		t.Fatalf("updated_at = %d", row.UpdatedAt)
	}
}

func TestFromMillisWorksInMapsAndConditions(t *testing.T) {
	db := timedTable(t)
	if err := db.Create(&timedRow{ID: 1, OpenTime: firstMillis}).Error; err != nil {
		t.Fatal(err)
	}

	err := db.Model(&timedRow{}).Where("id = ?", 1).Updates(map[string]any{"open_at": database.FromMillis(secondMillis)}).Error
	if err != nil {
		t.Fatalf("Updates: %v", err)
	}
	if row := readRow(t, db, 1); row.OpenTime != secondMillis {
		t.Fatalf("updated read = %d", row.OpenTime)
	}

	err = db.Model(&timedRow{}).Where("id = ?", 1).Updates(map[string]any{"open_at": gorm.Expr("GREATEST(open_at, ?)", database.FromMillis(firstMillis))}).Error
	if err != nil {
		t.Fatalf("Updates with an expression: %v", err)
	}
	if row := readRow(t, db, 1); row.OpenTime != secondMillis {
		t.Fatalf("GREATEST moved the time back: %d", row.OpenTime)
	}

	var count int64
	if err := db.Model(&timedRow{}).Where("open_at >= ?", database.FromMillis(secondMillis)).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("condition matched %d rows", count)
	}

	var latest sql.NullTime
	if err := db.Model(&timedRow{}).Select("MAX(open_at)").Scan(&latest).Error; err != nil {
		t.Fatal(err)
	}
	if !latest.Valid || latest.Time.UnixMilli() != secondMillis {
		t.Fatalf("MAX = %v", latest)
	}
}

func TestMultipleStatementsWithQuestionMarksPassThrough(t *testing.T) {
	db := testdb.Blank(t)
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	body := `
		-- Is a comment with a question mark safe?
		CREATE TABLE first_table (id int);
		CREATE TABLE second_table (value jsonb);
		COMMENT ON TABLE second_table IS 'Has it got ? and @ in it';`
	tx, err := conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(body); err != nil {
		t.Fatalf("multiple statements: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasTable("first_table") || !db.Migrator().HasTable("second_table") {
		t.Fatal("not every statement ran")
	}
}
