package agent

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"regexp"
	"strings"

	"github.com/iamankushpandit/db2-copilot-extension/copilot"
	"github.com/iamankushpandit/db2-copilot-extension/db2"
)

// Service is the core agent service that handles chat completion requests.
type Service struct {
	db2Client *db2.Client
	publicKey *ecdsa.PublicKey
}

// NewService creates a new agent Service.
func NewService(db2Client *db2.Client, publicKey *ecdsa.PublicKey) *Service {
	return &Service{
		db2Client: db2Client,
		publicKey: publicKey,
	}
}

// ChatCompletion is the HTTP handler for POST /agent.
func (s *Service) ChatCompletion(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("ERROR reading request body: %v", err)
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Verify the request came from GitHub.
	sig := r.Header.Get("Github-Public-Key-Signature")
	if !s.validPayload(body, sig) {
		log.Println("ERROR invalid payload signature")
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	apiToken := r.Header.Get("X-GitHub-Token")
	integrationID := r.Header.Get("Copilot-Integration-Id")

	var req copilot.ChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		log.Printf("ERROR unmarshaling request: %v", err)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	if err := s.generateCompletion(r.Context(), integrationID, apiToken, &req, w); err != nil {
		log.Printf("ERROR generating completion: %v", err)
		// Try to send an error SSE event so the user sees a message.
		_ = streamError(w, "I encountered an error while processing your request. Please try again.")
	}
}

// generateCompletion orchestrates SQL generation, execution, and SSE streaming.
func (s *Service) generateCompletion(ctx context.Context, integrationID, apiToken string, req *copilot.ChatRequest, w io.Writer) error {
	// 1. Fetch DB2 schema info (cached after first call).
	schemaInfo, err := s.db2Client.GetSchemaInfo()
	if err != nil {
		log.Printf("WARN could not fetch schema info: %v", err)
		schemaInfo = "(schema unavailable)"
	}

	// 2. Build system prompt with DB2 context.
	systemPrompt := `You are a helpful IBM DB2 database assistant. Your job is to translate the user's natural language questions into DB2-compatible SQL queries, execute them, and explain the results clearly.

Here is the current database schema:

` + schemaInfo + `

Rules:
- Always generate DB2-compatible SQL syntax.
- Wrap any SQL query in <sql>...</sql> tags.
- Only generate SELECT statements — never INSERT, UPDATE, DELETE, or DDL.
- If the user asks a question that requires a query, produce exactly one <sql>...</sql> block.
- If no query is needed (e.g., a general question), answer directly without <sql> tags.
- After presenting results, explain them in plain English.`

	messages := []copilot.ChatMessage{
		{Role: "system", Content: systemPrompt},
	}
	messages = append(messages, req.Messages...)

	// 3. Ask the LLM to generate SQL (non-streaming).
	sqlGenReq := &copilot.ChatCompletionsRequest{
		Model:    copilot.ModelGPT4,
		Messages: messages,
		Stream:   false,
	}

	sqlResp, err := copilot.ChatCompletionsNonStreaming(ctx, integrationID, apiToken, sqlGenReq)
	if err != nil {
		return fmt.Errorf("calling Copilot LLM for SQL generation: %w", err)
	}

	if len(sqlResp.Choices) == 0 {
		return fmt.Errorf("no choices returned from Copilot LLM")
	}

	llmText := sqlResp.Choices[0].Message.Content

	// 4. Extract SQL from <sql>...</sql> tags.
	sqlQuery := extractSQL(llmText)
	if sqlQuery == "" {
		// No SQL found — stream the LLM's answer directly.
		return streamDirect(ctx, integrationID, apiToken, messages, w)
	}

	// 5. Sanitize the SQL.
	sanitizedSQL, err := db2.SanitizeSQL(sqlQuery)
	if err != nil {
		log.Printf("WARN SQL sanitization failed: %v (query: %q)", err, sqlQuery)
		return streamError(w, fmt.Sprintf("The generated SQL query could not be executed safely: %v", err))
	}

	// 6. Execute against DB2.
	results, err := s.db2Client.ExecuteQuery(ctx, sanitizedSQL)
	if err != nil {
		log.Printf("ERROR DB2 query failed: %v (query: %q)", err, sanitizedSQL)
		return streamError(w, fmt.Sprintf("The query failed to execute: %v", err))
	}

	// 7. Build a follow-up prompt with the results.
	formattedResults := db2.FormatResults(results)
	followUpMessages := append(messages, //nolint:gocritic // intentional append to copy
		copilot.ChatMessage{Role: "assistant", Content: llmText},
		copilot.ChatMessage{
			Role: "system",
			Content: fmt.Sprintf("The query `%s` was executed successfully. Here are the results:\n\n%s\n\nNow explain these results to the user in a clear, concise, and well-formatted way. Use markdown where helpful.",
				sanitizedSQL, formattedResults),
		},
	)

	// 8. Stream the final explanation back to the user.
	streamReq := &copilot.ChatCompletionsRequest{
		Model:    copilot.ModelGPT4,
		Messages: followUpMessages,
		Stream:   true,
	}

	stream, err := copilot.ChatCompletions(ctx, integrationID, apiToken, streamReq)
	if err != nil {
		return fmt.Errorf("calling Copilot LLM for streaming response: %w", err)
	}
	defer stream.Close()

	_, err = io.Copy(w, stream)
	return err
}

// streamDirect streams an LLM response for messages that required no SQL.
func streamDirect(ctx context.Context, integrationID, apiToken string, messages []copilot.ChatMessage, w io.Writer) error {
	streamReq := &copilot.ChatCompletionsRequest{
		Model:    copilot.ModelGPT4,
		Messages: messages,
		Stream:   true,
	}

	stream, err := copilot.ChatCompletions(ctx, integrationID, apiToken, streamReq)
	if err != nil {
		return fmt.Errorf("calling Copilot LLM for direct streaming: %w", err)
	}
	defer stream.Close()

	_, err = io.Copy(w, stream)
	return err
}

// streamError writes a synthetic SSE error event so the user sees an error message.
func streamError(w io.Writer, msg string) error {
	payload := fmt.Sprintf("data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\ndata: [DONE]\n\n", msg)
	_, err := fmt.Fprint(w, payload)
	return err
}

// extractSQL parses the first <sql>...</sql> block from text.
func extractSQL(text string) string {
	re := regexp.MustCompile(`(?is)<sql>(.*?)</sql>`)
	match := re.FindStringSubmatch(text)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

// validPayload verifies that the request body was signed by GitHub using
// ECDSA with SHA-256.
func (s *Service) validPayload(payload []byte, sigHeader string) bool {
	if s.publicKey == nil || sigHeader == "" {
		return false
	}

	sigBytes, err := base64.StdEncoding.DecodeString(sigHeader)
	if err != nil {
		log.Printf("WARN signature base64 decode failed: %v", err)
		return false
	}

	// The signature is ASN.1 DER-encoded (r, s).
	var ecSig struct {
		R, S *big.Int
	}
	if _, err := asn1.Unmarshal(sigBytes, &ecSig); err != nil {
		log.Printf("WARN ASN.1 unmarshal failed: %v", err)
		return false
	}

	hash := sha256.Sum256(payload)
	return ecdsa.Verify(s.publicKey, hash[:], ecSig.R, ecSig.S)
}
