package config

import "time"

// AccessConfig controls which schemas, tables, and columns the agent can query.
type AccessConfig struct {
	Version        string          `json:"version"`
	LastModifiedBy string          `json:"last_modified_by"`
	LastModifiedAt time.Time       `json:"last_modified_at"`
	ApprovedSchemas []ApprovedSchema `json:"approved_schemas"`
	HiddenSchemas  []HiddenSchema  `json:"hidden_schemas"`
}

// AccessLevel defines how much of a schema or table is approved.
type AccessLevel string

const (
	// AccessLevelFull means all tables/columns are approved, including future ones.
	AccessLevelFull AccessLevel = "full"
	// AccessLevelPartial means only explicitly listed tables/columns are approved.
	AccessLevelPartial AccessLevel = "partial"
)

// ApprovedSchema defines a schema and the tables within it that are approved.
type ApprovedSchema struct {
	Schema         string          `json:"schema"`
	ApprovedBy     string          `json:"approved_by"`
	ApprovedAt     time.Time       `json:"approved_at"`
	Reason         string          `json:"reason"`
	AccessLevel    AccessLevel     `json:"access_level"`
	ApprovedTables []ApprovedTable `json:"approved_tables,omitempty"`
}

// ApprovedTable defines a table and the columns within it that are approved.
type ApprovedTable struct {
	Table           string      `json:"table"`
	ApprovedBy      string      `json:"approved_by"`
	ApprovedAt      time.Time   `json:"approved_at"`
	Reason          string      `json:"reason"`
	AccessLevel     AccessLevel `json:"access_level"`
	ApprovedColumns []string    `json:"approved_columns,omitempty"`
	DeniedColumns   []string    `json:"denied_columns,omitempty"`
	DeniedReason    string      `json:"denied_reason,omitempty"`
}

// HiddenSchema defines a schema that should never be mentioned to users.
type HiddenSchema struct {
	Schema   string    `json:"schema"`
	Reason   string    `json:"reason"`
	HiddenBy string    `json:"hidden_by"`
	HiddenAt time.Time `json:"hidden_at"`
}

// IsTableApproved returns whether the given schema.table combination is
// accessible to the LLM. It also returns whether the schema/table is hidden.
func (a *AccessConfig) IsTableApproved(schema, table string) (approved, hidden bool) {
	// Check hidden schemas first.
	for _, h := range a.HiddenSchemas {
		if h.Schema == schema {
			return false, true
		}
	}

	for _, s := range a.ApprovedSchemas {
		if s.Schema != schema {
			continue
		}
		if s.AccessLevel == AccessLevelFull {
			return true, false
		}
		for _, t := range s.ApprovedTables {
			if t.Table == table {
				return true, false
			}
		}
		return false, false
	}
	return false, false
}

// IsColumnApproved returns whether the given column in schema.table is
// accessible to the LLM.
func (a *AccessConfig) IsColumnApproved(schema, table, column string) bool {
	for _, s := range a.ApprovedSchemas {
		if s.Schema != schema {
			continue
		}
		if s.AccessLevel == AccessLevelFull {
			return true
		}
		for _, t := range s.ApprovedTables {
			if t.Table != table {
				continue
			}
			if t.AccessLevel == AccessLevelFull {
				return true
			}
			// Check denied columns first.
			for _, dc := range t.DeniedColumns {
				if dc == column {
					return false
				}
			}
			// Check approved columns.
			for _, ac := range t.ApprovedColumns {
				if ac == column {
					return true
				}
			}
			return false
		}
		return false
	}
	return false
}
