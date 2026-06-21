package main

import (
	"os"
	"testing"

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
