package admin

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/iamankushpandit/db2-copilot-extension/config"
	"github.com/iamankushpandit/db2-copilot-extension/database"
	"github.com/iamankushpandit/db2-copilot-extension/llm"
	"github.com/iamankushpandit/db2-copilot-extension/schema"
)

//go:embed static
var staticFS embed.FS

// templates is the parsed template set, loaded once at startup.
var templates *template.Template

func init() {
	var err error
	templates, err = template.New("").Funcs(template.FuncMap{
		"not": func(b bool) bool { return !b },
	}).ParseFS(staticFS,
		"static/base.html",
		"static/index.html",
		"static/schema.html",
		"static/safety.html",
		"static/llm.html",
		"static/glossary.html",
		"static/database.html",
		"static/login.html",
	)
	if err != nil {
		log.Fatalf("FATAL could not parse admin templates: %v", err)
	}
}

// Service handles all admin UI HTTP requests.
type Service struct {
	cfgMgr  *config.Manager
	crawler *schema.Crawler
	db      database.Client
	sqlGen  llm.TextToSQLProvider
	auth    *AuthService
	dbType  string
}

// NewService creates an admin Service with all dependencies.
func NewService(
	cfgMgr *config.Manager,
	crawler *schema.Crawler,
	db database.Client,
	sqlGen llm.TextToSQLProvider,
	auth *AuthService,
	dbType string,
) *Service {
	return &Service{
		cfgMgr:  cfgMgr,
		crawler: crawler,
		db:      db,
		sqlGen:  sqlGen,
		auth:    auth,
		dbType:  dbType,
	}
}

// RegisterRoutes registers all admin routes on mux.
func (s *Service) RegisterRoutes(mux *http.ServeMux) {
	// Unauthenticated auth routes.
	mux.HandleFunc("GET /admin/login", s.auth.HandleLogin)
	mux.HandleFunc("GET /admin/callback", s.auth.HandleCallback)
	mux.HandleFunc("GET /admin/logout", s.auth.HandleLogout)

	// Login page (show HTML).
	mux.HandleFunc("GET /admin/login/page", s.handleLoginPage)

	// Protected pages.
	mux.HandleFunc("GET /admin", s.auth.RequireAdmin(s.handleIndex))
	mux.HandleFunc("GET /admin/schema", s.auth.RequireAdmin(s.handleSchema))
	mux.HandleFunc("POST /admin/schema", s.auth.RequireAdmin(s.handleSchemaPost))
	mux.HandleFunc("GET /admin/safety", s.auth.RequireAdmin(s.handleSafety))
	mux.HandleFunc("POST /admin/safety", s.auth.RequireAdmin(s.handleSafetyPost))
	mux.HandleFunc("GET /admin/llm", s.auth.RequireAdmin(s.handleLLM))
	mux.HandleFunc("POST /admin/llm", s.auth.RequireAdmin(s.handleLLMPost))
	mux.HandleFunc("GET /admin/glossary", s.auth.RequireAdmin(s.handleGlossary))
	mux.HandleFunc("POST /admin/glossary", s.auth.RequireAdmin(s.handleGlossaryPost))
	mux.HandleFunc("GET /admin/database", s.auth.RequireAdmin(s.handleDatabase))
}

// ──────────────────────────────────────────────────────────────────────────────
// Base template data
// ──────────────────────────────────────────────────────────────────────────────

// baseData is embedded in every page's template data.
type baseData struct {
	PageTitle   string
	ActivePage  string
	Username    string
	StatusClass string
	StatusText  string
	Flash       string
	FlashType   string // "success" | "error"
}

func (s *Service) newBase(r *http.Request, page, title string) baseData {
	username, _ := s.auth.SessionUsername(r)
	class, text := s.systemStatus(r.Context())
	return baseData{
		PageTitle:   title,
		ActivePage:  page,
		Username:    username,
		StatusClass: class,
		StatusText:  text,
	}
}

