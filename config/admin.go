package config

type AdminConfig struct {
	AdminUI  AdminUI  `json:"admin_ui"`
	Database Database `json:"database"`
	OAuth    OAuth    `json:"oauth"`
}

type AdminUI struct {
	Enabled             bool     `json:"enabled"`
	Path                string   `json:"path"`
	AllowedGithubUsers  []string `json:"allowed_github_users"`
	SessionTimeoutHours int      `json:"session_timeout_hours"`
	Port                int      `json:"port"`
}

type Database struct {
	Type                string `json:"type"`
	ConnectionStringEnv string `json:"connection_string_env"`
	ReadOnlyEnforce     bool   `json:"read_only_enforce"`
}

type OAuth struct {
	ClientIDEnv     string `json:"client_id_env"`
	ClientSecretEnv string `json:"client_secret_env"`
	FQDNEnv         string `json:"fqdn_env"`
}
