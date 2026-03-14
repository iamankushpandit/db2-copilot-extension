package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/iamankushpandit/db2-copilot-extension/admin"
	"github.com/iamankushpandit/db2-copilot-extension/agent"
	"github.com/iamankushpandit/db2-copilot-extension/audit"
	"github.com/iamankushpandit/db2-copilot-extension/config"
	"github.com/iamankushpandit/db2-copilot-extension/database"
	"github.com/iamankushpandit/db2-copilot-extension/llm"
	"github.com/iamankushpandit/db2-copilot-extension/oauth"
	"github.com/iamankushpandit/db2-copilot-extension/pipeline"
	"github.com/iamankushpandit/db2-copilot-extension/postgres"
	"github.com/iamankushpandit/db2-copilot-extension/schema"
)

const publicKeysURL = "https://api.github.com/meta/public_keys/copilot_api"

type publicKeysResponse struct {
	PublicKeys []struct {
		Key       string `json:"key"`
		KeyID     string `json:"key_identifier"`
		IsCurrent bool   `json:"is_current"`
	} `json:"public_keys"`
}

func main() {
	// ── Step 1: Bootstrap config from environment variables ──────────────────
	cfg, err := config.New()
	if err != nil {
		log.Fatalf("FATAL configuration error: %v", err)
	}

	// ── Step 2: Load JSON config files (with hot-reload) ─────────────────────
	cfgMgr, err := config.NewManager(cfg.ConfigDir)
	if err != nil {
		log.Fatalf("FATAL could not load config files: %v", err)
	}
	defer cfgMgr.Close()
	log.Printf("INFO config loaded from %s", cfg.ConfigDir)

	safetyCfg := cfgMgr.Safety()
	adminCfg := cfgMgr.Admin()

	// ── Step 3: Start audit logger ────────────────────────────────────────────
	auditLogger, err := audit.NewLogger(safetyCfg.Audit.Directory, safetyCfg.Audit.Enabled)
	if err != nil {
		log.Fatalf("FATAL could not create audit logger: %v", err)
	}
	defer auditLogger.Close()

	auditLogger.Log(audit.EventConfigLoaded, "", "", nil)

	// ── Step 4: Connect to database ──────────────────────────────────────────
	dbType := adminCfg.Database.Type
	var dbClient database.Client

	switch dbType {
	case "postgres":
		connStr := cfg.DatabaseURL
		if connStr == "" {
			connStr = os.Getenv(adminCfg.Database.ConnectionStringEnv)
		}
		if connStr == "" {
			log.Fatalf("FATAL DATABASE_URL environment variable is required for postgres")
		}
		pgClient, pgErr := postgres.NewClient(connStr)
		if pgErr != nil {
			auditLogger.Log(audit.EventDBConnectionFailed, "", "", audit.DBConnectionDetails{DBType: "postgres", Error: pgErr.Error()})
			log.Fatalf("FATAL could not create PostgreSQL client: %v", pgErr)
		}
		defer func() {
			if closeErr := pgClient.Close(); closeErr != nil {
				log.Printf("WARN closing PostgreSQL client: %v", closeErr)
			}
		}()
		dbClient = pgClient
	default:
		// DB2 is imported separately via build tags; use a minimal shim for non-DB2 builds.
		log.Fatalf("FATAL unsupported database type %q — set database.type to 'postgres' in admin_config.json", dbType)
	}

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer pingCancel()
	if pingErr := dbClient.Ping(pingCtx); pingErr != nil {
		auditLogger.Log(audit.EventDBConnectionFailed, "", "", audit.DBConnectionDetails{DBType: dbType, Error: pingErr.Error()})
		log.Fatalf("FATAL database ping failed: %v", pingErr)
	}
	auditLogger.Log(audit.EventDBConnectionOK, "", "", audit.DBConnectionDetails{DBType: dbType})
	log.Printf("INFO connected to %s database", dbType)

	// ── Step 5: Verify read-only ──────────────────────────────────────────────
	roCtx, roCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer roCancel()
	readOnly, roErr := dbClient.VerifyReadOnly(roCtx)
	if roErr != nil {
		log.Printf("WARN could not verify read-only status: %v", roErr)
	} else if !readOnly {
		log.Printf("VERBOSE WARNING: The database user appears to have write permissions. " +
			"This connector is designed for read-only access only. " +
			"The operator accepts full responsibility for any unintended data modifications.")
		auditLogger.Log(audit.EventReadOnlyWarning, "", "", audit.ReadOnlyDetails{
			Verified: false,
			Warning:  "database user has write permissions",
			Disclaimer: "operator accepts responsibility for read-only enforcement",
		})
	} else {
		auditLogger.Log(audit.EventReadOnlyVerified, "", "", audit.ReadOnlyDetails{Verified: true})
		log.Println("INFO read-only status verified")
	}

	// ── Step 6: Fetch GitHub Copilot public key ───────────────────────────────
	publicKey, err := fetchPublicKey()
	if err != nil {
		log.Fatalf("FATAL could not fetch GitHub Copilot public key: %v", err)
	}
	log.Println("INFO fetched GitHub Copilot public key")

	// ── Step 7: Build LLM providers ──────────────────────────────────────────
	llmCfg := cfgMgr.LLM()
	sqlGen, explainer := buildLLMProviders(llmCfg, auditLogger)

	// ── Step 8: Build schema crawler and run initial crawl ───────────────────
	healthCfg := safetyCfg.Health
	crawler := schema.NewCrawler(dbClient, healthCfg.SchemaMaxAgeHours, healthCfg.SchemaAutoRefresh)

	crawlCtx, crawlCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer crawlCancel()

	crawlStart := time.Now()
	fullSchema, crawlErr := crawler.Get(crawlCtx)
	crawlDuration := time.Since(crawlStart)
	if crawlErr != nil {
		log.Printf("WARN initial schema crawl failed: %v", crawlErr)
	} else {
		schemaCount, tableCount, colCount := countSchemaInfo(fullSchema)
		auditLogger.Log(audit.EventSchemaCrawlComplete, "", "", audit.SchemaCrawlDetails{
			SchemaCount: schemaCount,
			TableCount:  tableCount,
			ColumnCount: colCount,
			DurationMS:  crawlDuration.Milliseconds(),
		})
		log.Printf("INFO schema crawled: %d schemas, %d tables, %d columns in %s",
			schemaCount, tableCount, colCount, crawlDuration.Round(time.Millisecond))
	}

	// Start background refresh.
	bgCtx, bgCancel := context.WithCancel(context.Background())
	defer bgCancel()
	crawler.Start(bgCtx)

	// ── Step 9: Load learned corrections ─────────────────────────────────────
	learningCfg := safetyCfg.Learning
	learningStore, lsErr := pipeline.NewLearningStore(learningCfg.CorrectionsFile, learningCfg.MaxCorrections)
	if lsErr != nil {
		log.Printf("WARN could not load corrections file: %v", lsErr)
		learningStore, _ = pipeline.NewLearningStore("", learningCfg.MaxCorrections)
	} else {
		auditLogger.Log(audit.EventCorrectionsLoaded, "", "", nil)
		log.Println("INFO learned corrections loaded")
	}

	// ── Step 10: Build rate limiter ───────────────────────────────────────────
	rlCfg := safetyCfg.RateLimiting
	rateLimiter := pipeline.NewRateLimiter(rlCfg.Enabled, rlCfg.RequestsPerMinutePerUser, rlCfg.RequestsPerMinuteGlobal)

	// ── Step 11: Build pipeline executor ─────────────────────────────────────
	executor := pipeline.NewExecutor(
		dbClient,
		crawler,
		cfgMgr,
		sqlGen,
		explainer,
		learningStore,
		rateLimiter,
		auditLogger,
	)

	// ── Step 12: Wire HTTP handlers ───────────────────────────────────────────
	agentSvc := agent.NewService(executor, publicKey)
	oauthSvc := oauth.NewService(cfg.ClientID, cfg.ClientSecret, cfg.FQDN)

	// ── Step 12a: Build admin UI ──────────────────────────────────────────────
	authSvc := admin.NewAuthService(cfg.ClientID, cfg.ClientSecret, cfg.FQDN, cfg.AdminSessionSecret, cfgMgr)
	adminSvc := admin.NewService(cfgMgr, crawler, dbClient, sqlGen, authSvc, dbType)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /agent", agentSvc.ChatCompletion)
	mux.HandleFunc("GET /auth/authorization", oauthSvc.PreAuth)
	mux.HandleFunc("GET /auth/callback", oauthSvc.PostAuth)
	mux.HandleFunc("GET /health", healthHandler)
	adminSvc.RegisterRoutes(mux)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: mux,
	}

	// ── Step 13: Graceful shutdown ────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-quit
		log.Println("INFO shutting down — waiting up to 30 seconds for in-flight requests")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		if shutdownErr := srv.Shutdown(shutdownCtx); shutdownErr != nil {
			log.Printf("WARN server shutdown error: %v", shutdownErr)
		}
		bgCancel()
	}()

	auditLogger.Log(audit.EventSystemStart, "", "", audit.SystemStartDetails{
		DBType:      dbType,
		LLMProvider: sqlGen.Name(),
		Port:        cfg.Port,
	})
	log.Printf("INFO server ready on :%s (db=%s, llm=%s)", cfg.Port, dbType, sqlGen.Name())

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("FATAL server error: %v", err)
	}
	log.Println("INFO server stopped")
}

