package database

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// limitRe matches an existing LIMIT clause in a SQL query (PostgreSQL style).
var limitRe = regexp.MustCompile(`(?i)\bLIMIT\s+(\d+)\b`)

// fetchFirstRe matches an existing FETCH FIRST clause (DB2 style).
var fetchFirstRe = regexp.MustCompile(`(?i)\bFETCH\s+FIRST\s+(\d+)\s+ROWS?\s+ONLY\b`)

// InjectPostgresLimit ensures query has a LIMIT clause capped at maxRows.
// If the query already has a lower LIMIT the original value is kept.
// This is the PostgreSQL-compatible implementation.
func InjectPostgresLimit(query string, maxRows int) string {
	if m := limitRe.FindStringSubmatchIndex(query); m != nil {
		// Extract existing limit value.
		existing, err := strconv.Atoi(query[m[2]:m[3]])
		if err == nil && existing <= maxRows {
			return query
		}
		// Replace with capped value.
		return query[:m[2]] + strconv.Itoa(maxRows) + query[m[3]:]
	}

	// No LIMIT found — append one.
	return fmt.Sprintf("%s LIMIT %d", strings.TrimRight(query, " \t\n"), maxRows)
}

// InjectDB2Limit ensures query has a FETCH FIRST clause capped at maxRows.
// This is the DB2-compatible implementation.
func InjectDB2Limit(query string, maxRows int) string {
	if m := fetchFirstRe.FindStringSubmatchIndex(query); m != nil {
		existing, err := strconv.Atoi(query[m[2]:m[3]])
		if err == nil && existing <= maxRows {
			return query
		}
		// Replace the numeric portion only.
		return query[:m[2]] + strconv.Itoa(maxRows) + query[m[3]:]
	}

	// Also handle LIMIT if present (e.g., LLM generated PostgreSQL syntax).
	if m := limitRe.FindStringSubmatchIndex(query); m != nil {
		existing, err := strconv.Atoi(query[m[2]:m[3]])
		if err == nil && existing <= maxRows {
			// Convert LIMIT N → FETCH FIRST N ROWS ONLY
			return limitRe.ReplaceAllStringFunc(query, func(s string) string {
				return fmt.Sprintf("FETCH FIRST %d ROWS ONLY", existing)
			})
		}
		return limitRe.ReplaceAllString(query, fmt.Sprintf("FETCH FIRST %d ROWS ONLY", maxRows))
	}

	// No limit found — append DB2-style.
	return fmt.Sprintf("%s FETCH FIRST %d ROWS ONLY", strings.TrimRight(query, " \t\n"), maxRows)
}

// LimitResults applies display-level result limiting.
// It returns the rows to display (up to displayRows) and the total count.
func LimitResults(rows []map[string]interface{}, displayRows int) (display []map[string]interface{}, total int) {
	total = len(rows)
	if displayRows >= total {
		return rows, total
	}
	return rows[:displayRows], total
}

// FormatResults converts query results into a markdown table string.
func FormatResults(results []map[string]interface{}) string {
	if len(results) == 0 {
		return "_No rows returned._"
	}

	// Collect ordered column names from the first row.
	columns := make([]string, 0, len(results[0]))
	for col := range results[0] {
		columns = append(columns, col)
	}

	var sb strings.Builder

	// Header row.
	sb.WriteString("| ")
	sb.WriteString(strings.Join(columns, " | "))
	sb.WriteString(" |\n")

	// Separator.
	sb.WriteString("| ")
	for i := range columns {
		if i > 0 {
			sb.WriteString(" | ")
		}
		sb.WriteString("---")
	}
	sb.WriteString(" |\n")

	// Data rows.
	for _, row := range results {
		sb.WriteString("| ")
		for i, col := range columns {
			if i > 0 {
				sb.WriteString(" | ")
			}
			v := row[col]
			if v == nil {
				sb.WriteString("NULL")
			} else {
				sb.WriteString(fmt.Sprintf("%v", v))
			}
		}
		sb.WriteString(" |\n")
	}

	return sb.String()
}
