package config

type QuerySafety struct {
	QueryLimits    QueryLimits    `json:"query_limits"`
	CostEstimation CostEstimation `json:"cost_estimation"`
	SelfCorrection SelfCorrection `json:"self_correction"`
	Transparency   Transparency   `json:"transparency"`
	RateLimiting   RateLimiting   `json:"rate_limiting"`
	Learning       Learning       `json:"learning"`
	Audit          Audit          `json:"audit"`
	Health         Health         `json:"health"`
}

type QueryLimits struct {
	MaxQueryRows        int  `json:"max_query_rows"`
	DisplayRows         int  `json:"display_rows"`
	ComputeSummaryStats bool `json:"compute_summary_stats"`
	EnforceLimit        bool `json:"enforce_limit"`
	QueryTimeoutSeconds int  `json:"query_timeout_seconds"`
}

type CostEstimation struct {
	ExplainBeforeExecute bool   `json:"explain_before_execute"`
	MaxEstimatedRows     int    `json:"max_estimated_rows"`
	MaxEstimatedCost     int    `json:"max_estimated_cost"`
	OnExceed             string `json:"on_exceed"`
}

type SelfCorrection struct {
	Enabled                 bool `json:"enabled"`
	MaxRetries              int  `json:"max_retries"`
	IncludeErrorContext     bool `json:"include_error_context"`
	IncludeAvailableColumns bool `json:"include_available_columns"`
}

type Transparency struct {
	ShowSQLToUser        bool `json:"show_sql_to_user"`
	ShowRetriesToUser    bool `json:"show_retries_to_user"`
	ShowErrorsToUser     bool `json:"show_errors_to_user"`
	ShowStatusMessages   bool `json:"show_status_messages"`
	UseCollapsibleDetails bool `json:"use_collapsible_details"`
}

type RateLimiting struct {
	Enabled                   bool `json:"enabled"`
	RequestsPerMinutePerUser  int  `json:"requests_per_minute_per_user"`
	RequestsPerMinuteGlobal   int  `json:"requests_per_minute_global"`
}

type Learning struct {
	Enabled                 bool   `json:"enabled"`
	CorrectionsFile         string `json:"corrections_file"`
	MaxCorrections          int    `json:"max_corrections"`
	IncludeInPrompt         bool   `json:"include_in_prompt"`
	MaxCorrectionsInPrompt  int    `json:"max_corrections_in_prompt"`
	EvictionPolicy          string `json:"eviction_policy"`
}

type Audit struct {
	Enabled           bool   `json:"enabled"`
	Directory         string `json:"directory"`
	Rotation          string `json:"rotation"`
	RetentionDays     int    `json:"retention_days"`
	LogUserQuestions  bool   `json:"log_user_questions"`
	LogGeneratedSQL   bool   `json:"log_generated_sql"`
	LogResults        bool   `json:"log_results"`
	LogLatency        bool   `json:"log_latency"`
	LogRowCounts      bool   `json:"log_row_counts"`
}

type Health struct {
	CheckIntervalSeconds int  `json:"check_interval_seconds"`
	DatabasePing         bool `json:"database_ping"`
	OllamaPing           bool `json:"ollama_ping"`
	SchemaMaxAgeHours    int  `json:"schema_max_age_hours"`
	SchemaAutoRefresh    bool `json:"schema_auto_refresh"`
}