// healthHandler returns a simple 200 OK response.
func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintln(w, `{"status":"ok"}`)
}

// buildLLMProviders creates SQL generation and explanation providers based on config.
func buildLLMProviders(llmCfg *config.LLMConfig, auditLogger *audit.Logger) (llm.TextToSQLProvider, llm.ExplanationProvider) {
	onFallback := func(failed, used string) {
		auditLogger.Log(audit.EventLLMFallback, "", "", audit.LLMFallbackDetails{
			FailedProvider:   failed,
			FallbackProvider: used,
		})
	}

	// SQL generator.
	var sqlGen llm.TextToSQLProvider
	copilotSQL := llm.NewCopilotProvider(llmCfg.SQLGenerator.Copilot.Model)
	switch llmCfg.SQLGenerator.Provider {
	case config.ProviderOllama:
		ollamaCfg := llmCfg.SQLGenerator.Ollama
		ollamaSQL := llm.NewOllamaProvider(ollamaCfg.URL, ollamaCfg.Model, ollamaCfg.TimeoutSeconds, ollamaCfg.Temperature)
		if llmCfg.Fallback.Enabled {
			sqlGen = llm.NewFallbackSQLProvider(ollamaSQL, copilotSQL, onFallback)
		} else {
			sqlGen = ollamaSQL
		}
	case config.ProviderOpenAICompat:
		compatCfg := llmCfg.SQLGenerator.OpenAICompat
		apiKey := os.Getenv(compatCfg.APIKeyEnv)
		compatSQL := llm.NewOpenAICompatProvider(compatCfg.URL, compatCfg.Model, apiKey, compatCfg.TimeoutSeconds, compatCfg.Temperature)
		if llmCfg.Fallback.Enabled {
			sqlGen = llm.NewFallbackSQLProvider(compatSQL, copilotSQL, onFallback)
		} else {
			sqlGen = compatSQL
		}
	default:
		sqlGen = copilotSQL
	}

	// Explainer.
	var explainer llm.ExplanationProvider
	copilotExp := llm.NewCopilotProvider(llmCfg.Explainer.Copilot.Model)
	switch llmCfg.Explainer.Provider {
	case config.ProviderOllama:
		ollamaCfg := llmCfg.Explainer.Ollama
		ollamaExp := llm.NewOllamaProvider(ollamaCfg.URL, ollamaCfg.Model, ollamaCfg.TimeoutSeconds, ollamaCfg.Temperature)
		if llmCfg.Fallback.Enabled {
			explainer = llm.NewFallbackExplanationProvider(ollamaExp, copilotExp, onFallback)
		} else {
			explainer = ollamaExp
		}
	case config.ProviderOpenAICompat:
		compatCfg := llmCfg.Explainer.OpenAICompat
		apiKey := os.Getenv(compatCfg.APIKeyEnv)
		compatExp := llm.NewOpenAICompatProvider(compatCfg.URL, compatCfg.Model, apiKey, compatCfg.TimeoutSeconds, compatCfg.Temperature)
		if llmCfg.Fallback.Enabled {
			explainer = llm.NewFallbackExplanationProvider(compatExp, copilotExp, onFallback)
		} else {
			explainer = compatExp
		}
	default:
		explainer = copilotExp
	}

	return sqlGen, explainer
}

