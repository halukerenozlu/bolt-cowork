package provider

import (
	"context"
	"fmt"

	"github.com/halukerenozlu/bolt-cowork/pkg/types"
)

// OpenAIProvider is a stub LLM provider for OpenAI models.
// Real API integration will be added when go-openai SDK is introduced.
type OpenAIProvider struct {
	apiKey    string
	model     string
	available bool
}

// NewOpenAI creates a new OpenAI provider stub.
func NewOpenAI(apiKey, model string) *OpenAIProvider {
	return &OpenAIProvider{
		apiKey:    apiKey,
		model:     model,
		available: apiKey != "",
	}
}

func (p *OpenAIProvider) Chat(ctx context.Context, messages []types.Message) (string, error) {
	if !p.Available() {
		return "", fmt.Errorf("openai: %w", ErrNotAvailable)
	}
	// TODO: replace with real go-openai SDK call
	return fmt.Sprintf("[openai/%s stub response]", p.model), nil
}

func (p *OpenAIProvider) StreamChat(ctx context.Context, messages []types.Message) (<-chan string, error) {
	if !p.Available() {
		return nil, fmt.Errorf("openai: %w", ErrNotAvailable)
	}
	// TODO: replace with real go-openai SDK streaming call
	ch := make(chan string, 1)
	go func() {
		defer close(ch)
		ch <- fmt.Sprintf("[openai/%s stub response]", p.model)
	}()
	return ch, nil
}

func (p *OpenAIProvider) Name() string {
	return "openai/" + p.model
}

func (p *OpenAIProvider) Available() bool {
	return p.apiKey != "" && p.available
}

// SetAvailable toggles availability (for testing fallback behavior).
func (p *OpenAIProvider) SetAvailable(v bool) {
	p.available = v
}
