package config

// GlossaryConfig holds business-term definitions injected into LLM prompts.
type GlossaryConfig struct {
	Terms []GlossaryTerm `json:"terms"`
}

// GlossaryTerm maps a business term to its technical definition.
type GlossaryTerm struct {
	Term       string `json:"term"`
	Definition string `json:"definition"`
}

// defaultGlossaryConfig returns an empty GlossaryConfig.
func defaultGlossaryConfig() *GlossaryConfig {
	return &GlossaryConfig{
		Terms: []GlossaryTerm{},
	}
}
