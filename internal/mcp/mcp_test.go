package mcp

import (
	"testing"
)

func TestParseServerConfigs_Stdio(t *testing.T) {
	raw := map[string]any{
		"filesystem": map[string]any{
			"transport": "stdio",
			"command":   "npx",
			"args":      []any{"-y", "@anthropic/mcp-server-filesystem"},
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
	if c.Name != "filesystem" {
		t.Errorf("Name = %q, want %q", c.Name, "filesystem")
	}
	if c.Transport != "stdio" {
		t.Errorf("Transport = %q, want %q", c.Transport, "stdio")
	}
	if c.Command != "npx" {
		t.Errorf("Command = %q, want %q", c.Command, "npx")
	}
	if len(c.Args) != 2 || c.Args[0] != "-y" {
		t.Errorf("Args = %v, want [-y @anthropic/mcp-server-filesystem]", c.Args)
	}
	if !c.Enabled {
		t.Error("Enabled should default to true")
	}
}

func TestParseServerConfigs_SSE(t *testing.T) {
	raw := map[string]any{
		"web": map[string]any{
			"transport": "sse",
			"url":       "https://example.com/mcp",
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
	if c.Name != "web" {
		t.Errorf("Name = %q, want %q", c.Name, "web")
	}
	if c.Transport != "sse" {
		t.Errorf("Transport = %q, want %q", c.Transport, "sse")
	}
	if c.URL != "https://example.com/mcp" {
		t.Errorf("URL = %q, want %q", c.URL, "https://example.com/mcp")
	}
}

func TestParseServerConfigs_WithEnv(t *testing.T) {
	raw := map[string]any{
		"srv": map[string]any{
			"transport": "stdio",
			"command":   "server",
			"env": map[string]any{
				"API_KEY": "secret",
			},
		},
	}

	configs, err := ParseServerConfigs(raw)
	if err != nil {
		t.Fatalf("ParseServerConfigs() error: %v", err)
	}

	c := configs[0]
	if c.Env["API_KEY"] != "secret" {
		t.Errorf("Env[API_KEY] = %q, want %q", c.Env["API_KEY"], "secret")
	}
}

func TestValidateServerConfig_MissingCommand(t *testing.T) {
	cfg := ServerConfig{
		Name:      "bad",
		Transport: "stdio",
		// Command intentionally empty
	}
	err := ValidateServerConfig(cfg)
	if err == nil {
		t.Fatal("expected error for stdio without command")
	}
}

func TestValidateServerConfig_MissingURL(t *testing.T) {
	cfg := ServerConfig{
		Name:      "bad",
		Transport: "sse",
		// URL intentionally empty
	}
	err := ValidateServerConfig(cfg)
	if err == nil {
		t.Fatal("expected error for sse without url")
	}
}

func TestValidateServerConfig_MissingTransport(t *testing.T) {
	cfg := ServerConfig{
		Name: "bad",
	}
	err := ValidateServerConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing transport")
	}
}

func TestValidateServerConfig_MissingName(t *testing.T) {
	cfg := ServerConfig{
		Transport: "stdio",
		Command:   "cmd",
	}
	err := ValidateServerConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestValidateServerConfig_Valid(t *testing.T) {
	tests := []struct {
		name string
		cfg  ServerConfig
	}{
		{
			name: "valid stdio",
			cfg:  ServerConfig{Name: "fs", Transport: "stdio", Command: "npx"},
		},
		{
			name: "valid sse",
			cfg:  ServerConfig{Name: "web", Transport: "sse", URL: "https://example.com"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateServerConfig(tt.cfg); err != nil {
				t.Errorf("ValidateServerConfig() unexpected error: %v", err)
			}
		})
	}
}

func TestRegistryAddAndGetServer(t *testing.T) {
	r := NewRegistry()
	cfg := ServerConfig{Name: "fs", Transport: "stdio", Command: "npx"}
	r.AddServer(cfg)

	got, ok := r.GetServer("fs")
	if !ok {
		t.Fatal("GetServer returned false for added server")
	}
	if got.Name != "fs" {
		t.Errorf("Name = %q, want %q", got.Name, "fs")
	}
	if got.Command != "npx" {
		t.Errorf("Command = %q, want %q", got.Command, "npx")
	}
}

func TestRegistryGetServerMissing(t *testing.T) {
	r := NewRegistry()
	_, ok := r.GetServer("nope")
	if ok {
		t.Error("GetServer returned true for missing server")
	}
}

func TestRegistryServers(t *testing.T) {
	r := NewRegistry()
	r.AddServer(ServerConfig{Name: "b", Transport: "stdio", Command: "b"})
	r.AddServer(ServerConfig{Name: "a", Transport: "stdio", Command: "a"})

	servers := r.Servers()
	if len(servers) != 2 {
		t.Fatalf("Servers() returned %d, want 2", len(servers))
	}
	if servers[0].Name != "a" || servers[1].Name != "b" {
		t.Errorf("Servers() not sorted: got %q, %q", servers[0].Name, servers[1].Name)
	}
}

func TestRegistryToolsByServer(t *testing.T) {
	r := NewRegistry()
	r.RegisterTool(MCPTool{Name: "read", ServerName: "fs"})
	r.RegisterTool(MCPTool{Name: "write", ServerName: "fs"})
	r.RegisterTool(MCPTool{Name: "fetch", ServerName: "web"})

	fsTools := r.ToolsByServer("fs")
	if len(fsTools) != 2 {
		t.Fatalf("ToolsByServer(fs) returned %d tools, want 2", len(fsTools))
	}
	if fsTools[0].Name != "read" || fsTools[1].Name != "write" {
		t.Errorf("ToolsByServer(fs) not sorted: got %q, %q", fsTools[0].Name, fsTools[1].Name)
	}

	webTools := r.ToolsByServer("web")
	if len(webTools) != 1 || webTools[0].Name != "fetch" {
		t.Errorf("ToolsByServer(web) = %v, want [fetch]", webTools)
	}

	empty := r.ToolsByServer("none")
	if len(empty) != 0 {
		t.Errorf("ToolsByServer(none) returned %d tools, want 0", len(empty))
	}
}

func TestRegistryTools(t *testing.T) {
	r := NewRegistry()
	r.RegisterTool(MCPTool{Name: "z-tool", ServerName: "s"})
	r.RegisterTool(MCPTool{Name: "a-tool", ServerName: "s"})

	tools := r.Tools()
	if len(tools) != 2 {
		t.Fatalf("Tools() returned %d, want 2", len(tools))
	}
	if tools[0].Name != "a-tool" || tools[1].Name != "z-tool" {
		t.Errorf("Tools() not sorted: got %q, %q", tools[0].Name, tools[1].Name)
	}
}

func TestRegistryGetTool(t *testing.T) {
	r := NewRegistry()
	r.RegisterTool(MCPTool{Name: "read", Description: "Read files", ServerName: "fs"})

	got, ok := r.GetTool("read")
	if !ok {
		t.Fatal("GetTool returned false for registered tool")
	}
	if got.Description != "Read files" {
		t.Errorf("Description = %q, want %q", got.Description, "Read files")
	}

	_, ok = r.GetTool("missing")
	if ok {
		t.Error("GetTool returned true for missing tool")
	}
}
