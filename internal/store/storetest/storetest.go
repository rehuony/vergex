// Package storetest opens a fully migrated Store for tests.
package storetest

import (
	"testing"

	"vergex/internal/store"
	"vergex/internal/testdb"
)

// New returns a Store with all tables and default data, on a schema of its own.
func New(t testing.TB) *store.Store {
	t.Helper()
	st, err := store.NewMigrated(testdb.Open(t))
	if err != nil {
		t.Fatalf("prepare test store: %v", err)
	}
	return st
}
