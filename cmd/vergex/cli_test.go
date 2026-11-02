package main

import (
	"slices"
	"strings"
	"testing"

	"vergex/internal/database/testdb"
	"vergex/internal/telegram"
	"vergex/internal/user"
)

func TestResetTablesCoverEveryTable(t *testing.T) {
	db := testdb.Open(t)
	var tables []string
	err := db.Raw(`
		SELECT regexp_replace(table_schema || '.' || table_name, '^public\.', '')
		FROM information_schema.tables
		WHERE table_type = 'BASE TABLE' AND table_schema NOT IN ('pg_catalog', 'information_schema')`).Scan(&tables).Error
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	if len(tables) == 0 {
		t.Fatal("found no tables")
	}
	// Market and forecast data outlive a user. Settings are cleared per module.
	kept := []string{"core.schema_migrations", "core.settings"}
	for _, table := range tables {
		if strings.HasPrefix(table, "market.") || strings.HasPrefix(table, "forecast.") || slices.Contains(kept, table) {
			continue
		}
		if !slices.Contains(resetTables, table) {
			t.Errorf("reset-user leaves table %q untouched; add it to resetTables", table)
		}
	}
}

func TestWipeAllDataEmptiesTables(t *testing.T) {
	db := testdb.Open(t)
	if err := user.NewStore(db).Create(&user.User{ID: testdb.UserID, Email: "me@example.com", PasswordHash: "x"}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := telegram.NewConfigStore(db).Save("123:token", ""); err != nil {
		t.Fatalf("save telegram config: %v", err)
	}
	if err := db.Exec("INSERT INTO agent.calls (feature, source_id, model, provider, cost_usd) VALUES ('trader', 't1', 'm', 'p', 0.1)").Error; err != nil {
		t.Fatalf("insert agent call: %v", err)
	}

	if err := wipeAllData(db); err != nil {
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
}
