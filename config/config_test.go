package config_test

import (
	"testing"
	"time"

	"github.com/iamankushpandit/db2-copilot-extension/config"
)

func TestAccessConfig_IsTableApproved_FullSchema(t *testing.T) {
	ac := &config.AccessConfig{
		ApprovedSchemas: []config.ApprovedSchema{
			{Schema: "public", AccessLevel: config.AccessLevelFull},
		},
	}
	approved, hidden := ac.IsTableApproved("public", "anything")
	if !approved {
		t.Error("expected approved for full-access schema")
	}
	if hidden {
		t.Error("expected not hidden")
	}
}

func TestAccessConfig_IsTableApproved_PartialSchema(t *testing.T) {
	ac := &config.AccessConfig{
		ApprovedSchemas: []config.ApprovedSchema{
			{
				Schema:      "public",
				AccessLevel: config.AccessLevelPartial,
				ApprovedTables: []config.ApprovedTable{
					{Table: "orders", AccessLevel: config.AccessLevelFull},
				},
			},
		},
	}

	approved, hidden := ac.IsTableApproved("public", "orders")
	if !approved || hidden {
		t.Errorf("expected approved=true hidden=false, got %v %v", approved, hidden)
	}

	approved2, hidden2 := ac.IsTableApproved("public", "users")
	if approved2 || hidden2 {
		t.Errorf("expected approved=false hidden=false for unlisted table, got %v %v", approved2, hidden2)
	}
}

func TestAccessConfig_IsTableApproved_HiddenSchema(t *testing.T) {
	ac := &config.AccessConfig{
		HiddenSchemas: []config.HiddenSchema{
			{Schema: "security"},
		},
	}
	approved, hidden := ac.IsTableApproved("security", "tokens")
	if approved || !hidden {
		t.Errorf("expected approved=false hidden=true, got %v %v", approved, hidden)
	}
}

func TestAccessConfig_IsColumnApproved(t *testing.T) {
	ac := &config.AccessConfig{
		ApprovedSchemas: []config.ApprovedSchema{
			{
				Schema:      "public",
				AccessLevel: config.AccessLevelPartial,
				ApprovedTables: []config.ApprovedTable{
					{
						Table:           "customers",
						AccessLevel:     config.AccessLevelPartial,
						ApprovedColumns: []string{"id", "name", "email"},
						DeniedColumns:   []string{"ssn"},
					},
				},
			},
		},
	}

	if !ac.IsColumnApproved("public", "customers", "id") {
		t.Error("id should be approved")
	}
	if !ac.IsColumnApproved("public", "customers", "name") {
		t.Error("name should be approved")
	}
	if ac.IsColumnApproved("public", "customers", "ssn") {
		t.Error("ssn should be denied")
	}
	if ac.IsColumnApproved("public", "customers", "credit_card") {
		t.Error("unlisted column should be denied")
	}
}

func TestAdminConfig_IsAdminUser(t *testing.T) {
	ac := &config.AdminConfig{
		AdminUI: config.AdminUIConfig{
			AllowedGithubUsers: []string{"alice", "bob"},
		},
	}
	if !ac.IsAdminUser("alice") {
		t.Error("alice should be admin")
	}
	if ac.IsAdminUser("eve") {
		t.Error("eve should not be admin")
	}
}

func TestDefaultSafetyConfig(t *testing.T) {
	sc := config.DefaultSafetyConfig()
	if sc.QueryLimits.MaxQueryRows != 100 {
		t.Errorf("MaxQueryRows: got %d, want 100", sc.QueryLimits.MaxQueryRows)
	}
	if sc.QueryLimits.DisplayRows != 10 {
		t.Errorf("DisplayRows: got %d, want 10", sc.QueryLimits.DisplayRows)
	}
	if sc.SelfCorrection.MaxRetries != 1 {
		t.Errorf("MaxRetries: got %d, want 1", sc.SelfCorrection.MaxRetries)
	}
}

func TestGlossaryConfig(t *testing.T) {
	gc := &config.GlossaryConfig{
		Terms: []config.GlossaryTerm{
			{
				Term:       "revenue",
				Definition: "SUM(orders.total_amount)",
				AddedBy:    "admin",
				AddedAt:    time.Now(),
			},
		},
	}
	if len(gc.Terms) != 1 {
		t.Errorf("expected 1 term, got %d", len(gc.Terms))
	}
	if gc.Terms[0].Term != "revenue" {
		t.Errorf("term: got %q, want %q", gc.Terms[0].Term, "revenue")
	}
}
