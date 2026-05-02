package mcp

import (
	"strings"
	"testing"
)

func TestMCPConfig_ValidFull(t *testing.T) {
	raw := map[string]any{
		"full-server": map[string]any{
			"transport": "stdio",
			"command":   "/usr/bin/server",
			"args":      []any{"--verbose", "--port", "8080"},
			"env": map[string]any{
				"API_KEY":   "secret123",
				"LOG_LEVEL": "debug",
			},
			"enabled": true,
		},
	}

	configs, err := ParseServerConfigs(raw)
	if err != nil {
		t.Fatalf("ParseServerConfigs() error: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("got %d configs, want 1", len(configs))
	}

	c := configs[0]
	if c.Name != "full-server" {
		t.Errorf("Name = %q, want %q", c.Name, "full-server")
	}
	if c.Transport != "stdio" {
		t.Errorf("Transport = %q, want %q", c.Transport, "stdio")
	}
	if c.Command != "/usr/bin/server" {
		t.Errorf("Command = %q, want %q", c.Command, "/usr/bin/server")
	}
	if len(c.Args) != 3 {
		t.Errorf("Args len = %d, want 3", len(c.Args))
	}
	if c.Env["API_KEY"] != "secret123" {
		t.Errorf("Env[API_KEY] = %q, want %q", c.Env["API_KEY"], "secret123")
	}
	if c.Env["LOG_LEVEL"] != "debug" {
		t.Errorf("Env[LOG_LEVEL] = %q, want %q", c.Env["LOG_LEVEL"], "debug")
	}
	if !c.Enabled {
		t.Error("Enabled should be true")
	}
}

func TestMCPConfig_ValidMinimal(t *testing.T) {
	raw := map[string]any{
		"minimal": map[string]any{
			"transport": "stdio",
			"command":   "server",
		},
	}

	configs, err := ParseServerConfigs(raw)
	if err != nil {
		t.Fatalf("ParseServerConfigs() error: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("got %d configs, want 1", len(configs))
	}

	c := configs[0]
	if c.Name != "minimal" {
		t.Errorf("Name = %q, want %q", c.Name, "minimal")
	}
	if !c.Enabled {
		t.Error("Enabled should default to true")
	}
}

func TestMCPConfig_MissingName(t *testing.T) {
	// ValidateServerConfig with empty name should fail.
	cfg := ServerConfig{Transport: "stdio", Command: "x"}
	err := ValidateServerConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("error = %q, want it to mention 'name'", err)
	}
}

func TestMCPConfig_MissingURL(t *testing.T) {
	cfg := ServerConfig{Name: "x", Transport: "sse"}
	err := ValidateServerConfig(cfg)
	if err == nil {
		t.Fatal("expected error for sse without url")
	}
	if !strings.Contains(err.Error(), "url") {
		t.Errorf("error = %q, want it to mention 'url'", err)
	}
}

func TestMCPConfig_InvalidTransport(t *testing.T) {
	cfg := ServerConfig{Name: "x", Transport: "grpc"}
	err := ValidateServerConfig(cfg)
	if err == nil {
		t.Fatal("expected error for unknown transport")
	}
	if !strings.Contains(err.Error(), "unknown transport") {
		t.Errorf("error = %q, want it to mention 'unknown transport'", err)
	}
}

func TestMCPConfig_DuplicateServerName(t *testing.T) {
	// Go maps deduplicate keys, so two entries with the same key in the raw
	// map literal result in only 1 entry. This documents the behavior.
	raw := map[string]any{
		"dup": map[string]any{
			"transport": "stdio",
			"command":   "second",
		},
	}
	// In Go, writing `"dup": ...` twice in a map literal is a compile error,
	// so we can only have one entry. This test confirms 1 config is returned.
	configs, err := ParseServerConfigs(raw)
	if err != nil {
		t.Fatalf("ParseServerConfigs() error: %v", err)
	}
	if len(configs) != 1 {
		t.Errorf("got %d configs, want 1 (Go maps deduplicate keys)", len(configs))
	}
}

func TestMCPConfig_EmptyServerList(t *testing.T) {
	raw := map[string]any{}
	configs, err := ParseServerConfigs(raw)
	if err != nil {
		t.Fatalf("ParseServerConfigs() error: %v", err)
	}
	if len(configs) != 0 {
		t.Errorf("got %d configs, want 0", len(configs))
	}
}

func TestMCPConfig_UnknownFields(t *testing.T) {
	raw := map[string]any{
		"with-extras": map[string]any{
			"transport": "stdio",
			"command":   "server",
			"timeout":   30,
			"custom":    true,
			"metadata":  map[string]any{"version": "1.0"},
		},
	}

	configs, err := ParseServerConfigs(raw)
	if err != nil {
		t.Fatalf("ParseServerConfigs() error: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("got %d configs, want 1", len(configs))
	}
	// Unknown fields should be silently ignored.
	c := configs[0]
	if c.Name != "with-extras" {
		t.Errorf("Name = %q, want %q", c.Name, "with-extras")
	}
	if c.Transport != "stdio" {
		t.Errorf("Transport = %q, want %q", c.Transport, "stdio")
	}
}

func TestMCPConfig_InvalidValueType(t *testing.T) {
	raw := map[string]any{
		"bad-server": "this should be a map",
	}

	_, err := ParseServerConfigs(raw)
	if err == nil {
		t.Fatal("expected error for string value instead of map")
	}
	if !strings.Contains(err.Error(), "expected map") {
		t.Errorf("error = %q, want it to mention 'expected map'", err)
	}
}
