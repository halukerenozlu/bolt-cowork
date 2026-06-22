package agent

import "testing"

func TestRedact(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		secrets []string
		want    string
	}{
		{
			name:    "env var value redacted",
			input:   "error: auth failed with key sk-abc123",
			secrets: []string{"sk-abc123"},
			want:    "error: auth failed with key [REDACTED]",
		},
		{
			name:    "config secret redacted",
			input:   "API returned 401 for key-xyz endpoint",
			secrets: []string{"key-xyz"},
			want:    "API returned 401 for [REDACTED] endpoint",
		},
		{
			name:    "partial match within word",
			input:   "token is mysk-abc123here",
			secrets: []string{"sk-abc123"},
			want:    "token is my[REDACTED]here",
		},
		{
			name:    "no false positive on normal text",
			input:   "hello world",
			secrets: []string{"sk-abc123"},
			want:    "hello world",
		},
		{
			name:    "multiple secrets redacted",
			input:   "key1 and key2 are both present",
			secrets: []string{"key1", "key2"},
			want:    "[REDACTED] and [REDACTED] are both present",
		},
		{
			name:    "short secret ignored",
			input:   "ab is short",
			secrets: []string{"ab"},
			want:    "ab is short",
		},
		{
			name:    "empty secret ignored",
			input:   "nothing changes",
			secrets: []string{""},
			want:    "nothing changes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRedactor(tt.secrets)
			got := r.Redact(tt.input)
			if got != tt.want {
				t.Errorf("Redact() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewRedactor_Dedup(t *testing.T) {
	r := NewRedactor([]string{"secret", "secret", "secret"})
	if len(r.secrets) != 1 {
		t.Errorf("expected 1 unique secret, got %d", len(r.secrets))
	}
}

func TestRedact_OverlappingSecretsLongestFirst(t *testing.T) {
	r := NewRedactor([]string{"sk-test", "sk-test-abcdef"})

	got := r.Redact("request failed for sk-test-abcdef")
	want := "request failed for [REDACTED]"
	if got != want {
		t.Errorf("Redact() = %q, want %q", got, want)
	}
}

func TestRedactor_AddSecret(t *testing.T) {
	tests := []struct {
		name   string
		secret string
		input  string
		want   string
	}{
		{name: "new secret", secret: "sk-new-secret", input: "failed with sk-new-secret", want: "failed with [REDACTED]"},
		{name: "short value ignored", secret: "abc", input: "abc", want: "abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRedactor(nil)
			r.AddSecret(tt.secret)
			if got := r.Redact(tt.input); got != tt.want {
				t.Fatalf("Redact() = %q, want %q", got, tt.want)
			}
		})
	}
}
