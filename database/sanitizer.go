package database

import (
	"fmt"
	"strings"
)

// forbiddenKeywords lists SQL operations that are not permitted.
// Only SELECT statements are allowed to prevent data modification.
var forbiddenKeywords = []string{
	"INSERT", "UPDATE", "DELETE", "DROP", "ALTER", "CREATE",
	"TRUNCATE", "EXEC", "EXECUTE", "MERGE", "GRANT", "REVOKE",
	"REPLACE", "UPSERT", "CALL",
}

// SanitizeSQL validates and sanitizes a SQL query for safe execution.
// It enforces SELECT-only queries, prevents SQL injection via comments and
// multi-statement attacks, and normalizes whitespace.
//
// The dbType parameter is informational and may be used for dialect-specific
// validation in the future (e.g., DB2 uses FETCH FIRST N ROWS ONLY while
// PostgreSQL uses LIMIT).
func SanitizeSQL(query string, dbType string) (string, error) {
	// Trim whitespace
	query = strings.TrimSpace(query)

	if query == "" {
		return "", fmt.Errorf("query is empty")
	}

	// Remove trailing semicolon
	query = strings.TrimRight(query, ";")
	query = strings.TrimSpace(query)

	// Reject SQL line comments (--)
	if strings.Contains(query, "--") {
		return "", fmt.Errorf("SQL comments are not allowed")
	}

	// Reject SQL block comments (/* */)
	if strings.Contains(query, "/*") || strings.Contains(query, "*/") {
		return "", fmt.Errorf("SQL comments are not allowed")
	}

	// Reject multiple statements (semicolons remaining after trimming trailing one)
	if strings.Contains(query, ";") {
		return "", fmt.Errorf("multiple SQL statements are not allowed")
	}

	// Normalise to uppercase for keyword checks
	upper := strings.ToUpper(query)

	// Must start with SELECT
	if !strings.HasPrefix(upper, "SELECT") {
		return "", fmt.Errorf("only SELECT statements are allowed")
	}

	// Reject forbidden keywords
	for _, keyword := range forbiddenKeywords {
		// Check as whole word to avoid false positives (e.g. "EXECUTE" inside an identifier)
		if containsWord(upper, keyword) {
			return "", fmt.Errorf("SQL keyword %q is not allowed", keyword)
		}
	}

	return query, nil
}

// containsWord reports whether s contains the given word as a whole word
// (surrounded by non-alphanumeric/underscore characters or at string boundaries).
func containsWord(s, word string) bool {
	idx := 0
	for {
		pos := strings.Index(s[idx:], word)
		if pos == -1 {
			return false
		}
		abs := idx + pos
		before := abs == 0 || !isIdentChar(rune(s[abs-1]))
		after := abs+len(word) >= len(s) || !isIdentChar(rune(s[abs+len(word)]))
		if before && after {
			return true
		}
		idx = abs + 1
	}
}

func isIdentChar(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') ||
		(r >= '0' && r <= '9') || r == '_'
}
