package db2

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	// blockedKeywords lists SQL keywords that must not appear in a query.
	blockedKeywords = []string{
		"INSERT", "UPDATE", "DELETE", "DROP", "ALTER",
		"CREATE", "TRUNCATE", "EXEC", "EXECUTE", "MERGE",
		"GRANT", "REVOKE",
	}

	// multipleStatementsRe detects semicolons that indicate multiple statements.
	// A trailing semicolon at the very end (after trimming) is removed before
	// this check runs, so the only semicolons that remain indicate mid-query
	// statement separators.
	multipleStatementsRe = regexp.MustCompile(`;`)

	// inlineCommentRe detects SQL single-line comment syntax.
	inlineCommentRe = regexp.MustCompile(`--`)

	// blockCommentRe detects SQL block comment syntax.
	blockCommentRe = regexp.MustCompile(`/\*`)
)

// SanitizeSQL validates and sanitizes a SQL query string.
// Only SELECT statements are permitted. The function:
//   - Trims surrounding whitespace
//   - Removes a single trailing semicolon
//   - Rejects non-SELECT statements
//   - Rejects multiple statements (embedded semicolons)
//   - Rejects SQL comments (-- and /* */)
//
// It returns the sanitized query or an error if validation fails.
func SanitizeSQL(query string) (string, error) {
	query = strings.TrimSpace(query)

	// Remove a single trailing semicolon so well-formed single statements pass.
	query = strings.TrimRight(query, ";")
	query = strings.TrimSpace(query)

	if query == "" {
		return "", fmt.Errorf("query is empty")
	}

	// Reject comments — they can be used to bypass keyword checks.
	if inlineCommentRe.MatchString(query) {
		return "", fmt.Errorf("SQL comments are not allowed")
	}
	if blockCommentRe.MatchString(query) {
		return "", fmt.Errorf("SQL comments are not allowed")
	}

	// Reject multiple statements.
	if multipleStatementsRe.MatchString(query) {
		return "", fmt.Errorf("multiple SQL statements are not allowed")
	}

	upper := strings.ToUpper(query)

	// Must start with SELECT.
	if !strings.HasPrefix(upper, "SELECT") {
		return "", fmt.Errorf("only SELECT statements are permitted")
	}

	// Reject any blocked keywords anywhere in the query.
	for _, kw := range blockedKeywords {
		// Use word-boundary style matching: the keyword must be preceded by
		// whitespace, punctuation, or the start of the string, to avoid false
		// positives on column/table names that merely contain the keyword as a
		// substring (e.g. a column named "executions").
		pattern := `(^|[^A-Z0-9_])` + kw + `([^A-Z0-9_]|$)`
		matched, err := regexp.MatchString(pattern, upper)
		if err != nil {
			return "", fmt.Errorf("internal validation error: %w", err)
		}
		if matched {
			return "", fmt.Errorf("SQL keyword %q is not allowed", kw)
		}
	}

	return query, nil
}
