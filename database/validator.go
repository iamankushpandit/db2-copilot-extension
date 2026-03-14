package database

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/iamankushpandit/db2-copilot-extension/config"
)

// ValidationResult is returned by ValidateSQL.
type ValidationResult struct {
	// Approved is true when all referenced tables/columns are approved.
	Approved bool
	// TablesReferenced lists every schema.table found in the query.
	TablesReferenced []string
	// UnapprovedTables lists tables that are not in the approved config.
	UnapprovedTables []string
	// HiddenTables lists tables that are in hidden_schemas.
	HiddenTables []string
	// UnapprovedColumns lists columns that are not in the approved config.
	UnapprovedColumns []string
}

// simpleTableRe is a best-effort regex to extract FROM/JOIN table references.
// It handles most common SQL patterns but is not a full SQL parser.
var simpleTableRe = regexp.MustCompile(
	`(?i)(?:FROM|JOIN)\s+([a-zA-Z_][a-zA-Z0-9_]*(?:\.[a-zA-Z_][a-zA-Z0-9_]*)?)`)

// ValidateSQL checks that every table referenced in query is in the approved
// access config. It returns a ValidationResult describing what it found.
func ValidateSQL(query string, access *config.AccessConfig) ValidationResult {
	if access == nil {
		return ValidationResult{Approved: true}
	}

	matches := simpleTableRe.FindAllStringSubmatch(query, -1)
	seen := make(map[string]bool)
	var tables []string
	for _, m := range matches {
		ref := m[1]
		if seen[ref] {
			continue
		}
		seen[ref] = true
		tables = append(tables, ref)
	}

	var result ValidationResult
	result.TablesReferenced = tables

	for _, ref := range tables {
		schema, table := splitSchemaTable(ref)
		approved, hidden := access.IsTableApproved(schema, table)
		if hidden {
			result.HiddenTables = append(result.HiddenTables, ref)
		} else if !approved {
			result.UnapprovedTables = append(result.UnapprovedTables, ref)
		}
	}

	result.Approved = len(result.UnapprovedTables) == 0 && len(result.HiddenTables) == 0
	return result
}

// UnapprovedTableError formats a user-visible error message for an unapproved table.
func UnapprovedTableError(tables []string) string {
	if len(tables) == 1 {
		return fmt.Sprintf("Table %q is not currently approved for querying. Please ask your admin to approve it.", tables[0])
	}
	return fmt.Sprintf("Tables %s are not currently approved for querying. Please ask your admin to approve them.",
		strings.Join(tables, ", "))
}

// splitSchemaTable splits "schema.table" into its components.
// If there is no dot the schema defaults to "public".
func splitSchemaTable(ref string) (schema, table string) {
	parts := strings.SplitN(ref, ".", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "public", parts[0]
}
