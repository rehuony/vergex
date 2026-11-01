package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"vergex/internal/store"
	"vergex/internal/store/storetest"
)

func openResetTestStore(t *testing.T) *store.Store {
	t.Helper()
	st := storetest.New(t)
	t.Cleanup(func() { st.Close() })
	return st
}

func TestResetTablesCoverEveryTable(t *testing.T) {
	st := openResetTestStore(t)
	var tables []string
	err := st.GormDB().Raw(`
		SELECT regexp_replace(table_schema || '.' || table_name, '^public\.', '')
		FROM information_schema.tables
		WHERE table_type = 'BASE TABLE' AND table_schema NOT IN ('pg_catalog', 'information_schema')`).Scan(&tables).Error
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	if len(tables) == 0 {
		t.Fatal("found no tables")
	}
	// Market and forecast data outlive an account. Settings are cleared per module.
	kept := []string{"core.schema_migrations", "core.settings"}
	for _, table := range tables {
		if strings.HasPrefix(table, "market.") || strings.HasPrefix(table, "forecast.") || strings.HasPrefix(table, "strategy.") || slices.Contains(kept, table) {
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
	if err := st.User().Create(&store.User{ID: storetest.UserID, Email: "me@example.com", PasswordHash: "x"}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := st.TelegramConfig().Save("123:token", ""); err != nil {
		t.Fatalf("save telegram config: %v", err)
	}
	if err := db.Exec("INSERT INTO agent.calls (feature, source_id, model, provider, cost_usd) VALUES ('trader', 't1', 'm', 'p', 0.1)").Error; err != nil {
		t.Fatalf("insert agent call: %v", err)
	}
	planningDir := filepath.Join(t.TempDir(), "planning")
	if err := os.MkdirAll(filepath.Join(planningDir, "abc", "entry-plugins"), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := wipeAllData(st, planningDir); err != nil {
		t.Fatalf("wipeAllData: %v", err)
	}

	for _, table := range []string{"core.users", "telegram.bot", "agent.calls"} {
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
