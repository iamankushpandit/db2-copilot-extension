package config

// AdminConfig holds admin UI and database connection reference settings.
type AdminConfig struct {
	AdminUI  AdminUIConfig      `json:"admin_ui"`
	Database AdminDBConfig      `json:"database"`
}

// AdminUIConfig controls the admin web interface.
type AdminUIConfig struct {
	Enabled             bool     `json:"enabled"`
	Path                string   `json:"path"`
	AllowedGithubUsers  []string `json:"allowed_github_users"`
	SessionTimeoutHours int      `json:"session_timeout_hours"`
}

// AdminDBConfig points at the database the admin UI should use.
type AdminDBConfig struct {
	Type                string `json:"type"`
	ConnectionStringEnv string `json:"connection_string_env"`
	ReadOnlyEnforce     bool   `json:"read_only_enforce"`
}

// defaultAdminConfig returns an AdminConfig populated with sensible defaults.
func defaultAdminConfig() *AdminConfig {
	return &AdminConfig{
		AdminUI: AdminUIConfig{
			Enabled:             true,
			Path:                "/admin",
			AllowedGithubUsers:  []string{},
			SessionTimeoutHours: 24,
		},
		Database: AdminDBConfig{
			Type:                "postgres",
			ConnectionStringEnv: "DATABASE_URL",
			ReadOnlyEnforce:     true,
		},
	}
}
