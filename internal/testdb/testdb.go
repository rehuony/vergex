// Package testdb gives each test its own empty PostgreSQL schema, so tests can
// run in parallel on one database and leave nothing behind.
package testdb

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func open(rawURL string) (*gorm.DB, error) {
	return gorm.Open(postgres.Open(rawURL), &gorm.Config{
		Logger:         logger.Default.LogMode(logger.Silent),
		TranslateError: true,
		NowFunc:        func() time.Time { return time.Now().UTC() },
	})
}

// repoEnv reads the .env at the module root, however deep the test's package is.
func repoEnv() (map[string]string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return godotenv.Read(filepath.Join(dir, ".env"))
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, errors.New("go.mod not found")
		}
		dir = parent
	}
}

// databaseURL is VERGEX_TEST_DATABASE_URL, or the `<DB_NAME>_test` database that
// `make run-postgresql` creates beside the one in .env. Tests never run on DB_NAME.
func databaseURL() (string, error) {
	if rawURL := os.Getenv("VERGEX_TEST_DATABASE_URL"); rawURL != "" {
		return rawURL, nil
	}
	env, err := repoEnv()
	if err != nil {
		return "", err
	}
	setting := func(key, fallback string) string {
		if value := env[key]; value != "" {
			return value
		}
		return fallback
	}
	target := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(setting("DB_USER", "vergex"), env["DB_PASSWORD"]),
		Host:     net.JoinHostPort(setting("DB_HOST", "127.0.0.1"), setting("DB_PORT", "5432")),
		Path:     "/" + setting("DB_NAME", "vergex") + "_test",
		RawQuery: "sslmode=" + setting("DB_SSLMODE", "disable"),
	}
	return target.String(), nil
}

// Open returns a connection to a new schema that is dropped when the test ends.
// A test that needs the database fails without one rather than skipping, so a
// missing database cannot quietly turn off the tests on live trading paths.
func Open(t testing.TB) *gorm.DB {
	t.Helper()
	const hint = "run `make run-postgresql`, or set VERGEX_TEST_DATABASE_URL"
	rawURL, err := databaseURL()
	if err != nil {
		t.Fatalf("find the test database: %v (%s)", err, hint)
	}
	admin, err := open(rawURL)
	if err != nil {
		t.Fatalf("connect to the test database: %v (%s)", err, hint)
	}
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	schema := "test_" + hex.EncodeToString(suffix)
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatalf("create test schema: %v", err)
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse the test database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	db, err := open(parsed.String())
	if err != nil {
		t.Fatalf("connect to test schema: %v", err)
	}

	t.Cleanup(func() {
		if conn, err := db.DB(); err == nil {
			conn.Close()
		}
		if err := admin.Exec("DROP SCHEMA " + schema + " CASCADE").Error; err != nil {
			t.Errorf("drop test schema: %v", err)
		}
		if conn, err := admin.DB(); err == nil {
			conn.Close()
		}
	})
	return db
}
