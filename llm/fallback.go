package llm

import (
	"context"
	"fmt"
	"io"
	"log"
)

// FallbackSQLProvider wraps a primary and a fallback TextToSQLProvider.
// If the primary is unavailable it transparently switches to the fallback.
type FallbackSQLProvider struct {
	primary  TextToSQLProvider
	fallback TextToSQLProvider
	// onFallback is called when the fallback is used.
	onFallback func(failed, used string)
}

// NewFallbackSQLProvider creates a FallbackSQLProvider.
// onFallback may be nil.
func NewFallbackSQLProvider(primary, fallback TextToSQLProvider, onFallback func(failed, used string)) *FallbackSQLProvider {
	return &FallbackSQLProvider{
		primary:    primary,
		fallback:   fallback,
		onFallback: onFallback,
	}
}

// Name returns the name of the active provider.
func (p *FallbackSQLProvider) Name() string {
	return fmt.Sprintf("%s(→%s)", p.primary.Name(), p.fallback.Name())
}

// Available returns true if at least one provider is available.
func (p *FallbackSQLProvider) Available(ctx context.Context) bool {
	return p.primary.Available(ctx) || p.fallback.Available(ctx)
}

// GenerateSQL tries the primary provider first; if unavailable uses the fallback.
func (p *FallbackSQLProvider) GenerateSQL(ctx context.Context, req SQLGenerationRequest) (string, error) {
	if p.primary.Available(ctx) {
		return p.primary.GenerateSQL(ctx, req)
	}
	log.Printf("INFO LLM primary %s unavailable, falling back to %s", p.primary.Name(), p.fallback.Name())
	if p.onFallback != nil {
		p.onFallback(p.primary.Name(), p.fallback.Name())
	}
	return p.fallback.GenerateSQL(ctx, req)
}

// FallbackExplanationProvider wraps a primary and a fallback ExplanationProvider.
type FallbackExplanationProvider struct {
	primary    ExplanationProvider
	fallback   ExplanationProvider
	onFallback func(failed, used string)
}

// NewFallbackExplanationProvider creates a FallbackExplanationProvider.
func NewFallbackExplanationProvider(primary, fallback ExplanationProvider, onFallback func(failed, used string)) *FallbackExplanationProvider {
	return &FallbackExplanationProvider{
		primary:    primary,
		fallback:   fallback,
		onFallback: onFallback,
	}
}

// Name returns the name of the active provider.
func (p *FallbackExplanationProvider) Name() string {
	return fmt.Sprintf("%s(→%s)", p.primary.Name(), p.fallback.Name())
}

// Available returns true if at least one provider is available.
func (p *FallbackExplanationProvider) Available(ctx context.Context) bool {
	return p.primary.Available(ctx) || p.fallback.Available(ctx)
}

// ExplainResults tries the primary provider first; if unavailable uses the fallback.
func (p *FallbackExplanationProvider) ExplainResults(ctx context.Context, req ExplanationRequest, w io.Writer) error {
	if p.primary.Available(ctx) {
		return p.primary.ExplainResults(ctx, req, w)
	}
	log.Printf("INFO LLM primary %s unavailable, falling back to %s", p.primary.Name(), p.fallback.Name())
	if p.onFallback != nil {
		p.onFallback(p.primary.Name(), p.fallback.Name())
	}
	return p.fallback.ExplainResults(ctx, req, w)
}