// countSchemaInfo counts schemas, tables, and columns in a SchemaInfo.
func countSchemaInfo(info *database.SchemaInfo) (schemas, tables, columns int) {
	if info == nil {
		return 0, 0, 0
	}
	schemas = len(info.Schemas)
	for _, s := range info.Schemas {
		tables += len(s.Tables)
		for _, t := range s.Tables {
			columns += len(t.Columns)
		}
	}
	return schemas, tables, columns
}

// fetchPublicKey downloads the current GitHub Copilot ECDSA public key.
func fetchPublicKey() (*ecdsa.PublicKey, error) {
	resp, err := http.Get(publicKeysURL) //nolint:noctx // startup, not a hot path
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", publicKeysURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	var keysResp publicKeysResponse
	if err := json.Unmarshal(body, &keysResp); err != nil {
		return nil, fmt.Errorf("unmarshaling public keys response: %w", err)
	}

	for _, k := range keysResp.PublicKeys {
		if k.IsCurrent {
			return parseECDSAKey(k.Key)
		}
	}

	// Fall back to the first key if none is marked current.
	if len(keysResp.PublicKeys) > 0 {
		return parseECDSAKey(keysResp.PublicKeys[0].Key)
	}

	return nil, fmt.Errorf("no public keys found in response")
}

// parseECDSAKey parses a PEM-encoded ECDSA public key.
func parseECDSAKey(pemKey string) (*ecdsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemKey))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsing PKIX public key: %w", err)
	}

	ecKey, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("public key is not ECDSA (got %T)", pub)
	}

	return ecKey, nil
}
