package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"vergex/internal/planning"
	"vergex/internal/planning/live"
	"vergex/internal/store"
)

// openResetTestStore opens a fresh SQLite store with the Frontier tables that
// the server creates on first use.
func openResetTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.New(filepath.Join(t.TempDir(), "vergex.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.GormDB().AutoMigrate(&planning.Draft{}, &planning.Conversation{}, &planning.Preferences{}, &live.Run{}, &live.Lease{}); err != nil {
		t.Fatalf("migrate planning tables: %v", err)
	}
	return st
}

func TestResetTablesCoverEveryTable(t *testing.T) {
	st := openResetTestStore(t)
	tables, err := st.GormDB().Migrator().GetTables()
	if err != nil {
		t.Fatalf("GetTables: %v", err)
	}
	for _, table := range tables {
		if table == "sqlite_sequence" {
			continue
		}
		if !slices.Contains(resetTables, table) {
			t.Errorf("reset-account leaves table %q untouched; add it to resetTables", table)
		}
	}
}

func TestWipeAllDataEmptiesTablesAndPlanningFiles(t *testing.T) {
	st := openResetTestStore(t)
	db := st.GormDB()
	if err := st.User().Create(&store.User{ID: "u1", Email: "me@example.com", PasswordHash: "x"}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := st.TelegramConfig().Save("123:token", ""); err != nil {
		t.Fatalf("save telegram config: %v", err)
	}
	if err := db.Exec("INSERT INTO ai_charges (trader_id, model, provider, cost_usd) VALUES ('t1', 'm', 'p', 0.1)").Error; err != nil {
		t.Fatalf("insert ai charge: %v", err)
	}
	planningDir := filepath.Join(t.TempDir(), "planning")
	if err := os.MkdirAll(filepath.Join(planningDir, "abc", "entry-plugins"), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := wipeAllData(st, planningDir); err != nil {
		t.Fatalf("wipeAllData: %v", err)
	}

	for _, table := range []string{"users", "telegram_configs", "ai_charges"} {
		var n int64
		if err := db.Table(table).Count(&n).Error; err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if n != 0 {
			t.Errorf("%s still has %d rows", table, n)
		}
	}
	if _, err := os.Stat(planningDir); !os.IsNotExist(err) {
		t.Errorf("planning data dir still exists (stat err: %v)", err)
	}
}
