package copilot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	chatCompletionsURL = "https://api.githubcopilot.com/chat/completions"
)

// ChatCompletions sends a non-streaming chat completion request to the Copilot LLM API
// and returns the assistant's reply text.
func ChatCompletions(ctx context.Context, integrationID, apiToken string, req *ChatCompletionsRequest) (string, error) {
	req.Stream = false

	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("copilot: failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, chatCompletionsURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("copilot: failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+apiToken)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Copilot-Integration-Id", integrationID)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("copilot: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("copilot: unexpected status %d: %s", resp.StatusCode, string(b))
	}

	var result ChatCompletionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("copilot: failed to decode response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("copilot: no choices in response")
	}

	return result.Choices[0].Message.Content, nil
}

// StreamChatCompletions sends a streaming chat completion request and writes
// Server-Sent Events directly to w.
func StreamChatCompletions(ctx context.Context, integrationID, apiToken string, req *ChatCompletionsRequest, w io.Writer) error {
	req.Stream = true

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("copilot: failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, chatCompletionsURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("copilot: failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+apiToken)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Copilot-Integration-Id", integrationID)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("copilot: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("copilot: unexpected status %d: %s", resp.StatusCode, string(b))
	}

	// Forward the SSE stream directly to the response writer
	_, err = io.Copy(w, resp.Body)
	return err
}

// GetPublicKey fetches the Copilot public key used for request signature verification.
func GetPublicKey(ctx context.Context, integrationID string) (string, string, error) {
	url := fmt.Sprintf("https://api.github.com/meta/public_keys/copilot_oidc")

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", fmt.Errorf("copilot: failed to create key request: %w", err)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", "", fmt.Errorf("copilot: public key request failed: %w", err)
	}
	defer resp.Body.Close()

	var keyResp struct {
		PublicKeys []struct {
			Key       string `json:"key"`
			KeyID     string `json:"key_identifier"`
			IsCurrent bool   `json:"is_current"`
		} `json:"public_keys"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&keyResp); err != nil {
		return "", "", fmt.Errorf("copilot: failed to decode key response: %w", err)
	}

	for _, k := range keyResp.PublicKeys {
		if k.IsCurrent {
			return k.Key, k.KeyID, nil
		}
	}

	if len(keyResp.PublicKeys) > 0 {
		return keyResp.PublicKeys[0].Key, keyResp.PublicKeys[0].KeyID, nil
	}

	return "", "", fmt.Errorf("copilot: no public key found")
}
