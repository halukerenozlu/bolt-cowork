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
	"github.com/halukerenozlu/bolt-cowork/internal/tool"
	"github.com/halukerenozlu/bolt-cowork/pkg/types"
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

func TestSkillNameValidation(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "empty", in: "", want: false},
		{name: "starts lowercase", in: "reviewer", want: true},
		{name: "allows digits and hyphen", in: "go-reviewer-2", want: true},
		{name: "starts uppercase", in: "Reviewer", want: false},
		{name: "starts digit", in: "2-reviewer", want: false},
		{name: "underscore", in: "go_reviewer", want: false},
		{name: "space", in: "go reviewer", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidSkillName(tt.in); got != tt.want {
				t.Fatalf("isValidSkillName(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestLowerArgs(t *testing.T) {
	got := lowerArgs([]string{"OpenAI", "GPT-4O", "MiXeD"})
	want := []string{"openai", "gpt-4o", "mixed"}

	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("lowerArgs() = %v, want %v", got, want)
	}
}

func TestHandleSkillsCommand(t *testing.T) {
	store := testSkillStore()

	output := captureStderr(func() {
		handleSkillsCommand(store)
	})

	for _, want := range []string{"Loaded skills (2)", "reviewer", "summarizer", "* = auto_trigger enabled"} {
		if !strings.Contains(output, want) {
			t.Fatalf("handleSkillsCommand() output = %q, want to contain %q", output, want)
		}
	}
}

func TestHandleSkillsCommandEmptyAndNil(t *testing.T) {
	tests := []struct {
		name  string
		store *skill.Store
		want  string
	}{
		{name: "nil store", store: nil, want: "no skill store available"},
		{name: "empty store", store: skill.NewStore(), want: "no skills loaded"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStderr(func() {
				handleSkillsCommand(tt.store)
			})
			if !strings.Contains(output, tt.want) {
				t.Fatalf("output = %q, want to contain %q", output, tt.want)
			}
		})
	}
}

func TestHandleSkillCommandDetailsAndSuggestion(t *testing.T) {
	store := testSkillStore()

	output := captureStderr(func() {
		handleSkillCommand([]string{"reviewer"}, store)
	})
	for _, want := range []string{"Name:", "reviewer", "Review code", "Content (first 5 lines)", "/use reviewer"} {
		if !strings.Contains(output, want) {
			t.Fatalf("handleSkillCommand() output = %q, want to contain %q", output, want)
		}
	}

	output = captureStderr(func() {
		handleSkillCommand([]string{"reviewr"}, store)
	})
	if !strings.Contains(output, `did you mean "reviewer"`) {
		t.Fatalf("suggestion output = %q, want reviewer suggestion", output)
	}
}

