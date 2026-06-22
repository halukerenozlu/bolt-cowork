package provider

import (
	"context"
	"testing"
)

func TestFallbackChain_LastActive(t *testing.T) {
	tests := []struct {
		name      string
		providers []LLMProvider
		useStream bool
		wantName  string
	}{
		{
			name: "chat skips unavailable",
			providers: []LLMProvider{
				&mockProvider{name: "p1", available: false},
				&mockProvider{name: "p2", available: true, response: "ok"},
			},
			wantName: "p2",
		},
		{
			name: "stream uses first available",
			providers: []LLMProvider{
				&mockProvider{name: "p1", available: true, response: "streamed"},
			},
			useStream: true,
			wantName:  "p1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain := NewFallbackChain(tt.providers)

			if chain.LastActive() != nil {
				t.Error("LastActive should be nil before any request")
			}

			if tt.useStream {
				ch, err := chain.StreamChat(context.Background(), nil)
				if err != nil {
					t.Fatalf("StreamChat: %v", err)
				}
				for range ch {
				}
			} else {
				_, err := chain.Chat(context.Background(), nil, nil)
				if err != nil {
					t.Fatalf("Chat: %v", err)
				}
			}

			active := chain.LastActive()
			if active == nil {
				t.Fatal("LastActive should not be nil after successful request")
			}
			if active.Name() != tt.wantName {
				t.Errorf("LastActive.Name() = %q, want %q", active.Name(), tt.wantName)
			}
		})
	}
}

func TestFallbackChain_OnFallbackCallback(t *testing.T) {
	var fromName, toName, gotReason string
	chain := NewFallbackChain(
		[]LLMProvider{
			&mockProvider{name: "p1", available: false},
			&mockProvider{name: "p2", available: true, response: "ok"},
		},
		WithOnFallback(func(from, to LLMProvider, reason string) {
			fromName = from.Name()
			toName = to.Name()
			gotReason = reason
		}),
	)

	_, err := chain.Chat(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if fromName != "p1" || toName != "p2" {
		t.Errorf("OnFallback called with from=%q, to=%q; want p1→p2", fromName, toName)
	}
	if gotReason == "" {
		t.Error("OnFallback reason should not be empty")
	}
}

func TestFallbackReason(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "nil error", err: nil, want: "unknown"},
		{name: "not available", err: ErrNotAvailable, want: "not available"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fallbackReason(tt.err)
			if got != tt.want {
				t.Errorf("fallbackReason() = %q, want %q", got, tt.want)
			}
		})
	}
}
