package config

import (
	"fmt"
	"os"
)

// Config holds the minimal set of bootstrap configuration values that must be
// present as environment variables before the JSON config files can be loaded.
type Config struct {
	Port         string
	ClientID     string
	ClientSecret string
	FQDN         string
	// DB2ConnStr is the IBM DB2 ODBC connection string (optional when using PostgreSQL).
	DB2ConnStr string
	// DatabaseURL is the PostgreSQL connection string (optional when using DB2).
	DatabaseURL string
	// ConfigDir is the directory that contains the JSON config files (default: "config").
	ConfigDir string
}

// New reads configuration from environment variables and validates that all
// required fields are present.
func New() (*Config, error) {
	cfg := &Config{
		Port:         getEnvOrDefault("PORT", "8080"),
		ClientID:     os.Getenv("CLIENT_ID"),
		ClientSecret: os.Getenv("CLIENT_SECRET"),
		FQDN:         os.Getenv("FQDN"),
		DB2ConnStr:   os.Getenv("DB2_CONN_STRING"),
		DatabaseURL:  os.Getenv("DATABASE_URL"),
		ConfigDir:    getEnvOrDefault("CONFIG_DIR", "config"),
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
	if cfg.DB2ConnStr == "" && cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("either DB2_CONN_STRING or DATABASE_URL environment variable is required")
	}

	return cfg, nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
