package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTempJSON marshals v as JSON into a file named filename inside dir and
// returns its absolute path. The test is failed immediately on any error.
func writeTempJSON(t *testing.T, dir, filename string, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("writeTempJSON: marshal: %v", err)
	}
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("writeTempJSON: write %q: %v", path, err)
	}
	return path
}

// --- LoadConfig ---

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) string // returns the path to pass to LoadConfig
		wantErr bool
		check   func(t *testing.T, cfg *MCPConfig)
	}{
		{
			name: "valid file with all fields parsed correctly",
			setup: func(t *testing.T) string {
				return writeTempJSON(t, t.TempDir(), "mcp.json", MCPConfig{
					Servers: []ServerConfig{
						{
							Name:      "filesystem",
							Transport: "stdio",
							Command:   "npx",
							Args:      []string{"-y", "@modelcontextprotocol/server-filesystem"},
							Env:       map[string]string{"API_KEY": "s3cr3t"},
							Enabled:   true,
						},
					},
				})
			},
			check: func(t *testing.T, cfg *MCPConfig) {
				if len(cfg.Servers) != 1 {
					t.Fatalf("Servers len = %d, want 1", len(cfg.Servers))
				}
				s := cfg.Servers[0]
				if s.Name != "filesystem" {
					t.Errorf("Name = %q, want filesystem", s.Name)
				}
				if s.Transport != "stdio" {
					t.Errorf("Transport = %q, want stdio", s.Transport)
				}
				if s.Command != "npx" {
					t.Errorf("Command = %q, want npx", s.Command)
				}
				if len(s.Args) != 2 || s.Args[0] != "-y" {
					t.Errorf("Args = %v, want [-y @modelcontextprotocol/server-filesystem]", s.Args)
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
			name: "valid file with multiple servers",
			setup: func(t *testing.T) string {
				return writeTempJSON(t, t.TempDir(), "mcp.json", MCPConfig{
					Servers: []ServerConfig{
						{Name: "a", Transport: "stdio", Command: "cmd-a"},
						{Name: "b", Transport: "stdio", Command: "cmd-b"},
					},
				})
			},
			check: func(t *testing.T, cfg *MCPConfig) {
				if len(cfg.Servers) != 2 {
					t.Errorf("Servers len = %d, want 2", len(cfg.Servers))
				}
			},
		},
		{
			name: "nonexistent file returns empty MCPConfig without error",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "does-not-exist.json")
			},
			check: func(t *testing.T, cfg *MCPConfig) {
				if cfg == nil {
					t.Fatal("cfg must not be nil")
				}
				if len(cfg.Servers) != 0 {
					t.Errorf("Servers len = %d, want 0", len(cfg.Servers))
				}
			},
		},
		{
			name: "tilde-prefixed nonexistent path returns empty MCPConfig without error",
			setup: func(t *testing.T) string {
				// Override userHomeDir so the test never touches the real home
				// directory (CLAUDE.md §Test Isolation).
				tmpHome := t.TempDir()
				orig := userHomeDir
				userHomeDir = func() (string, error) { return tmpHome, nil }
				t.Cleanup(func() { userHomeDir = orig })
				// The file does not exist inside tmpHome.
				return "~/nonexistent-subdir/mcp.json"
			},
			check: func(t *testing.T, cfg *MCPConfig) {
				if cfg == nil {
					t.Fatal("cfg must not be nil")
				}
				if len(cfg.Servers) != 0 {
					t.Errorf("Servers len = %d, want 0", len(cfg.Servers))
				}
			},
		},
		{
			name: "malformed JSON returns error",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "bad.json")
				if err := os.WriteFile(path, []byte("{not valid json!!!"), 0600); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
				return path
			},
			wantErr: true,
		},
		{
			name: "empty JSON object returns empty servers list",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "empty.json")
				if err := os.WriteFile(path, []byte(`{}`), 0600); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
				return path
			},
			check: func(t *testing.T, cfg *MCPConfig) {
				if len(cfg.Servers) != 0 {
					t.Errorf("Servers len = %d, want 0", len(cfg.Servers))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			cfg, err := LoadConfig(path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("LoadConfig() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, cfg)
			}
		})
	}
}

// --- DefaultConfigPath ---