// systemStatus returns a CSS class and a human-readable status bar text.
func (s *Service) systemStatus(ctx context.Context) (cssClass, text string) {
	pingCtx, pingCancel := context.WithTimeout(ctx, 3*time.Second)
	defer pingCancel()
	dbOK := s.db.Ping(pingCtx) == nil

	llmCtx, llmCancel := context.WithTimeout(ctx, 3*time.Second)
	defer llmCancel()
	llmOK := s.sqlGen.Available(llmCtx)

	access := s.cfgMgr.Access()
	approvedCount := 0
	for _, sch := range access.ApprovedSchemas {
		approvedCount += len(sch.ApprovedTables)
	}

	var parts []string
	if dbOK {
		parts = append(parts, "DB: "+s.dbType+" ✅")
	} else {
		parts = append(parts, "DB: "+s.dbType+" ❌")
	}
	if llmOK {
		parts = append(parts, "LLM: "+s.sqlGen.Name()+" ✅")
	} else {
		parts = append(parts, "LLM: "+s.sqlGen.Name()+" ❌")
	}
	parts = append(parts, fmt.Sprintf("Tables: %d approved", approvedCount))

	overall := "ok"
	icon := "🟢 All systems healthy"
	if !dbOK || !llmOK {
		overall = "warn"
		icon = "🟡 Some systems degraded"
		if !dbOK && !llmOK {
			overall = "err"
			icon = "🔴 Systems unavailable"
		}
	}

	statusText := icon
	for _, p := range parts {
		statusText += " | " + p
	}
	return overall, statusText
}

func render(w http.ResponseWriter, tmplName string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("ERROR rendering template %s: %v", tmplName, err)
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Login page
// ──────────────────────────────────────────────────────────────────────────────

func (s *Service) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "login.html", map[string]string{"Error": ""}); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}// ──────────────────────────────────────────────────────────────────────────────
// Index
// ──────────────────────────────────────────────────────────────────────────────

func (s *Service) handleIndex(w http.ResponseWriter, r *http.Request) {
	base := s.newBase(r, "index", "Dashboard")
	render(w, "index.html", base)
}

// ──────────────────────────────────────────────────────────────────────────────
// Schema Access
// ──────────────────────────────────────────────────────────────────────────────

type schemaPageData struct {
	baseData
	Access           *config.AccessConfig
	FullSchema       *database.SchemaInfo
	ApprovedSchemaMap map[string]*config.ApprovedSchema
	ApprovedTableMap  map[string]*config.ApprovedTable
	HiddenSet        map[string]bool
}

func (s *Service) handleSchema(w http.ResponseWriter, r *http.Request) {
	base := s.newBase(r, "schema", "Schema Access")

	flash := r.URL.Query().Get("flash")
	flashType := r.URL.Query().Get("type")
	if flash != "" {
		base.Flash = flash
		base.FlashType = flashType
	}

	access := s.cfgMgr.Access()
	fullSchema, _ := s.crawler.Get(r.Context())

	// Build lookup maps for the template.
	approvedSchemas := make(map[string]*config.ApprovedSchema)
	approvedTables := make(map[string]*config.ApprovedTable)
	for i := range access.ApprovedSchemas {
		sch := &access.ApprovedSchemas[i]
		approvedSchemas[sch.Schema] = sch
		for j := range sch.ApprovedTables {
			tbl := &sch.ApprovedTables[j]
			approvedTables[sch.Schema+"."+tbl.Table] = tbl
		}
	}
	hidden := make(map[string]bool)
	for _, h := range access.HiddenSchemas {
		hidden[h.Schema] = true
	}

	render(w, "schema.html", schemaPageData{
		baseData:          base,
		Access:            access,
		FullSchema:        fullSchema,
		ApprovedSchemaMap: approvedSchemas,
		ApprovedTableMap:  approvedTables,
		HiddenSet:         hidden,
	})
}

