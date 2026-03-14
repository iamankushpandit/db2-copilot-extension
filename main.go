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
	"time"

	"github.com/iamankushpandit/db2-copilot-extension/agent"
	"github.com/iamankushpandit/db2-copilot-extension/audit"
	"github.com/iamankushpandit/db2-copilot-extension/config"
	"github.com/iamankushpandit/db2-copilot-extension/oauth"
	"github.com/iamankushpandit/db2-copilot-extension/database"
	"github.com/iamankushpandit/db2-copilot-extension/postgres"
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
	if err := config.LoadAll("config"); err != nil {
		log.Fatalf("FATAL configuration error: %v", err)
	}
	cfg := config.Get()

	if err := audit.Init(&cfg.Safety.Audit); err != nil {
		log.Fatalf("FATAL audit logger initialization error: %v", err)
	}
	defer audit.Close()

	audit.Log(audit.SystemStart, audit.SystemStartPayload{ConfigSummary: "TODO"})

	dbClient, err := initDB(cfg.Admin.Database)
	if err != nil {
		log.Fatalf("FATAL database initialization error: %v", err)
	}
	defer dbClient.Close()

	sqlGenerator, explainer, err := initLLMProviders(cfg.LLM)
	if err != nil {
		log.Fatalf("FATAL LLM provider initialization error: %v", err)
	}

	publicKey, err := fetchPublicKey()
	if err != nil {
		log.Fatalf("FATAL could not fetch GitHub Copilot public key: %v", err)
	}
	log.Println("INFO fetched GitHub Copilot public key")

	agentSvc := agent.NewService(dbClient, publicKey, sqlGenerator, explainer)
	oauthSvc := oauth.NewService(os.Getenv(cfg.Admin.OAuth.ClientIDEnv), os.Getenv(cfg.Admin.OAuth.ClientSecretEnv), os.Getenv(cfg.Admin.OAuth.FQDNEnv))

	mux := http.NewServeMux()
	mux.HandleFunc("POST /agent", agentSvc.ChatCompletion)
	mux.HandleFunc("GET /auth/authorization", oauthSvc.PreAuth)
	mux.HandleFunc("GET /auth/callback", oauthSvc.PostAuth)
	mux.HandleFunc("GET /health", healthHandler)

	addr := fmt.Sprintf(":%d", cfg.Admin.AdminUI.Port)
	log.Printf("INFO starting server on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("FATAL server error: %v", err)
	}
}

func initLLMProviders(cfg *config.LLMConfig) (llm.TextToSQLProvider, llm.ExplanationProvider, error) {
	var sqlGenerator llm.TextToSQLProvider
	var explainer llm.ExplanationProvider
	var err error

	// TODO: Implement Ollama and fallback
	switch cfg.SQLGenerator.Provider {
	case "copilot":
		sqlGenerator = llm.NewCopilotProvider()
	default:
		return nil, nil, fmt.Errorf("unsupported SQL generator provider: %s", cfg.SQLGenerator.Provider)
	}

	switch cfg.Explainer.Provider {
	case "copilot":
		explainer = llm.NewCopilotProvider()
	default:
		return nil, nil, fmt.Errorf("unsupported explainer provider: %s", cfg.Explainer.Provider)
	}

	return sqlGenerator, explainer, err
}

func initDB(cfg config.Database) (database.Client, error) {
	connStr := os.Getenv(cfg.ConnectionStringEnv)
	if connStr == "" {
		return nil, fmt.Errorf("database connection string environment variable not set: %s", cfg.ConnectionStringEnv)
	}

	var client database.Client
	var err error

	switch cfg.Type {
	case "postgres":
		log.Println("INFO initialising postgres client")
		client, err = postgres.NewClient(connStr)
	case "db2":
		err = fmt.Errorf("db2 client not implemented yet") // TODO: implement
	default:
		err = fmt.Errorf("unsupported database type: %s", cfg.Type)
	}

	if err != nil {
		audit.Log(audit.DBConnectionFailed, map[string]string{"error": err.Error()})
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx); err != nil {
		audit.Log(audit.DBConnectionFailed, map[string]string{"error": err.Error()})
		return nil, fmt.Errorf("pinging database: %w", err)
	}
	audit.Log(audit.DBConnectionOK, nil)

	if err := client.VerifyReadOnly(); err != nil {
		// This is a warning, not a fatal error
		log.Printf("WARN read-only verification failed: %v", err)
	}

	return client, nil
}


// healthHandler returns a simple 200 OK response.
func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintln(w, `{"status":"ok"}`)
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
