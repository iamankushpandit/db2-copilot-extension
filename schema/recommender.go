package schema

import (
	"fmt"
	"strings"

	"github.com/iamankushpandit/db2-copilot-extension/config"
	"github.com/iamankushpandit/db2-copilot-extension/database"
)

// Recommendation describes a table that exists in Tier 1 but is not yet approved.
type Recommendation struct {
	Schema  string
	Table   database.TableDetail
	Message string
}

// FindRecommendations searches Tier 1 schema for tables that match the
// user's question (by name mention) and are not yet approved. Hidden tables
// are never recommended.
func FindRecommendations(question string, info *database.SchemaInfo, access *config.AccessConfig) []Recommendation {
	if info == nil {
		return nil
	}

	questionLower := strings.ToLower(question)
	var recommendations []Recommendation

	for _, schema := range info.Schemas {
		if isHidden(schema.Name, access) {
			continue
		}
		for _, table := range schema.Tables {
			approved, hidden := access.IsTableApproved(schema.Name, table.Name)
			if approved || hidden {
				continue
			}
			// Check if the table name appears in the question.
			if strings.Contains(questionLower, strings.ToLower(table.Name)) {
				recommendations = append(recommendations, Recommendation{
					Schema:  schema.Name,
					Table:   table,
					Message: buildRecommendationMessage(schema.Name, table),
				})
			}
		}
	}

	return recommendations
}

// FormatRecommendation builds a user-visible recommendation message for a
// table that exists but is not yet approved.
func FormatRecommendation(r Recommendation) string {
	return r.Message
}

func buildRecommendationMessage(schemaName string, table database.TableDetail) string {
	var sb strings.Builder

	sb.WriteString("I found a table that might have what you need, but it's not currently approved for querying.\n\n")
	sb.WriteString("**Recommended addition for your admin:**\n")
	sb.WriteString(fmt.Sprintf("- Schema: `%s`\n", schemaName))
	sb.WriteString(fmt.Sprintf("- Table: `%s`\n", table.Name))

	if len(table.Columns) > 0 {
		sb.WriteString("- Columns: ")
		colParts := make([]string, 0, len(table.Columns))
		for _, col := range table.Columns {
			nullable := ""
			if !col.IsNullable {
				nullable = ", NOT NULL"
			}
			pkTag := ""
			if col.IsPK {
				pkTag = ", PK"
			}
			colParts = append(colParts, fmt.Sprintf("`%s` (%s%s%s)", col.Name, col.DataType, nullable, pkTag))
		}
		sb.WriteString(strings.Join(colParts, ", "))
		sb.WriteString("\n")
	}

	for _, fk := range table.ForeignKeys {
		sb.WriteString(fmt.Sprintf("- Relationships: `%s` → `%s.%s` (FK)\n",
			fk.Column, fk.RefTable, fk.RefColumn))
	}

	if table.RowCountEstimate > 0 {
		sb.WriteString(fmt.Sprintf("- Approximate rows: ~%d\n", table.RowCountEstimate))
	}

	sb.WriteString("\nPlease ask your admin to add this to the approved configuration.")
	return sb.String()
}