func (s *Service) handleSchemaPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	action := r.FormValue("action")
	schemaName := r.FormValue("schema")
	tableName := r.FormValue("table")
	accessLevel := config.AccessLevel(r.FormValue("access_level"))
	reason := r.FormValue("reason")

	username, _ := s.auth.SessionUsername(r)
	now := time.Now().UTC()

	access := s.cfgMgr.Access()
	// Work on a copy.
	cfg := *access

	var flashMsg, flashType string

	switch action {
	case "approve_schema":
		// Remove from hidden if present.
		cfg.HiddenSchemas = removeHidden(cfg.HiddenSchemas, schemaName)
		// Add or update in approved.
		idx := findApprovedSchema(cfg.ApprovedSchemas, schemaName)
		entry := config.ApprovedSchema{
			Schema:      schemaName,
			ApprovedBy:  username,
			ApprovedAt:  now,
			Reason:      reason,
			AccessLevel: accessLevel,
		}
		if idx >= 0 {
			entry.ApprovedTables = cfg.ApprovedSchemas[idx].ApprovedTables
			cfg.ApprovedSchemas[idx] = entry
		} else {
			cfg.ApprovedSchemas = append(cfg.ApprovedSchemas, entry)
		}
		cfg.LastModifiedBy = username
		cfg.LastModifiedAt = now
		flashMsg = fmt.Sprintf("Schema %q approved.", schemaName)
		flashType = "success"

	case "deny_schema":
		cfg.ApprovedSchemas = removeApprovedSchema(cfg.ApprovedSchemas, schemaName)
		cfg.LastModifiedBy = username
		cfg.LastModifiedAt = now
		flashMsg = fmt.Sprintf("Schema %q removed from approved list.", schemaName)
		flashType = "success"

	case "hide_schema":
		cfg.HiddenSchemas = removeHidden(cfg.HiddenSchemas, schemaName)
		cfg.HiddenSchemas = append(cfg.HiddenSchemas, config.HiddenSchema{
			Schema:   schemaName,
			Reason:   reason,
			HiddenBy: username,
			HiddenAt: now,
		})
		cfg.LastModifiedBy = username
		cfg.LastModifiedAt = now
		flashMsg = fmt.Sprintf("Schema %q hidden.", schemaName)
		flashType = "success"

	case "unhide_schema":
		cfg.HiddenSchemas = removeHidden(cfg.HiddenSchemas, schemaName)
		cfg.LastModifiedBy = username
		cfg.LastModifiedAt = now
		flashMsg = fmt.Sprintf("Schema %q unhidden.", schemaName)
		flashType = "success"

	case "approve_table":
		idx := findApprovedSchema(cfg.ApprovedSchemas, schemaName)
		if idx < 0 {
			flashMsg = fmt.Sprintf("Schema %q is not approved; approve the schema first.", schemaName)
			flashType = "error"
			break
		}
		tIdx := findApprovedTable(cfg.ApprovedSchemas[idx].ApprovedTables, tableName)
		entry := config.ApprovedTable{
			Table:       tableName,
			ApprovedBy:  username,
			ApprovedAt:  now,
			Reason:      reason,
			AccessLevel: accessLevel,
		}
		if tIdx >= 0 {
			cfg.ApprovedSchemas[idx].ApprovedTables[tIdx] = entry
		} else {
			cfg.ApprovedSchemas[idx].ApprovedTables = append(cfg.ApprovedSchemas[idx].ApprovedTables, entry)
		}
		cfg.LastModifiedBy = username
		cfg.LastModifiedAt = now
		flashMsg = fmt.Sprintf("Table %q.%q approved.", schemaName, tableName)
		flashType = "success"

	case "deny_table":
		idx := findApprovedSchema(cfg.ApprovedSchemas, schemaName)
		if idx >= 0 {
			cfg.ApprovedSchemas[idx].ApprovedTables = removeApprovedTable(
				cfg.ApprovedSchemas[idx].ApprovedTables, tableName)
			cfg.LastModifiedBy = username
			cfg.LastModifiedAt = now
		}
		flashMsg = fmt.Sprintf("Table %q.%q removed from approved list.", schemaName, tableName)
		flashType = "success"

	default:
		flashMsg = "Unknown action: " + action
		flashType = "error"
	}

	if flashType != "error" {
		if err := s.cfgMgr.WriteAccess(&cfg); err != nil {
			log.Printf("ERROR writing access config: %v", err)
			flashMsg = "Failed to save: " + err.Error()
			flashType = "error"
		}
	}

	http.Redirect(w, r, "/admin/schema?flash="+urlEscape(flashMsg)+"&type="+flashType, http.StatusSeeOther)
}

// ──────────────────────────────────────────────────────────────────────────────
// Query Safety
// ──────────────────────────────────────────────────────────────────────────────

type safetyPageData struct {
	baseData
	Safety *config.SafetyConfig
}

func (s *Service) handleSafety(w http.ResponseWriter, r *http.Request) {
	base := s.newBase(r, "safety", "Query Safety")
	base.Flash = r.URL.Query().Get("flash")
	base.FlashType = r.URL.Query().Get("type")
	render(w, "safety.html", safetyPageData{baseData: base, Safety: s.cfgMgr.Safety()})
}

