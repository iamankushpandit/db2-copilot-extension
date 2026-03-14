package config

// SafetyConfig holds all query safety, cost, rate-limiting, learning, audit,
// and health-check configuration.
type SafetyConfig struct {
	QueryLimits    QueryLimitsConfig    `json:"query_limits"`
	CostEstimation CostEstimationConfig `json:"cost_estimation"`
	SelfCorrection SelfCorrectionConfig `json:"self_correction"`
	Transparency   TransparencyConfig   `json:"transparency"`
	RateLimiting   RateLimitingConfig   `json:"rate_limiting"`
	Learning       LearningConfig       `json:"learning"`
	Audit          AuditConfig          `json:"audit"`
	Health         HealthConfig         `json:"health"`
}

// QueryLimitsConfig controls how many rows are fetched and how long queries may run.
type QueryLimitsConfig struct {
	MaxQueryRows          int  `json:"max_query_rows"`
	DisplayRows           int  `json:"display_rows"`
	ComputeSummaryStats   bool `json:"compute_summary_stats"`
	EnforceLimit          bool `json:"enforce_limit"`
	QueryTimeoutSeconds   int  `json:"query_timeout_seconds"`
}

// CostEstimationConfig controls pre-execution EXPLAIN and cost gating.
type CostEstimationConfig struct {
	ExplainBeforeExecute bool   `json:"explain_before_execute"`
	MaxEstimatedRows     int    `json:"max_estimated_rows"`
	MaxEstimatedCost     int    `json:"max_estimated_cost"`
	OnExceed             string `json:"on_exceed"` // e.g. "reject_and_suggest"
}

// SelfCorrectionConfig controls automatic SQL retry on error.
type SelfCorrectionConfig struct {
	Enabled                bool `json:"enabled"`
	MaxRetries             int  `json:"max_retries"`
	IncludeErrorContext     bool `json:"include_error_context"`
	IncludeAvailableColumns bool `json:"include_available_columns"`
}

// TransparencyConfig controls what information is surfaced to the user.
type TransparencyConfig struct {
	ShowSQLToUser          bool `json:"show_sql_to_user"`
	ShowRetriesToUser      bool `json:"show_retries_to_user"`
	ShowErrorsToUser       bool `json:"show_errors_to_user"`
	ShowStatusMessages     bool `json:"show_status_messages"`
	UseCollapsibleDetails  bool `json:"use_collapsible_details"`
}

// RateLimitingConfig controls per-user and global request rate limits.
type RateLimitingConfig struct {
	Enabled                      bool `json:"enabled"`
	RequestsPerMinutePerUser     int  `json:"requests_per_minute_per_user"`
	RequestsPerMinuteGlobal      int  `json:"requests_per_minute_global"`
}

// LearningConfig controls the learned-corrections feedback loop.
type LearningConfig struct {
	Enabled                 bool   `json:"enabled"`
	CorrectionsFile         string `json:"corrections_file"`
	MaxCorrections          int    `json:"max_corrections"`
	IncludeInPrompt         bool   `json:"include_in_prompt"`
	MaxCorrectionsInPrompt  int    `json:"max_corrections_in_prompt"`
	EvictionPolicy          string `json:"eviction_policy"` // e.g. "least_recently_matched"
}

// AuditConfig controls audit-log behaviour.
type AuditConfig struct {
	Enabled           bool   `json:"enabled"`
	Directory         string `json:"directory"`
	Rotation          string `json:"rotation"` // "daily", "weekly", etc.
	RetentionDays     int    `json:"retention_days"`
	LogUserQuestions  bool   `json:"log_user_questions"`
	LogGeneratedSQL   bool   `json:"log_generated_sql"`
	LogResults        bool   `json:"log_results"`
	LogRowCounts      bool   `json:"log_row_counts"`
	LogLatency        bool   `json:"log_latency"`
}

// HealthConfig controls background health checks.
type HealthConfig struct {
	CheckIntervalSeconds int  `json:"check_interval_seconds"`
	DatabasePing         bool `json:"database_ping"`
	OllamaPing           bool `json:"ollama_ping"`
	SchemaMaxAgeHours    int  `json:"schema_max_age_hours"`
	SchemaAutoRefresh    bool `json:"schema_auto_refresh"`
}

// defaultSafetyConfig returns a SafetyConfig populated with sensible defaults.
func defaultSafetyConfig() *SafetyConfig {
	return &SafetyConfig{
		QueryLimits: QueryLimitsConfig{
			MaxQueryRows:        100,
			DisplayRows:         10,
			ComputeSummaryStats: true,
			EnforceLimit:        true,
			QueryTimeoutSeconds: 30,
		},
		CostEstimation: CostEstimationConfig{
			ExplainBeforeExecute: true,
			MaxEstimatedRows:     100000,
			MaxEstimatedCost:     50000,
			OnExceed:             "reject_and_suggest",
		},
		SelfCorrection: SelfCorrectionConfig{
			Enabled:                 true,
			MaxRetries:              1,
			IncludeErrorContext:     true,
			IncludeAvailableColumns: true,
		},
		Transparency: TransparencyConfig{
			ShowSQLToUser:         true,
			ShowRetriesToUser:     true,
			ShowErrorsToUser:      true,
			ShowStatusMessages:    true,
			UseCollapsibleDetails: true,
		},
		RateLimiting: RateLimitingConfig{
			Enabled:                  true,
			RequestsPerMinutePerUser: 10,
			RequestsPerMinuteGlobal:  50,
		},
		Learning: LearningConfig{
			Enabled:                true,
			CorrectionsFile:        "data/learned_corrections.jsonl",
			MaxCorrections:         100,
			IncludeInPrompt:        true,
			MaxCorrectionsInPrompt: 20,
			EvictionPolicy:         "least_recently_matched",
		},
		Audit: AuditConfig{
			Enabled:          true,
			Directory:        "data/audit",
			Rotation:         "daily",
			RetentionDays:    90,
			LogUserQuestions: true,
			LogGeneratedSQL:  true,
			LogResults:       false,
			LogRowCounts:     true,
			LogLatency:       true,
		},
		Health: HealthConfig{
			CheckIntervalSeconds: 60,
			DatabasePing:         true,
			OllamaPing:           true,
			SchemaMaxAgeHours:    6,
			SchemaAutoRefresh:    true,
		},
	}
}
