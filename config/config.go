package config

import (
	"fmt"
	"os"
)

// Config holds all configuration values for the DB2 Copilot Extension.
type Config struct {
	Port         string
	ClientID     string
	ClientSecret string
	FQDN         string
	DB2ConnStr   string
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
	if cfg.DB2ConnStr == "" {
		return nil, fmt.Errorf("DB2_CONN_STRING environment variable is required")
	}

	return cfg, nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
