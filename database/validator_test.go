package database_test

import (
	"testing"

	"github.com/iamankushpandit/db2-copilot-extension/config"
	"github.com/iamankushpandit/db2-copilot-extension/database"
)

func TestValidateSQL_NoAccessConfig(t *testing.T) {
	result := database.ValidateSQL("SELECT id FROM users", nil)
	if !result.Approved {
		t.Error("expected approved when no access config")
	}
}

func TestValidateSQL_ApprovedTable(t *testing.T) {
	access := &config.AccessConfig{
		ApprovedSchemas: []config.ApprovedSchema{
			{
				Schema:      "public",
				AccessLevel: config.AccessLevelFull,
			},
		},
	}
	result := database.ValidateSQL("SELECT id FROM public.users", access)
	if !result.Approved {
		t.Errorf("expected approved, got unapproved: %+v", result)
	}
}

func TestValidateSQL_UnapprovedTable(t *testing.T) {
	access := &config.AccessConfig{
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
	result := database.ValidateSQL("SELECT id FROM public.users", access)
	if result.Approved {
		t.Error("expected unapproved for non-listed table")
	}
	if len(result.UnapprovedTables) != 1 {
		t.Errorf("expected 1 unapproved table, got %d", len(result.UnapprovedTables))
	}
}

func TestValidateSQL_HiddenTable(t *testing.T) {
	access := &config.AccessConfig{
		HiddenSchemas: []config.HiddenSchema{
			{Schema: "security"},
		},
	}
	result := database.ValidateSQL("SELECT id FROM security.tokens", access)
	if result.Approved {
		t.Error("expected unapproved for hidden schema")
	}
	if len(result.HiddenTables) != 1 {
		t.Errorf("expected 1 hidden table, got %d", len(result.HiddenTables))
	}
}
