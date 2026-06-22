package provider

import (
	"strings"
	"testing"
)

func TestParseSSEStream(t *testing.T) {
	tests := []struct {
		name string
		input string
		want []string
	}{
		{
			name: "normal two chunks",
			input: strings.Join([]string{
				`data: {"choices":[{"delta":{"content":"Hello"}}]}`,
				"",
				`data: {"choices":[{"delta":{"content":" world"}}]}`,
				"",
				"data: [DONE]",
				"",
			}, "\n"),
			want: []string{"Hello", " world"},
		},
		{
			name: "empty content filtered",
			input: strings.Join([]string{
				`data: {"choices":[{"delta":{"content":""}}]}`,
				"",
				`data: {"choices":[{"delta":{"content":"ok"}}]}`,
				"",
				"data: [DONE]",
			}, "\n"),
			want: []string{"ok"},
		},
		{
			name: "malformed JSON skipped",
			input: strings.Join([]string{
				"data: {not json}",
				`data: {"choices":[{"delta":{"content":"valid"}}]}`,
				"data: [DONE]",
			}, "\n"),
			want: []string{"valid"},
		},
		{
			name: "non-data lines ignored",
			input: strings.Join([]string{
				": this is a comment",
				"event: message",
				`data: {"choices":[{"delta":{"content":"ok"}}]}`,
				"",
				"data: [DONE]",
			}, "\n"),
			want: []string{"ok"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := make(chan string, 16)
			parseSSEStream(strings.NewReader(tt.input), ch)
			close(ch)

			var got []string
			for s := range ch {
				got = append(got, s)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("got %d chunks %v, want %d %v", len(got), got, len(tt.want), tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("chunk[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestCustomProvider_Available_RequiresAPIKey(t *testing.T) {
	tests := []struct {
		name           string
		endpoint       string
		apiKey         string
		model          string
		requiresAPIKey bool
		want           bool
	}{
		{name: "hosted with key", endpoint: "http://api.example.com", apiKey: "key", model: "m", requiresAPIKey: true, want: true},
		{name: "hosted without key", endpoint: "http://api.example.com", apiKey: "", model: "m", requiresAPIKey: true, want: false},
		{name: "local no key needed", endpoint: "http://localhost:11434", apiKey: "", model: "m", requiresAPIKey: false, want: true},
		{name: "no endpoint", endpoint: "", apiKey: "key", model: "m", want: false},
		{name: "no model", endpoint: "http://api.example.com", apiKey: "key", model: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewCustom("test", tt.endpoint, tt.apiKey, tt.model)
			p.SetRequiresAPIKey(tt.requiresAPIKey)
			if got := p.Available(); got != tt.want {
				t.Errorf("Available() = %v, want %v", got, tt.want)
			}
		})
	}
}
