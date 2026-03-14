package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iamankushpandit/db2-copilot-extension/audit"
	"github.com/iamankushpandit/db2-copilot-extension/config"
	"github.com/iamankushpandit/db2-copilot-extension/database"
	"github.com/iamankushpandit/db2-copilot-extension/llm"
	"github.com/iamankushpandit/db2-copilot-extension/schema"
)

// errTestPing is a sentinel error used in tests.
var errTestPing = errors.New("connection refused")

// --- stub implementations for testing ---

type healthStubDB struct {
	pingErr error
	dbType  string
}

func (s *healthStubDB) Ping(_ context.Context) error { return s.pingErr }
func (s *healthStubDB) ExecuteQuery(_ context.Context, _ string) ([]map[string]interface{}, error) {
	return nil, nil
}
func (s *healthStubDB) InjectLimit(q string, _ int) string { return q }
func (s *healthStubDB) ExplainCost(_ context.Context, _ string) (int64, float64, error) {
	return 0, 0, nil
}
func (s *healthStubDB) VerifyReadOnly(_ context.Context) (bool, error) { return true, nil }
func (s *healthStubDB) CrawlSchema(_ context.Context) (*database.SchemaInfo, error) {
	return &database.SchemaInfo{Schemas: []database.SchemaDetail{{Name: "public"}}}, nil
}
func (s *healthStubDB) Close() error   { return nil }
func (s *healthStubDB) DBType() string { return s.dbType }

type healthStubLLM struct {
	available bool
	name      string
}

func (s *healthStubLLM) GenerateSQL(_ context.Context, _ llm.SQLGenerationRequest) (string, error) {
	return "SELECT 1", nil
}
func (s *healthStubLLM) Available(_ context.Context) bool { return s.available }
func (s *healthStubLLM) Name() string                     { return s.name }

// --- helpers ---

func newTestHealthChecker(t *testing.T, pingErr error, llmAvailable bool, schemaStale bool) *HealthChecker {
	t.Helper()

	db := &healthStubDB{pingErr: pingErr, dbType: "postgres"}

	// Build a crawler; populate its cache if we want it fresh.
	crawler := schema.NewCrawler(db, 6, true)
	if !schemaStale {
		if _, err := crawler.Get(context.Background()); err != nil {
			t.Fatalf("seeding crawler: %v", err)
		}
	}

	cfgDir := t.TempDir()
	mgr, err := config.NewManager(cfgDir)
	if err != nil {
		t.Fatalf("creating config manager: %v", err)
	}
	t.Cleanup(func() { mgr.Close() })

	al, _ := audit.NewLogger("", false)

	return NewHealthChecker(
		db,
		&healthStubLLM{available: llmAvailable, name: "test-llm"},
		crawler,
		mgr,
		al,
	)
}

// --- tests ---

func TestHealthCheck_AllHealthy(t *testing.T) {
	hc := newTestHealthChecker(t, nil, true, false)
	report := hc.Check(context.Background())

	if report.Status != StatusHealthy {
		t.Errorf("expected %s, got %s", StatusHealthy, report.Status)
	}
	for _, c := range report.Components {
		if c.Status != StatusHealthy {
			t.Errorf("component %s: expected %s, got %s", c.Name, StatusHealthy, c.Status)
		}
	}
}

func TestHealthCheck_DatabaseUnhealthy(t *testing.T) {
	hc := newTestHealthChecker(t, errTestPing, true, false)
	report := hc.Check(context.Background())

	if report.Status != StatusUnhealthy {
		t.Errorf("expected %s, got %s", StatusUnhealthy, report.Status)
	}
	found := false
	for _, c := range report.Components {
		if c.Name == "database" {
			found = true
			if c.Status != StatusUnhealthy {
				t.Errorf("database component: expected %s, got %s", StatusUnhealthy, c.Status)
			}
		}
	}
	if !found {
		t.Error("database component not found in report")
	}
}

func TestHealthCheck_LLMDegraded(t *testing.T) {
	hc := newTestHealthChecker(t, nil, false, false)
	report := hc.Check(context.Background())

	if report.Status != StatusDegraded {
		t.Errorf("expected %s, got %s", StatusDegraded, report.Status)
	}
	for _, c := range report.Components {
		if c.Name == "llm" && c.Status != StatusDegraded {
			t.Errorf("llm component: expected %s, got %s", StatusDegraded, c.Status)
		}
	}
}

func TestHealthCheck_SchemaStale(t *testing.T) {
	hc := newTestHealthChecker(t, nil, true, true)
	report := hc.Check(context.Background())

	if report.Status != StatusDegraded {
		t.Errorf("expected %s, got %s", StatusDegraded, report.Status)
	}
	for _, c := range report.Components {
		if c.Name == "schema" && c.Status != StatusDegraded {
			t.Errorf("schema component: expected %s, got %s", StatusDegraded, c.Status)
		}
	}
}

func TestHealthCheck_ServeHTTP_Healthy(t *testing.T) {
	hc := newTestHealthChecker(t, nil, true, false)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)

	hc.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var report HealthReport
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if report.Status != StatusHealthy {
		t.Errorf("expected %s, got %s", StatusHealthy, report.Status)
	}
}

func TestHealthCheck_ServeHTTP_Unhealthy(t *testing.T) {
	hc := newTestHealthChecker(t, errTestPing, true, false)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)

	hc.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
}

func TestHealthCheck_LastReport(t *testing.T) {
	hc := newTestHealthChecker(t, nil, true, false)

	// Before any check, LastReport is nil.
	if hc.LastReport() != nil {
		t.Error("expected nil before first check")
	}

	hc.Check(context.Background())

	report := hc.LastReport()
	if report == nil {
		t.Fatal("expected non-nil after check")
	}
	if report.Status != StatusHealthy {
		t.Errorf("expected %s, got %s", StatusHealthy, report.Status)
	}
}
