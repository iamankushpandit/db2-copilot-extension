package agent

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"regexp"
	"strings"

	"github.com/iamankushpandit/db2-copilot-extension/copilot"
	"github.com/iamankushpandit/db2-copilot-extension/database"
)

// Service is the core agent that handles Copilot chat requests.
type Service struct {
	dbClient  database.Client
	publicKey string
	keyID     string
}

// NewService creates a new agent Service.
func NewService(dbClient database.Client, publicKey, keyID string) *Service {
	return &Service{
		dbClient:  dbClient,
		publicKey: publicKey,
		keyID:     keyID,
	}
}

// Handler returns an http.HandlerFunc that processes Copilot agent requests.
func (s *Service) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Read the request body for signature verification
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}

		// Verify the request signature from GitHub
		if err := s.verifySignature(r, bodyBytes); err != nil {
			log.Printf("signature verification failed: %v", err)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Extract API token from the Authorization header
		apiToken := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if apiToken == "" {
			http.Error(w, "missing authorization token", http.StatusUnauthorized)
			return
		}

		integrationID := r.Header.Get("Copilot-Integration-Id")

		var chatReq copilot.ChatRequest
		if err := json.Unmarshal(bodyBytes, &chatReq); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		// Set SSE headers
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		if err := s.generateCompletion(r.Context(), integrationID, apiToken, &chatReq, w); err != nil {
			log.Printf("error generating completion: %v", err)
			// Write error as SSE event
			errMsg := fmt.Sprintf("Error: %v", err)
			event := copilot.StreamEvent{
				Choices: []copilot.StreamChoice{
					{Delta: copilot.StreamDelta{Content: errMsg}},
				},
			}
			if data, encErr := copilot.SSEEvent(event); encErr == nil {
				fmt.Fprint(w, data)
			}
		}

		fmt.Fprint(w, copilot.SSEDone())
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}

// generateCompletion implements the two-step LLM workflow:
//  1. Generate SQL from the user's natural-language question
//  2. Execute the SQL and stream the explanation back to the user
func (s *Service) generateCompletion(ctx context.Context, integrationID, apiToken string, req *copilot.ChatRequest, w io.Writer) error {
	// Step 1: Retrieve schema information to give the LLM context
	schemaInfo, err := s.dbClient.GetSchemaInfo()
	if err != nil {
		log.Printf("warning: failed to get schema info: %v", err)
		schemaInfo = "(schema information unavailable)"
	}

	systemPrompt := s.buildSystemPrompt(schemaInfo)

	messages := []copilot.ChatMessage{{Role: "system", Content: systemPrompt}}
	messages = append(messages, req.Messages...)

	// Step 2: Ask the LLM to generate SQL (non-streaming)
	sqlGenReq := &copilot.ChatCompletionsRequest{
		Model:    copilot.ModelGPT4o,
		Messages: messages,
	}

	sqlResponse, err := copilot.ChatCompletions(ctx, integrationID, apiToken, sqlGenReq)
	if err != nil {
		return fmt.Errorf("failed to generate SQL: %w", err)
	}

	// Step 3: Extract SQL from the LLM response
	sqlQuery := extractSQL(sqlResponse)
	var queryResults string

	if sqlQuery != "" {
		log.Printf("executing %s query: %s", s.dbClient.DatabaseType(), sqlQuery)
		results, execErr := s.dbClient.ExecuteQuery(ctx, sqlQuery)
		if execErr != nil {
			log.Printf("query execution error: %v", execErr)
			queryResults = fmt.Sprintf("Query execution failed: %v", execErr)
		} else {
			queryResults = s.dbClient.FormatResults(results)
			log.Printf("query returned %d rows", len(results))
		}
	}

	// Step 4: Stream the final explanation back to the user
	finalMessages := append(messages, copilot.ChatMessage{
		Role:    "assistant",
		Content: sqlResponse,
	})

	if queryResults != "" {
		finalMessages = append(finalMessages, copilot.ChatMessage{
			Role: "system",
			Content: fmt.Sprintf(
				"The query was executed. Here are the results:\n\n%s\n\n"+
					"Please explain these results to the user in a clear, friendly way. "+
					"Use markdown formatting where appropriate.",
				queryResults,
			),
		})
	}

	streamReq := &copilot.ChatCompletionsRequest{
		Model:    copilot.ModelGPT4o,
		Messages: finalMessages,
		Stream:   true,
	}

	return copilot.StreamChatCompletions(ctx, integrationID, apiToken, streamReq, w)
}

