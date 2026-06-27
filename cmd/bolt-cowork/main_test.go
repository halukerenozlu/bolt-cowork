package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/halukerenozlu/bolt-cowork/internal/agent"
	"github.com/halukerenozlu/bolt-cowork/internal/config"
	keyring "github.com/zalando/go-keyring"
)

func TestMain(m *testing.M) {
	keyring.MockInit()
	os.Exit(m.Run())
}

func TestBuildProvidersUsesLatestFallbackModel(t *testing.T) {
	cfg := &config.Config{
		DefaultProvider: "anthropic",
		Providers: map[string]config.ProviderConfig{
			"anthropic": {
				APIKey: "test-key",
				Models: []string{"claude-opus-4-5", "claude-haiku-4-5-20251001"},
			},
		},
		FallbackChain: []config.FallbackEntry{{
			Provider: "anthropic",
			Model:    "claude-opus-4-5",
		}},
	}

	cfg.FallbackChain[0].Model = "claude-haiku-4-5-20251001"
	providers := buildProviders(cfg)

	if len(providers) != 1 {
		t.Fatalf("buildProviders() returned %d providers, want 1", len(providers))
	}
	if got := providers[0].Name(); got != "anthropic/claude-haiku-4-5-20251001" {
		t.Fatalf("provider name = %q, want updated Haiku model", got)
	}
}

func TestProviderModelsCoversHostedSetupProviders(t *testing.T) {
	tests := []struct {
		name     string
		provider string
	}{
		{name: "OpenRouter", provider: "openrouter"},
		{name: "DeepSeek", provider: "deepseek"},
		{name: "Mistral", provider: "mistral"},
		{name: "Groq", provider: "groq"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(providerModels[tt.provider]) == 0 {
				t.Fatalf("providerModels[%q] has no default model", tt.provider)
			}
		})
	}
}

func TestBuildTUIRunner_ConfiguresProviderInMemory(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		apiKey   string
		endpoint string
	}{
		{
			name:     "hosted provider",
			provider: "openrouter",
			apiKey:   "sk-openrouter-test",
			endpoint: config.HostedPresets["openrouter"].Endpoint,
		},
		{
			name:     "local provider",
			provider: "ollama",
			endpoint: config.HostedPresets["ollama"].Endpoint,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspace := t.TempDir()
			cfg := config.Default()
			cfg.Providers = map[string]config.ProviderConfig{
				"anthropic": {APIKey: "key", Models: []string{"claude-sonnet-4-6"}},
			}
			cfg.FallbackChain = []config.FallbackEntry{{
				Provider: "anthropic",
				Model:    "claude-sonnet-4-6",
			}}
			cfg.Sandbox.AllowedDirs = []string{workspace}
			cfg.Skills.Dirs = []string{filepath.Join(workspace, "skills")}

			runner := buildTUIRunner(cfg)
			runner.ConfigureProvider(tt.provider, tt.apiKey)
			pc, ok := cfg.Providers[tt.provider]
			if !ok {
				t.Fatalf("provider %q was not added", tt.provider)
			}
			if pc.APIKey != tt.apiKey {
				t.Fatalf("APIKey = %q, want %q", pc.APIKey, tt.apiKey)
			}
			if pc.Endpoint != tt.endpoint {
				t.Fatalf("Endpoint = %q, want %q", pc.Endpoint, tt.endpoint)
			}
		})
	}
}

func TestTUIResponseText(t *testing.T) {
	tests := []struct {
		name   string
		result *agent.Result
		want   string
	}{
		{
			name: "conversational response",
			result: &agent.Result{
				Plan: &agent.Plan{Description: "Hello! How can I help?"},
			},
			want: "Hello! How can I help?",
		},
		{
			name: "actionable plan description is not a final response",
			result: &agent.Result{
				Plan: &agent.Plan{
					Description: "List files in the current directory.",
					Steps:       []agent.Step{{Action: agent.ActionList}},
				},
				StepResults: []string{`Listed ".": file-a, file-b`},
			},
		},
		{
			name:   "nil result",
			result: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tuiResponseText(tt.result); got != tt.want {
				t.Fatalf("tuiResponseText() = %q, want %q", got, tt.want)
			}
		})
	}
}
