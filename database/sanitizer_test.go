package database

import (
	"strings"
	"testing"
)

func TestSanitizeSQL(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		dbType  string
		wantErr bool
		errMsg  string
	}{
		// Valid queries
		{name: "simple select", query: "SELECT * FROM users", dbType: "postgres", wantErr: false},
		{name: "select with limit", query: "SELECT id, name FROM orders LIMIT 10", dbType: "postgres", wantErr: false},
		{name: "select with db2 fetch", query: "SELECT * FROM employees FETCH FIRST 10 ROWS ONLY", dbType: "db2", wantErr: false},
		{name: "trailing semicolon removed", query: "SELECT 1;", dbType: "postgres", wantErr: false},
		{name: "whitespace trimmed", query: "  SELECT id FROM t  ", dbType: "postgres", wantErr: false},

		// Invalid: empty
		{name: "empty query", query: "", dbType: "postgres", wantErr: true, errMsg: "empty"},

		// Invalid: non-SELECT
		{name: "insert", query: "INSERT INTO users VALUES (1,'a')", dbType: "postgres", wantErr: true, errMsg: "SELECT"},
		{name: "update", query: "UPDATE users SET name='x'", dbType: "postgres", wantErr: true},
		{name: "delete", query: "DELETE FROM users", dbType: "postgres", wantErr: true},
		{name: "drop", query: "DROP TABLE users", dbType: "postgres", wantErr: true},

		// Invalid: forbidden keywords inside SELECT
		{name: "select with drop", query: "SELECT * FROM t; DROP TABLE t", dbType: "postgres", wantErr: true},
		{name: "select with insert keyword", query: "SELECT INSERT INTO FROM t", dbType: "postgres", wantErr: true},
		{name: "select with delete keyword", query: "SELECT DELETE FROM t", dbType: "postgres", wantErr: true},

		// Invalid: SQL comments
		{name: "line comment", query: "SELECT 1 -- comment", dbType: "postgres", wantErr: true, errMsg: "comment"},
		{name: "block comment", query: "SELECT /* evil */ 1", dbType: "postgres", wantErr: true, errMsg: "comment"},

		// Invalid: multiple statements
		{name: "semicolon in middle", query: "SELECT 1; SELECT 2", dbType: "postgres", wantErr: true, errMsg: "multiple"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SanitizeSQL(tt.query, tt.dbType)
			if tt.wantErr {
				if err == nil {
					t.Errorf("SanitizeSQL(%q) expected error, got nil (result: %q)", tt.query, got)
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("SanitizeSQL(%q) error = %q, want to contain %q", tt.query, err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("SanitizeSQL(%q) unexpected error: %v", tt.query, err)
				}
			}
		})
	}
}

func TestSanitizeSQL_WholeWordMatching(t *testing.T) {
	// "EXECUTE" inside an identifier should not be flagged
	query := "SELECT executor_id, execution_count FROM job_executions"
	got, err := SanitizeSQL(query, "postgres")
	if err != nil {
		t.Errorf("SanitizeSQL(%q) unexpected error: %v — 'executor' is not the forbidden keyword 'EXECUTE'", query, err)
	}
	if got != query {
		t.Errorf("SanitizeSQL(%q) = %q, want %q", query, got, query)
	}
}

func TestSanitizeSQL_TrailingSemicolon(t *testing.T) {
	input := "SELECT id FROM users;"
	want := "SELECT id FROM users"
	got, err := SanitizeSQL(input, "postgres")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
