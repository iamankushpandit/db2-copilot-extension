package database_test

import (
	"testing"

	"github.com/iamankushpandit/db2-copilot-extension/database"
)

func TestInjectPostgresLimit(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		maxRows int
		want    string
	}{
		{
			name:    "no limit added",
			query:   "SELECT id FROM users",
			maxRows: 100,
			want:    "SELECT id FROM users LIMIT 100",
		},
		{
			name:    "existing limit below max preserved",
			query:   "SELECT id FROM users LIMIT 10",
			maxRows: 100,
			want:    "SELECT id FROM users LIMIT 10",
		},
		{
			name:    "existing limit above max capped",
			query:   "SELECT id FROM users LIMIT 500",
			maxRows: 100,
			want:    "SELECT id FROM users LIMIT 100",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := database.InjectPostgresLimit(tc.query, tc.maxRows)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestInjectDB2Limit(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		maxRows int
		want    string
	}{
		{
			name:    "no limit added",
			query:   "SELECT id FROM users",
			maxRows: 100,
			want:    "SELECT id FROM users FETCH FIRST 100 ROWS ONLY",
		},
		{
			name:    "existing fetch first below max preserved",
			query:   "SELECT id FROM users FETCH FIRST 10 ROWS ONLY",
			maxRows: 100,
			want:    "SELECT id FROM users FETCH FIRST 10 ROWS ONLY",
		},
		{
			name:    "existing fetch first above max capped",
			query:   "SELECT id FROM users FETCH FIRST 500 ROWS ONLY",
			maxRows: 100,
			want:    "SELECT id FROM users FETCH FIRST 100 ROWS ONLY",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := database.InjectDB2Limit(tc.query, tc.maxRows)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLimitResults(t *testing.T) {
	rows := make([]map[string]interface{}, 20)
	for i := range rows {
		rows[i] = map[string]interface{}{"id": i}
	}

	display, total := database.LimitResults(rows, 10)
	if total != 20 {
		t.Errorf("total: got %d, want 20", total)
	}
	if len(display) != 10 {
		t.Errorf("display len: got %d, want 10", len(display))
	}

	// When displayRows >= total, return all.
	display2, total2 := database.LimitResults(rows, 50)
	if total2 != 20 {
		t.Errorf("total2: got %d, want 20", total2)
	}
	if len(display2) != 20 {
		t.Errorf("display2 len: got %d, want 20", len(display2))
	}
}
