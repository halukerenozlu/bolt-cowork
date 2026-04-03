package provider

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/halukerenozlu/bolt-cowork/pkg/types"
)

// mockProvider implements LLMProvider for testing.
type mockProvider struct {
	name      string
	available bool
	response  string
	err       error
}

func (m *mockProvider) Chat(_ context.Context, _ []types.Message) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func (m *mockProvider) StreamChat(_ context.Context, _ []types.Message) (<-chan string, error) {
	if m.err != nil {
		return nil, m.err
	}
	ch := make(chan string, 1)
	go func() {
		defer close(ch)
		ch <- m.response
	}()
	return ch, nil
}

func (m *mockProvider) Name() string    { return m.name }
func (m *mockProvider) Available() bool { return m.available }

// --- FallbackChain Tests ---

func TestFallbackChain_FirstAvailable(t *testing.T) {
	chain := NewFallbackChain([]LLMProvider{
		&mockProvider{name: "p1", available: true, response: "from-p1"},
		&mockProvider{name: "p2", available: true, response: "from-p2"},
	})

	resp, err := chain.Chat(context.Background(), nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp != "from-p1" {
		t.Errorf("response = %q, want %q", resp, "from-p1")
	}
}

func TestFallbackChain_SkipsUnavailable(t *testing.T) {
	chain := NewFallbackChain([]LLMProvider{
		&mockProvider{name: "p1", available: false},
		&mockProvider{name: "p2", available: true, response: "from-p2"},
	})

	resp, err := chain.Chat(context.Background(), nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp != "from-p2" {
		t.Errorf("response = %q, want %q", resp, "from-p2")
	}
}

func TestFallbackChain_AllUnavailable(t *testing.T) {
	chain := NewFallbackChain([]LLMProvider{
		&mockProvider{name: "p1", available: false},
		&mockProvider{name: "p2", available: false},
	})

	_, err := chain.Chat(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error when all providers unavailable")
	}
	if !errors.Is(err, ErrNoAvailableProvider) {
		t.Errorf("expected ErrNoAvailableProvider, got: %v", err)
	}
}

func TestFallbackChain_FallbackOnNotAvailable(t *testing.T) {
	chain := NewFallbackChain([]LLMProvider{
		&mockProvider{name: "p1", available: true, err: fmt.Errorf("rate limited: %w", ErrNotAvailable)},
		&mockProvider{name: "p2", available: true, response: "from-p2"},
	})

	resp, err := chain.Chat(context.Background(), nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp != "from-p2" {
		t.Errorf("response = %q, want %q", resp, "from-p2")
	}
}

func TestFallbackChain_NonRetryableError(t *testing.T) {
	chain := NewFallbackChain([]LLMProvider{
		&mockProvider{name: "p1", available: true, err: fmt.Errorf("invalid request")},
		&mockProvider{name: "p2", available: true, response: "from-p2"},
	})

	_, err := chain.Chat(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for non-retryable failure")
	}
	// Should NOT fall back — error returned directly from p1.
	if errors.Is(err, ErrNoAvailableProvider) {
		t.Error("should not be ErrNoAvailableProvider — should return p1's error directly")
	}
}

func TestFallbackChain_OnFallbackCalled(t *testing.T) {
	var transitions []string
	chain := NewFallbackChain(
		[]LLMProvider{
			&mockProvider{name: "p1", available: false},
			&mockProvider{name: "p2", available: true, response: "ok"},
		},
		WithOnFallback(func(from, to LLMProvider) {
			transitions = append(transitions, from.Name()+"->"+to.Name())
		}),
	)

	_, err := chain.Chat(context.Background(), nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(transitions) != 1 {
		t.Fatalf("transitions count = %d, want 1", len(transitions))
	}
	if transitions[0] != "p1->p2" {
		t.Errorf("transition = %q, want %q", transitions[0], "p1->p2")
	}
}

func TestFallbackChain_SingleProvider(t *testing.T) {
	chain := NewFallbackChain([]LLMProvider{
		&mockProvider{name: "solo", available: true, response: "solo-response"},
	})

	resp, err := chain.Chat(context.Background(), nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp != "solo-response" {
		t.Errorf("response = %q, want %q", resp, "solo-response")
	}
}

func TestFallbackChain_Empty(t *testing.T) {
	chain := NewFallbackChain(nil)

	_, err := chain.Chat(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for empty chain")
	}
	if !errors.Is(err, ErrNoAvailableProvider) {
		t.Errorf("expected ErrNoAvailableProvider, got: %v", err)
	}
}

func TestFallbackChain_StreamChat(t *testing.T) {
	chain := NewFallbackChain([]LLMProvider{
		&mockProvider{name: "p1", available: false},
		&mockProvider{name: "p2", available: true, response: "streamed"},
	})

	ch, err := chain.StreamChat(context.Background(), nil)
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}

	var chunks []string
	for chunk := range ch {
		chunks = append(chunks, chunk)
	}
	if len(chunks) != 1 || chunks[0] != "streamed" {
		t.Errorf("chunks = %v, want [streamed]", chunks)
	}
}

// --- Provider Stub Tests ---

func TestOpenAI_Name(t *testing.T) {
	p := NewOpenAI("sk-test", "gpt-4o")
	if p.Name() != "openai/gpt-4o" {
		t.Errorf("Name() = %q, want %q", p.Name(), "openai/gpt-4o")
	}
}

func TestOpenAI_Available(t *testing.T) {
	tests := []struct {
		name   string
		apiKey string
		want   bool
	}{
		{"with key", "sk-test", true},
		{"empty key", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewOpenAI(tt.apiKey, "gpt-4o")
			if p.Available() != tt.want {
				t.Errorf("Available() = %v, want %v", p.Available(), tt.want)
			}
		})
	}
}

func TestOpenAI_SetAvailable(t *testing.T) {
	p := NewOpenAI("sk-test", "gpt-4o")
	p.SetAvailable(false)
	if p.Available() {
		t.Error("Available() should be false after SetAvailable(false)")
	}
}

func TestAnthropic_Name(t *testing.T) {
	p := NewAnthropic("sk-ant-test", "claude-opus-4-6")
	if p.Name() != "anthropic/claude-opus-4-6" {
		t.Errorf("Name() = %q, want %q", p.Name(), "anthropic/claude-opus-4-6")
	}
}

func TestAnthropic_Available(t *testing.T) {
	tests := []struct {
		name   string
		apiKey string
		want   bool
	}{
		{"with key", "sk-ant-test", true},
		{"empty key", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewAnthropic(tt.apiKey, "claude-opus-4-6")
			if p.Available() != tt.want {
				t.Errorf("Available() = %v, want %v", p.Available(), tt.want)
			}
		})
	}
}

func TestOpenAI_Chat_Unavailable(t *testing.T) {
	p := NewOpenAI("", "gpt-4o")
	_, err := p.Chat(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for unavailable provider")
	}
	if !errors.Is(err, ErrNotAvailable) {
		t.Errorf("expected ErrNotAvailable, got: %v", err)
	}
}

func TestAnthropic_Chat_Stub(t *testing.T) {
	p := NewAnthropic("sk-test", "claude-sonnet-4-6")
	resp, err := p.Chat(context.Background(), nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp == "" {
		t.Error("expected non-empty stub response")
	}
}
