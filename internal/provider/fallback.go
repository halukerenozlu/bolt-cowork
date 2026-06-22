package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/halukerenozlu/bolt-cowork/pkg/types"
)

// OnFallback is called when the chain switches from one provider to the next.
// reason describes why the switch occurred (e.g. "no API key", "HTTP 401").
type OnFallback func(from, to LLMProvider, reason string)

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
	lastActive LLMProvider // provider that handled the most recent request
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
			fc.notifyFallback(p, i, "not available")
			lastErr = fmt.Errorf("%s: %w", p.Name(), ErrNotAvailable)
			continue
		}

		resp, err := p.Chat(ctx, messages, tools)
		if err == nil {
			fc.lastActive = p
			return resp, nil
		}

		if !errors.Is(err, ErrNotAvailable) {
			return "", fmt.Errorf("%s: %w", p.Name(), err)
		}

		lastErr = fmt.Errorf("%s: %w", p.Name(), err)
		fc.notifyFallback(p, i, fallbackReason(err))
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
			fc.notifyFallback(p, i, "not available")
			lastErr = fmt.Errorf("%s: %w", p.Name(), ErrNotAvailable)
			continue
		}

		ch, err := p.StreamChat(ctx, messages)
		if err == nil {
			fc.lastActive = p
			return ch, nil
		}

		if !errors.Is(err, ErrNotAvailable) {
			return nil, fmt.Errorf("%s: %w", p.Name(), err)
		}

		lastErr = fmt.Errorf("%s: %w", p.Name(), err)
		fc.notifyFallback(p, i, fallbackReason(err))
	}

	return nil, fmt.Errorf("%w: %w", ErrNoAvailableProvider, lastErr)
}

// LastActive returns the provider that handled the most recent request, or nil.
func (fc *FallbackChain) LastActive() LLMProvider {
	return fc.lastActive
}

// fallbackReason extracts a short human-readable reason from the error.
func fallbackReason(err error) string {
	if err == nil {
		return "unknown"
	}
	msg := err.Error()
	for _, kw := range []struct{ sub, label string }{
		{"401", "auth error"},
		{"403", "forbidden"},
		{"429", "rate limited"},
		{"500", "server error"},
		{"502", "bad gateway"},
		{"503", "service unavailable"},
		{"timeout", "timeout"},
		{"connection refused", "connection refused"},
	} {
		if strings.Contains(msg, kw.sub) {
			return kw.label
		}
	}
	if errors.Is(err, ErrNotAvailable) {
		return "not available"
	}
	return "provider error"
}

// notifyFallback calls the OnFallback callback if set and a next provider exists.
func (fc *FallbackChain) notifyFallback(from LLMProvider, fromIdx int, reason string) {
	if fc.onFallback == nil {
		return
	}
	next := fromIdx + 1
	if next < len(fc.providers) {
		fc.onFallback(from, fc.providers[next], reason)
	}
}
