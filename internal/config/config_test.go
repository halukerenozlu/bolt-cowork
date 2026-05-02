package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	if cfg.DefaultProvider != "anthropic" {
		t.Errorf("DefaultProvider = %q, want %q", cfg.DefaultProvider, "anthropic")
	}
	if cfg.ApprovalMode != "full" {
		t.Errorf("ApprovalMode = %q, want %q", cfg.ApprovalMode, "full")
	}
	if len(cfg.Sandbox.DeniedPatterns) != 3 {
		t.Errorf("DeniedPatterns count = %d, want 3", len(cfg.Sandbox.DeniedPatterns))
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
