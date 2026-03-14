package config

// AdminConfig holds admin UI and database connection settings.
type AdminConfig struct {
	AdminUI  AdminUIConfig  `json:"admin_ui"`
	Database DatabaseConfig `json:"database"`
}

// AdminUIConfig controls access to the admin interface.
type AdminUIConfig struct {
	Enabled             bool     `json:"enabled"`
	Path                string   `json:"path"`
	AllowedGithubUsers  []string `json:"allowed_github_users"`
	SessionTimeoutHours int      `json:"session_timeout_hours"`
}

// DatabaseConfig holds database connection settings.
type DatabaseConfig struct {
	Type                string `json:"type"`
	ConnectionStringEnv string `json:"connection_string_env"`
	ReadOnlyEnforce     bool   `json:"read_only_enforce"`
}

// DBType returns the configured database type.
func (a *AdminConfig) DBType() string {
	return a.Database.Type
}

// IsAdminUser returns whether the given GitHub username is in the allowed list.
func (a *AdminConfig) IsAdminUser(username string) bool {
	for _, u := range a.AdminUI.AllowedGithubUsers {
		if u == username {
			return true
		}
	}
	return false
}

// DefaultAdminConfig returns an AdminConfig populated with sensible defaults.
func DefaultAdminConfig() *AdminConfig {
	return &AdminConfig{
		AdminUI: AdminUIConfig{
			Enabled:             true,
			Path:                "/admin",
			AllowedGithubUsers:  []string{},
			SessionTimeoutHours: 24,
		},
		Database: DatabaseConfig{
			Type:                "postgres",
			ConnectionStringEnv: "DATABASE_URL",
			ReadOnlyEnforce:     true,
		},
	}
}
