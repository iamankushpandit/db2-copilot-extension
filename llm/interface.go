package llm

import (
	"context"
	"io"

	"github.com/iamankushpandit/db2-copilot-extension/database"
)

type SQLGenerationRequest struct {
	Prompt   string
	Schema   *database.Schema
	Glossary string // TODO: use struct
}

type ExplanationRequest struct {
	Question        string
	SQL             string
	Results         []map[string]interface{}
	SummaryStatistics string // TODO: use struct
}

// TextToSQLProvider generates SQL from natural language
type TextToSQLProvider interface {
	GenerateSQL(ctx context.Context, req SQLGenerationRequest) (string, error)
	Available() bool  // health check
	Name() string     // "ollama/sqlcoder:7b" or "copilot/gpt-4o"
}

// ExplanationProvider explains query results in natural language
type ExplanationProvider interface {
	ExplainResults(ctx context.Context, req ExplanationRequest, w io.Writer) error
	Available() bool
	Name() string
}