func TestDefaultConfigPath(t *testing.T) {
	// Override userHomeDir so the test never touches the real home directory
	// (CLAUDE.md §Test Isolation).
	tmpHome := t.TempDir()
	orig := userHomeDir
	userHomeDir = func() (string, error) { return tmpHome, nil }
	t.Cleanup(func() { userHomeDir = orig })

	path := DefaultConfigPath()

	wantSuffix := filepath.Join(".bolt-cowork", "mcp.json")
	if !strings.HasSuffix(path, wantSuffix) {
		t.Errorf("DefaultConfigPath() = %q, want suffix %q", path, wantSuffix)
	}
	if !strings.HasPrefix(path, tmpHome) {
		t.Errorf("DefaultConfigPath() = %q, want prefix %q (controlled home)", path, tmpHome)
	}
}

// --- NormalizeConfig ---

func TestNormalizeConfig(t *testing.T) {
	tests := []struct {
		name        string
		input       *MCPConfig
		wantErr     bool
		errContains string                  // substring expected in error message
		check       func(*testing.T, *MCPConfig)
	}{
		{
			name: "deduplication keeps first occurrence",
			input: &MCPConfig{
				Servers: []ServerConfig{
					{Name: "fs", Command: "cmd-first"},
					{Name: "fs", Command: "cmd-second"}, // duplicate — discarded
					{Name: "git", Command: "cmd-git"},
				},
			},
			check: func(t *testing.T, cfg *MCPConfig) {
				if len(cfg.Servers) != 2 {
					t.Fatalf("Servers len = %d, want 2 (duplicate removed)", len(cfg.Servers))
				}
				if cfg.Servers[0].Name != "fs" {
					t.Errorf("Servers[0].Name = %q, want fs", cfg.Servers[0].Name)
				}
				if cfg.Servers[0].Command != "cmd-first" {
					t.Errorf("Servers[0].Command = %q, want cmd-first (first occurrence kept)", cfg.Servers[0].Command)
				}
				if cfg.Servers[1].Name != "git" {
					t.Errorf("Servers[1].Name = %q, want git", cfg.Servers[1].Name)
				}
			},
		},
		{
			name: "whitespace trimmed from name and command",
			input: &MCPConfig{
				Servers: []ServerConfig{
					{Name: "  fs  ", Command: "  /usr/bin/server  "},
				},
			},
			check: func(t *testing.T, cfg *MCPConfig) {
				if cfg.Servers[0].Name != "fs" {
					t.Errorf("Name = %q, want %q (trimmed)", cfg.Servers[0].Name, "fs")
				}
				if cfg.Servers[0].Command != "/usr/bin/server" {
					t.Errorf("Command = %q, want %q (trimmed)", cfg.Servers[0].Command, "/usr/bin/server")
				}
			},
		},
		{
			name: "empty name returns error",
			input: &MCPConfig{
				Servers: []ServerConfig{
					{Name: "", Command: "cmd"},
				},
			},
			wantErr:     true,
			errContains: "empty name",
		},
		{
			name: "whitespace-only name returns error",
			input: &MCPConfig{
				Servers: []ServerConfig{
					{Name: "   \t  ", Command: "cmd"},
				},
			},
			wantErr:     true,
			errContains: "empty name",
		},
		{
			name: "empty command returns error",
			input: &MCPConfig{
				Servers: []ServerConfig{
					{Name: "fs", Command: ""},
				},
			},
			wantErr:     true,
			errContains: "empty command",
		},
		{
			name: "whitespace-only command returns error",
			input: &MCPConfig{
				Servers: []ServerConfig{
					{Name: "fs", Command: "   "},
				},
			},
			wantErr:     true,
			errContains: "empty command",
		},
		{
			name:  "empty servers slice is valid",
			input: &MCPConfig{Servers: []ServerConfig{}},
			check: func(t *testing.T, cfg *MCPConfig) {
				if len(cfg.Servers) != 0 {
					t.Errorf("Servers len = %d, want 0", len(cfg.Servers))
				}
			},
		},
		{
			name:  "nil servers slice is valid",
			input: &MCPConfig{},
			check: func(t *testing.T, cfg *MCPConfig) {
				if len(cfg.Servers) != 0 {
					t.Errorf("Servers len = %d, want 0", len(cfg.Servers))
				}
			},
		},
		{
			name: "three unique servers all preserved",
			input: &MCPConfig{
				Servers: []ServerConfig{
					{Name: "a", Command: "cmd-a"},
					{Name: "b", Command: "cmd-b"},
					{Name: "c", Command: "cmd-c"},
				},
			},
			check: func(t *testing.T, cfg *MCPConfig) {
				if len(cfg.Servers) != 3 {
					t.Errorf("Servers len = %d, want 3", len(cfg.Servers))
				}
			},
		},
		{
			name: "sse server with URL and no command passes validation",
			input: &MCPConfig{
				Servers: []ServerConfig{
					{Name: "remote", Transport: "sse", URL: "https://example.com/mcp"},
				},
			},
			check: func(t *testing.T, cfg *MCPConfig) {
				if len(cfg.Servers) != 1 {
					t.Fatalf("Servers len = %d, want 1", len(cfg.Servers))
				}
				if cfg.Servers[0].URL != "https://example.com/mcp" {
					t.Errorf("URL = %q, want https://example.com/mcp", cfg.Servers[0].URL)
				}
			},
		},
		{
			name: "SSE uppercase transport normalized to lowercase sse",
			input: &MCPConfig{
				Servers: []ServerConfig{
					{Name: "remote", Transport: "SSE", URL: "https://example.com/mcp"},
				},
			},
			check: func(t *testing.T, cfg *MCPConfig) {
				if len(cfg.Servers) != 1 {
					t.Fatalf("Servers len = %d, want 1", len(cfg.Servers))
				}
				if cfg.Servers[0].Transport != "sse" {
					t.Errorf("Transport = %q, want %q (lowercased)", cfg.Servers[0].Transport, "sse")
				}
			},
		},
		{
			name: "sse server with empty URL returns error",
			input: &MCPConfig{
				Servers: []ServerConfig{
					{Name: "remote", Transport: "sse", URL: ""},
				},
			},
			wantErr:     true,
			errContains: "empty url",
		},
		{
			name: "mixed stdio and sse servers both validated correctly",
			input: &MCPConfig{
				Servers: []ServerConfig{
					{Name: "local", Transport: "stdio", Command: "npx"},
					{Name: "remote", Transport: "sse", URL: "https://example.com/mcp"},
				},
			},
			check: func(t *testing.T, cfg *MCPConfig) {
				if len(cfg.Servers) != 2 {
					t.Errorf("Servers len = %d, want 2", len(cfg.Servers))
				}
			},
		},
		{
			name: "unknown transport grpc returns error",
			input: &MCPConfig{
				Servers: []ServerConfig{
					{Name: "grpc-srv", Transport: "grpc", Command: "server"},
				},
			},
			wantErr:     true,
			errContains: "unknown transport",
		},
		{
			name: "Transport STDIO uppercase normalized to stdio",
			input: &MCPConfig{
				Servers: []ServerConfig{
					{Name: "fs", Transport: "STDIO", Command: "npx"},
				},
			},
			check: func(t *testing.T, cfg *MCPConfig) {
				if len(cfg.Servers) != 1 {
					t.Fatalf("Servers len = %d, want 1", len(cfg.Servers))
				}
				if cfg.Servers[0].Transport != "stdio" {
					t.Errorf("Transport = %q, want %q (lowercased)", cfg.Servers[0].Transport, "stdio")
				}
			},
		},
		{
			name: "sse server with whitespace-only URL returns error",
			input: &MCPConfig{
				Servers: []ServerConfig{
					{Name: "remote", Transport: "sse", URL: "   "},
				},
			},
			wantErr:     true,
			errContains: "empty url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NormalizeConfig(tt.input)

			if (err != nil) != tt.wantErr {
				t.Fatalf("NormalizeConfig() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errContains != "" {
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.errContains)
				}
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, tt.input)
			}
		})
	}
}

