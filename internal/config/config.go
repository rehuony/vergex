package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// insecureDefaultJWTSecret is the historical fallback value. Refusing to boot when
// JWT_SECRET matches it (or is missing) prevents the server from silently signing
// tokens with a well-known secret.
const insecureDefaultJWTSecret = "default-jwt-secret-change-in-production"

// minJWTSecretLength is the minimum byte length we accept for HS256 signing keys.
// HS256 keys shorter than 32 bytes are brute-forceable.
const minJWTSecretLength = 32

// Global configuration instance
var global *Config

// Config is the global configuration (loaded from .env)
// Only contains truly global config, trading related config is at trader/strategy level
type Config struct {
	// Service configuration
	APIServerPort int
	JWTSecret     string

	// Database configuration
	DBType     string // sqlite or postgres
	DBPath     string // SQLite database file path
	DBHost     string // PostgreSQL host
	DBPort     int    // PostgreSQL port
	DBUser     string // PostgreSQL user
	DBPassword string // PostgreSQL password
	DBName     string // PostgreSQL database name
	DBSSLMode  string // PostgreSQL SSL mode

	// Security configuration
	// TransportEncryption enables browser-side encryption for API keys
	// Requires HTTPS or localhost. Set to false for HTTP access via IP.
	TransportEncryption bool
}

// MustInit initializes global configuration or panics. Use from main() so the
// process refuses to start under an insecure config (e.g. default JWT secret).
func MustInit() {
	if err := initConfig(); err != nil {
		panic(fmt.Sprintf("config: %v", err))
	}
}

// Init initializes global configuration (from .env). Prefer MustInit from main.
func Init() {
	if err := initConfig(); err != nil {
		// Preserve historical fail-soft behavior for non-main callers (tests, tools);
		// the process can still observe the error via Get() returning nil.
		fmt.Fprintf(os.Stderr, "config init failed: %v\n", err)
	}
}

func initConfig() error {
	cfg := &Config{
		APIServerPort: 3000,
		// Database defaults
		DBType:    "sqlite",
		DBPath:    "data/data.db",
		DBHost:    "localhost",
		DBPort:    5432,
		DBUser:    "postgres",
		DBName:    "vergex",
		DBSSLMode: "disable",
	}

	// Load from environment variables
	if v := os.Getenv("JWT_SECRET"); v != "" {
		cfg.JWTSecret = strings.TrimSpace(v)
	}
	if cfg.JWTSecret == "" {
		return fmt.Errorf("JWT_SECRET is required (set a random %d+ byte value in .env)", minJWTSecretLength)
	}
	if cfg.JWTSecret == insecureDefaultJWTSecret {
		return fmt.Errorf("JWT_SECRET matches the insecure default; generate a fresh random value (e.g. `openssl rand -base64 48`)")
	}
	if len(cfg.JWTSecret) < minJWTSecretLength {
		return fmt.Errorf("JWT_SECRET must be at least %d bytes (got %d); generate via `openssl rand -base64 48`", minJWTSecretLength, len(cfg.JWTSecret))
	}

	if v := os.Getenv("API_SERVER_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil && port > 0 {
			cfg.APIServerPort = port
		}
	}

	// Transport encryption: default false for easier deployment
	// Set TRANSPORT_ENCRYPTION=true to enable (requires HTTPS or localhost)
	if v := os.Getenv("TRANSPORT_ENCRYPTION"); v != "" {
		cfg.TransportEncryption = strings.ToLower(v) == "true"
	}

	// Database configuration
	if v := os.Getenv("DB_TYPE"); v != "" {
		cfg.DBType = strings.ToLower(v)
	}
	if v := os.Getenv("DB_PATH"); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv("DB_HOST"); v != "" {
		cfg.DBHost = v
	}
	if v := os.Getenv("DB_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil && port > 0 {
			cfg.DBPort = port
		}
	}
	if v := os.Getenv("DB_USER"); v != "" {
		cfg.DBUser = v
	}
	if v := os.Getenv("DB_PASSWORD"); v != "" {
		cfg.DBPassword = v
	}
	if v := os.Getenv("DB_NAME"); v != "" {
		cfg.DBName = v
	}
	if v := os.Getenv("DB_SSLMODE"); v != "" {
		cfg.DBSSLMode = v
	}

	global = cfg
	return nil
}

// Get returns the global configuration
func Get() *Config {
	if global == nil {
		Init()
	}
	return global
}
