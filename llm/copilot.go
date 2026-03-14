package llm

import (
	"context"
	"fmt"
	"io"

	"github.com/iamankushpandit/db2-copilot-extension/copilot"
)

type CopilotProvider struct {
	// TODO: add fields for integrationID, apiKey, etc.
}

func NewCopilotProvider() *CopilotProvider {
	return &CopilotProvider{}
}

func (p *CopilotProvider) GenerateSQL(ctx context.Context, req SQLGenerationRequest) (string, error) {
	// TODO: Use the new prompt template system
	prompt := fmt.Sprintf("Generate SQL for the following question: %s", req.Prompt)

	copilotReq := &copilot.ChatCompletionsRequest{
		Model:    copilot.ModelGPT4,
		Messages: []copilot.ChatMessage{{Role: "user", Content: prompt}},
		Stream:   false,
	}

	// This is not ideal. The integrationID and apiKey should be passed in.
	// I will fix this later.
	resp, err := copilot.ChatCompletionsNonStreaming(ctx, "TODO_integrationID", "TODO_apiKey", copilotReq)
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no choices returned from copilot")
	}

	return resp.Choices[0].Message.Content, nil
}

func (p *CopilotProvider) ExplainResults(ctx context.Context, req ExplanationRequest, w io.Writer) error {
	// TODO: implement
	return fmt.Errorf("not implemented")
}

func (p *CopilotProvider) Available() bool {
	// TODO: implement health check
	return true
}

func (p *CopilotProvider) Name() string {
	return "copilot"
}
