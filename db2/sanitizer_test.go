package db2_test

import (
	"strings"
	"testing"

	"github.com/iamankushpandit/db2-copilot-extension/db2"
)

func TestSanitizeSQL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
		errMsg  string
	}{
		// Valid cases
		{
			name:  "simple select",
			input: "SELECT * FROM EMPLOYEES",
			want:  "SELECT * FROM EMPLOYEES",
		},
		{
			name:  "select with where clause",
			input: "SELECT NAME, SALARY FROM EMPLOYEES WHERE DEPT = 'HR'",
			want:  "SELECT NAME, SALARY FROM EMPLOYEES WHERE DEPT = 'HR'",
		},
		{
			name:  "trailing semicolon is removed",
			input: "SELECT 1 FROM SYSIBM.SYSDUMMY1;",
			want:  "SELECT 1 FROM SYSIBM.SYSDUMMY1",
		},
		{
			name:  "leading/trailing whitespace is trimmed",
			input: "   SELECT 1 FROM SYSIBM.SYSDUMMY1   ",
			want:  "SELECT 1 FROM SYSIBM.SYSDUMMY1",
		},
		{
			name:  "lowercase select is accepted",
			input: "select * from employees",
			want:  "select * from employees",
		},
		{
			name:  "mixed case select is accepted",
			input: "Select * From Employees",
			want:  "Select * From Employees",
		},

		// Invalid — empty
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
			errMsg:  "empty",
		},
		{
			name:    "only whitespace",
			input:   "   ",
			wantErr: true,
			errMsg:  "empty",
		},

		// Invalid — non-SELECT statements
		{
			name:    "insert statement",
			input:   "INSERT INTO T VALUES (1)",
			wantErr: true,
			errMsg:  "only SELECT",
		},
		{
			name:    "update statement",
			input:   "UPDATE T SET A=1",
			wantErr: true,
			errMsg:  "only SELECT",
		},
		{
			name:    "delete statement",
			input:   "DELETE FROM T",
			wantErr: true,
			errMsg:  "only SELECT",
		},
		{
			name:    "drop statement",
			input:   "DROP TABLE T",
			wantErr: true,
			errMsg:  "only SELECT",
		},
		{
			name:    "create statement",
			input:   "CREATE TABLE T (ID INT)",
			wantErr: true,
			errMsg:  "only SELECT",
		},
		{
			name:    "alter statement",
			input:   "ALTER TABLE T ADD COLUMN X INT",
			wantErr: true,
			errMsg:  "only SELECT",
		},
		{
			name:    "truncate statement",
			input:   "TRUNCATE TABLE T",
			wantErr: true,
			errMsg:  "only SELECT",
		},
		{
			name:    "exec statement",
			input:   "EXEC SP_HELP",
			wantErr: true,
			errMsg:  "only SELECT",
		},
		{
			name:    "grant statement",
			input:   "GRANT SELECT ON T TO USER1",
			wantErr: true,
			errMsg:  "only SELECT",
		},
		{
			name:    "revoke statement",
			input:   "REVOKE SELECT ON T FROM USER1",
			wantErr: true,
			errMsg:  "only SELECT",
		},
		{
			name:    "merge statement",
			input:   "MERGE INTO T USING S ON ...",
			wantErr: true,
			errMsg:  "only SELECT",
		},

		// Invalid — comments
		{
			name:    "inline comment",
			input:   "SELECT * FROM T -- comment",
			wantErr: true,
			errMsg:  "comments",
		},
		{
			name:    "block comment",
			input:   "SELECT /* all */ * FROM T",
			wantErr: true,
			errMsg:  "comments",
		},

		// Invalid — multiple statements
		{
			name:    "multiple statements via semicolon",
			input:   "SELECT 1; SELECT 2",
			wantErr: true,
			errMsg:  "multiple",
		},

		// Keyword injection attempts embedded inside SELECT
		{
			name:    "delete keyword in select",
			input:   "SELECT * FROM T WHERE X = 'a'; DELETE FROM T",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := db2.SanitizeSQL(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("SanitizeSQL(%q) = %q, nil; want error", tc.input, got)
				} else if tc.errMsg != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.errMsg)) {
					t.Errorf("SanitizeSQL(%q) error = %q; want it to contain %q", tc.input, err.Error(), tc.errMsg)
				}
				return
			}
			if err != nil {
				t.Errorf("SanitizeSQL(%q) unexpected error: %v", tc.input, err)
				return
			}
			if got != tc.want {
				t.Errorf("SanitizeSQL(%q) = %q; want %q", tc.input, got, tc.want)
			}
		})
	}
}
