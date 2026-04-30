package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/halukerenozlu/bolt-cowork/pkg/types"
)

// OnFallback is called when the chain switches from one provider to the next.
type OnFallback func(from, to LLMProvider)

// ChainOption configures a FallbackChain.
type ChainOption func(*FallbackChain)

// WithOnFallback sets a callback that fires on each provider switch.
func WithOnFallback(fn OnFallback) ChainOption {
	return func(fc *FallbackChain) {
		fc.onFallback = fn
	}
}

// FallbackChain tries providers in order until one succeeds.
type FallbackChain struct {
	providers  []LLMProvider
	onFallback OnFallback
}

// NewFallbackChain creates a chain from the given providers.
func NewFallbackChain(providers []LLMProvider, opts ...ChainOption) *FallbackChain {
	fc := &FallbackChain{providers: providers}
	for _, opt := range opts {
		opt(fc)
	}
	return fc
}

// Chat tries each provider in order. If a provider is unavailable or returns
// an error, the chain moves to the next one and fires the OnFallback callback.
// tools may be nil; it is passed through to each provider.
func (fc *FallbackChain) Chat(ctx context.Context, messages []types.Message, tools []ToolSpec) (string, error) {
	if len(fc.providers) == 0 {
		return "", ErrNoAvailableProvider
	}

	var lastErr error
	for i, p := range fc.providers {
		if !p.Available() {
			fc.notifyFallback(p, i)
			lastErr = fmt.Errorf("%s: %w", p.Name(), ErrNotAvailable)
			continue
		}

		resp, err := p.Chat(ctx, messages, tools)
		if err == nil {
			return resp, nil
		}

		if !errors.Is(err, ErrNotAvailable) {
			return "", fmt.Errorf("%s: %w", p.Name(), err)
		}

		lastErr = fmt.Errorf("%s: %w", p.Name(), err)
		fc.notifyFallback(p, i)
	}

	return "", fmt.Errorf("%w: %w", ErrNoAvailableProvider, lastErr)
}

// StreamChat tries each provider in order for streaming responses.
func (fc *FallbackChain) StreamChat(ctx context.Context, messages []types.Message) (<-chan string, error) {
	if len(fc.providers) == 0 {
		return nil, ErrNoAvailableProvider
	}

	var lastErr error
	for i, p := range fc.providers {
		if !p.Available() {
			fc.notifyFallback(p, i)
			lastErr = fmt.Errorf("%s: %w", p.Name(), ErrNotAvailable)
			continue
		}

		ch, err := p.StreamChat(ctx, messages)
		if err == nil {
			return ch, nil
		}

		if !errors.Is(err, ErrNotAvailable) {
			return nil, fmt.Errorf("%s: %w", p.Name(), err)
		}

		lastErr = fmt.Errorf("%s: %w", p.Name(), err)
		fc.notifyFallback(p, i)
	}

	return nil, fmt.Errorf("%w: %w", ErrNoAvailableProvider, lastErr)
}

// notifyFallback calls the OnFallback callback if set and a next provider exists.
func (fc *FallbackChain) notifyFallback(from LLMProvider, fromIdx int) {
	if fc.onFallback == nil {
		return
	}
	next := fromIdx + 1
	if next < len(fc.providers) {
		fc.onFallback(from, fc.providers[next])
	}
}
