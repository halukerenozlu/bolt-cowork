package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	keyring "github.com/zalando/go-keyring"
)

// TestMain initialises an in-memory keyring mock for all tests in this
// package so that keyring calls never touch the real system credential store.
func TestMain(m *testing.M) {
	keyring.MockInit()
	os.Exit(m.Run())
}

func TestDefault(t *testing.T) {
	cfg := Default()

	if cfg.DefaultProvider != "anthropic" {
		t.Errorf("DefaultProvider = %q, want %q", cfg.DefaultProvider, "anthropic")
	}
	if cfg.ApprovalMode != "full" {
		t.Errorf("ApprovalMode = %q, want %q", cfg.ApprovalMode, "full")
	}
	if cfg.Theme != "dark" {
		t.Errorf("Theme = %q, want %q", cfg.Theme, "dark")
	}
	if len(cfg.Sandbox.DeniedPatterns) != len(defaultDeniedPatterns) {
		t.Errorf("DeniedPatterns count = %d, want %d", len(cfg.Sandbox.DeniedPatterns), len(defaultDeniedPatterns))
	}
}

func TestLoadFile_Valid(t *testing.T) {
	// Use the project's testdata fixture.
	path := filepath.Join("..", "..", "testdata", "fixtures", "sample-config.yaml")
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile(%q): %v", path, err)
	}

	if cfg.DefaultProvider != "anthropic" {
		t.Errorf("DefaultProvider = %q, want %q", cfg.DefaultProvider, "anthropic")
	}

	if len(cfg.Providers) != 2 {
		t.Errorf("Providers count = %d, want 2", len(cfg.Providers))
	}

	anthropic, ok := cfg.Providers["anthropic"]
	if !ok {
		t.Fatal("missing anthropic provider")
	}
	if len(anthropic.Models) != 2 {
		t.Errorf("anthropic models count = %d, want 2", len(anthropic.Models))
	}

	if len(cfg.FallbackChain) != 3 {
		t.Errorf("FallbackChain count = %d, want 3", len(cfg.FallbackChain))
	}

	if cfg.Sandbox.AllowedDirs[0] != "./workspace" {
		t.Errorf("AllowedDirs[0] = %q, want %q", cfg.Sandbox.AllowedDirs[0], "./workspace")
	}

	if len(cfg.Sandbox.ReadOnlyDirs) != 2 {
		t.Errorf("ReadOnlyDirs count = %d, want 2", len(cfg.Sandbox.ReadOnlyDirs))
	}
	if len(cfg.Sandbox.ReadOnlyDirs) >= 2 {
		if cfg.Sandbox.ReadOnlyDirs[0] != "./docs" {
			t.Errorf("ReadOnlyDirs[0] = %q, want %q", cfg.Sandbox.ReadOnlyDirs[0], "./docs")
		}
		if cfg.Sandbox.ReadOnlyDirs[1] != "./reference" {
			t.Errorf("ReadOnlyDirs[1] = %q, want %q", cfg.Sandbox.ReadOnlyDirs[1], "./reference")
		}
	}

	if len(cfg.Sandbox.DeniedPatterns) != 3 {
		t.Errorf("DeniedPatterns count = %d, want 3", len(cfg.Sandbox.DeniedPatterns))
	}

	if cfg.ApprovalMode != "full" {
		t.Errorf("ApprovalMode = %q, want %q", cfg.ApprovalMode, "full")
	}
}

func TestLoadFile_NonExistent(t *testing.T) {
	_, err := LoadFile(filepath.Join(t.TempDir(), "nonexistent.yaml"))
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoadFile_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	os.WriteFile(path, []byte("{{invalid: yaml: [}"), 0644)

	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadFile_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.yaml")
	os.WriteFile(path, []byte(""), 0644)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile empty: %v", err)
	}
	// Should have defaults applied.
	if cfg.ApprovalMode != "full" {
		t.Errorf("ApprovalMode = %q, want %q", cfg.ApprovalMode, "full")
	}
}