// --- Registry.LoadFromConfig ---

func TestRegistry_LoadFromConfig(t *testing.T) {
	tests := []struct {
		name  string
		cfg   MCPConfig
		check func(*testing.T, *Registry)
	}{
		{
			name: "registers all servers from config",
			cfg: MCPConfig{
				Servers: []ServerConfig{
					{Name: "fs", Transport: "stdio", Command: "npx"},
					{Name: "git", Transport: "stdio", Command: "git-mcp"},
				},
			},
			check: func(t *testing.T, r *Registry) {
				if len(r.Servers()) != 2 {
					t.Fatalf("Servers() len = %d, want 2", len(r.Servers()))
				}
				fs, ok := r.GetServer("fs")
				if !ok {
					t.Fatal("server fs not found in registry")
				}
				if fs.Command != "npx" {
					t.Errorf("fs Command = %q, want npx", fs.Command)
				}
				if _, ok := r.GetServer("git"); !ok {
					t.Error("server git not found in registry")
				}
			},
		},
		{
			name: "empty config registers nothing",
			cfg:  MCPConfig{},
			check: func(t *testing.T, r *Registry) {
				if len(r.Servers()) != 0 {
					t.Errorf("Servers() len = %d, want 0", len(r.Servers()))
				}
			},
		},
		{
			name: "overwrites existing server with same name",
			cfg: MCPConfig{
				Servers: []ServerConfig{
					{Name: "fs", Transport: "stdio", Command: "new-cmd"},
				},
			},
			check: func(t *testing.T, r *Registry) {
				// Pre-register the old version before LoadFromConfig is called in
				// the loop body — we simulate that here by re-reading the registry.
				s, ok := r.GetServer("fs")
				if !ok {
					t.Fatal("server fs not found")
				}
				if s.Command != "new-cmd" {
					t.Errorf("Command = %q, want new-cmd (overwritten)", s.Command)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRegistry()
			// Pre-seed registry for the overwrite test.
			if tt.name == "overwrites existing server with same name" {
				r.AddServer(ServerConfig{Name: "fs", Transport: "stdio", Command: "old-cmd"})
			}
			r.LoadFromConfig(&tt.cfg)
			tt.check(t, r)
		})
	}
}

// --- Registry.LoadFromFile (end-to-end) ---

func TestRegistry_LoadFromFile(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) string // returns path to pass to LoadFromFile
		wantErr bool
		check   func(t *testing.T, r *Registry)
	}{
		{
			name: "two servers loaded into registry end-to-end",
			setup: func(t *testing.T) string {
				return writeTempJSON(t, t.TempDir(), "mcp.json", MCPConfig{
					Servers: []ServerConfig{
						{Name: "fs", Transport: "stdio", Command: "npx", Enabled: true},
						{Name: "git", Transport: "stdio", Command: "git-mcp", Enabled: true},
					},
				})
			},
			check: func(t *testing.T, r *Registry) {
				servers := r.Servers()
				if len(servers) != 2 {
					t.Fatalf("Servers() len = %d, want 2", len(servers))
				}
				fs, ok := r.GetServer("fs")
				if !ok {
					t.Fatal("server fs not found in registry")
				}
				if fs.Command != "npx" {
					t.Errorf("fs Command = %q, want npx", fs.Command)
				}
				if _, ok := r.GetServer("git"); !ok {
					t.Error("server git not found in registry")
				}
			},
		},
		{
			name: "nonexistent file leaves registry empty without error",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing.json")
			},
			check: func(t *testing.T, r *Registry) {
				if len(r.Servers()) != 0 {
					t.Errorf("Servers() len = %d, want 0", len(r.Servers()))
				}
			},
		},
		{
			name: "malformed JSON returns parse error",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "bad.json")
				if err := os.WriteFile(path, []byte("{bad json"), 0600); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
				return path
			},
			wantErr: true,
		},
		{
			name: "server with empty command returns normalize error",
			setup: func(t *testing.T) string {
				return writeTempJSON(t, t.TempDir(), "mcp.json", MCPConfig{
					Servers: []ServerConfig{
						{Name: "fs", Transport: "stdio", Command: ""},
					},
				})
			},
			wantErr: true,
		},
		{
			name: "server with empty name returns normalize error",
			setup: func(t *testing.T) string {
				return writeTempJSON(t, t.TempDir(), "mcp.json", MCPConfig{
					Servers: []ServerConfig{
						{Name: "", Transport: "stdio", Command: "npx"},
					},
				})
			},
			wantErr: true,
		},
		{
			name: "whitespace in name and command trimmed before registration",
			setup: func(t *testing.T) string {
				return writeTempJSON(t, t.TempDir(), "mcp.json", MCPConfig{
					Servers: []ServerConfig{
						{Name: "  fs  ", Transport: "stdio", Command: "  npx  "},
					},
				})
			},
			check: func(t *testing.T, r *Registry) {
				s, ok := r.GetServer("fs")
				if !ok {
					t.Fatal("server 'fs' not found after trim (key should be 'fs', not '  fs  ')")
				}
				if s.Command != "npx" {
					t.Errorf("Command = %q, want npx (trimmed)", s.Command)
				}
			},
		},
		{
			name: "sse server with URL and no command loads into registry",
			setup: func(t *testing.T) string {
				return writeTempJSON(t, t.TempDir(), "mcp.json", MCPConfig{
					Servers: []ServerConfig{
						{Name: "remote", Transport: "sse", URL: "https://example.com/mcp", Enabled: true},
					},
				})
			},
			check: func(t *testing.T, r *Registry) {
				s, ok := r.GetServer("remote")
				if !ok {
					t.Fatal("server remote not found in registry")
				}
				if s.URL != "https://example.com/mcp" {
					t.Errorf("URL = %q, want https://example.com/mcp", s.URL)
				}
				if s.Transport != "sse" {
					t.Errorf("Transport = %q, want sse", s.Transport)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRegistry()
			path := tt.setup(t)
			err := r.LoadFromFile(path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("LoadFromFile() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, r)
			}
		})
	}
}
