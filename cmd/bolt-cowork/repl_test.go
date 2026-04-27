package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/halukerenozlu/bolt-cowork/internal/agent"
	"github.com/halukerenozlu/bolt-cowork/internal/config"
	"github.com/halukerenozlu/bolt-cowork/internal/skill"
	"gopkg.in/yaml.v3"
)

// captureStderr runs f() and returns everything written to os.Stderr during the call.
func captureStderr(f func()) string {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	f()
	w.Close()
	os.Stderr = old
	buf := make([]byte, 8192)
	n, _ := r.Read(buf)
	return string(buf[:n])
}

func TestHandleConfigCommand_NoArgs(t *testing.T) {
	// No-arg /config is backward-compatible: shows masked config (same as /config show).
	cfg := config.Default()
	cfg.Providers = map[string]config.ProviderConfig{
		"anthropic": {APIKey: "sk-noarg-testkey-long", Models: []string{"claude-sonnet-4-6"}},
	}
	output := captureStderr(func() {
		handleConfigCommand([]string{}, cfg)
	})
	if strings.Contains(output, "sk-noarg-testkey-long") {
		t.Error("handleConfigCommand() no-args must not expose the full API key")
	}
	if !strings.Contains(output, "***") {
		t.Error("handleConfigCommand() no-args output should contain masked key marker ***")
	}
}

func TestHandleConfigCommand_Help(t *testing.T) {
	cfg := config.Default()
	output := captureStderr(func() {
		handleConfigCommand([]string{"help"}, cfg)
	})
	for _, want := range []string{"show", "path", "reload", "set", "planned"} {
		if !strings.Contains(output, want) {
			t.Errorf("handleConfigCommand(help) output missing %q:\n%s", want, output)
		}
	}
}

func TestHandleConfigCommand_Show(t *testing.T) {
	cfg := config.Default()
	cfg.Providers = map[string]config.ProviderConfig{
		"anthropic": {APIKey: "sk-test-verylongkey-show", Models: []string{"claude-sonnet-4-6"}},
	}
	output := captureStderr(func() {
		handleConfigCommand([]string{"show"}, cfg)
	})
	if strings.Contains(output, "sk-test-verylongkey-show") {
		t.Error("handleConfigCommand(show) must not expose the full API key")
	}
	if !strings.Contains(output, "***") {
		t.Error("handleConfigCommand(show) output should contain masked key marker ***")
	}
}

func TestHandleConfigCommand_UnknownSubcommand(t *testing.T) {
	cfg := config.Default()
	output := captureStderr(func() {
		handleConfigCommand([]string{"xyz"}, cfg)
	})
	if !strings.Contains(output, "xyz") {
		t.Errorf("unknown subcommand error should echo the unknown token, got:\n%s", output)
	}
	if !strings.Contains(output, "Available") {
		t.Errorf("unknown subcommand error should list Available subcommands, got:\n%s", output)
	}
}

func TestHandleSkillCommand_NoArgs(t *testing.T) {
	store := skill.NewStore()
	output := captureStderr(func() {
		handleSkillCommand([]string{}, store)
	})
	for _, want := range []string{"/skills", "/skill", "/use"} {
		if !strings.Contains(output, want) {
			t.Errorf("handleSkillCommand() no-args output missing %q:\n%s", want, output)
		}
	}
}

func TestHandleKeyCommand_NoArgs(t *testing.T) {
	// No-arg /key is backward-compatible: shows active provider's key (masked).
	cfg := config.Default()
	cfg.Providers = map[string]config.ProviderConfig{
		"anthropic": {APIKey: "sk-noarg-key-verylongkey", Models: []string{"claude-sonnet-4-6"}},
	}
	cfg.DefaultProvider = "anthropic"
	output := captureStderr(func() {
		handleKeyCommand([]string{}, cfg, nil)
	})
	if strings.Contains(output, "sk-noarg-key-verylongkey") {
		t.Error("handleKeyCommand() no-args must not expose the full API key")
	}
	if !strings.Contains(output, "***") {
		t.Error("handleKeyCommand() no-args output should contain masked key marker ***")
	}
}

func TestHandleKeyCommand_Help(t *testing.T) {
	cfg := config.Default()
	output := captureStderr(func() {
		handleKeyCommand([]string{"help"}, cfg, nil)
	})
	for _, want := range []string{"/key", "set"} {
		if !strings.Contains(output, want) {
			t.Errorf("handleKeyCommand(help) output missing %q:\n%s", want, output)
		}
	}
}

