package database

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	// blockedKeywords lists SQL keywords that must not appear in a SELECT query.
	blockedKeywords = []string{
		"INSERT", "UPDATE", "DELETE", "DROP", "ALTER",
		"CREATE", "TRUNCATE", "EXEC", "EXECUTE", "MERGE",
		"GRANT", "REVOKE",
	}

	// multipleStatementsRe detects embedded semicolons (multiple statements).
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
//   - Rejects any blocked DML/DDL keywords
//
// It returns the sanitized query or an error if validation fails.
func SanitizeSQL(query string) (string, error) {
	query = strings.TrimSpace(query)
	query = strings.TrimRight(query, ";")
	query = strings.TrimSpace(query)

	if query == "" {
		return "", fmt.Errorf("query is empty")
	}

	if inlineCommentRe.MatchString(query) {
		return "", fmt.Errorf("SQL comments are not allowed")
	}
	if blockCommentRe.MatchString(query) {
		return "", fmt.Errorf("SQL comments are not allowed")
	}
	if multipleStatementsRe.MatchString(query) {
		return "", fmt.Errorf("multiple SQL statements are not allowed")
	}

	upper := strings.ToUpper(query)
	if !strings.HasPrefix(upper, "SELECT") {
		return "", fmt.Errorf("only SELECT statements are permitted")
	}

	for _, kw := range blockedKeywords {
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
