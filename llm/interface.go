package llm

import (
	"context"
	"io"
)

// SQLGenerationRequest is the input to the TextToSQLProvider.
type SQLGenerationRequest struct {
	// Question is the user's natural language question.
	Question string
	// SchemaContext is the Tier 2 approved schema text.
	SchemaContext string
	// Glossary terms to include in the prompt.
	GlossaryTerms []GlossaryTerm
	// LearnedCorrections are recent corrections to include in the prompt.
	LearnedCorrections []LearnedCorrection
	// DBType is "postgres" or "db2".
	DBType string
	// PreviousError is set on retry attempts.
	PreviousError string
	// PreviousSQL is set on retry attempts.
	PreviousSQL string
	// Attempt number (1 = first attempt).
	Attempt int
	// The Copilot API token (for CopilotProvider).
	CopilotToken string
	// The Copilot integration ID (for CopilotProvider).
	CopilotIntegrationID string
}

// GlossaryTerm is a business term and its definition for prompt injection.
type GlossaryTerm struct {
	Term       string
	Definition string
}

// LearnedCorrection is a past correction for prompt injection.
type LearnedCorrection struct {
	OriginalQuestion string
	FailedSQL        string
	Error            string
	CorrectedSQL     string
}

// ExplanationRequest is the input to the ExplanationProvider.
type ExplanationRequest struct {
	// Question is the original user question.
	Question string
	// SQL is the query that was executed.
	SQL string
	// DisplayRows is the subset of rows shown to the user.
	DisplayRows []map[string]interface{}
	// TotalRows is the full count of rows returned.
	TotalRows int
	// SummaryStats contains statistics over all rows.
	SummaryStats string
	// ResultShape describes the result shape (empty/scalar/single/small/large).
	ResultShape string
	// Attempt is how many SQL generation attempts were made.
	Attempt int
	// PreviousSQL is set when there was a retry (for transparency).
	PreviousSQL string
	// PreviousError is the error from the failed attempt.
	PreviousError string
	// ShowSQL controls whether SQL is included in the response.
	ShowSQL bool
	// UseCollapsibleDetails wraps SQL in a <details> block.
	UseCollapsibleDetails bool
	// The Copilot API token (for CopilotProvider).
	CopilotToken string
	// The Copilot integration ID (for CopilotProvider).
	CopilotIntegrationID string
}

// TextToSQLProvider generates SQL from natural language.
type TextToSQLProvider interface {
	// GenerateSQL generates a SQL query from the request.
	// It returns the raw SQL string (without tags).
	GenerateSQL(ctx context.Context, req SQLGenerationRequest) (string, error)
	// Available returns true if the provider is ready to serve requests.
	Available(ctx context.Context) bool
	// Name returns a human-readable provider identifier.
	Name() string
}

// ExplanationProvider explains query results in natural language.
type ExplanationProvider interface {
	// ExplainResults streams a natural language explanation to w.
	ExplainResults(ctx context.Context, req ExplanationRequest, w io.Writer) error
	// Available returns true if the provider is ready to serve requests.
	Available(ctx context.Context) bool
	// Name returns a human-readable provider identifier.
	Name() string
}
