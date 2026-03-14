package config

import "time"

type AccessConfig struct {
	Version         string          `json:"version"`
	LastModifiedBy  string          `json:"last_modified_by"`
	LastModifiedAt  time.Time       `json:"last_modified_at"`
	ApprovedSchemas []ApprovedSchema `json:"approved_schemas"`
	HiddenSchemas   []HiddenSchema  `json:"hidden_schemas"`
}

type ApprovedSchema struct {
	Schema         string          `json:"schema"`
	ApprovedBy     string          `json:"approved_by"`
	ApprovedAt     time.Time       `json:"approved_at"`
	Reason         string          `json:"reason"`
	AccessLevel    string          `json:"access_level"`
	ApprovedTables []ApprovedTable `json:"approved_tables"`
}

type ApprovedTable struct {
	Table           string    `json:"table"`
	ApprovedBy      string    `json:"approved_by"`
	ApprovedAt      time.Time `json:"approved_at"`
	Reason          string    `json:"reason"`
	AccessLevel     string    `json:"access_level"`
	ApprovedColumns []string  `json:"approved_columns"`
	DeniedColumns   []string  `json:"denied_columns"`
	DeniedReason    string    `json:"denied_reason"`
}

type HiddenSchema struct {
	Schema    string    `json:"schema"`
	Reason    string    `json:"reason"`
	HiddenBy  string    `json:"hidden_by"`
	HiddenAt  time.Time `json:"hidden_at"`
}
