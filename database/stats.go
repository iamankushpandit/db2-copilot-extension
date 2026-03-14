package database

import (
	"fmt"
	"math"
	"sort"
)

// ColumnStats holds summary statistics for a single column.
type ColumnStats struct {
	ColumnName    string
	NullCount     int
	DistinctCount int

	// Numeric stats (zero if not numeric).
	Min float64
	Max float64
	Avg float64
	Sum float64

	// Categorical stats: populated when fewer than 20 distinct non-null values
	// are present. Maps value → count.
	ValueCounts map[string]int
}

// SummaryStats holds computed stats for all columns in a result set.
type SummaryStats struct {
	TotalRows int
	Columns   []ColumnStats
}

// ComputeStats computes summary statistics from all returned rows.
// The statistics are calculated across ALL rows (not just the display subset).
func ComputeStats(rows []map[string]interface{}) SummaryStats {
	if len(rows) == 0 {
		return SummaryStats{}
	}

	// Collect ordered column names.
	var columns []string
	colSet := make(map[string]bool)
	for _, row := range rows {
		for k := range row {
			if !colSet[k] {
				colSet[k] = true
				columns = append(columns, k)
			}
		}
	}
	sort.Strings(columns)

	stats := SummaryStats{TotalRows: len(rows)}

	for _, col := range columns {
		cs := ColumnStats{ColumnName: col}

		distinctVals := make(map[string]int)
		var numericVals []float64

		for _, row := range rows {
			v := row[col]
			if v == nil {
				cs.NullCount++
				continue
			}

			str := fmt.Sprintf("%v", v)
			distinctVals[str]++

			if f, ok := toFloat64(v); ok {
				numericVals = append(numericVals, f)
			}
		}

		cs.DistinctCount = len(distinctVals)

		if len(numericVals) > 0 {
			cs.Min = numericVals[0]
			cs.Max = numericVals[0]
			var sum float64
			for _, f := range numericVals {
				sum += f
				if f < cs.Min {
					cs.Min = f
				}
				if f > cs.Max {
					cs.Max = f
				}
			}
			cs.Sum = sum
			cs.Avg = sum / float64(len(numericVals))
		}

		if cs.DistinctCount < 20 {
			cs.ValueCounts = distinctVals
		}

		stats.Columns = append(stats.Columns, cs)
	}

	return stats
}

// toFloat64 attempts a best-effort conversion of v to float64.
func toFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case float32:
		f := float64(n)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, false
		}
		return f, true
	case float64:
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return 0, false
		}
		return n, true
	case []byte:
		var f float64
		if _, err := fmt.Sscanf(string(n), "%f", &f); err == nil {
			return f, true
		}
	}
	return 0, false
}
