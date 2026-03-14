package config

// LLMConfig holds configuration for all LLM providers used by the agent.
type LLMConfig struct {
	SQLGenerator LLMRoleConfig  `json:"sql_generator"`
	Explainer    LLMRoleConfig  `json:"explainer"`
	Fallback     FallbackConfig `json:"fallback"`
}

// LLMRoleConfig configures a specific agent role (SQL generation or explanation).
type LLMRoleConfig struct {
	Provider string        `json:"provider"` // "copilot" or "ollama"
	Ollama   OllamaConfig  `json:"ollama"`
	Copilot  CopilotConfig `json:"copilot"`
}

// OllamaConfig holds settings for a locally-running Ollama instance.
type OllamaConfig struct {
	URL            string  `json:"url"`
	Model          string  `json:"model"`
	TimeoutSeconds int     `json:"timeout_seconds"`
	Temperature    float64 `json:"temperature"`
	AutoPull       bool    `json:"auto_pull"`
}

// CopilotConfig holds settings for the GitHub Copilot LLM provider.
type CopilotConfig struct {
	Model string `json:"model"`
}

// FallbackConfig controls automatic provider fallback behaviour.
type FallbackConfig struct {
	Enabled              bool   `json:"enabled"`
	SQLGeneratorFallback string `json:"sql_generator_fallback"`
	ExplainerFallback    string `json:"explainer_fallback"`
}

// defaultLLMConfig returns an LLMConfig populated with sensible defaults.
func defaultLLMConfig() *LLMConfig {
	return &LLMConfig{
		SQLGenerator: LLMRoleConfig{
			Provider: "copilot",
			Ollama: OllamaConfig{
				URL:            "http://localhost:11434",
				Model:          "sqlcoder:7b",
				TimeoutSeconds: 15,
				Temperature:    0.0,
				AutoPull:       true,
			},
			Copilot: CopilotConfig{
				Model: "gpt-4o",
			},
		},
		Explainer: LLMRoleConfig{
			Provider: "copilot",
			Ollama: OllamaConfig{
				URL:            "http://localhost:11434",
				Model:          "llama3.1:8b",
				TimeoutSeconds: 30,
				Temperature:    0.3,
				AutoPull:       true,
			},
			Copilot: CopilotConfig{
				Model: "gpt-4o",
			},
		},
		Fallback: FallbackConfig{
			Enabled:              true,
			SQLGeneratorFallback: "copilot",
			ExplainerFallback:    "copilot",
		},
	}
}
