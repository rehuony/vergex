// Package store provides unified database storage layer
package store

// DBConfig holds the PostgreSQL connection settings
type DBConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}
