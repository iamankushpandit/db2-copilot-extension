package pipeline

import (
	"fmt"
	"sort"
	"strings"
)

// ResultShape describes the shape of query results, used to tailor
// the explanation prompt and response formatting.
type ResultShape string

const (
	// ShapeEmpty means the query returned no rows.
	ShapeEmpty ResultShape = "empty"
	// ShapeScalar means a single value (1 row, 1 column).
	ShapeScalar ResultShape = "scalar"
	// ShapeSingle means a single row with multiple columns.
	ShapeSingle ResultShape = "single"
	// ShapeSmall means a few rows that fit nicely in a table.
	ShapeSmall ResultShape = "small"
	// ShapeLarge means many rows where summarization is needed.
	ShapeLarge ResultShape = "large"
)

// DetectShape determines the result shape from the displayed rows and total count.
func DetectShape(displayRows []map[string]interface{}, totalRows int) ResultShape {
	if totalRows == 0 || len(displayRows) == 0 {
		return ShapeEmpty
	}
	if totalRows == 1 {
		if len(displayRows[0]) == 1 {
			return ShapeScalar
		}
		return ShapeSingle
	}
	if totalRows <= len(displayRows) {
		return ShapeSmall
	}
	return ShapeLarge
}

// AttemptDetail holds details about a single query attempt for presentation.
type AttemptDetail struct {
	SQL   string
	Error string
}

// FormatRetryNote builds a markdown note about self-correction for the response.
// Returns an empty string if there was no retry (attempt == 1).
func FormatRetryNote(attempt int, previousSQL, previousError string) string {
	if attempt <= 1 || previousError == "" {
		return ""
	}
	return fmt.Sprintf(
		"> ℹ️ **Note:** My first query had an error (%s). I corrected it automatically. I've learned this for future queries.\n",
		previousError,
	)
}

// FormatSQLBlock builds a SQL block, optionally wrapped in a collapsible <details> element.
func FormatSQLBlock(sql string, collapsible bool, corrected bool) string {
	if sql == "" {
		return ""
	}
	label := "🔍 SQL Query Used"
	if corrected {
		label = "🔍 SQL Query Used (corrected)"
	}
	if collapsible {
		return fmt.Sprintf(
			"<details>\n<summary>%s</summary>\n\n```sql\n%s\n```\n\n</details>\n",
			label, strings.TrimSpace(sql),
		)
	}
	return fmt.Sprintf("**%s:**\n```sql\n%s\n```\n", label, strings.TrimSpace(sql))
}

// FormatFailedResponse builds the full response when all query attempts have failed.
func FormatFailedResponse(attempts []AttemptDetail) string {
	var sb strings.Builder
	sb.WriteString("❌ **Couldn't complete this query**\n\n")
	sb.WriteString(fmt.Sprintf("I tried %d approach(es) but all failed:\n\n", len(attempts)))
	for i, a := range attempts {
		sb.WriteString(fmt.Sprintf("<details>\n<summary>🔍 Attempt %d</summary>\n\n", i+1))
		if a.SQL != "" {
			sb.WriteString(fmt.Sprintf("```sql\n%s\n```\n", strings.TrimSpace(a.SQL)))
		}
		if a.Error != "" {
			sb.WriteString(fmt.Sprintf("Error: %s\n", a.Error))
		}
		sb.WriteString("\n</details>\n\n")
	}
	sb.WriteString("**Suggestion:** Try rephrasing your question — for example, add a date range or ask for a specific metric (count, average, total).\n")
	return sb.String()
}

// FormatResultsTable builds a markdown table from rows.
func FormatResultsTable(rows []map[string]interface{}) string {
	if len(rows) == 0 {
		return ""
	}
	// Collect column names in sorted order for deterministic output.
	var cols []string
	for k := range rows[0] {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	var sb strings.Builder
	sb.WriteString("| " + strings.Join(cols, " | ") + " |\n")
	sb.WriteString("|")
	for range cols {
		sb.WriteString("---|")
	}
	sb.WriteString("\n")
	for _, row := range rows {
		sb.WriteString("|")
		for _, col := range cols {
			v := row[col]
			if v == nil {
				sb.WriteString(" NULL |")
			} else {
				sb.WriteString(fmt.Sprintf(" %v |", v))
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// ShapeHint returns a human-readable description of the result shape
// for inclusion in the explanation prompt.
func ShapeHint(shape ResultShape, displayCount, totalRows int) string {
	switch shape {
	case ShapeEmpty:
		return "The query returned no rows."
	case ShapeScalar:
		return "The query returned a single value."
	case ShapeSingle:
		return "The query returned a single row with multiple columns."
	case ShapeSmall:
		return fmt.Sprintf("The query returned %d rows — all results fit in the table below.", totalRows)
	case ShapeLarge:
		return fmt.Sprintf(
			"The query returned %d rows total. Only the first %d are shown — use the summary statistics to describe the full dataset.",
			totalRows, displayCount,
		)
	default:
		return ""
	}
}
