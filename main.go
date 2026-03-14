package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/iamankushpandit/db2-copilot-extension/agent"
	"github.com/iamankushpandit/db2-copilot-extension/config"
	"github.com/iamankushpandit/db2-copilot-extension/copilot"
	"github.com/iamankushpandit/db2-copilot-extension/database"
	"github.com/iamankushpandit/db2-copilot-extension/db2"
	"github.com/iamankushpandit/db2-copilot-extension/oauth"
	"github.com/iamankushpandit/db2-copilot-extension/postgres"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}

	// Create the database client based on the configured DB type
	dbClient, err := newDatabaseClient(cfg)
	if err != nil {
		log.Fatalf("failed to create database client: %v", err)
	}
	defer dbClient.Close()

	// Verify database connectivity at startup
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := dbClient.Ping(ctx); err != nil {
		log.Printf("warning: database ping failed: %v (will retry on first request)", err)
	} else {
		log.Printf("connected to %s database", dbClient.DatabaseType())
	}

	// Fetch the Copilot public key for signature verification
	publicKey, keyID, err := copilot.GetPublicKey(context.Background(), cfg.ClientID)
	if err != nil {
		log.Printf("warning: failed to fetch Copilot public key: %v (signature verification disabled)", err)
	} else {
		log.Printf("fetched Copilot public key (key ID: %s)", keyID)
	}

	// Wire up services
	agentSvc := agent.NewService(dbClient, publicKey, keyID)
	oauthSvc := oauth.NewService(cfg)

	// Register HTTP routes
	mux := http.NewServeMux()
	mux.HandleFunc("/agent", agentSvc.Handler())
	mux.HandleFunc("/auth/authorization", oauthSvc.HandleAuthorization)
	mux.HandleFunc("/auth/callback", oauthSvc.HandleCallback)
	mux.HandleFunc("/health", healthHandler(dbClient))

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("server starting on port %s (DB type: %s)", cfg.Port, dbClient.DatabaseType())
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Graceful shutdown on signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down server...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}
	log.Println("server stopped")
}

// newDatabaseClient creates a database.Client based on the configured DB type.
func newDatabaseClient(cfg *config.Config) (database.Client, error) {
	switch cfg.DBType {
	case "db2":
		client, err := db2.NewClient(cfg.DB2ConnStr)
		if err != nil {
			return nil, fmt.Errorf("failed to create DB2 client: %w", err)
		}
		return client, nil
	case "postgres":
		client, err := postgres.NewClient(cfg.PGConnStr)
		if err != nil {
			return nil, fmt.Errorf("failed to create PostgreSQL client: %w", err)
		}
		return client, nil
	default:
		return nil, fmt.Errorf("unsupported database type: %s", cfg.DBType)
	}
}

// healthHandler returns an HTTP handler that reports database connectivity.
func healthHandler(dbClient database.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		status := "ok"
		code := http.StatusOK
		if err := dbClient.Ping(ctx); err != nil {
			status = fmt.Sprintf("database ping failed: %v", err)
			code = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		fmt.Fprintf(w, `{"status":%q,"database":%q}`, status, dbClient.DatabaseType())
	}
}
