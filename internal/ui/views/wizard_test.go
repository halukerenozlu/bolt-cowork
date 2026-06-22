package views

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/halukerenozlu/bolt-cowork/internal/config"
)

func TestWizardAuthOptionsMasksEnvironmentKeysSafely(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want string
	}{
		{name: "short key", key: "short", want: "****"},
		{name: "long key", key: "sk-1234567890", want: "sk-1...7890"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OPENAI_API_KEY", tt.key)
			options := wizardAuthOptions("openai")
			if len(options) == 0 || !strings.Contains(options[0], tt.want) {
				t.Fatalf("wizardAuthOptions() = %v, want masked value %q", options, tt.want)
			}
		})
	}
}

func TestWizardConfiguresCredentialBeforeVerification(t *testing.T) {
	tests := []struct {
		name         string
		configureErr error
		verifyErr    error
		wantWarning  bool
		wantPersist  bool
	}{
		{name: "keyring success", wantPersist: true},
		{name: "keyring unavailable uses session key", configureErr: errors.New("keyring unavailable"), wantWarning: true, wantPersist: true},
		{name: "invalid key is not persisted", verifyErr: errors.New("unauthorized")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.Providers = map[string]config.ProviderConfig{}
			var configured bool
			var persisted bool
			runner := AgentRunner{
				ConfigureProvider: func(name, apiKey string) {
					configured = true
					pc := cfg.Providers[name]
					pc.APIKey = apiKey
					cfg.Providers[name] = pc
				},
				PersistProviderKey: func(_, _ string) error {
					persisted = true
					return tt.configureErr
				},
				VerifyProvider: func(_ context.Context, name string) error {
					if got := cfg.Providers[name].APIKey; got != "sk-test-key" {
						return errors.New("credential was not available during verification")
					}
					return tt.verifyErr
				},
			}
			s := NewSession(cfg, "", "hi", runner)
			s, _ = s.startWizard("openrouter")
			s.wizardStep = wizStepKey
			s.wizardInput.SetValue("sk-test-key")

			model, cmd := s.wizardKeyConfirm()
			got := model.(Session)
			if !configured {
				t.Fatal("ConfigureProvider was not called")
			}
			if cmd == nil {
				t.Fatal("verification command was not returned")
			}
			msg := cmd()
			result, _ := got.Update(msg)
			updated := result.(Session)
			if tt.verifyErr == nil && strings.Contains(updated.wizardErr, "Verification failed") {
				t.Fatalf("verification could not see configured credential: %s", updated.wizardErr)
			}
			if persisted != tt.wantPersist {
				t.Fatalf("persisted = %v, want %v", persisted, tt.wantPersist)
			}
			if tt.wantWarning != strings.Contains(updated.wizardErr, "session only") {
				t.Fatalf("wizard warning = %q, wantWarning=%v", updated.wizardErr, tt.wantWarning)
			}
		})
	}
}

func TestWizardUsesDetectedLocalModels(t *testing.T) {
	cfg := config.Default()
	cfg.Providers = map[string]config.ProviderConfig{}
	s := NewSession(cfg, "", "hi", AgentRunner{})
	s.localDetected = map[string]LocalProviderInfo{
		"ollama": {
			Endpoint: "http://localhost:11434/v1/chat/completions",
			Models:   []string{"qwen3:8b", "deepseek-r1:8b"},
		},
	}
	s, _ = s.startWizard("ollama")

	model, cmd := s.wizardAuthConfirm()
	got := model.(Session)
	if cmd == nil {
		t.Fatal("verification command was not returned")
	}
	verifyMsg := cmd()
	model, cmd = got.Update(verifyMsg)
	got = model.(Session)
	if cmd != nil {
		t.Fatal("model discovery callback should not be required when runner has none")
	}
	if got.wizardStep != wizStepModels {
		t.Fatalf("wizardStep = %d, want model selection", got.wizardStep)
	}
	if strings.Join(got.wizardModels, "\x00") != "qwen3:8b\x00deepseek-r1:8b" {
		t.Fatalf("wizardModels = %v, want detected local models", got.wizardModels)
	}

	got.wizardCursor = 1
	model, cmd = got.wizardModelConfirm()
	got = model.(Session)
	if cmd == nil {
		t.Fatal("runtime model change command was not returned")
	}
	if got.runner.Provider != "ollama" || got.runner.Model != "deepseek-r1:8b" {
		t.Fatalf("runner = %s/%s, want ollama/deepseek-r1:8b", got.runner.Provider, got.runner.Model)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("local provider config is invalid: %v", err)
	}
	if _, ok := cmd().(RuntimeModelChangedMsg); !ok {
		t.Fatalf("command returned %T, want RuntimeModelChangedMsg", cmd())
	}
}

func TestWizardDoesNotReusePreviousModelWhenDiscoveryIsEmpty(t *testing.T) {
	cfg := config.Default()
	cfg.Providers = map[string]config.ProviderConfig{}
	s := NewSession(cfg, "", "hi", AgentRunner{Provider: "anthropic", Model: "claude-sonnet-4-6"})
	s, _ = s.startWizard("ollama")

	model, cmd := s.wizardAuthConfirm()
	got := model.(Session)
	model, _ = got.Update(cmd())
	got = model.(Session)

	if !got.wizardOpen {
		t.Fatal("wizard closed without a local model")
	}
	if got.runner.Provider != "anthropic" || got.runner.Model != "claude-sonnet-4-6" {
		t.Fatalf("runner changed to %s/%s without a discovered model", got.runner.Provider, got.runner.Model)
	}
	if !strings.Contains(got.wizardErr, "No models") {
		t.Fatalf("wizardErr = %q, want no-model error", got.wizardErr)
	}
}
