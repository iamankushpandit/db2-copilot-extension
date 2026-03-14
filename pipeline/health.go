package pipeline

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/iamankushpandit/db2-copilot-extension/audit"
	"github.com/iamankushpandit/db2-copilot-extension/config"
	"github.com/iamankushpandit/db2-copilot-extension/database"
	"github.com/iamankushpandit/db2-copilot-extension/llm"
	"github.com/iamankushpandit/db2-copilot-extension/schema"
)

const (
	StatusHealthy   = "healthy"
	StatusDegraded  = "degraded"
	StatusUnhealthy = "unhealthy"
)

// ComponentStatus describes the health of a single component.
type ComponentStatus struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
	LatencyMS int64  `json:"latency_ms,omitempty"`
}

// HealthReport is the result of a full health check.
type HealthReport struct {
	Status     string            `json:"status"`
	Timestamp  time.Time         `json:"timestamp"`
	Components []ComponentStatus `json:"components"`
}

// HealthChecker periodically checks the health of all system components and
// serves a detailed /health endpoint.
type HealthChecker struct {
	db          database.Client
	sqlGen      llm.TextToSQLProvider
	crawler     *schema.Crawler
	cfgMgr      *config.Manager
	auditLogger *audit.Logger

	mu         sync.RWMutex
	lastReport *HealthReport
}

// NewHealthChecker creates a HealthChecker. Callers should invoke Start to
// begin periodic background checks.
func NewHealthChecker(
	db database.Client,
	sqlGen llm.TextToSQLProvider,
	crawler *schema.Crawler,
	cfgMgr *config.Manager,
	auditLogger *audit.Logger,
) *HealthChecker {
	return &HealthChecker{
		db:          db,
		sqlGen:      sqlGen,
		crawler:     crawler,
		cfgMgr:      cfgMgr,
		auditLogger: auditLogger,
	}
}

// Check runs all configured health checks and returns a HealthReport.
func (h *HealthChecker) Check(ctx context.Context) *HealthReport {
	healthCfg := h.cfgMgr.Safety().Health
	var components []ComponentStatus

	// 1. Database ping.
	if healthCfg.DatabasePing {
		components = append(components, h.checkDatabase(ctx))
	}

	// 2. LLM provider availability.
	if healthCfg.OllamaPing {
		components = append(components, h.checkLLM(ctx))
	}

	// 3. Schema freshness.
	components = append(components, h.checkSchema())

	// Compute overall status.
	overall := StatusHealthy
	for _, c := range components {
		if c.Status == StatusUnhealthy {
			overall = StatusUnhealthy
			break
		}
		if c.Status == StatusDegraded {
			overall = StatusDegraded
		}
	}

	report := &HealthReport{
		Status:     overall,
		Timestamp:  time.Now().UTC(),
		Components: components,
	}

	h.mu.Lock()
	h.lastReport = report
	h.mu.Unlock()

	// Audit log.
	dbStatus, llmStatus, schemaStatus := "", "", ""
	for _, c := range components {
		switch c.Name {
		case "database":
			dbStatus = c.Status
		case "llm":
			llmStatus = c.Status
		case "schema":
			schemaStatus = c.Status
		}
	}
	h.auditLogger.Log(audit.EventHealthCheck, "", "", audit.HealthCheckDetails{
		Database: dbStatus,
		LLM:      llmStatus,
		Schema:   schemaStatus,
	})

	return report
}

// Start launches a background goroutine that runs periodic health checks.
// It exits when ctx is cancelled.
func (h *HealthChecker) Start(ctx context.Context) {
	healthCfg := h.cfgMgr.Safety().Health
	interval := time.Duration(healthCfg.CheckIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 60 * time.Second
	}

	// Run an initial check immediately.
	checkCtx, checkCancel := context.WithTimeout(ctx, 10*time.Second)
	h.Check(checkCtx)
	checkCancel()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				checkCtx, checkCancel := context.WithTimeout(ctx, 10*time.Second)
				report := h.Check(checkCtx)
				checkCancel()
				if report.Status != StatusHealthy {
					log.Printf("WARN health check: %s", report.Status)
				}
			}
		}
	}()
}

// LastReport returns the most recent HealthReport, or nil if no check has run.
func (h *HealthChecker) LastReport() *HealthReport {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lastReport
}

// ServeHTTP implements http.Handler, returning the latest health report as JSON.
func (h *HealthChecker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	report := h.LastReport()
	if report == nil {
		// No check has run yet; run one now.
		report = h.Check(r.Context())
	}

	w.Header().Set("Content-Type", "application/json")
	if report.Status != StatusHealthy {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	_ = json.NewEncoder(w).Encode(report)
}

// checkDatabase pings the database and reports its status.
func (h *HealthChecker) checkDatabase(ctx context.Context) ComponentStatus {
	start := time.Now()
	err := h.db.Ping(ctx)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return ComponentStatus{
			Name:      "database",
			Status:    StatusUnhealthy,
			Message:   err.Error(),
			LatencyMS: latency,
		}
	}
	return ComponentStatus{
		Name:      "database",
		Status:    StatusHealthy,
		Message:   h.db.DBType() + " reachable",
		LatencyMS: latency,
	}
}

// checkLLM checks the SQL generation LLM provider availability.
func (h *HealthChecker) checkLLM(ctx context.Context) ComponentStatus {
	start := time.Now()
	ok := h.sqlGen.Available(ctx)
	latency := time.Since(start).Milliseconds()

	if !ok {
		return ComponentStatus{
			Name:      "llm",
			Status:    StatusDegraded,
			Message:   h.sqlGen.Name() + " unavailable",
			LatencyMS: latency,
		}
	}
	return ComponentStatus{
		Name:      "llm",
		Status:    StatusHealthy,
		Message:   h.sqlGen.Name() + " available",
		LatencyMS: latency,
	}
}

// checkSchema checks whether the cached schema is stale.
func (h *HealthChecker) checkSchema() ComponentStatus {
	if h.crawler.IsStale() {
		return ComponentStatus{
			Name:    "schema",
			Status:  StatusDegraded,
			Message: "schema cache is stale or empty",
		}
	}
	last := h.crawler.LastCrawled()
	return ComponentStatus{
		Name:    "schema",
		Status:  StatusHealthy,
		Message: "last crawled " + last.Format(time.RFC3339),
	}
}