func (s *Service) handleSafetyPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	safety := *s.cfgMgr.Safety() // copy

	safety.QueryLimits.MaxQueryRows = parseInt(r, "max_query_rows", safety.QueryLimits.MaxQueryRows)
	safety.QueryLimits.DisplayRows = parseInt(r, "display_rows", safety.QueryLimits.DisplayRows)
	safety.QueryLimits.QueryTimeoutSeconds = parseInt(r, "query_timeout_seconds", safety.QueryLimits.QueryTimeoutSeconds)
	safety.QueryLimits.ComputeSummaryStats = r.FormValue("compute_summary_stats") == "on"

	safety.CostEstimation.ExplainBeforeExecute = r.FormValue("explain_before_execute") == "on"
	safety.CostEstimation.MaxEstimatedRows = int64(parseInt(r, "max_estimated_rows", int(safety.CostEstimation.MaxEstimatedRows)))
	safety.CostEstimation.MaxEstimatedCost = parseFloat(r, "max_estimated_cost", safety.CostEstimation.MaxEstimatedCost)

	safety.SelfCorrection.Enabled = r.FormValue("self_correction_enabled") == "on"
	safety.SelfCorrection.MaxRetries = parseInt(r, "max_retries", safety.SelfCorrection.MaxRetries)

	safety.Transparency.ShowSQLToUser = r.FormValue("show_sql_to_user") == "on"
	safety.Transparency.ShowRetriesToUser = r.FormValue("show_retries_to_user") == "on"
	safety.Transparency.ShowErrorsToUser = r.FormValue("show_errors_to_user") == "on"
	safety.Transparency.ShowStatusMessages = r.FormValue("show_status_messages") == "on"
	safety.Transparency.UseCollapsibleDetails = r.FormValue("use_collapsible_details") == "on"

	safety.RateLimiting.Enabled = r.FormValue("rate_limiting_enabled") == "on"
	safety.RateLimiting.RequestsPerMinutePerUser = parseInt(r, "requests_per_minute_per_user", safety.RateLimiting.RequestsPerMinutePerUser)
	safety.RateLimiting.RequestsPerMinuteGlobal = parseInt(r, "requests_per_minute_global", safety.RateLimiting.RequestsPerMinuteGlobal)

	safety.Learning.Enabled = r.FormValue("learning_enabled") == "on"
	safety.Learning.MaxCorrections = parseInt(r, "max_corrections", safety.Learning.MaxCorrections)
	safety.Learning.MaxCorrectionsInPrompt = parseInt(r, "max_corrections_in_prompt", safety.Learning.MaxCorrectionsInPrompt)

	flash, flashType := "Changes saved.", "success"
	if err := s.cfgMgr.WriteSafety(&safety); err != nil {
		log.Printf("ERROR writing safety config: %v", err)
		flash, flashType = "Failed to save: "+err.Error(), "error"
	}
	http.Redirect(w, r, "/admin/safety?flash="+urlEscape(flash)+"&type="+flashType, http.StatusSeeOther)
}

// ──────────────────────────────────────────────────────────────────────────────
// LLM Configuration
// ──────────────────────────────────────────────────────────────────────────────

type llmPageData struct {
	baseData
	LLM *config.LLMConfig
}

func (s *Service) handleLLM(w http.ResponseWriter, r *http.Request) {
	base := s.newBase(r, "llm", "LLM Configuration")
	base.Flash = r.URL.Query().Get("flash")
	base.FlashType = r.URL.Query().Get("type")
	render(w, "llm.html", llmPageData{baseData: base, LLM: s.cfgMgr.LLM()})
}

