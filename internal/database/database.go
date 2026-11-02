// Package database opens PostgreSQL and owns the schema: the tables are defined by
// the SQL files in migrations/, and GORM only queries them.
package database

import (
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Config holds the PostgreSQL connection settings.
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

const (
	maxOpenConnections = 25
	maxIdleConnections = 5
)

// OpenMigrated connects to the database in cfg and brings its tables up to date.
func OpenMigrated(cfg Config) (*gorm.DB, error) {
	db, err := Open(cfg)
	if err != nil {
		return nil, err
	}
	if err := Migrate(db); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}
	return db, nil
}

// Close closes a connection's pool.
func Close(db *gorm.DB) error {
	conn, err := db.DB()
	if err != nil {
		return err
	}
	return conn.Close()
}

// Open connects to the database in cfg.
func Open(cfg Config) (*gorm.DB, error) {
	db, err := OpenDSN(fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode,
	))
	if err != nil {
		return nil, fmt.Errorf("failed to open PostgreSQL database: %w", err)
	}
	conn, err := db.DB()
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(maxOpenConnections)
	conn.SetMaxIdleConns(maxIdleConnections)
	return db, nil
}

// OpenDSN connects with a connection string and the settings every connection shares.
func OpenDSN(dsn string) (*gorm.DB, error) {
	return gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger:         logger.Default.LogMode(logger.Silent),
		TranslateError: true,
		// Use UTC for all auto-generated timestamps (autoCreateTime, autoUpdateTime)
		NowFunc: func() time.Time { return time.Now().UTC() },
	})
}
