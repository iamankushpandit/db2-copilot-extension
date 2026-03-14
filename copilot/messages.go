package copilot

// ChatRequest is the payload sent by the Copilot platform to the agent.
type ChatRequest struct {
	Messages []ChatMessage `json:"messages"`
}

// ChatMessage represents a single message in a chat conversation.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Model represents a Copilot/OpenAI model identifier.
type Model string

const (
	// ModelGPT35 is GPT-3.5 Turbo.
	ModelGPT35 Model = "gpt-3.5-turbo"
	// ModelGPT4 is GPT-4.
	ModelGPT4 Model = "gpt-4"
)

// ChatCompletionsRequest is the request body sent to the Copilot chat completions API.
type ChatCompletionsRequest struct {
	Messages []ChatMessage `json:"messages"`
	Model    Model         `json:"model"`
	Stream   bool          `json:"stream"`
}

// ChatCompletionsResponse is the response body from the Copilot chat completions API
// when streaming is disabled.
type ChatCompletionsResponse struct {
	Choices []ChatCompletionsChoice `json:"choices"`
}

// ChatCompletionsChoice represents a single choice in a chat completions response.
type ChatCompletionsChoice struct {
	Message ChatMessage `json:"message"`
}
