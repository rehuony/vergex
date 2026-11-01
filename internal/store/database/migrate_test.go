package database_test

import (
	"strings"
	"testing"
	"time"

	"vergex/internal/store/database"
	"vergex/internal/store/database/testdb"
)

func TestMigrateBuildsAnEmptyDatabaseAndChangesNothingTheSecondTime(t *testing.T) {
	db := testdb.Blank(t)
	if err := database.Migrate(db); err != nil {
		t.Fatalf("first run: %v", err)
	}

	var schemas []string
	err := db.Raw(`SELECT schema_name FROM information_schema.schemata ORDER BY 1`).Scan(&schemas).Error
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"agent", "core", "forecast", "frontier", "market", "strategy", "telegram", "trader", "trading"} {
		if !strings.Contains(strings.Join(schemas, " "), want) {
			t.Errorf("schema %s is missing from %v", want, schemas)
		}
	}

	var firstRun time.Time
	if err := db.Raw(`SELECT MAX(applied_at) FROM core.schema_migrations`).Scan(&firstRun).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("second run: %v", err)
	}
	var secondRun time.Time
	if err := db.Raw(`SELECT MAX(applied_at) FROM core.schema_migrations`).Scan(&secondRun).Error; err != nil {
		t.Fatal(err)
	}
	if !firstRun.Equal(secondRun) {
		t.Fatal("the second run applied something")
	}
}

func TestMigrateRefusesAFileChangedAfterItWasApplied(t *testing.T) {
	db := testdb.Open(t)
	if err := db.Exec(`UPDATE core.schema_migrations SET checksum = 'something else'`).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(db); err == nil || !strings.Contains(err.Error(), "changed after it was applied") {
		t.Fatalf("err = %v", err)
	}
}

func TestOpenGivesEachTestItsOwnDatabase(t *testing.T) {
	first, second := testdb.Open(t), testdb.Open(t)
	var firstName, secondName string
	if err := first.Raw(`SELECT current_database()`).Scan(&firstName).Error; err != nil {
		t.Fatal(err)
	}
	if err := second.Raw(`SELECT current_database()`).Scan(&secondName).Error; err != nil {
		t.Fatal(err)
	}
	if firstName == secondName {
		t.Fatalf("both tests got %s", firstName)
	}
}
