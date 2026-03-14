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

	"github.com/iamankushpandit/db2-copilot-extension/config"
	"github.com/iamankushpandit/db2-copilot-extension/database"
	"github.com/iamankushpandit/db2-copilot-extension/llm"
)

// Service is the core agent service that handles chat completion requests.
type Service struct {
	dbClient          database.Client
	publicKey         *ecdsa.PublicKey
	sqlGenerator      llm.TextToSQLProvider
	explainer         llm.ExplanationProvider
}

// NewService creates a new agent Service.
func NewService(dbClient database.Client, publicKey *ecdsa.PublicKey, sqlGenerator llm.TextToSQLProvider, explainer llm.ExplanationProvider) *Service {
	return &Service{
		dbClient:          dbClient,
		publicKey:         publicKey,
		sqlGenerator:      sqlGenerator,
		explainer:         explainer,
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
	// 1. Fetch schema info (cached after first call).
	schema, err := s.dbClient.GetTier1Schema(ctx)
	if err != nil {
		log.Printf("WARN could not fetch schema info: %v", err)
		schema = &database.Schema{} // empty schema
	}

	// TODO: Get this from a more appropriate place
	userQuestion := req.Messages[len(req.Messages)-1].Content

	// 2. Ask the LLM to generate SQL.
	sqlGenReq := llm.SQLGenerationRequest{
		Prompt: userQuestion,
		Schema: schema,
		// TODO: add glossary
	}

	sqlQuery, err := s.sqlGenerator.GenerateSQL(ctx, sqlGenReq)
	if err != nil {
		return fmt.Errorf("calling LLM for SQL generation: %w", err)
	}

	if sqlQuery == "" {
		// No SQL found — stream the LLM's answer directly.
		// TODO: This part needs to be refactored to not use the copilot package directly.
		return streamDirect(ctx, integrationID, apiToken, req.Messages, w)
	}

	// 5. Sanitize and validate the SQL.
	sanitizedSQL, err := database.SanitizeSQL(sqlQuery)
	if err != nil {
		log.Printf("WARN SQL sanitization failed: %v (query: %q)", err, sqlQuery)
		return streamError(w, fmt.Sprintf("The generated SQL query could not be executed safely: %v", err))
	}

	accessConfig := config.Get().Access
	if err := database.ValidateSQL(sanitizedSQL, accessConfig); err != nil {
		log.Printf("WARN SQL validation failed: %v (query: %q)", err, sanitizedSQL)
		return streamError(w, fmt.Sprintf("The generated SQL query is not allowed: %v", err))
	}

	// 6. Estimate query cost.
	costConfig := &config.Get().Safety.CostEstimation
	if costConfig.ExplainBeforeExecute {
		cost, err := s.dbClient.EstimateQueryCost(ctx, sanitizedSQL)
		if err != nil {
			log.Printf("WARN query cost estimation failed: %v", err)
			// Decide if this should be a fatal error. For now, we'll continue.
		} else {
			if cost.EstimatedRows > costConfig.MaxEstimatedRows || cost.EstimatedCost > costConfig.MaxEstimatedCost {
				// TODO: Implement self-correction
				return streamError(w, "Query is too expensive. Please try to narrow your question.")
			}
		}
	}

	// 7. Inject limit.
	queryLimits := &config.Get().Safety.QueryLimits
	limitedSQL, err := database.InjectLimit(sanitizedSQL, queryLimits)
	if err != nil {
		// This should not happen with the current regex-based implementation, but good practice to have it.
		log.Printf("WARN SQL limit injection failed: %v", err)
		return streamError(w, fmt.Sprintf("Could not inject a limit into the SQL query: %v", err))
	}

	// 8. Execute against the database.
	results, err := s.dbClient.ExecuteQuery(ctx, limitedSQL)
	if err != nil {
		log.Printf("ERROR query failed: %v (query: %q)", err, limitedSQL)
		return streamError(w, fmt.Sprintf("The query failed to execute: %v", err))
	}

	// 9. Explain the results.
	// TODO: implement this properly
	formattedResults, err := json.Marshal(results)
	if err != nil {
		return fmt.Errorf("marshaling results: %w", err)
	}

	_, err = io.WriteString(w, string(formattedResults))
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
