package provider

import (
	"context"
	"fmt"

	"github.com/halukerenozlu/bolt-cowork/pkg/types"
)

// AnthropicProvider is a stub LLM provider for Anthropic models.
// Real API integration will be added when anthropic-sdk-go is introduced.
type AnthropicProvider struct {
	apiKey    string
	model     string
	available bool
}

// NewAnthropic creates a new Anthropic provider stub.
func NewAnthropic(apiKey, model string) *AnthropicProvider {
	return &AnthropicProvider{
		apiKey:    apiKey,
		model:     model,
		available: apiKey != "",
	}
}

func (p *AnthropicProvider) Chat(ctx context.Context, messages []types.Message) (string, error) {
	if !p.Available() {
		return "", fmt.Errorf("anthropic: %w", ErrNotAvailable)
	}
	// TODO: replace with real anthropic-sdk-go call
	return fmt.Sprintf("[anthropic/%s stub response]", p.model), nil
}

func (p *AnthropicProvider) StreamChat(ctx context.Context, messages []types.Message) (<-chan string, error) {
	if !p.Available() {
		return nil, fmt.Errorf("anthropic: %w", ErrNotAvailable)
	}
	// TODO: replace with real anthropic-sdk-go streaming call
	ch := make(chan string, 1)
	go func() {
		defer close(ch)
		ch <- fmt.Sprintf("[anthropic/%s stub response]", p.model)
	}()
	return ch, nil
}

func (p *AnthropicProvider) Name() string {
	return "anthropic/" + p.model
}

func (p *AnthropicProvider) Available() bool {
	return p.apiKey != "" && p.available
}

// SetAvailable toggles availability (for testing fallback behavior).
func (p *AnthropicProvider) SetAvailable(v bool) {
	p.available = v
}
