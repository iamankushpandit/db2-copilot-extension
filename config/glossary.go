package config

import "time"

// GlossaryConfig holds business term definitions for the LLM prompt.
type GlossaryConfig struct {
	Terms []GlossaryTerm `json:"terms"`
}

// GlossaryTerm defines a business term and its technical meaning.
type GlossaryTerm struct {
	Term       string    `json:"term"`
	Definition string    `json:"definition"`
	AddedBy    string    `json:"added_by"`
	AddedAt    time.Time `json:"added_at"`
}
