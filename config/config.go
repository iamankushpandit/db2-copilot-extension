package config

import (
	"fmt"
	"os"
)

// Config holds all configuration values for the application.
type Config struct {
	Port         string // HTTP server port, default "8080"
	ClientID     string // GitHub App Client ID (required)
	ClientSecret string // GitHub App Client Secret (required)
	FQDN         string // Public URL of the agent (required)
	DBType       string // "db2" or "postgres" (default "db2")
	DB2ConnStr   string // DB2 connection string (required if DBType=db2)
	PGConnStr    string // PostgreSQL connection string (required if DBType=postgres)
}

// Load reads configuration from environment variables and validates required fields.
func Load() (*Config, error) {
	cfg := &Config{
		Port:         getEnv("PORT", "8080"),
		ClientID:     os.Getenv("CLIENT_ID"),
		ClientSecret: os.Getenv("CLIENT_SECRET"),
		FQDN:         os.Getenv("FQDN"),
		DBType:       getEnv("DB_TYPE", "db2"),
		DB2ConnStr:   os.Getenv("DB2_CONN_STRING"),
		PGConnStr:    os.Getenv("POSTGRES_CONN_STRING"),
	}

	if cfg.ClientID == "" {
		return nil, fmt.Errorf("CLIENT_ID environment variable is required")
	}
	if cfg.ClientSecret == "" {
		return nil, fmt.Errorf("CLIENT_SECRET environment variable is required")
	}
	if cfg.FQDN == "" {
		return nil, fmt.Errorf("FQDN environment variable is required")
	}

	switch cfg.DBType {
	case "db2":
		if cfg.DB2ConnStr == "" {
			return nil, fmt.Errorf("DB2_CONN_STRING environment variable is required when DB_TYPE=db2")
		}
	case "postgres":
		if cfg.PGConnStr == "" {
			return nil, fmt.Errorf("POSTGRES_CONN_STRING environment variable is required when DB_TYPE=postgres")
		}
	default:
		return nil, fmt.Errorf("unsupported DB_TYPE %q (supported: db2, postgres)", cfg.DBType)
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
