package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAICompatProvider implements TextToSQLProvider and ExplanationProvider for
// any server that speaks the OpenAI chat completions format — including vLLM,
// LocalAI, LM Studio, Ollama's OpenAI-compatible endpoint, and others.
type OpenAICompatProvider struct {
	baseURL string
	apiKey  string
	model   string
	timeout time.Duration
	temp    float64
}

// NewOpenAICompatProvider creates an OpenAICompatProvider.
// baseURL should include the path prefix, e.g. "http://localhost:8000/v1".
// apiKey may be empty for servers that require no authentication.
func NewOpenAICompatProvider(baseURL, model, apiKey string, timeoutSeconds int, temperature float64) *OpenAICompatProvider {
	return &OpenAICompatProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		timeout: time.Duration(timeoutSeconds) * time.Second,
		temp:    temperature,
	}
}

// Name returns the provider identifier.
func (p *OpenAICompatProvider) Name() string {
	return fmt.Sprintf("openai_compat/%s", p.model)
}

// Available returns true if the server is reachable.
func (p *OpenAICompatProvider) Available(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/models", nil)
	if err != nil {
		return false
	}
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// GenerateSQL generates SQL using the OpenAI chat completions API.
func (p *OpenAICompatProvider) GenerateSQL(ctx context.Context, req SQLGenerationRequest) (string, error) {
	systemPrompt := BuildSystemPrompt(req)

	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	body, err := p.complete(ctx, []openAIMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: req.Question},
	}, false)
	if err != nil {
		return "", err
	}
	defer body.Close()

	var result openAIResponse
	if err := json.NewDecoder(body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding openai-compat SQL response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices returned from openai-compat API")
	}
	return extractSQL(result.Choices[0].Message.Content), nil
}

// ExplainResults streams an explanation using the OpenAI chat completions API.
func (p *OpenAICompatProvider) ExplainResults(ctx context.Context, req ExplanationRequest, w io.Writer) error {
	systemPrompt := BuildExplanationSystemPrompt(req)

	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	stream, err := p.complete(ctx, []openAIMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: req.Question},
	}, true)
	if err != nil {
		return err
	}
	defer stream.Close()

	return parseOpenAIStream(stream, w)
}

// openAIMessage is a single message in the chat.
type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIRequest is the request body for the chat completions endpoint.
type openAIRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Stream      bool            `json:"stream"`
	Temperature float64         `json:"temperature"`
}

// openAIResponse is the non-streaming response from the chat completions endpoint.
type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (p *OpenAICompatProvider) complete(ctx context.Context, messages []openAIMessage, stream bool) (io.ReadCloser, error) {
	reqBody := openAIRequest{
		Model:       p.model,
		Messages:    messages,
		Stream:      stream,
		Temperature: p.temp,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling openai-compat request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("creating openai-compat request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("calling openai-compat API: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("openai-compat API returned status %d: %s",
			resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return resp.Body, nil
}

// parseOpenAIStream reads a server-sent event stream in OpenAI format and
// writes the content tokens to w.
//
// Each SSE line looks like:
//
//	data: {"choices":[{"delta":{"content":"token"},"finish_reason":null}]}
//
// The stream ends with the sentinel:
//
//	data: [DONE]
func parseOpenAIStream(r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			break
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue // Skip malformed chunks.
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				if _, err := fmt.Fprint(w, choice.Delta.Content); err != nil {
					return err
				}
			}
		}
	}
	return scanner.Err()
}
