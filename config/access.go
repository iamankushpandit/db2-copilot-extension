package config

import "strings"

// AccessConfig controls which schemas and tables the agent is allowed to query.
type AccessConfig struct {
	Version        string          `json:"version"`
	LastModifiedBy string          `json:"last_modified_by"`
	LastModifiedAt string          `json:"last_modified_at"`
	ApprovedSchemas []ApprovedSchema `json:"approved_schemas"`
	HiddenSchemas   []HiddenSchema   `json:"hidden_schemas"`
}

// ApprovedSchema represents a schema approved for access.
type ApprovedSchema struct {
	Schema         string          `json:"schema"`
	ApprovedBy     string          `json:"approved_by"`
	ApprovedAt     string          `json:"approved_at"`
	Reason         string          `json:"reason"`
	AccessLevel    string          `json:"access_level"` // "full" or "partial"
	ApprovedTables []ApprovedTable `json:"approved_tables"`
}

// ApprovedTable represents a table approved for access within a schema.
type ApprovedTable struct {
	Table          string   `json:"table"`
	ApprovedBy     string   `json:"approved_by"`
	ApprovedAt     string   `json:"approved_at"`
	Reason         string   `json:"reason"`
	AccessLevel    string   `json:"access_level"` // "full" or "partial"
	ApprovedColumns []string `json:"approved_columns"`
	DeniedColumns  []string `json:"denied_columns"`
	DeniedReason   string   `json:"denied_reason"`
}

// HiddenSchema represents a schema that should be hidden from users.
type HiddenSchema struct {
	Schema   string `json:"schema"`
	Reason   string `json:"reason"`
	HiddenBy string `json:"hidden_by"`
	HiddenAt string `json:"hidden_at"`
}

// IsSchemaApproved returns true if the given schema has been approved for access.
func (c *AccessConfig) IsSchemaApproved(schema string) bool {
	return c.GetApprovedSchema(schema) != nil
}

// IsSchemaHidden returns true if the given schema is in the hidden list.
func (c *AccessConfig) IsSchemaHidden(schema string) bool {
	for i := range c.HiddenSchemas {
		if strings.EqualFold(c.HiddenSchemas[i].Schema, schema) {
			return true
		}
	}
	return false
}

// IsTableApproved returns true if the given table within the schema is approved.
// For schemas with access_level "full", all tables are considered approved.
func (c *AccessConfig) IsTableApproved(schema, table string) bool {
	return c.GetApprovedTable(schema, table) != nil
}

// IsColumnApproved returns true if the given column is accessible on the table.
// A column is approved when:
//   - The table has access_level "full" and the column is not in denied_columns, or
//   - The table has access_level "partial" and the column is in approved_columns.
func (c *AccessConfig) IsColumnApproved(schema, table, column string) bool {
	t := c.GetApprovedTable(schema, table)
	if t == nil {
		return false
	}
	if t.AccessLevel == "full" {
		for _, denied := range t.DeniedColumns {
			if strings.EqualFold(denied, column) {
				return false
			}
		}
		return true
	}
	// partial access — column must be explicitly approved
	for _, approved := range t.ApprovedColumns {
		if strings.EqualFold(approved, column) {
			return true
		}
	}
	return false
}

// GetApprovedSchema returns the ApprovedSchema for the given schema, or nil if not found.
func (c *AccessConfig) GetApprovedSchema(schema string) *ApprovedSchema {
	for i := range c.ApprovedSchemas {
		if strings.EqualFold(c.ApprovedSchemas[i].Schema, schema) {
			return &c.ApprovedSchemas[i]
		}
	}
	return nil
}

// GetApprovedTable returns the ApprovedTable for the given schema/table pair, or nil.
// For schemas with access_level "full", a synthetic ApprovedTable is returned for any table.
func (c *AccessConfig) GetApprovedTable(schema, table string) *ApprovedTable {
	s := c.GetApprovedSchema(schema)
	if s == nil {
		return nil
	}
	for i := range s.ApprovedTables {
		if strings.EqualFold(s.ApprovedTables[i].Table, table) {
			return &s.ApprovedTables[i]
		}
	}
	// For full-access schemas, every table is implicitly approved.
	if s.AccessLevel == "full" {
		return &ApprovedTable{
			Table:       table,
			AccessLevel: "full",
		}
	}
	return nil
}

// defaultAccessConfig returns an AccessConfig populated with safe defaults.
func defaultAccessConfig() *AccessConfig {
	return &AccessConfig{
		Version:         "1.0",
		LastModifiedBy:  "",
		LastModifiedAt:  "",
		ApprovedSchemas: []ApprovedSchema{},
		HiddenSchemas:   []HiddenSchema{},
	}
}
