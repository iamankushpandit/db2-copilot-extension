package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const copilotChatURL = "https://api.githubcopilot.com/chat/completions"

// CopilotProvider implements TextToSQLProvider and ExplanationProvider using
// the GitHub Copilot API with the request token.
type CopilotProvider struct {
	model string
}

// NewCopilotProvider creates a new CopilotProvider.
func NewCopilotProvider(model string) *CopilotProvider {
	return &CopilotProvider{model: model}
}

// Name returns the provider identifier.
func (p *CopilotProvider) Name() string {
	return fmt.Sprintf("copilot/%s", p.model)
}

// Available returns true if the Copilot API is reachable (no token needed for
// health check — we assume it's always available if we have a token at
// request time).
func (p *CopilotProvider) Available(_ context.Context) bool {
	return true
}

// GenerateSQL generates SQL using the Copilot chat completions API.
func (p *CopilotProvider) GenerateSQL(ctx context.Context, req SQLGenerationRequest) (string, error) {
	systemPrompt := BuildSystemPrompt(req)
	messages := []copilotMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: req.Question},
	}

	resp, err := p.complete(ctx, req.CopilotToken, req.CopilotIntegrationID, messages, false)
	if err != nil {
		return "", err
	}
	defer resp.Close()

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding copilot SQL response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices returned from Copilot API")
	}

	content := result.Choices[0].Message.Content
	return extractSQL(content), nil
}

// ExplainResults streams an explanation using the Copilot API.
func (p *CopilotProvider) ExplainResults(ctx context.Context, req ExplanationRequest, w io.Writer) error {
	systemPrompt := BuildExplanationSystemPrompt(req)
	messages := []copilotMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: req.Question},
	}

	stream, err := p.complete(ctx, req.CopilotToken, req.CopilotIntegrationID, messages, true)
	if err != nil {
		return err
	}
	defer stream.Close()

	_, err = io.Copy(w, stream)
	return err
}

type copilotMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type copilotRequest struct {
	Model    string           `json:"model"`
	Messages []copilotMessage `json:"messages"`
	Stream   bool             `json:"stream"`
}

func (p *CopilotProvider) complete(ctx context.Context, token, integrationID string, messages []copilotMessage, stream bool) (io.ReadCloser, error) {
	reqBody := copilotRequest{
		Model:    p.model,
		Messages: messages,
		Stream:   stream,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling copilot request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, copilotChatURL, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("creating copilot request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)
	if integrationID != "" {
		httpReq.Header.Set("Copilot-Integration-Id", integrationID)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("calling copilot API: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("copilot API returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return resp.Body, nil
}
