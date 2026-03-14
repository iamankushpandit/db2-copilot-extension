package database_test

import (
	"testing"

	"github.com/iamankushpandit/db2-copilot-extension/database"
)

func TestSanitizeSQL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantSQL string
		wantErr bool
	}{
		{name: "valid SELECT", input: "SELECT id FROM users", wantSQL: "SELECT id FROM users"},
		{name: "trailing semicolon", input: "SELECT 1;", wantSQL: "SELECT 1"},
		{name: "empty", input: "", wantErr: true},
		{name: "whitespace only", input: "   ", wantErr: true},
		{name: "INSERT rejected", input: "INSERT INTO t VALUES (1)", wantErr: true},
		{name: "UPDATE rejected", input: "UPDATE t SET x=1", wantErr: true},
		{name: "DELETE rejected", input: "DELETE FROM t", wantErr: true},
		{name: "DROP rejected", input: "DROP TABLE t", wantErr: true},
		{name: "inline comment rejected", input: "SELECT 1 -- comment", wantErr: true},
		{name: "block comment rejected", input: "SELECT /* x */ 1", wantErr: true},
		{name: "multiple statements rejected", input: "SELECT 1; SELECT 2", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := database.SanitizeSQL(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil (result: %q)", got)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if got != tc.wantSQL {
				t.Errorf("got %q, want %q", got, tc.wantSQL)
			}
		})
	}
}