func (s *Service) handleLLMPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	llmCfg := *s.cfgMgr.LLM() // copy

	llmCfg.SQLGenerator.Provider = config.ProviderName(r.FormValue("sql_provider"))
	llmCfg.SQLGenerator.Copilot.Model = r.FormValue("sql_copilot_model")
	llmCfg.SQLGenerator.Ollama.URL = r.FormValue("sql_ollama_url")
	llmCfg.SQLGenerator.Ollama.Model = r.FormValue("sql_ollama_model")
	llmCfg.SQLGenerator.Ollama.TimeoutSeconds = parseInt(r, "sql_ollama_timeout", llmCfg.SQLGenerator.Ollama.TimeoutSeconds)
	llmCfg.SQLGenerator.Ollama.Temperature = parseFloat(r, "sql_ollama_temperature", llmCfg.SQLGenerator.Ollama.Temperature)
	llmCfg.SQLGenerator.Ollama.AutoPull = r.FormValue("sql_ollama_auto_pull") == "on"
	llmCfg.SQLGenerator.OpenAICompat.URL = r.FormValue("sql_compat_url")
	llmCfg.SQLGenerator.OpenAICompat.Model = r.FormValue("sql_compat_model")
	llmCfg.SQLGenerator.OpenAICompat.APIKeyEnv = r.FormValue("sql_compat_api_key_env")
	llmCfg.SQLGenerator.OpenAICompat.TimeoutSeconds = parseInt(r, "sql_compat_timeout", llmCfg.SQLGenerator.OpenAICompat.TimeoutSeconds)
	llmCfg.SQLGenerator.OpenAICompat.Temperature = parseFloat(r, "sql_compat_temperature", llmCfg.SQLGenerator.OpenAICompat.Temperature)

	llmCfg.Explainer.Provider = config.ProviderName(r.FormValue("exp_provider"))
	llmCfg.Explainer.Copilot.Model = r.FormValue("exp_copilot_model")
	llmCfg.Explainer.Ollama.URL = r.FormValue("exp_ollama_url")
	llmCfg.Explainer.Ollama.Model = r.FormValue("exp_ollama_model")
	llmCfg.Explainer.Ollama.TimeoutSeconds = parseInt(r, "exp_ollama_timeout", llmCfg.Explainer.Ollama.TimeoutSeconds)
	llmCfg.Explainer.Ollama.Temperature = parseFloat(r, "exp_ollama_temperature", llmCfg.Explainer.Ollama.Temperature)
	llmCfg.Explainer.OpenAICompat.URL = r.FormValue("exp_compat_url")
	llmCfg.Explainer.OpenAICompat.Model = r.FormValue("exp_compat_model")
	llmCfg.Explainer.OpenAICompat.APIKeyEnv = r.FormValue("exp_compat_api_key_env")
	llmCfg.Explainer.OpenAICompat.TimeoutSeconds = parseInt(r, "exp_compat_timeout", llmCfg.Explainer.OpenAICompat.TimeoutSeconds)
	llmCfg.Explainer.OpenAICompat.Temperature = parseFloat(r, "exp_compat_temperature", llmCfg.Explainer.OpenAICompat.Temperature)

	llmCfg.Fallback.Enabled = r.FormValue("fallback_enabled") == "on"
	llmCfg.Fallback.SQLGeneratorFallback = config.ProviderName(r.FormValue("sql_fallback"))
	llmCfg.Fallback.ExplainerFallback = config.ProviderName(r.FormValue("exp_fallback"))

	flash, flashType := "LLM configuration saved.", "success"
	if err := s.cfgMgr.WriteLLM(&llmCfg); err != nil {
		log.Printf("ERROR writing LLM config: %v", err)
		flash, flashType = "Failed to save: "+err.Error(), "error"
	}
	http.Redirect(w, r, "/admin/llm?flash="+urlEscape(flash)+"&type="+flashType, http.StatusSeeOther)
}

// ──────────────────────────────────────────────────────────────────────────────
// Business Glossary
// ──────────────────────────────────────────────────────────────────────────────

type glossaryPageData struct {
	baseData
	Glossary *config.GlossaryConfig
}

func (s *Service) handleGlossary(w http.ResponseWriter, r *http.Request) {
	base := s.newBase(r, "glossary", "Business Glossary")
	base.Flash = r.URL.Query().Get("flash")
	base.FlashType = r.URL.Query().Get("type")
	render(w, "glossary.html", glossaryPageData{baseData: base, Glossary: s.cfgMgr.Glossary()})
}

