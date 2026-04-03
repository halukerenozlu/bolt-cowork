package provider

import (
	"context"
	"errors"

	"github.com/halukerenozlu/bolt-cowork/pkg/types"
)

var (
	// ErrNoAvailableProvider is returned when all providers in the chain are exhausted.
	ErrNoAvailableProvider = errors.New("no available provider in chain")

	// ErrNotAvailable is returned when a provider is not available.
	ErrNotAvailable = errors.New("provider is not available")
)

// LLMProvider abstracts communication with an LLM model.
type LLMProvider interface {
	// Chat sends messages and returns a complete response.
	Chat(ctx context.Context, messages []types.Message) (string, error)

	// StreamChat sends messages and returns a channel of response chunks.
	StreamChat(ctx context.Context, messages []types.Message) (<-chan string, error)

	// Name returns the provider identifier (e.g. "openai/gpt-4o").
	Name() string

	// Available reports whether the provider can accept requests.
	Available() bool
}
