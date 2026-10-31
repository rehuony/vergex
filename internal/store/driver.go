// Package store provides unified database storage layer
package store

// DBType represents database type
type DBType string

const (
	DBTypeSQLite   DBType = "sqlite"
	DBTypePostgres DBType = "postgres"
)

// DBConfig database configuration
type DBConfig struct {
	Type     DBType // sqlite or postgres
	Path     string // SQLite file path (for sqlite)
	Host     string // PostgreSQL host (for postgres)
	Port     int    // PostgreSQL port (for postgres)
	User     string // PostgreSQL user (for postgres)
	Password string // PostgreSQL password (for postgres)
	DBName   string // PostgreSQL database name (for postgres)
	SSLMode  string // PostgreSQL SSL mode (for postgres)
}