// buildSystemPrompt creates a database-aware system prompt for the LLM.
func (s *Service) buildSystemPrompt(schemaInfo string) string {
	dbType := s.dbClient.DatabaseType()

	var syntaxNotes string
	switch dbType {
	case "postgres":
		syntaxNotes = `PostgreSQL-specific syntax:
- Use LIMIT n to limit rows (e.g., SELECT * FROM table LIMIT 10)
- Use OFFSET n for pagination
- Use NOW() or CURRENT_TIMESTAMP for current timestamp
- Use ILIKE for case-insensitive pattern matching
- Use :: for type casting (e.g., value::text)
- Use TRUE/FALSE for booleans`
	default: // db2
		syntaxNotes = `IBM DB2-specific syntax:
- Use FETCH FIRST n ROWS ONLY to limit rows (e.g., SELECT * FROM table FETCH FIRST 10 ROWS ONLY)
- Use CURRENT TIMESTAMP for current timestamp
- Use LIKE for pattern matching (case-sensitive by default)
- Use COALESCE or VALUE for null handling
- Schema-qualified table names: SCHEMA.TABLE`
	}

	return fmt.Sprintf(`You are a helpful %s database assistant for GitHub Copilot.
Your role is to translate natural language questions into %s SQL queries and explain the results.

DATABASE SCHEMA:
%s

SQL GUIDELINES:
- Only generate SELECT queries (no INSERT, UPDATE, DELETE, DROP, etc.)
- %s
- Wrap your SQL query in <sql>...</sql> tags
- If the user's question cannot be answered with a SQL query, explain why
- If you're unsure about table/column names, use the schema above as reference

RESPONSE FORMAT:
1. Brief explanation of what you'll query
2. The SQL query in <sql>...</sql> tags
3. After results are provided, explain them clearly in plain language
4. Use markdown tables or lists for structured data

Always be helpful, accurate, and concise.`,
		dbType, dbType, schemaInfo, syntaxNotes)
}

// extractSQL extracts the SQL query from an LLM response that wraps it in <sql>...</sql> tags.
var sqlTagRe = regexp.MustCompile(`(?is)<sql>(.*?)</sql>`)

func extractSQL(response string) string {
	match := sqlTagRe.FindStringSubmatch(response)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

// verifySignature verifies that the request came from GitHub using ECDSA signature verification.
func (s *Service) verifySignature(r *http.Request, body []byte) error {
	if s.publicKey == "" {
		log.Println("warning: no public key configured, skipping signature verification")
		return nil
	}

	sigHeader := r.Header.Get("Github-Public-Key-Signature")
	keyIDHeader := r.Header.Get("Github-Public-Key-Identifier")

	if sigHeader == "" || keyIDHeader == "" {
		return fmt.Errorf("missing signature headers")
	}

	if keyIDHeader != s.keyID {
		return fmt.Errorf("key ID mismatch: got %s, expected %s", keyIDHeader, s.keyID)
	}

	// Decode the PEM public key
	block, _ := pem.Decode([]byte(s.publicKey))
	if block == nil {
		return fmt.Errorf("failed to decode PEM public key")
	}

	pubKeyInterface, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse public key: %w", err)
	}

	ecKey, ok := pubKeyInterface.(*ecdsa.PublicKey)
	if !ok {
		return fmt.Errorf("public key is not an ECDSA key")
	}

	// Decode the base64 signature
	sigBytes, err := base64.StdEncoding.DecodeString(sigHeader)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}

	// Parse the ASN.1 signature
	var sig struct {
		R, S *big.Int
	}
	if _, err := asn1.Unmarshal(sigBytes, &sig); err != nil {
		return fmt.Errorf("failed to parse signature: %w", err)
	}

	// Verify the signature against the SHA-256 hash of the body
	hash := sha256.Sum256(body)
	if !ecdsa.Verify(ecKey, hash[:], sig.R, sig.S) {
		return fmt.Errorf("signature verification failed")
	}

	return nil
}