func TestHandleConfigCommand_Reload(t *testing.T) {
	// Create a temporary config file.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	cfg := config.Default()
	cfg.Providers = map[string]config.ProviderConfig{
		"anthropic": {APIKey: "sk-test-original", Models: []string{"claude-sonnet-4-6"}},
	}
	cfg.DefaultProvider = "anthropic"
	cfg.FallbackChain = []config.FallbackEntry{
		{Provider: "anthropic", Model: "claude-sonnet-4-6"},
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(cfgPath, data, 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Point the config flag to the temp file.
	oldVal := *configFlag
	*configFlag = cfgPath
	defer func() { *configFlag = oldVal }()

	// Modify the file on disk.
	cfg.ApprovalMode = "none"
	data, _ = yaml.Marshal(cfg)
	os.WriteFile(cfgPath, data, 0600)

	// Reload into the live config.
	liveCfg := config.Default()
	liveCfg.Providers = map[string]config.ProviderConfig{
		"anthropic": {APIKey: "sk-test-original", Models: []string{"claude-sonnet-4-6"}},
	}
	liveCfg.DefaultProvider = "anthropic"
	liveCfg.FallbackChain = []config.FallbackEntry{
		{Provider: "anthropic", Model: "claude-sonnet-4-6"},
	}

	handleConfigCommand([]string{"reload"}, liveCfg)

	if liveCfg.ApprovalMode != "none" {
		t.Errorf("ApprovalMode = %q after reload, want %q", liveCfg.ApprovalMode, "none")
	}
}

func TestHandleDirCommand_Override(t *testing.T) {
	dir := t.TempDir()

	// Reset global state.
	oldOverride := workDirOverride
	defer func() { workDirOverride = oldOverride }()
	workDirOverride = ""

	cfg := config.Default()

	handleDirCommand([]string{dir}, cfg)

	if workDirOverride == "" {
		t.Fatal("workDirOverride should be set after /dir <path>")
	}
	abs, _ := filepath.Abs(dir)
	if workDirOverride != abs {
		t.Errorf("workDirOverride = %q, want %q", workDirOverride, abs)
	}

	// resolveWorkDir should now return the override.
	got := resolveWorkDir(cfg)
	if got != abs {
		t.Errorf("resolveWorkDir = %q, want %q", got, abs)
	}
}

func TestHandleDirCommand_NonExistentPath(t *testing.T) {
	oldOverride := workDirOverride
	defer func() { workDirOverride = oldOverride }()
	workDirOverride = ""

	cfg := config.Default()

	handleDirCommand([]string{"/nonexistent/path/that/should/not/exist"}, cfg)

	if workDirOverride != "" {
		t.Error("workDirOverride should remain empty for non-existent path")
	}
}

func TestHandleDirCommand_OutsideAllowedDirs(t *testing.T) {
	allowed := t.TempDir()
	outside := t.TempDir()

	oldOverride := workDirOverride
	defer func() { workDirOverride = oldOverride }()
	workDirOverride = ""

	cfg := config.Default()
	cfg.Sandbox.AllowedDirs = []string{allowed}

	handleDirCommand([]string{outside}, cfg)

	if workDirOverride != "" {
		t.Error("workDirOverride should remain empty for path outside allowed dirs")
	}
}

func TestShowMaskedConfig_MasksAPIKeys(t *testing.T) {
	cfg := config.Default()
	cfg.Providers = map[string]config.ProviderConfig{
		"anthropic": {APIKey: "sk-ant-api03-verylongapikeythatshouldbepartiallymasked", Models: []string{"claude-sonnet-4-6"}},
	}

	// Capture stderr output.
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	showMaskedConfig(cfg)

	w.Close()
	os.Stderr = old

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	// The full API key should NOT appear in output.
	if strings.Contains(output, "sk-ant-api03-verylongapikeythatshouldbepartiallymasked") {
		t.Error("full API key should not appear in masked config output")
	}
	// The masked version should appear.
	if !strings.Contains(output, "***...") {
		t.Error("masked config output should contain ***... for API key")
	}
}

func TestMaskKey(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{"short", "***"},
		{"12345678", "***"},
		{"123456789", "***...23456789"},
		{"sk-ant-api03-verylongkey", "***...ylongkey"},
	}
	for _, tt := range tests {
		got := maskKey(tt.key)
		if got != tt.want {
			t.Errorf("maskKey(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestDetectProvider(t *testing.T) {
	tests := []struct {
		model string
		want  string
	}{
		{"claude-sonnet-4-6", "anthropic"},
		{"claude-opus-4-6", "anthropic"},
		{"claude-haiku-4-5-20251001", "anthropic"},
		{"haiku", "anthropic"},
		{"sonnet", "anthropic"},
		{"opus", "anthropic"},
		{"gpt-4o", "openai"},
		{"gpt-4o-mini", "openai"},
		{"o3-mini", "openai"},
		{"gemini-2.5-pro", "gemini"},
		{"gemini-2.5-flash", "gemini"},
		{"unknown-model", ""},
		{"llama-3", ""},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := detectProvider(tt.model)
			if got != tt.want {
				t.Errorf("detectProvider(%q) = %q, want %q", tt.model, got, tt.want)
			}
		})
	}
}

func TestHandleModelCommand_CrossProvider(t *testing.T) {
	cfg := config.Default()
	cfg.Providers = map[string]config.ProviderConfig{
		"anthropic": {APIKey: "sk-ant", Models: []string{"claude-sonnet-4-6"}},
		"openai":    {APIKey: "sk-oai", Models: []string{"gpt-4o"}},
		"gemini":    {APIKey: "gem-key", Models: []string{"gemini-2.5-pro"}},
	}
	cfg.FallbackChain = []config.FallbackEntry{
		{Provider: "anthropic", Model: "claude-sonnet-4-6"},
	}
	cfg.DefaultProvider = "anthropic"

	// Switch to OpenAI.
	handleModelCommand([]string{"gpt-4o"}, cfg)
	if cfg.FallbackChain[0].Provider != "openai" {
		t.Errorf("provider = %q, want openai", cfg.FallbackChain[0].Provider)
	}
	if cfg.FallbackChain[0].Model != "gpt-4o" {
		t.Errorf("model = %q, want gpt-4o", cfg.FallbackChain[0].Model)
	}

	// Switch to Gemini.
	handleModelCommand([]string{"gemini-2.5-pro"}, cfg)
	if cfg.FallbackChain[0].Provider != "gemini" {
		t.Errorf("provider = %q, want gemini", cfg.FallbackChain[0].Provider)
	}

	// Switch back via alias.
	handleModelCommand([]string{"sonnet"}, cfg)
	if cfg.FallbackChain[0].Provider != "anthropic" {
		t.Errorf("provider = %q, want anthropic", cfg.FallbackChain[0].Provider)
	}
	if cfg.FallbackChain[0].Model != "claude-sonnet-4-6" {
		t.Errorf("model = %q, want claude-sonnet-4-6", cfg.FallbackChain[0].Model)
	}
}

// captureBoth runs f() and returns everything written to os.Stdout and os.Stderr.
func captureBoth(f func()) (stdout, stderr string) {
	// Capture stdout.
	oldOut := os.Stdout
	rOut, wOut, _ := os.Pipe()
	os.Stdout = wOut

	// Capture stderr.
	oldErr := os.Stderr
	rErr, wErr, _ := os.Pipe()
	os.Stderr = wErr

	f()

	wOut.Close()
	wErr.Close()
	os.Stdout = oldOut
	os.Stderr = oldErr

	outBuf, _ := io.ReadAll(rOut)
	errBuf, _ := io.ReadAll(rErr)
	return string(outBuf), string(errBuf)
}

func TestDisplayAgentResult(t *testing.T) {
	tests := []struct {
		name           string
		result         *agent.Result
		wantStdout     string
		notWantStderr  string
		wantStderrFrag string
	}{
		{
			name: "conversational reply shown on stdout",
			result: &agent.Result{
				Success:     true,
				Plan:        &agent.Plan{Description: "Hello! How can I help?"},
				StepResults: nil,
			},
			wantStdout:    "Hello! How can I help?",
			notWantStderr: "No actionable steps",
		},
		{
			name: "empty plan shows warning on stderr",
			result: &agent.Result{
				Success:        false,
				Plan:           &agent.Plan{Description: ""},
				StepResults:    nil,
			},
			wantStdout:     "",
			wantStderrFrag: "No actionable steps",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr := captureBoth(func() {
				displayAgentResult(tt.result)
			})

			if tt.wantStdout != "" && !strings.Contains(stdout, tt.wantStdout) {
				t.Errorf("stdout = %q, want to contain %q", stdout, tt.wantStdout)
			}
			if tt.notWantStderr != "" && strings.Contains(stderr, tt.notWantStderr) {
				t.Errorf("stderr = %q, must NOT contain %q", stderr, tt.notWantStderr)
			}
			if tt.wantStderrFrag != "" && !strings.Contains(stderr, tt.wantStderrFrag) {
				t.Errorf("stderr = %q, want to contain %q", stderr, tt.wantStderrFrag)
			}
		})
	}
}
