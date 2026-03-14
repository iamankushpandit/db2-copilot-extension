package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// OllamaProvider implements TextToSQLProvider and ExplanationProvider using a
// local Ollama server.
type OllamaProvider struct {
	url     string
	model   string
	timeout time.Duration
	temp    float64
}

// NewOllamaProvider creates a new OllamaProvider.
func NewOllamaProvider(url, model string, timeoutSeconds int, temperature float64) *OllamaProvider {
	return &OllamaProvider{
		url:     url,
		model:   model,
		timeout: time.Duration(timeoutSeconds) * time.Second,
		temp:    temperature,
	}
}

// Name returns the provider identifier.
func (p *OllamaProvider) Name() string {
	return fmt.Sprintf("ollama/%s", p.model)
}

// Available returns true if the Ollama server is reachable.
func (p *OllamaProvider) Available(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url+"/api/tags", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// GenerateSQL generates SQL using the Ollama chat completion API.
func (p *OllamaProvider) GenerateSQL(ctx context.Context, req SQLGenerationRequest) (string, error) {
	prompt := BuildSQLPrompt(req)

	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	body, err := p.generate(ctx, prompt)
	if err != nil {
		return "", err
	}
	return extractSQL(body), nil
}

// ExplainResults streams an explanation using Ollama.
func (p *OllamaProvider) ExplainResults(ctx context.Context, req ExplanationRequest, w io.Writer) error {
	prompt := BuildExplanationSystemPrompt(req)

	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	return p.generateStream(ctx, prompt, w)
}

type ollamaGenerateRequest struct {
	Model  string  `json:"model"`
	Prompt string  `json:"prompt"`
	Stream bool    `json:"stream"`
	Options *ollamaOptions `json:"options,omitempty"`
}

type ollamaOptions struct {
	Temperature float64 `json:"temperature"`
}

type ollamaGenerateResponse struct {
	Response string `json:"response"`
}

func (p *OllamaProvider) generate(ctx context.Context, prompt string) (string, error) {
	reqBody := ollamaGenerateRequest{
		Model:  p.model,
		Prompt: prompt,
		Stream: false,
		Options: &ollamaOptions{Temperature: p.temp},
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling ollama request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url+"/api/generate", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("creating ollama request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("calling ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, body)
	}

	var result ollamaGenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding ollama response: %w", err)
	}
	return result.Response, nil
}

func (p *OllamaProvider) generateStream(ctx context.Context, prompt string, w io.Writer) error {
	reqBody := ollamaGenerateRequest{
		Model:  p.model,
		Prompt: prompt,
		Stream: true,
		Options: &ollamaOptions{Temperature: p.temp},
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshaling ollama request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url+"/api/generate", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("creating ollama request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("calling ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, body)
	}

	dec := json.NewDecoder(resp.Body)
	for {
		var chunk struct {
			Response string `json:"response"`
			Done     bool   `json:"done"`
		}
		if err := dec.Decode(&chunk); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("decoding ollama stream: %w", err)
		}
		if chunk.Response != "" {
			if _, err := fmt.Fprint(w, chunk.Response); err != nil {
				return err
			}
		}
		if chunk.Done {
			break
		}
	}
	return nil
}

// extractSQL extracts the first SQL statement from model output.
// Tries <sql>...</sql> tags, then a bare SQL block.
var sqlTagRe = regexp.MustCompile(`(?is)<sql>(.*?)</sql>`)

func extractSQL(text string) string {
	if m := sqlTagRe.FindStringSubmatch(text); len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	// Fall back: return the first non-empty line that starts with SELECT.
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(line), "SELECT") {
			return line
		}
	}
	return strings.TrimSpace(text)
}
