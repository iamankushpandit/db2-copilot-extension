package schema

import (
	"fmt"
	"strings"

	"github.com/iamankushpandit/db2-copilot-extension/config"
	"github.com/iamankushpandit/db2-copilot-extension/database"
)

// BuildApprovedContext produces the Tier 2 approved schema text that is
// injected into the SQL generation prompt. Only approved schemas, tables,
// and columns are included.
func BuildApprovedContext(info *database.SchemaInfo, access *config.AccessConfig) string {
	if info == nil || access == nil {
		return "(schema unavailable)"
	}

	var sb strings.Builder

	for _, schema := range info.Schemas {
		// Skip hidden schemas entirely.
		if isHidden(schema.Name, access) {
			continue
		}

		approvedTables := filterTables(schema, access)
		if len(approvedTables) == 0 {
			continue
		}

		sb.WriteString(fmt.Sprintf("Schema: %s\n", schema.Name))
		for _, table := range approvedTables {
			sb.WriteString(fmt.Sprintf("  Table: %s\n", table.Name))
			if table.Comment != "" {
				sb.WriteString(fmt.Sprintf("    Comment: %s\n", table.Comment))
			}
			for _, col := range table.Columns {
				nullable := ""
				if !col.IsNullable {
					nullable = ", NOT NULL"
				}
				pkTag := ""
				if col.IsPK {
					pkTag = ", PK"
				}
				sb.WriteString(fmt.Sprintf("      - %s (%s%s%s)\n", col.Name, col.DataType, nullable, pkTag))
			}
			for _, fk := range table.ForeignKeys {
				sb.WriteString(fmt.Sprintf("      Relationship: %s.%s → %s.%s\n",
					table.Name, fk.Column, fk.RefTable, fk.RefColumn))
			}
			if table.RowCountEstimate > 0 {
				sb.WriteString(fmt.Sprintf("      Row count: ~%d\n", table.RowCountEstimate))
			}
			if len(table.SampleRows) > 0 {
				sb.WriteString("      Sample: ")
				parts := make([]string, 0, len(table.SampleRows[0]))
				for k, v := range table.SampleRows[0] {
					parts = append(parts, fmt.Sprintf("%s=%q", k, fmt.Sprintf("%v", v)))
				}
				sb.WriteString(strings.Join(parts, " "))
				sb.WriteString("\n")
			}
		}
	}

	if sb.Len() == 0 {
		return "(no tables approved — ask your admin to configure access)"
	}
	return sb.String()
}

// filterTables returns the TableDetails for tables in a schema that are
// approved according to the access config, with columns filtered.
func filterTables(schema database.SchemaDetail, access *config.AccessConfig) []database.TableDetail {
	var result []database.TableDetail

	for _, table := range schema.Tables {
		approved, hidden := access.IsTableApproved(schema.Name, table.Name)
		if hidden || !approved {
			continue
		}

		// Check if schema or table is full-access.
		filtered := filterColumns(table, schema.Name, access)
		result = append(result, filtered)
	}
	return result
}

// filterColumns removes denied / unapproved columns from a TableDetail.
func filterColumns(table database.TableDetail, schemaName string, access *config.AccessConfig) database.TableDetail {
	out := table
	out.Columns = nil
	for _, col := range table.Columns {
		if access.IsColumnApproved(schemaName, table.Name, col.Name) {
			out.Columns = append(out.Columns, col)
		}
	}
	return out
}

// isHidden returns true if the schema is in the hidden list.
func isHidden(schemaName string, access *config.AccessConfig) bool {
	for _, h := range access.HiddenSchemas {
		if h.Schema == schemaName {
			return true
		}
	}
	return false
}
