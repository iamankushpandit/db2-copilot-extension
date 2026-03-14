package config

import "testing"

func baseAccessConfig() *AccessConfig {
	return &AccessConfig{
		Version: "1.0",
		ApprovedSchemas: []ApprovedSchema{
			{
				Schema:      "PUBLIC",
				AccessLevel: "full",
				ApprovedTables: []ApprovedTable{
					{
						Table:       "ORDERS",
						AccessLevel: "partial",
						ApprovedColumns: []string{"order_id", "status"},
						DeniedColumns:   []string{},
					},
					{
						Table:          "CUSTOMERS",
						AccessLevel:    "full",
						ApprovedColumns: []string{},
						DeniedColumns:  []string{"ssn", "credit_card"},
					},
				},
			},
			{
				Schema:      "PRIVATE",
				AccessLevel: "partial",
				ApprovedTables: []ApprovedTable{
					{
						Table:       "LEDGER",
						AccessLevel: "full",
						DeniedColumns: []string{},
					},
				},
			},
		},
		HiddenSchemas: []HiddenSchema{
			{Schema: "INTERNAL", Reason: "internal use only"},
		},
	}
}

// ---- IsSchemaApproved ------------------------------------------------------

func TestIsSchemaApproved(t *testing.T) {
	cfg := baseAccessConfig()

	cases := []struct {
		schema string
		want   bool
	}{
		{"PUBLIC", true},
		{"public", true},   // case-insensitive
		{"PRIVATE", true},
		{"UNKNOWN", false},
		{"INTERNAL", false}, // hidden ≠ approved
	}

	for _, tc := range cases {
		if got := cfg.IsSchemaApproved(tc.schema); got != tc.want {
			t.Errorf("IsSchemaApproved(%q) = %v, want %v", tc.schema, got, tc.want)
		}
	}
}

// ---- IsSchemaHidden --------------------------------------------------------

func TestIsSchemaHidden(t *testing.T) {
	cfg := baseAccessConfig()

	cases := []struct {
		schema string
		want   bool
	}{
		{"INTERNAL", true},
		{"internal", true}, // case-insensitive
		{"PUBLIC", false},
		{"UNKNOWN", false},
	}

	for _, tc := range cases {
		if got := cfg.IsSchemaHidden(tc.schema); got != tc.want {
			t.Errorf("IsSchemaHidden(%q) = %v, want %v", tc.schema, got, tc.want)
		}
	}
}

// ---- IsTableApproved -------------------------------------------------------

func TestIsTableApproved(t *testing.T) {
	cfg := baseAccessConfig()

	cases := []struct {
		schema, table string
		want          bool
	}{
		// Explicit table in partial-access schema
		{"PRIVATE", "LEDGER", true},
		// Explicit table in full-access schema
		{"PUBLIC", "ORDERS", true},
		// Any table in a full-access schema should be implicitly approved
		{"PUBLIC", "ANY_TABLE", true},
		// Table not listed in partial-access schema
		{"PRIVATE", "UNKNOWN_TABLE", false},
		// Unknown schema
		{"UNKNOWN", "ORDERS", false},
	}

	for _, tc := range cases {
		if got := cfg.IsTableApproved(tc.schema, tc.table); got != tc.want {
			t.Errorf("IsTableApproved(%q, %q) = %v, want %v", tc.schema, tc.table, got, tc.want)
		}
	}
}

// ---- IsColumnApproved ------------------------------------------------------

func TestIsColumnApproved(t *testing.T) {
	cfg := baseAccessConfig()

	cases := []struct {
		schema, table, column string
		want                  bool
	}{
		// partial table — column in approved list
		{"PUBLIC", "ORDERS", "order_id", true},
		{"PUBLIC", "ORDERS", "status", true},
		// partial table — column NOT in approved list
		{"PUBLIC", "ORDERS", "customer_id", false},
		// full-access table — column not denied
		{"PUBLIC", "CUSTOMERS", "name", true},
		// full-access table — denied column
		{"PUBLIC", "CUSTOMERS", "ssn", false},
		{"PUBLIC", "CUSTOMERS", "credit_card", false},
		// case-insensitive
		{"PUBLIC", "CUSTOMERS", "SSN", false},
		{"PUBLIC", "ORDERS", "ORDER_ID", true},
		// full-access schema: unspecified table is implicitly approved for any column
		{"PUBLIC", "UNKNOWN", "col", true},
		// unknown schema
		{"UNKNOWN", "ORDERS", "col", false},
	}

	for _, tc := range cases {
		got := cfg.IsColumnApproved(tc.schema, tc.table, tc.column)
		if got != tc.want {
			t.Errorf("IsColumnApproved(%q, %q, %q) = %v, want %v",
				tc.schema, tc.table, tc.column, got, tc.want)
		}
	}
}

// ---- GetApprovedSchema / GetApprovedTable ----------------------------------

func TestGetApprovedSchema(t *testing.T) {
	cfg := baseAccessConfig()

	if s := cfg.GetApprovedSchema("PUBLIC"); s == nil {
		t.Error("expected non-nil for PUBLIC")
	}
	if s := cfg.GetApprovedSchema("MISSING"); s != nil {
		t.Errorf("expected nil for MISSING, got %+v", s)
	}
}

func TestGetApprovedTable(t *testing.T) {
	cfg := baseAccessConfig()

	if tbl := cfg.GetApprovedTable("PUBLIC", "ORDERS"); tbl == nil {
		t.Error("expected non-nil for PUBLIC.ORDERS")
	}
	// Implicit table from full-access schema
	if tbl := cfg.GetApprovedTable("PUBLIC", "NEW_TABLE"); tbl == nil {
		t.Error("expected non-nil synthetic table for full-access schema")
	}
	// Non-existent table in partial schema
	if tbl := cfg.GetApprovedTable("PRIVATE", "MISSING"); tbl != nil {
		t.Errorf("expected nil for PRIVATE.MISSING, got %+v", tbl)
	}
}
