package database_test

import (
	"testing"

	"github.com/iamankushpandit/db2-copilot-extension/database"
)

func TestComputeStats_Empty(t *testing.T) {
	stats := database.ComputeStats(nil)
	if stats.TotalRows != 0 {
		t.Errorf("expected 0 rows, got %d", stats.TotalRows)
	}
}

func TestComputeStats_Numeric(t *testing.T) {
	rows := []map[string]interface{}{
		{"amount": int64(10)},
		{"amount": int64(20)},
		{"amount": int64(30)},
	}
	stats := database.ComputeStats(rows)
	if stats.TotalRows != 3 {
		t.Errorf("total rows: got %d, want 3", stats.TotalRows)
	}
	if len(stats.Columns) != 1 {
		t.Fatalf("columns: got %d, want 1", len(stats.Columns))
	}
	col := stats.Columns[0]
	if col.ColumnName != "amount" {
		t.Errorf("column name: got %q, want %q", col.ColumnName, "amount")
	}
	if col.Min != 10 {
		t.Errorf("min: got %.2f, want 10", col.Min)
	}
	if col.Max != 30 {
		t.Errorf("max: got %.2f, want 30", col.Max)
	}
	if col.Avg != 20 {
		t.Errorf("avg: got %.2f, want 20", col.Avg)
	}
	if col.Sum != 60 {
		t.Errorf("sum: got %.2f, want 60", col.Sum)
	}
}

func TestComputeStats_NullHandling(t *testing.T) {
	rows := []map[string]interface{}{
		{"name": "Alice"},
		{"name": nil},
		{"name": "Bob"},
	}
	stats := database.ComputeStats(rows)
	if stats.TotalRows != 3 {
		t.Errorf("total rows: got %d, want 3", stats.TotalRows)
	}
	col := stats.Columns[0]
	if col.NullCount != 1 {
		t.Errorf("null count: got %d, want 1", col.NullCount)
	}
	if col.DistinctCount != 2 {
		t.Errorf("distinct count: got %d, want 2 (Alice, Bob)", col.DistinctCount)
	}
}
