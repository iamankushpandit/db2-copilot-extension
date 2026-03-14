package audit

import "time"

// EventType identifies the kind of audit event.
type EventType string

const (
	EventSystemStart         EventType = "SYSTEM_START"
	EventDBConnectionOK      EventType = "DB_CONNECTION_OK"
	EventDBConnectionFailed  EventType = "DB_CONNECTION_FAILED"
	EventReadOnlyVerified    EventType = "READ_ONLY_VERIFIED"
	EventReadOnlyWarning     EventType = "READ_ONLY_WARNING"
	EventSchemaCrawlComplete EventType = "SCHEMA_CRAWL_COMPLETE"
	EventConfigLoaded        EventType = "CONFIG_LOADED"
	EventConfigReloaded      EventType = "CONFIG_RELOADED"
	EventConfigReloadFailed  EventType = "CONFIG_RELOAD_FAILED"
	EventCorrectionsLoaded   EventType = "CORRECTIONS_LOADED"
	EventQueryStart          EventType = "QUERY_START"
	EventSQLGenerated        EventType = "SQL_GENERATED"
	EventSQLValidated        EventType = "SQL_VALIDATED"
	EventSQLValidationFailed EventType = "SQL_VALIDATION_FAILED"
	EventExplainCheck        EventType = "EXPLAIN_CHECK"
	EventQueryExecuted       EventType = "QUERY_EXECUTED"
	EventQueryFailed         EventType = "QUERY_FAILED"
	EventQueryBlocked        EventType = "QUERY_BLOCKED"
	EventQueryCostExceeded   EventType = "QUERY_COST_EXCEEDED"
	EventQueryComplete       EventType = "QUERY_COMPLETE"
	EventCorrectionLearned   EventType = "CORRECTION_LEARNED"
	EventLLMFallback         EventType = "LLM_FALLBACK"
	EventRateLimited         EventType = "RATE_LIMITED"
	EventRecommendationMade  EventType = "RECOMMENDATION_MADE"
	EventHealthCheck         EventType = "HEALTH_CHECK"
)

// Event is the base structure included in every audit log entry.
type Event struct {
	Timestamp  time.Time `json:"timestamp"`
	Type       EventType `json:"type"`
	GitHubUser string    `json:"github_user,omitempty"`
	GitHubUID  string    `json:"github_uid,omitempty"`
	Details    any       `json:"details,omitempty"`
}

// SystemStartDetails carries details for SYSTEM_START events.
type SystemStartDetails struct {
	Version     string `json:"version"`
	DBType      string `json:"db_type"`
	LLMProvider string `json:"llm_provider"`
	Port        string `json:"port"`
}

// DBConnectionDetails carries details for DB_CONNECTION_* events.
type DBConnectionDetails struct {
	DBType string `json:"db_type"`
	Error  string `json:"error,omitempty"`
}

// ReadOnlyDetails carries details for READ_ONLY_* events.
type ReadOnlyDetails struct {
	Verified   bool   `json:"verified"`
	Warning    string `json:"warning,omitempty"`
	Disclaimer string `json:"disclaimer,omitempty"`
}

// SchemaCrawlDetails carries details for SCHEMA_CRAWL_COMPLETE events.
type SchemaCrawlDetails struct {
	SchemaCount int   `json:"schema_count"`
	TableCount  int   `json:"table_count"`
	ColumnCount int   `json:"column_count"`
	DurationMS  int64 `json:"duration_ms"`
}

// QueryStartDetails carries details for QUERY_START events.
type QueryStartDetails struct {
	Question string `json:"question"`
}

// SQLGeneratedDetails carries details for SQL_GENERATED events.
type SQLGeneratedDetails struct {
	Attempt   int    `json:"attempt"`
	SQL       string `json:"sql"`
	Model     string `json:"model"`
	LatencyMS int64  `json:"latency_ms"`
}

// SQLValidatedDetails carries details for SQL_VALIDATED/SQL_VALIDATION_FAILED events.
type SQLValidatedDetails struct {
	TablesReferenced []string `json:"tables_referenced"`
	ApprovalStatus   string   `json:"approval_status"`
	Reason           string   `json:"reason,omitempty"`
}

// ExplainCheckDetails carries details for EXPLAIN_CHECK events.
type ExplainCheckDetails struct {
	EstimatedRows int64   `json:"estimated_rows"`
	EstimatedCost float64 `json:"estimated_cost"`
	WithinLimits  bool    `json:"within_limits"`
}

// QueryExecutedDetails carries details for QUERY_EXECUTED events.
type QueryExecutedDetails struct {
	RowsReturned  int   `json:"rows_returned"`
	RowsDisplayed int   `json:"rows_displayed"`
	LatencyMS     int64 `json:"latency_ms"`
}

// QueryFailedDetails carries details for QUERY_FAILED events.
type QueryFailedDetails struct {
	Error     string `json:"error"`
	WillRetry bool   `json:"will_retry"`
}

// QueryBlockedDetails carries details for QUERY_BLOCKED events.
type QueryBlockedDetails struct {
	Reason         string `json:"reason"`
	Recommendation string `json:"recommendation,omitempty"`
}

// QueryCostExceededDetails carries details for QUERY_COST_EXCEEDED events.
type QueryCostExceededDetails struct {
	EstimatedRows int64   `json:"estimated_rows"`
	EstimatedCost float64 `json:"estimated_cost"`
	MaxRows       int64   `json:"max_rows"`
	MaxCost       float64 `json:"max_cost"`
	WillRetry     bool    `json:"will_retry"`
}

// QueryCompleteDetails carries details for QUERY_COMPLETE events.
type QueryCompleteDetails struct {
	TotalLatencyMS int64  `json:"total_latency_ms"`
	RetryCount     int    `json:"retry_count"`
	Status         string `json:"status"`
}

// CorrectionLearnedDetails carries details for CORRECTION_LEARNED events.
type CorrectionLearnedDetails struct {
	OriginalSQL    string `json:"original_sql"`
	CorrectedSQL   string `json:"corrected_sql"`
	CorrectionType string `json:"correction_type"`
}

// LLMFallbackDetails carries details for LLM_FALLBACK events.
type LLMFallbackDetails struct {
	FailedProvider   string `json:"failed_provider"`
	FallbackProvider string `json:"fallback_provider"`
	Reason           string `json:"reason"`
}

// RateLimitedDetails carries details for RATE_LIMITED events.
type RateLimitedDetails struct {
	User         string `json:"user"`
	RequestCount int    `json:"request_count"`
	Limit        int    `json:"limit"`
}

// RecommendationDetails carries details for RECOMMENDATION_MADE events.
type RecommendationDetails struct {
	Schema     string `json:"schema"`
	Table      string `json:"table"`
	ReasonCode string `json:"reason_code"`
}

// HealthCheckDetails carries details for HEALTH_CHECK events.
type HealthCheckDetails struct {
	Database string `json:"database"`
	LLM      string `json:"llm"`
	Schema   string `json:"schema"`
}
