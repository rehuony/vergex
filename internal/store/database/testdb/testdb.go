// Package testdb gives each test a database of its own, cloned from a template with
// every migration applied, so tests see the real schema and leave nothing behind.
// The template is `<DB_NAME>_test`. It is rebuilt whenever the migrations change.
package testdb

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"vergex/internal/store/database"

	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

const (
	hint = "run `make run-postgresql`, or set VERGEX_TEST_DATABASE_URL"
	// templateLock lets clones share the template while a rebuild has it alone.
	templateLock = 7235111
	// staleClone is how old a clone must be to count as left behind by a killed run.
	staleClone = time.Hour
)

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

// templateURL is VERGEX_TEST_DATABASE_URL, or the `<DB_NAME>_test` database beside the
// one in .env. Tests never run on DB_NAME. That database is dropped and rebuilt, so its
// name must end in _test, and its role must be able to create databases.
func templateURL() (*url.URL, error) {
	if rawURL := os.Getenv("VERGEX_TEST_DATABASE_URL"); rawURL != "" {
		return url.Parse(rawURL)
	}
	env, err := repoEnv()
	if err != nil {
		return nil, err
	}
	setting := func(key, fallback string) string {
		if value := env[key]; value != "" {
			return value
		}
		return fallback
	}
	return &url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(setting("DB_USER", "vergex"), env["DB_PASSWORD"]),
		Host:     net.JoinHostPort(setting("DB_HOST", "127.0.0.1"), setting("DB_PORT", "5432")),
		Path:     "/" + setting("DB_NAME", "vergex") + "_test",
		RawQuery: "sslmode=" + setting("DB_SSLMODE", "disable"),
	}, nil
}

// server holds what a test process shares: the server connection and the template.
type server struct {
	template *url.URL
	name     string
	admin    *sql.DB
}

var (
	shared     *server
	sharedErr  error
	sharedOnce sync.Once
)

func connect() (*server, error) {
	sharedOnce.Do(func() {
		shared, sharedErr = prepare()
	})
	return shared, sharedErr
}

func prepare() (*server, error) {
	template, err := templateURL()
	if err != nil {
		return nil, fmt.Errorf("find the test database: %w", err)
	}
	name := strings.TrimPrefix(template.Path, "/")
	if !strings.HasSuffix(name, "_test") {
		return nil, fmt.Errorf("refusing to use database %q for tests: they drop and rebuild it, so its name must end in _test", name)
	}
	// A template can't be cloned while connected to, so the server work happens from
	// the maintenance database.
	maintenance := *template
	maintenance.Path = "/postgres"
	gdb, err := database.OpenDSN(maintenance.String())
	if err != nil {
		return nil, fmt.Errorf("connect to the test server: %w", err)
	}
	admin, err := gdb.DB()
	if err != nil {
		return nil, err
	}
	s := &server{template: template, name: name, admin: admin}
	if err := s.refreshTemplate(); err != nil {
		return nil, err
	}
	s.dropStaleClones()
	return s, nil
}

// refreshTemplate rebuilds the template when the migrations differ from the ones it
// was built from, which it records in the database's comment.
func (s *server) refreshTemplate() error {
	want, err := database.MigrationsChecksum()
	if err != nil {
		return err
	}
	ctx := context.Background()
	conn, err := s.admin.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", templateLock); err != nil {
		return err
	}
	defer conn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", templateLock)

	var built sql.NullString
	err = conn.QueryRowContext(ctx, "SELECT shobj_description(oid, 'pg_database') FROM pg_database WHERE datname = $1", s.name).Scan(&built)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if built.String == want {
		return nil
	}
	if _, err := conn.ExecContext(ctx, "DROP DATABASE IF EXISTS "+quote(s.name)+" WITH (FORCE)"); err != nil {
		return fmt.Errorf("drop the old test template: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "CREATE DATABASE "+quote(s.name)); err != nil {
		return fmt.Errorf("create the test template: %w", err)
	}
	template, err := database.OpenDSN(s.template.String())
	if err != nil {
		return err
	}
	migrateErr := database.Migrate(template)
	if closing, err := template.DB(); err == nil {
		closing.Close()
	}
	if migrateErr != nil {
		return fmt.Errorf("migrate the test template: %w", migrateErr)
	}
	_, err = conn.ExecContext(ctx, "COMMENT ON DATABASE "+quote(s.name)+" IS '"+want+"'")
	return err
}

