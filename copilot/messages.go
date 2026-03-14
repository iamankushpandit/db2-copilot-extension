package copilot

import "encoding/json"

// ChatMessage represents a single message in a conversation.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is the request body sent by the Copilot platform to the agent.
type ChatRequest struct {
	Messages        []ChatMessage `json:"messages"`
	CopilotThreadID string        `json:"copilot_thread_id"`
	// Additional fields from the Copilot platform may be present.
}

// ChatCompletionsRequest is the payload sent to the Copilot LLM API.
type ChatCompletionsRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

// ChatCompletionsResponse is a non-streaming response from the Copilot LLM API.
type ChatCompletionsResponse struct {
	Choices []struct {
		Message ChatMessage `json:"message"`
	} `json:"choices"`
}

// StreamEvent represents a single server-sent event chunk from the streaming API.
type StreamEvent struct {
	ID      string          `json:"id,omitempty"`
	Object  string          `json:"object,omitempty"`
	Choices []StreamChoice  `json:"choices,omitempty"`
}

// StreamChoice holds a delta for a streaming response.
type StreamChoice struct {
	Delta        StreamDelta `json:"delta"`
	FinishReason *string     `json:"finish_reason"`
}

// StreamDelta holds the incremental content of a streaming message.
type StreamDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

// SSEEvent formats data as a Server-Sent Event line.
func SSEEvent(data interface{}) (string, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return "data: " + string(b) + "\n\n", nil
}

// SSEDone returns the SSE stream termination sentinel.
func SSEDone() string {
	return "data: [DONE]\n\n"
}

// Model constants for the Copilot LLM API.
const (
	ModelGPT4o     = "gpt-4o"
	ModelGPT4      = "gpt-4"
	ModelGPT35     = "gpt-3.5-turbo"
)
