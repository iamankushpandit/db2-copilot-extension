package copilot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const chatCompletionsURL = "https://api.githubcopilot.com/chat/completions"

// ChatCompletions sends a streaming chat completions request to the Copilot API
// and returns the raw response body for the caller to stream.
func ChatCompletions(ctx context.Context, integrationID, apiKey string, req *ChatCompletionsRequest) (io.ReadCloser, error) {
	req.Stream = true
	return doRequest(ctx, integrationID, apiKey, req)
}

// ChatCompletionsNonStreaming sends a non-streaming chat completions request to
// the Copilot API and returns the fully parsed response.
func ChatCompletionsNonStreaming(ctx context.Context, integrationID, apiKey string, req *ChatCompletionsRequest) (*ChatCompletionsResponse, error) {
	req.Stream = false
	body, err := doRequest(ctx, integrationID, apiKey, req)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	var resp ChatCompletionsResponse
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("decoding chat completions response: %w", err)
	}
	return &resp, nil
}

func doRequest(ctx context.Context, integrationID, apiKey string, req *ChatCompletionsRequest) (io.ReadCloser, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, chatCompletionsURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("creating HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Copilot-Integration-Id", integrationID)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("executing HTTP request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("copilot API returned status %d: %s", resp.StatusCode, string(body))
	}

	return resp.Body, nil
}
