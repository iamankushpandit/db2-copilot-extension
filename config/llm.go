package config

// LLMConfig defines which LLM providers are used for SQL generation and explanation.
type LLMConfig struct {
	SQLGenerator SQLGeneratorConfig `json:"sql_generator"`
	Explainer    ExplainerConfig    `json:"explainer"`
	Fallback     FallbackConfig     `json:"fallback"`
}

// ProviderName identifies an LLM provider.
type ProviderName string

const (
	// ProviderOllama uses a local Ollama server.
	ProviderOllama ProviderName = "ollama"
	// ProviderCopilot uses the GitHub Copilot API.
	ProviderCopilot ProviderName = "copilot"
	// ProviderOpenAICompat uses an OpenAI-compatible API.
	ProviderOpenAICompat ProviderName = "openai_compat"
)

// SQLGeneratorConfig specifies which provider generates SQL from natural language.
type SQLGeneratorConfig struct {
	Provider     ProviderName            `json:"provider"`
	Ollama       OllamaProviderConfig    `json:"ollama"`
	Copilot      CopilotProviderConfig   `json:"copilot"`
	OpenAICompat OpenAICompatConfig      `json:"openai_compat"`
}

// ExplainerConfig specifies which provider explains query results.
type ExplainerConfig struct {
	Provider     ProviderName            `json:"provider"`
	Ollama       OllamaProviderConfig    `json:"ollama"`
	Copilot      CopilotProviderConfig   `json:"copilot"`
	OpenAICompat OpenAICompatConfig      `json:"openai_compat"`
}

// OllamaProviderConfig holds connection settings for an Ollama server.
type OllamaProviderConfig struct {
	URL            string  `json:"url"`
	Model          string  `json:"model"`
	TimeoutSeconds int     `json:"timeout_seconds"`
	Temperature    float64 `json:"temperature"`
	AutoPull       bool    `json:"auto_pull"`
}

// CopilotProviderConfig holds settings for the GitHub Copilot API.
type CopilotProviderConfig struct {
	Model string `json:"model"`
}

// OpenAICompatConfig holds connection settings for an OpenAI-compatible API server
// such as vLLM, LocalAI, LM Studio, or any service that speaks the OpenAI format.
// The default URL (http://localhost:8000/v1) matches vLLM's default bind address.
// The default SQL generator model ("sqlcoder") matches the Defog/sqlcoder family of
// code models commonly deployed on vLLM for text-to-SQL tasks.
type OpenAICompatConfig struct {
	// URL is the base URL of the OpenAI-compatible server, e.g. "http://localhost:8000/v1".
	URL string `json:"url"`
	// APIKeyEnv is the name of the environment variable that holds the API key.
	// Leave empty for servers that require no key.
	APIKeyEnv string `json:"api_key_env"`
	// Model is the model identifier to send in requests.
	Model string `json:"model"`
	// TimeoutSeconds is the per-request HTTP timeout.
	TimeoutSeconds int `json:"timeout_seconds"`
	// Temperature controls generation randomness (0.0 = deterministic).
	Temperature float64 `json:"temperature"`
}

// FallbackConfig defines fallback providers when the primary is unavailable.
type FallbackConfig struct {
	Enabled              bool         `json:"enabled"`
	SQLGeneratorFallback ProviderName `json:"sql_generator_fallback"`
	ExplainerFallback    ProviderName `json:"explainer_fallback"`
}

// DefaultLLMConfig returns an LLMConfig populated with sensible defaults.
func DefaultLLMConfig() *LLMConfig {
	return &LLMConfig{
		SQLGenerator: SQLGeneratorConfig{
			Provider: ProviderCopilot,
			Ollama: OllamaProviderConfig{
				URL:            "http://localhost:11434",
				Model:          "sqlcoder:7b",
				TimeoutSeconds: 15,
				Temperature:    0.0,
				AutoPull:       true,
			},
			Copilot: CopilotProviderConfig{
				Model: "gpt-4o",
			},
			OpenAICompat: OpenAICompatConfig{
				URL:            "http://localhost:8000/v1",
				APIKeyEnv:      "OPENAI_COMPAT_API_KEY",
				Model:          "sqlcoder",
				TimeoutSeconds: 15,
				Temperature:    0.0,
			},
		},
		Explainer: ExplainerConfig{
			Provider: ProviderCopilot,
			Ollama: OllamaProviderConfig{
				URL:            "http://localhost:11434",
				Model:          "llama3.1:8b",
				TimeoutSeconds: 30,
				Temperature:    0.3,
			},
			Copilot: CopilotProviderConfig{
				Model: "gpt-4o",
			},
			OpenAICompat: OpenAICompatConfig{
				URL:            "http://localhost:8000/v1",
				APIKeyEnv:      "OPENAI_COMPAT_API_KEY",
				Model:          "llama3",
				TimeoutSeconds: 30,
				Temperature:    0.3,
			},
		},
		Fallback: FallbackConfig{
			Enabled:              true,
			SQLGeneratorFallback: ProviderCopilot,
			ExplainerFallback:    ProviderCopilot,
		},
	}
}