func (s *Service) handleGlossaryPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	username, _ := s.auth.SessionUsername(r)
	glossary := *s.cfgMgr.Glossary() // copy
	glossary.Terms = append([]config.GlossaryTerm(nil), glossary.Terms...) // deep copy slice

	action := r.FormValue("action")
	flash, flashType := "", "success"

	switch action {
	case "add":
		term := r.FormValue("term")
		definition := r.FormValue("definition")
		if term == "" || definition == "" {
			flash, flashType = "Term and definition are required.", "error"
			break
		}
		glossary.Terms = append(glossary.Terms, config.GlossaryTerm{
			Term:       term,
			Definition: definition,
			AddedBy:    username,
			AddedAt:    time.Now().UTC(),
		})
		flash = fmt.Sprintf("Term %q added.", term)

	case "remove":
		idx, err := strconv.Atoi(r.FormValue("index"))
		if err != nil || idx < 0 || idx >= len(glossary.Terms) {
			flash, flashType = "Invalid term index.", "error"
			break
		}
		removed := glossary.Terms[idx].Term
		glossary.Terms = append(glossary.Terms[:idx], glossary.Terms[idx+1:]...)
		flash = fmt.Sprintf("Term %q removed.", removed)

	default:
		flash, flashType = "Unknown action.", "error"
	}

	if flashType != "error" {
		if err := s.cfgMgr.WriteGlossary(&glossary); err != nil {
			log.Printf("ERROR writing glossary: %v", err)
			flash, flashType = "Failed to save: "+err.Error(), "error"
		}
	}
	http.Redirect(w, r, "/admin/glossary?flash="+urlEscape(flash)+"&type="+flashType, http.StatusSeeOther)
}

// ──────────────────────────────────────────────────────────────────────────────
// Database Settings
// ──────────────────────────────────────────────────────────────────────────────

type databasePageData struct {
	baseData
	DBType             string
	ConnStrEnv         string
	DBPingOK           bool
	ReadOnlyEnforce    bool
	SchemaCount        int
	TableCount         int
	ApprovedTableCount int
	AdminUsers         []string
}

func (s *Service) handleDatabase(w http.ResponseWriter, r *http.Request) {
	base := s.newBase(r, "database", "Database Settings")

	pingCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	pingOK := s.db.Ping(pingCtx) == nil

	adminCfg := s.cfgMgr.Admin()
	access := s.cfgMgr.Access()

	schemaCount, tableCount, approvedCount := 0, 0, 0
	if fullSchema, err := s.crawler.Get(r.Context()); err == nil && fullSchema != nil {
		schemaCount = len(fullSchema.Schemas)
		for _, sch := range fullSchema.Schemas {
			tableCount += len(sch.Tables)
		}
	}
	for _, sch := range access.ApprovedSchemas {
		approvedCount += len(sch.ApprovedTables)
	}

	render(w, "database.html", databasePageData{
		baseData:           base,
		DBType:             adminCfg.Database.Type,
		ConnStrEnv:         adminCfg.Database.ConnectionStringEnv,
		DBPingOK:           pingOK,
		ReadOnlyEnforce:    adminCfg.Database.ReadOnlyEnforce,
		SchemaCount:        schemaCount,
		TableCount:         tableCount,
		ApprovedTableCount: approvedCount,
		AdminUsers:         adminCfg.AdminUI.AllowedGithubUsers,
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────────────────────

func parseInt(r *http.Request, name string, fallback int) int {
	v, err := strconv.Atoi(r.FormValue(name))
	if err != nil || v < 0 {
		if r.FormValue(name) != "" {
			log.Printf("WARN admin: ignoring invalid value %q for field %q, using %d", r.FormValue(name), name, fallback)
		}
		return fallback
	}
	return v
}

func parseFloat(r *http.Request, name string, fallback float64) float64 {
	v, err := strconv.ParseFloat(r.FormValue(name), 64)
	if err != nil {
		return fallback
	}
	return v
}

func urlEscape(s string) string {
	return url.QueryEscape(s)
}

// ──────────────────────────────────────────────────────────────────────────────
// Access config helpers
// ──────────────────────────────────────────────────────────────────────────────

func findApprovedSchema(schemas []config.ApprovedSchema, name string) int {
	for i, s := range schemas {
		if s.Schema == name {
			return i
		}
	}
	return -1
}

func removeApprovedSchema(schemas []config.ApprovedSchema, name string) []config.ApprovedSchema {
	result := schemas[:0]
	for _, s := range schemas {
		if s.Schema != name {
			result = append(result, s)
		}
	}
	return result
}

func removeHidden(schemas []config.HiddenSchema, name string) []config.HiddenSchema {
	result := schemas[:0]
	for _, s := range schemas {
		if s.Schema != name {
			result = append(result, s)
		}
	}
	return result
}

func findApprovedTable(tables []config.ApprovedTable, name string) int {
	for i, t := range tables {
		if t.Table == name {
			return i
		}
	}
	return -1
}

func removeApprovedTable(tables []config.ApprovedTable, name string) []config.ApprovedTable {
	result := tables[:0]
	for _, t := range tables {
		if t.Table != name {
			result = append(result, t)
		}
	}
	return result
}
