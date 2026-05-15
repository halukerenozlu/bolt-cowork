package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// ----- IsTrusted tests -----

func TestIsTrusted_ExactMatch(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{TrustedDirs: []string{dir}}
	if !IsTrusted(cfg, dir) {
		t.Errorf("IsTrusted: expected true for exact match %q", dir)
	}
}

func TestIsTrusted_Subdirectory(t *testing.T) {
	parent := t.TempDir()
	sub := filepath.Join(parent, "sub")
	if err := os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}
	cfg := &Config{TrustedDirs: []string{parent}}
	if !IsTrusted(cfg, sub) {
		t.Errorf("IsTrusted: expected true for subdir %q of trusted %q", sub, parent)
	}
}

func TestIsTrusted_NotTrusted(t *testing.T) {
	dir := t.TempDir()
	other := t.TempDir()
	cfg := &Config{TrustedDirs: []string{other}}
	if IsTrusted(cfg, dir) {
		t.Errorf("IsTrusted: expected false for unrelated dir %q", dir)
	}
}

func TestIsTrusted_TrailingSlash(t *testing.T) {
	dir := t.TempDir()
	withSlash := dir + string(filepath.Separator)
	cfg := &Config{TrustedDirs: []string{dir}}
	if !IsTrusted(cfg, withSlash) {
		t.Errorf("IsTrusted: expected true for dir with trailing separator")
	}
}

func TestIsTrusted_Empty(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{TrustedDirs: []string{}}
	if IsTrusted(cfg, dir) {
		t.Errorf("IsTrusted: expected false for empty TrustedDirs")
	}
}

func TestIsTrusted_WindowsCaseInsensitive(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only test")
	}
	dir := t.TempDir()
	upper := strings.ToUpper(dir)
	cfg := &Config{TrustedDirs: []string{upper}}
	if !IsTrusted(cfg, dir) {
		t.Errorf("IsTrusted: expected true for case-insensitive match on Windows")
	}
}

// ----- addTrustedDirToFile tests -----

func TestAddTrustedDir_Basic(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	dir := t.TempDir()

	if err := addTrustedDirToFile(dir, cfgPath); err != nil {
		t.Fatalf("addTrustedDirToFile: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	normDir, _ := normalizePath(dir)
	if !strings.Contains(string(data), normDir) {
		t.Errorf("config file does not contain %q\ngot:\n%s", normDir, data)
	}
}

func TestAddTrustedDir_Duplicate(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	dir := t.TempDir()

	// Call twice; second call should be a no-op.
	if err := addTrustedDirToFile(dir, cfgPath); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := addTrustedDirToFile(dir, cfgPath); err != nil {
		t.Fatalf("second call: %v", err)
	}

	// Parse and count occurrences.
	data, _ := os.ReadFile(cfgPath)
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	normDir, _ := normalizePath(dir)
	count := 0
	if v, ok := raw["trusted_dirs"]; ok {
		if slice, ok := v.([]interface{}); ok {
			for _, item := range slice {
				if s, ok := item.(string); ok {
					n, _ := normalizePath(s)
					if pathEqual(n, normDir) {
						count++
					}
				}
			}
		}
	}
	if count != 1 {
		t.Errorf("expected 1 occurrence of %q in trusted_dirs, got %d", normDir, count)
	}
}

func TestAddTrustedDir_NoFileYet(t *testing.T) {
	// Config file does not exist; function should create it.
	cfgPath := filepath.Join(t.TempDir(), "subdir", "config.yaml")
	dir := t.TempDir()

	if err := addTrustedDirToFile(dir, cfgPath); err != nil {
		t.Fatalf("addTrustedDirToFile: %v", err)
	}

	if _, err := os.Stat(cfgPath); err != nil {
		t.Errorf("config file not created: %v", err)
	}

	data, _ := os.ReadFile(cfgPath)
	normDir, _ := normalizePath(dir)
	if !strings.Contains(string(data), normDir) {
		t.Errorf("config file does not contain %q\ngot:\n%s", normDir, data)
	}
}

func TestAddTrustedDir_WithExplicitPath(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	dir := t.TempDir()

	if err := AddTrustedDir(dir, cfgPath); err != nil {
		t.Fatalf("AddTrustedDir: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	normDir, _ := normalizePath(dir)
	if !strings.Contains(string(data), normDir) {
		t.Errorf("config file does not contain %q\ngot:\n%s", normDir, data)
	}
}

func TestAddTrustedDir_RoundTrip(t *testing.T) {
	// Verify that other YAML fields survive the round-trip untouched,
	// especially unexpanded ${ENV_VAR} placeholders.
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")

	original := `approval_mode: full
default_provider: anthropic
providers:
  anthropic:
    api_key: ${API_KEY}
    models:
      - claude-opus-4-6
`
	if err := os.WriteFile(cfgPath, []byte(original), 0600); err != nil {
		t.Fatalf("write initial config: %v", err)
	}

	dir := t.TempDir()
	if err := addTrustedDirToFile(dir, cfgPath); err != nil {
		t.Fatalf("addTrustedDirToFile: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read after update: %v", err)
	}

	// The raw ${API_KEY} placeholder must survive the round-trip.
	if !strings.Contains(string(data), "${API_KEY}") {
		t.Errorf("env var placeholder ${API_KEY} was lost after round-trip\ngot:\n%s", data)
	}

	// trusted_dirs must be present.
	normDir, _ := normalizePath(dir)
	if !strings.Contains(string(data), normDir) {
		t.Errorf("trusted dir %q not found in config\ngot:\n%s", normDir, data)
	}

	// Parse and verify other fields are still readable.
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal after round-trip: %v", err)
	}
	if raw["approval_mode"] != "full" {
		t.Errorf("approval_mode = %v, want full", raw["approval_mode"])
	}
	if raw["default_provider"] != "anthropic" {
		t.Errorf("default_provider = %v, want anthropic", raw["default_provider"])
	}
}
