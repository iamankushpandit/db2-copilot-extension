package config

type LLMConfig struct {
	SQLGenerator LLMProvider `json:"sql_generator"`
	Explainer    LLMProvider `json:"explainer"`
	Fallback     Fallback    `json:"fallback"`
}

type LLMProvider struct {
	Provider string      `json:"provider"`
	Ollama   Ollama      `json:"ollama"`
	Copilot  Copilot     `json:"copilot"`
}

type Ollama struct {
	URL            string  `json:"url"`
	Model          string  `json:"model"`
	TimeoutSeconds int     `json:"timeout_seconds"`
	Temperature    float64 `json:"temperature"`
	AutoPull       bool    `json:"auto_pull"`
}

type Copilot struct {
	Model string `json:"model"`
}

type Fallback struct {
	Enabled              bool   `json:"enabled"`
	SQLGeneratorFallback string `json:"sql_generator_fallback"`
	ExplainerFallback    string `json:"explainer_fallback"`
}
