package database

import (
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"

	"gorm.io/gorm"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// migrationLock keeps two processes from migrating at once. Any fixed number works.
const migrationLock = 7235110

const createMigrationsTable = `
	CREATE SCHEMA IF NOT EXISTS core;
	CREATE TABLE IF NOT EXISTS core.schema_migrations (
		name       text PRIMARY KEY,
		checksum   text NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now()
	);`

type migration struct {
	name, body, checksum string
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return nil, err
	}
	migrations := make([]migration, 0, len(entries))
	for _, entry := range entries {
		body, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(body)
		migrations = append(migrations, migration{name: entry.Name(), body: string(body), checksum: hex.EncodeToString(sum[:])})
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].name < migrations[j].name })
	return migrations, nil
}

// Migrate brings the database up to date with the embedded migrations. They run in
// name order and in one transaction, and only forward. A file that changed after it
// was applied is an error, since the database no longer matches what the file says.
func Migrate(db *gorm.DB) error {
	migrations, err := loadMigrations()
	if err != nil {
		return err
	}
	conn, err := db.DB()
	if err != nil {
		return err
	}
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("SELECT pg_advisory_xact_lock($1)", migrationLock); err != nil {
		return fmt.Errorf("lock migrations: %w", err)
	}
	if _, err := tx.Exec(createMigrationsTable); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}
	applied, err := appliedMigrations(tx)
	if err != nil {
		return err
	}
	for _, m := range migrations {
		checksum, done := applied[m.name]
		if done {
			if checksum != m.checksum {
				return fmt.Errorf("migration %s changed after it was applied", m.name)
			}
			continue
		}
		if _, err := tx.Exec(m.body); err != nil {
			return fmt.Errorf("apply migration %s: %w", m.name, err)
		}
		if _, err := tx.Exec("INSERT INTO core.schema_migrations (name, checksum) VALUES ($1, $2)", m.name, m.checksum); err != nil {
			return fmt.Errorf("record migration %s: %w", m.name, err)
		}
	}
	return tx.Commit()
}

func appliedMigrations(tx *sql.Tx) (map[string]string, error) {
	rows, err := tx.Query("SELECT name, checksum FROM core.schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("read applied migrations: %w", err)
	}
	defer rows.Close()
	applied := map[string]string{}
	for rows.Next() {
		var name, checksum string
		if err := rows.Scan(&name, &checksum); err != nil {
			return nil, err
		}
		applied[name] = checksum
	}
	return applied, rows.Err()
}

// MigrationsChecksum identifies the current set of migrations.
func MigrationsChecksum() (string, error) {
	migrations, err := loadMigrations()
	if err != nil {
		return "", err
	}
	all := sha256.New()
	for _, m := range migrations {
		all.Write([]byte(m.name + m.checksum))
	}
	return hex.EncodeToString(all.Sum(nil)), nil
}
