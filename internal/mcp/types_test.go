package mcp

import (
	"encoding/json"
	"testing"
)

// roundTrip marshals v to JSON then unmarshals into a new value of the same type.
func roundTrip[T any](t *testing.T, v T) T {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var got T
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	return got
}

// checkJSON asserts JSON key presence or absence.
// wantJSON maps a key to true (must be present) or false (must be absent).
func checkJSON(t *testing.T, data []byte, wantJSON map[string]bool) {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("checkJSON: json.Unmarshal: %v", err)
	}
	for key, mustPresent := range wantJSON {
		_, ok := m[key]
		if mustPresent && !ok {
			t.Errorf("JSON key %q must be present", key)
		}
		if !mustPresent && ok {
			t.Errorf("JSON key %q must be absent, got %v", key, m[key])
		}
	}
}

// --- Tool ---

func TestTool(t *testing.T) {
	tests := []struct {
		name     string
		input    Tool
		wantJSON map[string]bool         // true=must be present, false=must be absent
		check    func(*testing.T, Tool)  // extra assertions after round-trip (may be nil)
	}{
		{
			name: "full fields preserve on round-trip",
			input: Tool{
				Name:        "read_file",
				Description: "Read a file from disk",
				InputSchema: ToolSchema{
					Type: "object",
					Properties: map[string]ToolProperty{
						"path": {Type: "string", Description: "Absolute file path"},
					},
					Required: []string{"path"},
				},
			},
			wantJSON: map[string]bool{
				"name":        true,
				"description": true,
				"inputSchema": true,
			},
			check: func(t *testing.T, got Tool) {
				if got.Name != "read_file" {
					t.Errorf("Name = %q, want read_file", got.Name)
				}
				if got.Description != "Read a file from disk" {
					t.Errorf("Description = %q, want %q", got.Description, "Read a file from disk")
				}
				if got.InputSchema.Type != "object" {
					t.Errorf("InputSchema.Type = %q, want object", got.InputSchema.Type)
				}
				if got.InputSchema.Properties["path"].Type != "string" {
					t.Errorf("Properties[path].Type = %q, want string", got.InputSchema.Properties["path"].Type)
				}
				if len(got.InputSchema.Required) != 1 || got.InputSchema.Required[0] != "path" {
					t.Errorf("Required = %v, want [path]", got.InputSchema.Required)
				}
			},
		},
		{
			name:  "description absent when empty",
			input: Tool{Name: "ping", InputSchema: ToolSchema{Type: "object"}},
			wantJSON: map[string]bool{
				"name":        true,
				"inputSchema": true,
				"description": false,
			},
		},
		{
			name:  "schema properties and required absent when nil",
			input: Tool{Name: "x", InputSchema: ToolSchema{Type: "object"}},
			check: func(t *testing.T, got Tool) {
				data, err := json.Marshal(got.InputSchema)
				if err != nil {
					t.Fatalf("json.Marshal(InputSchema): %v", err)
				}
				checkJSON(t, data, map[string]bool{
					"type":       true,
					"properties": false,
					"required":   false,
				})
			},
		},
		{
			name: "property type and description preserved",
			input: Tool{
				Name: "query",
				InputSchema: ToolSchema{
					Type: "object",
					Properties: map[string]ToolProperty{
						"limit": {Type: "integer", Description: "Max rows"},
					},
				},
			},
			check: func(t *testing.T, got Tool) {
				p, ok := got.InputSchema.Properties["limit"]
				if !ok {
					t.Fatal("Properties[limit] missing after round-trip")
				}
				if p.Type != "integer" {
					t.Errorf("Properties[limit].Type = %q, want integer", p.Type)
				}
				if p.Description != "Max rows" {
					t.Errorf("Properties[limit].Description = %q, want %q", p.Description, "Max rows")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}
			checkJSON(t, data, tt.wantJSON)

			got := roundTrip(t, tt.input)
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

// --- CallToolResult ---

func TestCallToolResult(t *testing.T) {
	tests := []struct {
		name     string
		input    CallToolResult
		wantJSON map[string]bool
		check    func(*testing.T, CallToolResult)
	}{
		{
			name: "isError false: round-trip and absent from JSON",
			input: CallToolResult{
				Content: []ToolResultContent{{Type: "text", Text: "hello"}},
				IsError: false,
			},
			wantJSON: map[string]bool{
				"content": true,
				"isError": false,
			},
			check: func(t *testing.T, got CallToolResult) {
				if got.IsError {
					t.Error("IsError should be false after round-trip")
				}
				if len(got.Content) != 1 || got.Content[0].Text != "hello" {
					t.Errorf("Content = %v, want [{text hello}]", got.Content)
				}
			},
		},
		{
			name: "isError true: round-trip and present in JSON",
			input: CallToolResult{
				Content: []ToolResultContent{{Type: "text", Text: "tool panic: out of bounds"}},
				IsError: true,
			},
			wantJSON: map[string]bool{
				"content": true,
				"isError": true,
			},
			check: func(t *testing.T, got CallToolResult) {
				if !got.IsError {
					t.Error("IsError should be true after round-trip")
				}
				if len(got.Content) != 1 || got.Content[0].Text != "tool panic: out of bounds" {
					t.Errorf("Content[0].Text = %q, want %q", got.Content[0].Text, "tool panic: out of bounds")
				}
			},
		},
		{
			name: "content text absent when empty (ToolResultContent)",
			input: CallToolResult{
				Content: []ToolResultContent{{Type: "image"}},
			},
			check: func(t *testing.T, got CallToolResult) {
				if len(got.Content) != 1 {
					t.Fatalf("Content len = %d, want 1", len(got.Content))
				}
				data, err := json.Marshal(got.Content[0])
				if err != nil {
					t.Fatalf("json.Marshal(Content[0]): %v", err)
				}
				checkJSON(t, data, map[string]bool{
					"type": true,
					"text": false,
				})
			},
		},
		{
			name: "text content type and text preserved",
			input: CallToolResult{
				Content: []ToolResultContent{{Type: "text", Text: "result value"}},
			},
			check: func(t *testing.T, got CallToolResult) {
				if got.Content[0].Type != "text" {
					t.Errorf("Content[0].Type = %q, want text", got.Content[0].Type)
				}
				if got.Content[0].Text != "result value" {
					t.Errorf("Content[0].Text = %q, want %q", got.Content[0].Text, "result value")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}
			checkJSON(t, data, tt.wantJSON)

			got := roundTrip(t, tt.input)
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

// --- ServerCapabilities ---

func TestServerCapabilities(t *testing.T) {
	tests := []struct {
		name     string
		input    ServerCapabilities
		wantJSON map[string]bool
		check    func(*testing.T, ServerCapabilities)
	}{
		{
			name:  "tools only: tools present, resources absent",
			input: ServerCapabilities{Tools: &ToolsCapability{}},
			wantJSON: map[string]bool{
				"tools":     true,
				"resources": false,
			},
			check: func(t *testing.T, got ServerCapabilities) {
				if got.Tools == nil {
					t.Error("Tools should be non-nil after round-trip")
				}
				if got.Resources != nil {
					t.Error("Resources should be nil after round-trip")
				}
			},
		},
		{
			name:  "resources only: resources present, tools absent",
			input: ServerCapabilities{Resources: &ResourcesCapability{}},
			wantJSON: map[string]bool{
				"resources": true,
				"tools":     false,
			},
			check: func(t *testing.T, got ServerCapabilities) {
				if got.Resources == nil {
					t.Error("Resources should be non-nil after round-trip")
				}
				if got.Tools != nil {
					t.Error("Tools should be nil after round-trip")
				}
			},
		},
		{
			name:  "both set: both keys present",
			input: ServerCapabilities{Tools: &ToolsCapability{}, Resources: &ResourcesCapability{}},
			wantJSON: map[string]bool{
				"tools":     true,
				"resources": true,
			},
			check: func(t *testing.T, got ServerCapabilities) {
				if got.Tools == nil {
					t.Error("Tools should be non-nil after round-trip")
				}
				if got.Resources == nil {
					t.Error("Resources should be non-nil after round-trip")
				}
			},
		},
		{
			name:  "neither set: both keys absent",
			input: ServerCapabilities{},
			wantJSON: map[string]bool{
				"tools":     false,
				"resources": false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}
			checkJSON(t, data, tt.wantJSON)

			got := roundTrip(t, tt.input)
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

// --- ServerConfig ---

func TestServerConfig(t *testing.T) {
	tests := []struct {
		name     string
		input    ServerConfig
		wantJSON map[string]bool
		check    func(*testing.T, ServerConfig)
	}{
		{
			name: "full fields: all keys present",
			input: ServerConfig{
				Name:      "srv",
				Transport: "stdio",
				Command:   "cmd",
				Args:      []string{"--flag"},
				Env:       map[string]string{"K": "V"},
				Enabled:   true,
			},
			wantJSON: map[string]bool{
				"name":      true,
				"transport": true,
				"command":   true,
				"args":      true,
				"env":       true,
				"enabled":   true,
			},
			check: func(t *testing.T, got ServerConfig) {
				if got.Name != "srv" {
					t.Errorf("Name = %q, want srv", got.Name)
				}
				if got.Command != "cmd" {
					t.Errorf("Command = %q, want cmd", got.Command)
				}
				if len(got.Args) != 1 || got.Args[0] != "--flag" {
					t.Errorf("Args = %v, want [--flag]", got.Args)
				}
				if got.Env["K"] != "V" {
					t.Errorf("Env[K] = %q, want V", got.Env["K"])
				}
			},
		},
		{
			name: "optional fields absent when zero",
			input: ServerConfig{
				Name:      "s",
				Transport: "stdio",
				Command:   "c",
			},
			wantJSON: map[string]bool{
				"args": false,
				"url":  false,
				"env":  false,
			},
		},
		{
			name: "sse transport uses url field",
			input: ServerConfig{
				Name:      "remote",
				Transport: "sse",
				URL:       "https://example.com/mcp",
				Enabled:   true,
			},
			wantJSON: map[string]bool{
				"url":     true,
				"command": false,
			},
			check: func(t *testing.T, got ServerConfig) {
				if got.URL != "https://example.com/mcp" {
					t.Errorf("URL = %q, want %q", got.URL, "https://example.com/mcp")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}
			checkJSON(t, data, tt.wantJSON)

			got := roundTrip(t, tt.input)
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

// --- InitializeParams ---

func TestInitializeParams(t *testing.T) {
	tests := []struct {
		name     string
		input    InitializeParams
		wantJSON map[string]bool
		check    func(*testing.T, InitializeParams)
	}{
		{
			name: "round-trip preserves all fields",
			input: InitializeParams{
				ProtocolVersion: "2024-11-05",
				ClientInfo:      ClientInfo{Name: "bolt-cowork", Version: "0.3.3"},
			},
			wantJSON: map[string]bool{
				"protocolVersion": true,
				"clientInfo":      true,
			},
			check: func(t *testing.T, got InitializeParams) {
				if got.ProtocolVersion != "2024-11-05" {
					t.Errorf("ProtocolVersion = %q, want 2024-11-05", got.ProtocolVersion)
				}
				if got.ClientInfo.Name != "bolt-cowork" {
					t.Errorf("ClientInfo.Name = %q, want bolt-cowork", got.ClientInfo.Name)
				}
				if got.ClientInfo.Version != "0.3.3" {
					t.Errorf("ClientInfo.Version = %q, want 0.3.3", got.ClientInfo.Version)
				}
			},
		},
		{
			name: "different client version preserved",
			input: InitializeParams{
				ProtocolVersion: "2024-11-05",
				ClientInfo:      ClientInfo{Name: "test-client", Version: "1.0.0"},
			},
			wantJSON: map[string]bool{
				"protocolVersion": true,
				"clientInfo":      true,
			},
			check: func(t *testing.T, got InitializeParams) {
				if got.ClientInfo.Name != "test-client" {
					t.Errorf("ClientInfo.Name = %q, want test-client", got.ClientInfo.Name)
				}
				if got.ClientInfo.Version != "1.0.0" {
					t.Errorf("ClientInfo.Version = %q, want 1.0.0", got.ClientInfo.Version)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}
			checkJSON(t, data, tt.wantJSON)

			got := roundTrip(t, tt.input)
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

// --- InitializeResult ---

func TestInitializeResult(t *testing.T) {
	tests := []struct {
		name     string
		input    InitializeResult
		wantJSON map[string]bool
		check    func(*testing.T, InitializeResult)
	}{
		{
			name: "tools capability set, resources nil",
			input: InitializeResult{
				ProtocolVersion: "2024-11-05",
				ServerInfo:      ServerInfo{Name: "my-mcp-server", Version: "1.0.0"},
				Capabilities:    ServerCapabilities{Tools: &ToolsCapability{}},
			},
			wantJSON: map[string]bool{
				"protocolVersion": true,
				"serverInfo":      true,
				"capabilities":    true,
			},
			check: func(t *testing.T, got InitializeResult) {
				if got.ProtocolVersion != "2024-11-05" {
					t.Errorf("ProtocolVersion = %q, want 2024-11-05", got.ProtocolVersion)
				}
				if got.ServerInfo.Name != "my-mcp-server" {
					t.Errorf("ServerInfo.Name = %q, want my-mcp-server", got.ServerInfo.Name)
				}
				if got.ServerInfo.Version != "1.0.0" {
					t.Errorf("ServerInfo.Version = %q, want 1.0.0", got.ServerInfo.Version)
				}
				if got.Capabilities.Tools == nil {
					t.Error("Capabilities.Tools should be non-nil after round-trip")
				}
				if got.Capabilities.Resources != nil {
					t.Error("Capabilities.Resources should be nil after round-trip")
				}
			},
		},
		{
			name: "both capabilities set",
			input: InitializeResult{
				ProtocolVersion: "2024-11-05",
				ServerInfo:      ServerInfo{Name: "srv", Version: "0.1.0"},
				Capabilities: ServerCapabilities{
					Tools:     &ToolsCapability{},
					Resources: &ResourcesCapability{},
				},
			},
			wantJSON: map[string]bool{
				"protocolVersion": true,
				"serverInfo":      true,
				"capabilities":    true,
			},
			check: func(t *testing.T, got InitializeResult) {
				if got.Capabilities.Tools == nil {
					t.Error("Capabilities.Tools should be non-nil")
				}
				if got.Capabilities.Resources == nil {
					t.Error("Capabilities.Resources should be non-nil")
				}
			},
		},
		{
			name: "no capabilities: capabilities key still present",
			input: InitializeResult{
				ProtocolVersion: "2024-11-05",
				ServerInfo:      ServerInfo{Name: "srv", Version: "0"},
			},
			wantJSON: map[string]bool{
				"protocolVersion": true,
				"serverInfo":      true,
				"capabilities":    true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}
			checkJSON(t, data, tt.wantJSON)

			got := roundTrip(t, tt.input)
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

// --- MCPConfig ---

func TestMCPConfig(t *testing.T) {
	tests := []struct {
		name     string
		input    MCPConfig  // used when rawJSON is empty
		rawJSON  string     // when set, unmarshal this instead of round-tripping input
		wantJSON map[string]bool
		check    func(*testing.T, MCPConfig)
	}{
		{
			name: "round-trip with stdio and sse servers",
			input: MCPConfig{
				Servers: []ServerConfig{
					{
						Name:      "filesystem",
						Transport: "stdio",
						Command:   "npx",
						Args:      []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"},
						Env:       map[string]string{"DEBUG": "mcp*"},
						Enabled:   true,
					},
					{
						Name:      "remote",
						Transport: "sse",
						URL:       "https://example.com/mcp",
						Enabled:   true,
					},
				},
			},
			wantJSON: map[string]bool{"servers": true},
			check: func(t *testing.T, got MCPConfig) {
				if len(got.Servers) != 2 {
					t.Fatalf("Servers len = %d, want 2", len(got.Servers))
				}
				fs := got.Servers[0]
				if fs.Name != "filesystem" {
					t.Errorf("Servers[0].Name = %q, want filesystem", fs.Name)
				}
				if fs.Command != "npx" {
					t.Errorf("Servers[0].Command = %q, want npx", fs.Command)
				}
				if len(fs.Args) != 3 {
					t.Errorf("Servers[0].Args len = %d, want 3", len(fs.Args))
				}
				if fs.Env["DEBUG"] != "mcp*" {
					t.Errorf("Servers[0].Env[DEBUG] = %q, want mcp*", fs.Env["DEBUG"])
				}
				rem := got.Servers[1]
				if rem.Transport != "sse" {
					t.Errorf("Servers[1].Transport = %q, want sse", rem.Transport)
				}
				if rem.URL != "https://example.com/mcp" {
					t.Errorf("Servers[1].URL = %q, want %q", rem.URL, "https://example.com/mcp")
				}
			},
		},
		{
			name: "parse real mcp.json format",
			rawJSON: `{
				"servers": [
					{
						"name": "filesystem",
						"transport": "stdio",
						"command": "npx",
						"args": ["-y", "@modelcontextprotocol/server-filesystem"],
						"env": {"API_KEY": "s3cr3t"},
						"enabled": true
					}
				]
			}`,
			check: func(t *testing.T, got MCPConfig) {
				if len(got.Servers) != 1 {
					t.Fatalf("Servers len = %d, want 1", len(got.Servers))
				}
				s := got.Servers[0]
				if s.Name != "filesystem" {
					t.Errorf("Name = %q, want filesystem", s.Name)
				}
				if s.Transport != "stdio" {
					t.Errorf("Transport = %q, want stdio", s.Transport)
				}
				if s.Command != "npx" {
					t.Errorf("Command = %q, want npx", s.Command)
				}
				if len(s.Args) != 2 {
					t.Errorf("Args len = %d, want 2", len(s.Args))
				}
				if s.Env["API_KEY"] != "s3cr3t" {
					t.Errorf("Env[API_KEY] = %q, want s3cr3t", s.Env["API_KEY"])
				}
				if !s.Enabled {
					t.Error("Enabled should be true")
				}
			},
		},
		{
			name:     "empty servers list round-trip",
			input:    MCPConfig{Servers: []ServerConfig{}},
			wantJSON: map[string]bool{"servers": true},
			check: func(t *testing.T, got MCPConfig) {
				if got.Servers == nil {
					t.Error("Servers should be a non-nil empty slice after round-trip")
				}
				if len(got.Servers) != 0 {
					t.Errorf("Servers len = %d, want 0", len(got.Servers))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got MCPConfig

			if tt.rawJSON != "" {
				if err := json.Unmarshal([]byte(tt.rawJSON), &got); err != nil {
					t.Fatalf("json.Unmarshal(rawJSON): %v", err)
				}
			} else {
				data, err := json.Marshal(tt.input)
				if err != nil {
					t.Fatalf("json.Marshal: %v", err)
				}
				checkJSON(t, data, tt.wantJSON)
				got = roundTrip(t, tt.input)
			}

			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}