// dropStaleClones removes clones left by runs that were killed. A clone's name carries
// the time it was made, and anything else is left alone.
func (s *server) dropStaleClones() {
	rows, err := s.admin.Query("SELECT datname FROM pg_database WHERE starts_with(datname, $1)", s.name+"_")
	if err != nil {
		return
	}
	defer rows.Close()
	var stale []string
	for rows.Next() {
		var name string
		if rows.Scan(&name) != nil {
			continue
		}
		if made, ok := cloneTime(name, s.name); ok && time.Since(made) > staleClone {
			stale = append(stale, name)
		}
	}
	for _, name := range stale {
		s.admin.Exec("DROP DATABASE IF EXISTS " + quote(name) + " WITH (FORCE)")
	}
}

var cloneSuffix = regexp.MustCompile(`^_([0-9]{10})_[0-9a-f]{12}$`)

// cloneTime reads when a clone of the template was made, and reports whether the name
// is one newName gives.
func cloneTime(name, template string) (time.Time, bool) {
	match := cloneSuffix.FindStringSubmatch(strings.TrimPrefix(name, template))
	if match == nil || !strings.HasPrefix(name, template) {
		return time.Time{}, false
	}
	made, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(made, 0), true
}

func quote(name string) string { return `"` + strings.ReplaceAll(name, `"`, `""`) + `"` }

func (s *server) newName() (string, error) {
	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s_%d_%s", s.name, time.Now().Unix(), hex.EncodeToString(suffix)), nil
}

// clone copies the template. Something else connected to it, such as a database tool,
// would make the copy fail, so those sessions are ended first: the template holds
// nothing but empty tables.
func (s *server) clone(name string) error {
	ctx := context.Background()
	conn, err := s.admin.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock_shared($1)", templateLock); err != nil {
		return err
	}
	defer conn.ExecContext(ctx, "SELECT pg_advisory_unlock_shared($1)", templateLock)
	if _, err := conn.ExecContext(ctx, "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1", s.name); err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, "CREATE DATABASE "+quote(name)+" TEMPLATE "+quote(s.name))
	return err
}

// open connects to a test's database and drops it when the test ends.
func (s *server) open(t testing.TB, name string) *gorm.DB {
	t.Helper()
	target := *s.template
	target.Path = "/" + name
	db, err := database.OpenDSN(target.String())
	if err != nil {
		t.Fatalf("connect to the test database: %v", err)
	}
	t.Cleanup(func() {
		if conn, err := db.DB(); err == nil {
			conn.Close()
		}
		if _, err := s.admin.Exec("DROP DATABASE IF EXISTS " + quote(name) + " WITH (FORCE)"); err != nil {
			t.Errorf("drop the test database: %v", err)
		}
	})
	return db
}

// Open returns a connection to a new database with every migration applied, dropped
// when the test ends. A test that needs the database fails without one rather than
// skipping, so a missing database cannot quietly turn off the tests on live trading
// paths.
func Open(t testing.TB) *gorm.DB {
	t.Helper()
	s, err := connect()
	if err != nil {
		t.Fatalf("%v (%s)", err, hint)
	}
	name, err := s.newName()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.clone(name); err != nil {
		t.Fatalf("clone the test template: %v", err)
	}
	return s.open(t, name)
}

// Blank returns a connection to a new database with nothing in it, for tests of the
// migrations themselves.
func Blank(t testing.TB) *gorm.DB {
	t.Helper()
	s, err := connect()
	if err != nil {
		t.Fatalf("%v (%s)", err, hint)
	}
	name, err := s.newName()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.admin.Exec("CREATE DATABASE " + quote(name)); err != nil {
		t.Fatalf("create the test database: %v", err)
	}
	return s.open(t, name)
}
