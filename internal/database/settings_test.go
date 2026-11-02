package database_test

import (
	"sync"
	"testing"

	"vergex/internal/database"
	"vergex/internal/database/testdb"
)

func TestSettingsMergeFieldsAndReportAbsence(t *testing.T) {
	db := testdb.Open(t)
	var values struct {
		Enabled bool     `json:"enabled"`
		TopN    int      `json:"top_n"`
		Symbols []string `json:"symbols"`
	}
	found, err := database.LoadSetting(db, "market", &values)
	if err != nil || found {
		t.Fatalf("empty: found = %v, err = %v", found, err)
	}

	if err := database.PatchSetting(db, "market", map[string]any{"enabled": true, "symbols": []string{"BTCUSDT"}}); err != nil {
		t.Fatal(err)
	}
	if err := database.PatchSetting(db, "market", map[string]any{"top_n": 5}); err != nil {
		t.Fatal(err)
	}
	found, err = database.LoadSetting(db, "market", &values)
	if err != nil || !found {
		t.Fatalf("found = %v, err = %v", found, err)
	}
	if !values.Enabled || values.TopN != 5 || len(values.Symbols) != 1 {
		t.Fatalf("values = %+v", values)
	}
}

func TestSettingsWritersOfDifferentFieldsKeepBoth(t *testing.T) {
	db := testdb.Open(t)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			field := map[string]any{"a": i}
			if i%2 == 1 {
				field = map[string]any{"b": i}
			}
			if err := database.PatchSetting(db, "m", field); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	var values map[string]any
	if _, err := database.LoadSetting(db, "m", &values); err != nil {
		t.Fatal(err)
	}
	if values["a"] == nil || values["b"] == nil {
		t.Fatalf("values = %v", values)
	}
}
