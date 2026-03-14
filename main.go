package main

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/iamankushpandit/db2-copilot-extension/agent"
	"github.com/iamankushpandit/db2-copilot-extension/config"
	"github.com/iamankushpandit/db2-copilot-extension/db2"
	"github.com/iamankushpandit/db2-copilot-extension/oauth"
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
	cfg, err := config.New()
	if err != nil {
		log.Fatalf("FATAL configuration error: %v", err)
	}

	publicKey, err := fetchPublicKey()
	if err != nil {
		log.Fatalf("FATAL could not fetch GitHub Copilot public key: %v", err)
	}
	log.Println("INFO fetched GitHub Copilot public key")

	db2Client, err := db2.NewClient(cfg.DB2ConnStr)
	if err != nil {
		log.Fatalf("FATAL could not create DB2 client: %v", err)
	}
	defer func() {
		if err := db2Client.Close(); err != nil {
			log.Printf("WARN closing DB2 client: %v", err)
		}
	}()
	log.Println("INFO DB2 client initialised")

	agentSvc := agent.NewService(db2Client, publicKey)
	oauthSvc := oauth.NewService(cfg.ClientID, cfg.ClientSecret, cfg.FQDN)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /agent", agentSvc.ChatCompletion)
	mux.HandleFunc("GET /auth/authorization", oauthSvc.PreAuth)
	mux.HandleFunc("GET /auth/callback", oauthSvc.PostAuth)
	mux.HandleFunc("GET /health", healthHandler)

	addr := ":" + cfg.Port
	log.Printf("INFO starting server on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("FATAL server error: %v", err)
	}
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
