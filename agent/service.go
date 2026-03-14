package agent

import (
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

	"github.com/iamankushpandit/db2-copilot-extension/copilot"
	"github.com/iamankushpandit/db2-copilot-extension/pipeline"
)

// Service is the core agent service that handles chat completion requests.
type Service struct {
	executor  *pipeline.Executor
	publicKey *ecdsa.PublicKey
}

// NewService creates a new agent Service backed by the given pipeline executor.
func NewService(executor *pipeline.Executor, publicKey *ecdsa.PublicKey) *Service {
	return &Service{
		executor:  executor,
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
	githubUser := r.Header.Get("X-Github-Login")
	githubUID := r.Header.Get("X-Github-Id")

	var req copilot.ChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		log.Printf("ERROR unmarshaling request: %v", err)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Extract the last user message as the question.
	question := lastUserMessage(req.Messages)
	if question == "" {
		http.Error(w, "no user message found", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	pipelineReq := pipeline.Request{
		Question:             question,
		GitHubUser:           githubUser,
		GitHubUID:            githubUID,
		CopilotToken:         apiToken,
		CopilotIntegrationID: integrationID,
	}

	if err := s.executor.Execute(r.Context(), pipelineReq, w); err != nil {
		log.Printf("ERROR pipeline execution: %v", err)
		_ = streamError(w, "I encountered an error while processing your request. Please try again.")
	}
}

// streamError writes a synthetic SSE error event so the user sees an error message.
func streamError(w io.Writer, msg string) error {
	payload := fmt.Sprintf("data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\ndata: [DONE]\n\n", msg)
	_, err := fmt.Fprint(w, payload)
	return err
}

// lastUserMessage returns the content of the last message with role "user".
func lastUserMessage(messages []copilot.ChatMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i].Content
		}
	}
	return ""
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
