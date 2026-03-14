package pipeline

import (
	"strings"
	"testing"
)

func TestDetectShape(t *testing.T) {
	tests := []struct {
		name     string
		rows     []map[string]interface{}
		total    int
		expected ResultShape
	}{
		{
			name:     "nil rows returns empty",
			rows:     nil,
			total:    0,
			expected: ShapeEmpty,
		},
		{
			name:     "zero total returns empty",
			rows:     []map[string]interface{}{},
			total:    0,
			expected: ShapeEmpty,
		},
		{
			name:     "single value is scalar",
			rows:     []map[string]interface{}{{"count": 42}},
			total:    1,
			expected: ShapeScalar,
		},
		{
			name: "single row multiple columns is single",
			rows: []map[string]interface{}{
				{"id": 1, "name": "Alice", "email": "alice@example.com"},
			},
			total:    1,
			expected: ShapeSingle,
		},
		{
			name: "few rows fitting display limit is small",
			rows: []map[string]interface{}{
				{"id": 1}, {"id": 2}, {"id": 3},
			},
			total:    3,
			expected: ShapeSmall,
		},
		{
			name: "total exceeds display rows is large",
			rows: []map[string]interface{}{
				{"id": 1}, {"id": 2},
			},
			total:    100,
			expected: ShapeLarge,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DetectShape(tc.rows, tc.total)
			if got != tc.expected {
				t.Errorf("DetectShape(..., %d) = %q, want %q", tc.total, got, tc.expected)
			}
		})
	}
}

func TestFormatRetryNote(t *testing.T) {
	t.Run("no retry returns empty", func(t *testing.T) {
		got := FormatRetryNote(1, "", "")
		if got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})

	t.Run("retry with error returns note", func(t *testing.T) {
		got := FormatRetryNote(2, "SELECT bad", "column not found")
		if !strings.Contains(got, "ℹ️") {
			t.Error("expected info emoji in retry note")
		}
		if !strings.Contains(got, "column not found") {
			t.Error("expected error message in retry note")
		}
	})
}

func TestFormatSQLBlock(t *testing.T) {
	t.Run("empty sql returns empty", func(t *testing.T) {
		got := FormatSQLBlock("", true, false)
		if got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})

	t.Run("collapsible non-corrected", func(t *testing.T) {
		got := FormatSQLBlock("SELECT 1", true, false)
		if !strings.Contains(got, "<details>") {
			t.Error("expected <details> tag")
		}
		if !strings.Contains(got, "SQL Query Used</summary>") {
			t.Error("expected summary label")
		}
		if strings.Contains(got, "corrected") {
			t.Error("should not contain 'corrected' label")
		}
	})

	t.Run("collapsible corrected", func(t *testing.T) {
		got := FormatSQLBlock("SELECT 1", true, true)
		if !strings.Contains(got, "corrected") {
			t.Error("expected 'corrected' label")
		}
	})

	t.Run("non-collapsible", func(t *testing.T) {
		got := FormatSQLBlock("SELECT 1", false, false)
		if strings.Contains(got, "<details>") {
			t.Error("should not contain <details> tag")
		}
		if !strings.Contains(got, "SELECT 1") {
			t.Error("expected SQL content")
		}
	})
}

func TestFormatFailedResponse(t *testing.T) {
	attempts := []AttemptDetail{
		{SQL: "SELECT bad_col FROM t", Error: "column does not exist"},
		{SQL: "SELECT wrong FROM t", Error: "column does not exist"},
	}
	got := FormatFailedResponse(attempts)
	if !strings.Contains(got, "❌") {
		t.Error("expected failure emoji")
	}
	if !strings.Contains(got, "Attempt 1") {
		t.Error("expected attempt 1")
	}
	if !strings.Contains(got, "Attempt 2") {
		t.Error("expected attempt 2")
	}
	if !strings.Contains(got, "Suggestion") {
		t.Error("expected suggestion")
	}
}

func TestFormatResultsTable(t *testing.T) {
	t.Run("empty rows returns empty", func(t *testing.T) {
		got := FormatResultsTable(nil)
		if got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})

	t.Run("formats table with data", func(t *testing.T) {
		rows := []map[string]interface{}{
			{"name": "Alice", "age": 30},
			{"name": "Bob", "age": 25},
		}
		got := FormatResultsTable(rows)
		if !strings.Contains(got, "Alice") {
			t.Error("expected Alice in table")
		}
		if !strings.Contains(got, "Bob") {
			t.Error("expected Bob in table")
		}
		if !strings.Contains(got, "|") {
			t.Error("expected markdown table pipes")
		}
	})

	t.Run("handles nil values", func(t *testing.T) {
		rows := []map[string]interface{}{
			{"val": nil},
		}
		got := FormatResultsTable(rows)
		if !strings.Contains(got, "NULL") {
			t.Error("expected NULL for nil value")
		}
	})
}

func TestShapeHint(t *testing.T) {
	tests := []struct {
		shape    ResultShape
		display  int
		total    int
		contains string
	}{
		{ShapeEmpty, 0, 0, "no rows"},
		{ShapeScalar, 1, 1, "single value"},
		{ShapeSingle, 1, 1, "single row"},
		{ShapeSmall, 5, 5, "5 rows"},
		{ShapeLarge, 10, 100, "100 rows total"},
	}

	for _, tc := range tests {
		t.Run(string(tc.shape), func(t *testing.T) {
			got := ShapeHint(tc.shape, tc.display, tc.total)
			if !strings.Contains(got, tc.contains) {
				t.Errorf("ShapeHint(%q) = %q, want to contain %q", tc.shape, got, tc.contains)
			}
		})
	}
}