func TestHandleUseCommand(t *testing.T) {
	store := testSkillStore()
	forceSkills := []string{}

	output := captureStderr(func() {
		handleUseCommand([]string{}, store, &forceSkills)
	})
	if !strings.Contains(output, "Usage: /use") {
		t.Fatalf("no-arg output = %q, want usage", output)
	}

	output = captureStderr(func() {
		handleUseCommand([]string{"reviewer"}, store, &forceSkills)
	})
	if !strings.Contains(output, `Skill "reviewer" activated`) {
		t.Fatalf("activation output = %q", output)
	}
	if len(forceSkills) != 1 || forceSkills[0] != "reviewer" {
		t.Fatalf("forceSkills = %v, want [reviewer]", forceSkills)
	}

	output = captureStderr(func() {
		handleUseCommand([]string{"reviewer"}, store, &forceSkills)
	})
	if !strings.Contains(output, "already activated") {
		t.Fatalf("duplicate output = %q, want already activated", output)
	}

	output = captureStderr(func() {
		handleUseCommand([]string{"summarizr"}, store, &forceSkills)
	})
	if !strings.Contains(output, `did you mean "summarizer"`) {
		t.Fatalf("missing skill output = %q, want suggestion", output)
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

	handleDirCommand([]string{dir}, cfg, nil, nil, nil, func(_ *config.Config, _ string) bool { return true })

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

	handleDirCommand([]string{"/nonexistent/path/that/should/not/exist"}, cfg, nil, nil, nil, func(_ *config.Config, _ string) bool { return true })

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

	handleDirCommand([]string{outside}, cfg, nil, nil, nil, func(_ *config.Config, _ string) bool { return true })

	if workDirOverride != "" {
		t.Error("workDirOverride should remain empty for path outside allowed dirs")
	}
}

func TestDirShow(t *testing.T) {
	dir := t.TempDir()

	oldOverride := workDirOverride
	defer func() { workDirOverride = oldOverride }()
	workDirOverride = dir

	cfg := config.Default()
	abs, _ := filepath.Abs(dir)

	output := captureStderr(func() {
		handleDirCommand([]string{}, cfg, nil, nil, nil, func(_ *config.Config, _ string) bool { return true })
	})

	if !strings.Contains(output, abs) {
		t.Errorf("output = %q, want to contain absolute path %q", output, abs)
	}
	if !strings.Contains(output, "Current workspace") {
		t.Errorf("output = %q, want to contain 'Current workspace'", output)
	}
}

func TestDirChange(t *testing.T) {
	dir := t.TempDir()

	oldOverride := workDirOverride
	defer func() { workDirOverride = oldOverride }()
	workDirOverride = ""

	cfg := config.Default()
	abs, _ := filepath.Abs(dir)
	var history []types.Message
	history = append(history, types.Message{Role: "user", Content: "old"})
	var previousDir string

	output := captureStderr(func() {
		handleDirCommand([]string{dir}, cfg, &history, nil, &previousDir, func(_ *config.Config, _ string) bool { return true })
	})

	if workDirOverride != abs {
		t.Errorf("workDirOverride = %q, want %q", workDirOverride, abs)
	}
	if len(history) != 0 {
		t.Errorf("history should be cleared after /dir change, got %d entries", len(history))
	}
	if !strings.Contains(output, "changed") {
		t.Errorf("output = %q, want to contain 'changed'", output)
	}
}

func TestDirNotFound(t *testing.T) {
	oldOverride := workDirOverride
	defer func() { workDirOverride = oldOverride }()
	workDirOverride = ""

	cfg := config.Default()

	output := captureStderr(func() {
		handleDirCommand([]string{"/no/such/dir/xyz"}, cfg, nil, nil, nil, func(_ *config.Config, _ string) bool { return true })
	})

	if workDirOverride != "" {
		t.Error("workDirOverride should remain empty for non-existent path")
	}
	if !strings.Contains(output, "not found") && !strings.Contains(output, "Directory not found") {
		t.Errorf("output = %q, want to contain 'not found'", output)
	}
}

func TestDirNotDirectory(t *testing.T) {
	dir := t.TempDir()
	f, err := os.CreateTemp(dir, "testfile")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	f.Close()
	filePath := f.Name()

	oldOverride := workDirOverride
	defer func() { workDirOverride = oldOverride }()
	workDirOverride = ""

	cfg := config.Default()

	output := captureStderr(func() {
		handleDirCommand([]string{filePath}, cfg, nil, nil, nil, func(_ *config.Config, _ string) bool { return true })
	})

	if workDirOverride != "" {
		t.Error("workDirOverride should remain empty for file path")
	}
	if !strings.Contains(output, "not a directory") {
		t.Errorf("output = %q, want to contain 'not a directory'", output)
	}
}

func TestDirBack(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	abs1, _ := filepath.Abs(dir1)
	abs2, _ := filepath.Abs(dir2)

	oldOverride := workDirOverride
	defer func() { workDirOverride = oldOverride }()
	workDirOverride = ""

	cfg := config.Default()
	previousDir := abs1

	// Set current to dir2, then go back to dir1 via /dir -.
	workDirOverride = abs2

	output := captureStderr(func() {
		handleDirCommand([]string{"-"}, cfg, nil, nil, &previousDir, func(_ *config.Config, _ string) bool { return true })
	})

	if workDirOverride != abs1 {
		t.Errorf("workDirOverride = %q after /dir -, want %q", workDirOverride, abs1)
	}
	if previousDir != abs2 {
		t.Errorf("previousDir = %q after /dir -, want %q", previousDir, abs2)
	}
	if !strings.Contains(output, "changed") {
		t.Errorf("output = %q, want to contain 'changed'", output)
	}
}

func TestDirBackOutsideAllowedDirs(t *testing.T) {
	allowed := t.TempDir()
	outside := t.TempDir()
	absAllowed, _ := filepath.Abs(allowed)
	absOutside, _ := filepath.Abs(outside)

	oldOverride := workDirOverride
	defer func() { workDirOverride = oldOverride }()
	workDirOverride = absAllowed

	cfg := config.Default()
	cfg.Sandbox.AllowedDirs = []string{allowed}
	previousDir := absOutside

	output := captureStderr(func() {
		handleDirCommand([]string{"-"}, cfg, nil, nil, &previousDir, func(_ *config.Config, _ string) bool { return true })
	})

	if workDirOverride != absAllowed {
		t.Errorf("workDirOverride = %q, want unchanged %q", workDirOverride, absAllowed)
	}
	if !strings.Contains(output, "allowed directories") {
		t.Errorf("output = %q, want to contain 'allowed directories'", output)
	}
}

func TestDirCommand_RelativeToWorkspace(t *testing.T) {
	// Create workspace with a subdirectory.
	workspace := t.TempDir()
	subdir := filepath.Join(workspace, "src")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	absWorkspace, _ := filepath.Abs(workspace)
	absSubdir, _ := filepath.Abs(subdir)

	oldOverride := workDirOverride
	defer func() { workDirOverride = oldOverride }()
	workDirOverride = absWorkspace

	cfg := config.Default()

	// /dir src should resolve relative to workspace, not process cwd.
	output := captureStderr(func() {
		handleDirCommand([]string{"src"}, cfg, nil, nil, nil, func(_ *config.Config, _ string) bool { return true })
	})

	if workDirOverride != absSubdir {
		t.Errorf("workDirOverride = %q, want %q (relative to workspace)", workDirOverride, absSubdir)
	}
	if !strings.Contains(output, "changed") {
		t.Errorf("output = %q, want to contain 'changed'", output)
	}
}

func TestDirCommand_DotDotNormalized(t *testing.T) {
	// Create parent/child directory structure.
	parent := t.TempDir()
	child := filepath.Join(parent, "child")
	if err := os.MkdirAll(child, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	absChild, _ := filepath.Abs(child)
	absParent, _ := filepath.Abs(parent)

	oldOverride := workDirOverride
	defer func() { workDirOverride = oldOverride }()
	workDirOverride = absChild

	cfg := config.Default()

	// /dir .. from child should go to parent.
	output := captureStderr(func() {
		handleDirCommand([]string{".."}, cfg, nil, nil, nil, func(_ *config.Config, _ string) bool { return true })
	})

	if workDirOverride != absParent {
		t.Errorf("workDirOverride = %q, want %q (parent via ..)", workDirOverride, absParent)
	}
	if !strings.Contains(output, "changed") {
		t.Errorf("output = %q, want to contain 'changed'", output)
	}
}

func TestDirCommand_TildeExpansion(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("USERPROFILE", fakeHome)

	oldOverride := workDirOverride
	defer func() { workDirOverride = oldOverride }()
	workDirOverride = ""

	cfg := config.Default()

	// /dir ~ should resolve to home directory.
	output := captureStderr(func() {
		handleDirCommand([]string{"~"}, cfg, nil, nil, nil, func(_ *config.Config, _ string) bool { return true })
	})

	absHome, _ := filepath.Abs(fakeHome)
	if workDirOverride != absHome {
		t.Errorf("workDirOverride = %q, want %q (tilde expansion)", workDirOverride, absHome)
	}
	if !strings.Contains(output, "changed") {
		t.Errorf("output = %q, want to contain 'changed'", output)
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
	// The masked version should appear in the keyring status line.
	if !strings.Contains(output, "***...") {
		t.Error("masked config output should contain ***... for API key")
	}
	// API key should NOT appear in the YAML body (yaml:"-").
	if strings.Contains(output, "api_key:") && !strings.Contains(output, "# ") {
		t.Error("api_key should not appear in YAML body")
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

func TestActiveProviderAndModelFallbacks(t *testing.T) {
	cfg := config.Default()
	cfg.DefaultProvider = "anthropic"
	cfg.Providers = map[string]config.ProviderConfig{
		"anthropic": {Models: []string{"claude-sonnet-4-6"}},
	}

	if got := activeProvider(cfg); got != "anthropic" {
		t.Fatalf("activeProvider() = %q, want anthropic", got)
	}
	if got := activeModel(cfg); got != "claude-sonnet-4-6" {
		t.Fatalf("activeModel() = %q, want claude-sonnet-4-6", got)
	}

	cfg.FallbackChain = []config.FallbackEntry{{Provider: "openai", Model: "gpt-4o"}}
	if got := activeProvider(cfg); got != "openai" {
		t.Fatalf("activeProvider() with fallback = %q, want openai", got)
	}
	if got := activeModel(cfg); got != "gpt-4o" {
		t.Fatalf("activeModel() with fallback = %q, want gpt-4o", got)
	}

	cfg = config.Default()
	cfg.Providers = nil
	if got := activeModel(cfg); got != "(unknown)" {
		t.Fatalf("activeModel() without provider models = %q, want (unknown)", got)
	}
}

func TestHandleModelCommandCreatesFallbackAndAddsModel(t *testing.T) {
	cfg := config.Default()
	cfg.Providers = map[string]config.ProviderConfig{
		"openai": {Models: []string{"gpt-4o"}},
	}
	cfg.FallbackChain = nil

	output := captureStderr(func() {
		handleModelCommand([]string{"gpt-4.1"}, cfg)
	})

	if !strings.Contains(output, "Switched to openai/gpt-4.1") {
		t.Fatalf("output = %q, want switch message", output)
	}
	if cfg.DefaultProvider != "openai" {
		t.Fatalf("DefaultProvider = %q, want openai", cfg.DefaultProvider)
	}
	if len(cfg.FallbackChain) != 1 || cfg.FallbackChain[0].Model != "gpt-4.1" {
		t.Fatalf("FallbackChain = %#v, want gpt-4.1 entry", cfg.FallbackChain)
	}
	if !containsString(cfg.Providers["openai"].Models, "gpt-4.1") {
		t.Fatalf("openai models = %v, want gpt-4.1 appended", cfg.Providers["openai"].Models)
	}
}

func TestHandleModelCommandErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		cfg  *config.Config
		want string
	}{
		{
			name: "show current",
			args: nil,
			cfg: func() *config.Config {
				cfg := config.Default()
				cfg.Providers = map[string]config.ProviderConfig{
					"anthropic": {Models: []string{"claude-sonnet-4-6"}},
				}
				return cfg
			}(),
			want: "Current model:",
		},
		{
			name: "unknown model",
			args: []string{"llama-3"},
			cfg:  config.Default(),
			want: "Unknown model",
		},
		{
			name: "provider not configured",
			args: []string{"gpt-4o"},
			cfg:  config.Default(),
			want: `Provider "openai" is not configured`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStderr(func() {
				handleModelCommand(tt.args, tt.cfg)
			})
			if !strings.Contains(output, tt.want) {
				t.Fatalf("output = %q, want to contain %q", output, tt.want)
			}
		})
	}
}

// captureBoth runs f() and returns everything written to os.Stdout and os.Stderr.
func captureBoth(t *testing.T, f func()) (stdout, stderr string) {
	t.Helper()

	// Capture stdout.
	oldOut := os.Stdout
	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}
	os.Stdout = wOut

	// Capture stderr.
	oldErr := os.Stderr
	rErr, wErr, err := os.Pipe()
	if err != nil {
		wOut.Close()
		os.Stdout = oldOut
		t.Fatalf("failed to create stderr pipe: %v", err)
	}
	os.Stderr = wErr

	f()

	wOut.Close()
	wErr.Close()
	os.Stdout = oldOut
	os.Stderr = oldErr

	outBuf, err := io.ReadAll(rOut)
	if err != nil {
		t.Fatalf("failed to read stdout pipe: %v", err)
	}
	errBuf, err := io.ReadAll(rErr)
	if err != nil {
		t.Fatalf("failed to read stderr pipe: %v", err)
	}
	return string(outBuf), string(errBuf)
}

// mockLineReader satisfies lineReader for testing, returning preset values.
type mockLineReader struct {
	line   string
	masked string
}

func (m *mockLineReader) ReadLine() (string, error)                   { return m.line, nil }
func (m *mockLineReader) ReadLineWithPrompt(_ string) (string, error) { return m.line, nil }
func (m *mockLineReader) ReadMasked(_ string) (string, error)         { return m.masked, nil }

func TestKeySetNewProvider(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	old := *configFlag
	*configFlag = cfgPath
	defer func() { *configFlag = old }()

	cfg := config.Default()
	// No providers configured.
	cfg.Providers = make(map[string]config.ProviderConfig)

	lr := &mockLineReader{masked: "gemini-test-key-long"}

	output := captureStderr(func() {
		handleKeyCommand([]string{"set", "gemini"}, cfg, lr)
	})

	if cfg.Providers["gemini"].APIKey != "gemini-test-key-long" {
		t.Errorf("API key not set; providers = %v", cfg.Providers)
	}
	if !strings.Contains(output, "created with default settings") {
		t.Errorf("expected 'created with default settings' in output, got: %s", output)
	}
}

func TestKeySetNewProviderWithExistingProviders(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	old := *configFlag
	*configFlag = cfgPath
	defer func() { *configFlag = old }()

	cfg := config.Default()
	cfg.Providers = map[string]config.ProviderConfig{
		"anthropic": {APIKey: "sk-ant", Models: []string{"claude-sonnet-4-6"}},
	}

	lr := &mockLineReader{masked: "gemini-new-key-long"}

	output := captureStderr(func() {
		handleKeyCommand([]string{"set", "gemini"}, cfg, lr)
	})

	if cfg.Providers["gemini"].APIKey != "gemini-new-key-long" {
		t.Errorf("API key not set; providers = %v", cfg.Providers)
	}
	if !strings.Contains(output, "added to config") {
		t.Errorf("expected 'added to config' in output, got: %s", output)
	}
}

func TestKeySetFirstProviderSetsDefault(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	old := *configFlag
	*configFlag = cfgPath
	defer func() { *configFlag = old }()

	cfg := config.Default()
	cfg.DefaultProvider = "anthropic" // default from config.Default()
	cfg.Providers = make(map[string]config.ProviderConfig)

	lr := &mockLineReader{masked: "gemini-first-key-long"}

	captureStderr(func() {
		handleKeyCommand([]string{"set", "gemini"}, cfg, lr)
	})

	if cfg.DefaultProvider != "gemini" {
		t.Errorf("DefaultProvider = %q after first provider added, want %q", cfg.DefaultProvider, "gemini")
	}
	// Validate should pass: default_provider must exist in providers.
	cfg.FallbackChain = nil // remove chain to isolate provider validation
	if err := cfg.Validate(); err != nil {
		t.Errorf("cfg.Validate() after first provider set: %v", err)
	}
}

func TestKeySetExistingProvider(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	old := *configFlag
	*configFlag = cfgPath
	defer func() { *configFlag = old }()

	cfg := config.Default()
	cfg.Providers = map[string]config.ProviderConfig{
		"anthropic": {APIKey: "sk-old", Models: []string{"claude-sonnet-4-6"}},
	}
	cfg.DefaultProvider = "anthropic"

	lr := &mockLineReader{masked: "sk-new-key-very-long"}

	output := captureStderr(func() {
		handleKeyCommand([]string{"set", "anthropic"}, cfg, lr)
	})

	if cfg.Providers["anthropic"].APIKey != "sk-new-key-very-long" {
		t.Errorf("API key not updated; got %q", cfg.Providers["anthropic"].APIKey)
	}
	if !strings.Contains(output, "API key updated for") {
		t.Errorf("expected 'API key updated for' in output, got: %s", output)
	}
}

func TestKeySetUnknownProvider(t *testing.T) {
	cfg := config.Default()
	cfg.Providers = make(map[string]config.ProviderConfig)

	output := captureStderr(func() {
		handleKeyCommand([]string{"set", "xyz"}, cfg, nil)
	})

	if !strings.Contains(output, "Unknown provider") {
		t.Errorf("expected 'Unknown provider' in output, got: %s", output)
	}
	if !strings.Contains(output, "anthropic") {
		t.Errorf("error should list supported providers, got: %s", output)
	}
}

func TestModeShow(t *testing.T) {
	cfg := config.Default()
	cfg.ApprovalMode = "plan-only"

	output := captureStderr(func() {
		handleModeCommand([]string{}, cfg)
	})

	if !strings.Contains(output, "plan-only") {
		t.Errorf("expected current mode in output, got: %s", output)
	}
}

func TestModeChange(t *testing.T) {
	cfg := config.Default()
	cfg.ApprovalMode = "full"

	output := captureStderr(func() {
		handleModeCommand([]string{"build"}, cfg)
	})

	if cfg.ApprovalMode != "dangerous-only" {
		t.Errorf("ApprovalMode = %q after /mode build, want 'dangerous-only'", cfg.ApprovalMode)
	}
	if !strings.Contains(output, "build") {
		t.Errorf("expected mode alias 'build' in output, got: %s", output)
	}
}

func TestModeInvalid(t *testing.T) {
	cfg := config.Default()
	cfg.ApprovalMode = "full"

	output := captureStderr(func() {
		handleModeCommand([]string{"xyz"}, cfg)
	})

	if cfg.ApprovalMode != "full" {
		t.Error("ApprovalMode should not change on invalid /mode input")
	}
	if !strings.Contains(output, "Unknown mode") {
		t.Errorf("expected 'Unknown mode' in output, got: %s", output)
	}
}

func TestHandleDirCommand_UntrustedBlocked(t *testing.T) {
	dir := t.TempDir()

	oldOverride := workDirOverride
	defer func() { workDirOverride = oldOverride }()
	workDirOverride = ""

	cfg := config.Default() // cfg.TrustedDirs is empty

	captureStderr(func() {
		handleDirCommand([]string{dir}, cfg, nil, nil, nil, config.IsTrusted)
	})

	if workDirOverride != "" {
		t.Error("workDirOverride should remain empty for untrusted directory")
	}
}

func TestHandleDirCommand_UntrustedDirBack(t *testing.T) {
	previousDir := t.TempDir()

	oldOverride := workDirOverride
	defer func() { workDirOverride = oldOverride }()
	workDirOverride = ""

	cfg := config.Default() // cfg.TrustedDirs is empty

	captureStderr(func() {
		handleDirCommand([]string{"-"}, cfg, nil, nil, &previousDir, config.IsTrusted)
	})

	if workDirOverride != "" {
		t.Error("workDirOverride should remain empty for untrusted previous directory")
	}
}

func TestDisplayAgentResult(t *testing.T) {
	tests := []struct {
		name           string
		result         *agent.Result
		wantStdout     string
		notWantStdout  string
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
				Success:     false,
				Plan:        &agent.Plan{Description: ""},
				StepResults: nil,
			},
			wantStdout:     "",
			wantStderrFrag: "No actionable steps",
		},
		{
			name: "list result is rendered without transport JSON",
			result: &agent.Result{
				Success: true,
				StepResults: []string{
					tool.FormatListOutput(".", []string{"docs/", "report, final.pdf"}),
				},
			},
			wantStdout:    "docs/\n     report, final.pdf",
			notWantStdout: `"entries"`,
		},
		{
			name: "auto-approved empty list keeps its marker",
			result: &agent.Result{
				Success:     true,
				StepResults: []string{"[auto] " + tool.FormatListOutput("empty", nil)},
			},
			wantStdout:    "[auto] (empty)",
			notWantStdout: `"path"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr := captureBoth(t, func() {
				displayAgentResult(tt.result)
			})

			if tt.wantStdout != "" && !strings.Contains(stdout, tt.wantStdout) {
				t.Errorf("stdout = %q, want to contain %q", stdout, tt.wantStdout)
			}
			if tt.notWantStdout != "" && strings.Contains(stdout, tt.notWantStdout) {
				t.Errorf("stdout = %q, must NOT contain %q", stdout, tt.notWantStdout)
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

func testSkillStore() *skill.Store {
	store := skill.NewStore()
	store.Upsert(&skill.Skill{
		Metadata: skill.SkillMetadata{
			Name:        "reviewer",
			Description: "Review code",
			AutoTrigger: true,
		},
		Scope:    skill.ScopeProject,
		Content:  "Line 1\nLine 2\nLine 3\nLine 4\nLine 5\nLine 6",
		FilePath: "reviewer/SKILL.md",
	})
	store.Upsert(&skill.Skill{
		Metadata: skill.SkillMetadata{
			Name:        "summarizer",
			Description: "Summarize text",
		},
		Scope:    skill.ScopeBundled,
		Content:  "Summarize carefully.",
		FilePath: "summarizer/SKILL.md",
	})
	return store
}