func TestExpandEnvVars(t *testing.T) {
	tests := []struct {
		name  string
		input string
		env   map[string]string
		want  string
	}{
		{
			name:  "single var",
			input: "${MY_KEY}",
			env:   map[string]string{"MY_KEY": "secret123"},
			want:  "secret123",
		},
		{
			name:  "var in context",
			input: "api_key: ${API_KEY}",
			env:   map[string]string{"API_KEY": "sk-abc"},
			want:  "api_key: sk-abc",
		},
		{
			name:  "multiple vars",
			input: "${A} and ${B}",
			env:   map[string]string{"A": "hello", "B": "world"},
			want:  "hello and world",
		},
		{
			name:  "unset var becomes empty",
			input: "${UNSET_VAR}",
			env:   map[string]string{},
			want:  "",
		},
		{
			name:  "no vars unchanged",
			input: "no variables here",
			env:   map[string]string{},
			want:  "no variables here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			got := expandEnvVars(tt.input)
			if got != tt.want {
				t.Errorf("expandEnvVars(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidate_Valid(t *testing.T) {
	cfg := &Config{
		DefaultProvider: "anthropic",
		ApprovalMode:    "full",
		Providers: map[string]ProviderConfig{
			"anthropic": {Models: []string{"claude-opus-4-6", "claude-sonnet-4-6"}},
			"openai":    {Models: []string{"gpt-4o"}},
		},
		FallbackChain: []FallbackEntry{
			{Provider: "anthropic", Model: "claude-opus-4-6"},
			{Provider: "openai", Model: "gpt-4o"},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() returned error for valid config: %v", err)
	}
}

func TestDefault_MCPApprovalModeInheritsGlobalApproval(t *testing.T) {
	cfg := Default()
	if cfg.MCPApprovalMode != "" {
		t.Fatalf("MCPApprovalMode = %q, want empty string", cfg.MCPApprovalMode)
	}
}

func TestLoadFile_MCPApprovalMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	yamlContent := `approval_mode: full
mcp_approval_mode: dangerous-only
`
	if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if cfg.MCPApprovalMode != "dangerous-only" {
		t.Fatalf("MCPApprovalMode = %q, want dangerous-only", cfg.MCPApprovalMode)
	}
}

func TestLoadFile_MCPServerPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	yamlContent := `mcp:
  servers:
    - name: fs
      transport: stdio
      command: fs-mcp
      allowed_tools:
        - read_*
      denied_tools:
        - delete_*
`
	if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if len(cfg.MCP.Servers) != 1 {
		t.Fatalf("MCP.Servers len = %d, want 1", len(cfg.MCP.Servers))
	}
	server := cfg.MCP.Servers[0]
	if len(server.AllowedTools) != 1 || server.AllowedTools[0] != "read_*" {
		t.Fatalf("AllowedTools = %#v, want [read_*]", server.AllowedTools)
	}
	if len(server.DeniedTools) != 1 || server.DeniedTools[0] != "delete_*" {
		t.Fatalf("DeniedTools = %#v, want [delete_*]", server.DeniedTools)
	}
}

func TestValidate_InvalidApprovalMode(t *testing.T) {
	cfg := &Config{
		ApprovalMode: "invalid-mode",
	}
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should reject invalid approval_mode")
	}
}

func TestValidate_MissingDefaultProvider(t *testing.T) {
	cfg := &Config{
		DefaultProvider: "nonexistent",
		ApprovalMode:    "full",
		Providers: map[string]ProviderConfig{
			"anthropic": {Models: []string{"claude-opus-4-6"}},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should reject missing default_provider")
	}
}

func TestValidate_FallbackChainUnknownProvider(t *testing.T) {
	cfg := &Config{
		ApprovalMode: "full",
		Providers: map[string]ProviderConfig{
			"anthropic": {Models: []string{"claude-opus-4-6"}},
		},
		FallbackChain: []FallbackEntry{
			{Provider: "nonexistent", Model: "some-model"},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should reject unknown provider in fallback chain")
	}
}

func TestValidate_FallbackChainUnknownModel(t *testing.T) {
	cfg := &Config{
		ApprovalMode: "full",
		Providers: map[string]ProviderConfig{
			"anthropic": {Models: []string{"claude-opus-4-6"}},
		},
		FallbackChain: []FallbackEntry{
			{Provider: "anthropic", Model: "nonexistent-model"},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should reject unknown model in fallback chain")
	}
}

func TestValidate_FallbackChainDefaultModel(t *testing.T) {
	cfg := &Config{
		DefaultProvider: "anthropic",
		ApprovalMode:    "full",
		Providers: map[string]ProviderConfig{
			"anthropic": {Models: []string{"claude-sonnet-4-6"}},
		},
		FallbackChain: []FallbackEntry{
			{Provider: "anthropic", Model: "claude-opus-4-5"},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() should accept default model in fallback chain: %v", err)
	}
}

func TestLoad_NoConfigFile(t *testing.T) {
	// Set HOME to a temp dir without config file.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.ApprovalMode != "full" {
		t.Errorf("ApprovalMode = %q, want %q", cfg.ApprovalMode, "full")
	}
}

func TestLoadFile_EnvVarExpansion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	yamlContent := `default_provider: anthropic
providers:
  anthropic:
    api_key: ${TEST_ANTHROPIC_KEY}
    models:
      - claude-opus-4-6
approval_mode: full
`
	os.WriteFile(path, []byte(yamlContent), 0644)
	t.Setenv("TEST_ANTHROPIC_KEY", "sk-test-12345")

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	key := cfg.Providers["anthropic"].APIKey
	if key != "sk-test-12345" {
		t.Errorf("APIKey = %q, want %q", key, "sk-test-12345")
	}
}

func TestExpandTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	tests := []struct {
		name string
		path string
		want string
	}{
		{"home slash path", "~/Documents", filepath.Join(home, "Documents")},
		{"home only", "~", home},
		{"relative path unchanged", "./workspace", "./workspace"},
		{"absolute path unchanged", "/usr/local", "/usr/local"},
		{"empty string unchanged", "", ""},
		{"tilde in middle unchanged", "foo/~/bar", "foo/~/bar"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandTilde(tt.path)
			if got != tt.want {
				t.Errorf("expandTilde(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestLoadFile_TildeExpansion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	yamlContent := `default_provider: anthropic
providers:
  anthropic:
    api_key: test-key
    models:
      - claude-opus-4-6
sandbox:
  allowed_dirs:
    - ~/test-workspace
  read_only_dirs:
    - ~/docs
skills:
  dirs:
    - ~/my-skills
approval_mode: full
`
	os.WriteFile(path, []byte(yamlContent), 0644)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	wantAllowed := filepath.Join(home, "test-workspace")
	if len(cfg.Sandbox.AllowedDirs) != 1 || cfg.Sandbox.AllowedDirs[0] != wantAllowed {
		t.Errorf("AllowedDirs[0] = %q, want %q", cfg.Sandbox.AllowedDirs[0], wantAllowed)
	}

	wantRO := filepath.Join(home, "docs")
	if len(cfg.Sandbox.ReadOnlyDirs) != 1 || cfg.Sandbox.ReadOnlyDirs[0] != wantRO {
		t.Errorf("ReadOnlyDirs[0] = %q, want %q", cfg.Sandbox.ReadOnlyDirs[0], wantRO)
	}

	wantSkill := filepath.Join(home, "my-skills")
	if len(cfg.Skills.Dirs) != 1 || cfg.Skills.Dirs[0] != wantSkill {
		t.Errorf("Skills.Dirs[0] = %q, want %q", cfg.Skills.Dirs[0], wantSkill)
	}
}

func TestSetGetAPIKey(t *testing.T) {
	const provider = "test-provider-setget"
	const key = "sk-test-setget-key"

	if err := SetAPIKey(provider, key); err != nil {
		t.Fatalf("SetAPIKey: %v", err)
	}

	got := GetAPIKey(provider)
	if got != key {
		t.Errorf("GetAPIKey(%q) = %q, want %q", provider, got, key)
	}
}

func TestDeleteAPIKey(t *testing.T) {
	const provider = "test-provider-delete"

	if err := SetAPIKey(provider, "sk-to-delete"); err != nil {
		t.Fatalf("SetAPIKey: %v", err)
	}
	if err := DeleteAPIKey(provider); err != nil {
		t.Fatalf("DeleteAPIKey: %v", err)
	}

	got := GetAPIKey(provider)
	if got != "" {
		t.Errorf("GetAPIKey after delete = %q, want empty", got)
	}
}

func TestGetAPIKey_Missing(t *testing.T) {
	got := GetAPIKey("provider-that-does-not-exist-ever")
	if got != "" {
		t.Errorf("GetAPIKey for missing key = %q, want empty", got)
	}
}

func TestMigrateAPIKeys_HardcodedKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	const rawKey = "sk-hardcoded-migration-test"
	yamlContent := `default_provider: anthropic
providers:
  anthropic:
    api_key: ` + rawKey + `
    models:
      - claude-sonnet-4-6
approval_mode: none
`
	if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	// Key should be available at runtime (populated from keyring).
	if cfg.Providers["anthropic"].APIKey != rawKey {
		t.Errorf("APIKey after migration = %q, want %q", cfg.Providers["anthropic"].APIKey, rawKey)
	}

	// Keyring should now hold the key.
	if got := GetAPIKey("anthropic"); got != rawKey {
		t.Errorf("keyring GetAPIKey = %q, want %q", got, rawKey)
	}

	// Config file should no longer contain the hardcoded API key value.
	fileBytes, _ := os.ReadFile(path)
	if strings.Contains(string(fileBytes), rawKey) {
		t.Errorf("config file still contains API key after migration:\n%s", string(fileBytes))
	}
}

func TestMigrateAPIKeys_EnvVarSkipped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	yamlContent := `default_provider: anthropic
providers:
  anthropic:
    api_key: ${SOME_ENV_VAR}
    models:
      - claude-sonnet-4-6
`
	if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	originalBytes, _ := os.ReadFile(path)

	LoadFile(path) //nolint — only checking side-effects

	// File should NOT be rewritten when the key is an env-var placeholder.
	afterBytes, _ := os.ReadFile(path)
	if string(afterBytes) != string(originalBytes) {
		t.Errorf("config file was unexpectedly rewritten for env-var key")
	}
}

func TestGetModelsForProvider_MergesConfigAndDefaults(t *testing.T) {
	cfg := &Config{
		Providers: map[string]ProviderConfig{
			"openai": {Models: []string{"gpt-4o", "my-custom-model"}},
		},
	}

	got := cfg.GetModelsForProvider("openai")

	want := append([]string(nil), DefaultModels["openai"]...)
	want = append(want, "my-custom-model")
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("models = %v, want %v", got, want)
	}

	count := 0
	for _, m := range got {
		if m == "gpt-4o" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("gpt-4o appears %d times, want 1", count)
	}
}

func TestGetModelsForProvider_DefaultsOnlyWhenNoConfig(t *testing.T) {
	cfg := &Config{}
	got := cfg.GetModelsForProvider("anthropic")

	if len(got) != len(DefaultModels["anthropic"]) {
		t.Fatalf("got %d models, want %d", len(got), len(DefaultModels["anthropic"]))
	}
}

func TestGetModelsForProvider_NilConfig(t *testing.T) {
	var cfg *Config
	got := cfg.GetModelsForProvider("gemini")

	if len(got) != len(DefaultModels["gemini"]) {
		t.Fatalf("got %d models, want %d", len(got), len(DefaultModels["gemini"]))
	}
}

func TestGetProviders_MergesConfigAndDefaults(t *testing.T) {
	cfg := &Config{
		Providers: map[string]ProviderConfig{
			"anthropic":  {Models: []string{"claude-sonnet-4-6"}},
			"custom-a":   {Models: []string{"local-a"}},
			"custom-llm": {Models: []string{"local-7b"}},
		},
	}

	got := cfg.GetProviders()
	// Default order includes native + compatible presets, then custom alphabetically.
	want := []string{"anthropic", "openai", "gemini", "openrouter", "deepseek", "mistral", "groq", "custom-a", "custom-llm"}

	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("providers = %v, want %v", got, want)
	}
	seen := map[string]bool{}
	for _, p := range got {
		if seen[p] {
			t.Fatalf("duplicate provider %q in result: %v", p, got)
		}
		seen[p] = true
	}
}

func TestGetProviders_NilConfig(t *testing.T) {
	var cfg *Config
	got := cfg.GetProviders()
	want := []string{"anthropic", "openai", "gemini", "openrouter", "deepseek", "mistral", "groq"}

	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("providers = %v, want %v", got, want)
	}
}

func TestHostedPresets_HaveEndpoints(t *testing.T) {
	compatibles := []string{"openrouter", "deepseek", "mistral", "groq"}
	for _, name := range compatibles {
		preset, ok := HostedPresets[name]
		if !ok {
			t.Errorf("HostedPresets missing %q", name)
			continue
		}
		if preset.Endpoint == "" {
			t.Errorf("HostedPresets[%q].Endpoint is empty", name)
		}
		if preset.Group != "compatible" {
			t.Errorf("HostedPresets[%q].Group = %q, want compatible", name, preset.Group)
		}
		if preset.EnvVar == "" {
			t.Errorf("HostedPresets[%q].EnvVar is empty", name)
		}
	}
}

func TestHostedPresets_NativeHaveNoEndpoint(t *testing.T) {
	natives := []string{"anthropic", "openai", "gemini"}
	for _, name := range natives {
		preset, ok := HostedPresets[name]
		if !ok {
			t.Errorf("HostedPresets missing %q", name)
			continue
		}
		if preset.Endpoint != "" {
			t.Errorf("HostedPresets[%q].Endpoint should be empty for native, got %q", name, preset.Endpoint)
		}
		if preset.Group != "native" {
			t.Errorf("HostedPresets[%q].Group = %q, want native", name, preset.Group)
		}
	}
}

func TestDefaultModels_HostedPresetsHaveModels(t *testing.T) {
	for name := range HostedPresets {
		models := DefaultModels[name]
		if len(models) == 0 {
			t.Errorf("DefaultModels[%q] is empty", name)
		}
	}
}

func TestSaveFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "config.yaml")

	cfg := Default()
	cfg.DefaultProvider = "openai"

	if err := SaveFile(cfg, path); err != nil {
		t.Fatalf("SaveFile: %v", err)
	}

	loaded, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile after SaveFile: %v", err)
	}
	if loaded.DefaultProvider != "openai" {
		t.Errorf("DefaultProvider = %q, want openai", loaded.DefaultProvider)
	}
}

func TestSaveFilePreservingSecretsUpdatesFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	original := `default_provider: anthropic
theme: dark
providers:
  openai:
    api_key: ${OPENAI_API_KEY}
    models:
      - gpt-4o
approval_mode: full
`
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatalf("write original config: %v", err)
	}

	cfg := Default()
	cfg.DefaultProvider = "openai"
	cfg.Theme = "light"
	cfg.ApprovalMode = "none"
	cfg.MCPApprovalMode = "dangerous-only"
	cfg.FallbackChain = []FallbackEntry{{Provider: "openai", Model: "gpt-4.1"}}

	if err := SaveFilePreservingSecrets(cfg, path); err != nil {
		t.Fatalf("SaveFilePreservingSecrets: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read updated config: %v", err)
	}
	got := string(data)
	for _, want := range []string{
		"api_key: ${OPENAI_API_KEY}",
		"default_provider: openai",
		"theme: light",
		"approval_mode: none",
		"mcp_approval_mode: dangerous-only",
		"model: gpt-4.1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("updated config =\n%s\nwant to contain %q", got, want)
		}
	}
}

func TestSaveFilePreservingSecretsFallbacks(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "empty document", content: ""},
		{name: "sequence document", content: "- not\n- mapping\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tt.content), 0600); err != nil {
				t.Fatalf("write config: %v", err)
			}

			cfg := Default()
			cfg.DefaultProvider = "openai"
			if err := SaveFilePreservingSecrets(cfg, path); err != nil {
				t.Fatalf("SaveFilePreservingSecrets: %v", err)
			}

			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read config: %v", err)
			}
			if !strings.Contains(string(data), "default_provider: openai") {
				t.Fatalf("config =\n%s\nwant default_provider fallback save", string(data))
			}
		})
	}
}

func TestSaveFilePreservingSecretsMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.yaml")

	cfg := Default()
	cfg.DefaultProvider = "openai"
	if err := SaveFilePreservingSecrets(cfg, path); err != nil {
		t.Fatalf("SaveFilePreservingSecrets: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(data), "default_provider: openai") {
		t.Fatalf("config =\n%s\nwant saved default_provider", string(data))
	}
}

func TestSaveFilePreservingSecretsInvalidYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("{{invalid: yaml: [}"), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := SaveFilePreservingSecrets(Default(), path); err == nil {
		t.Fatal("SaveFilePreservingSecrets should reject invalid YAML")
	}
}
