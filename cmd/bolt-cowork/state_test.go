package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/halukerenozlu/bolt-cowork/internal/config"
	"github.com/halukerenozlu/bolt-cowork/pkg/types"
)

func TestNewAppState(t *testing.T) {
	cfg := config.Default()
	cfg.ApprovalMode = "plan-only"

	state := NewAppState(cfg, "1.0.0")

	if state.Cfg != cfg {
		t.Error("Cfg should be the same pointer as input")
	}
	if state.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", state.Version, "1.0.0")
	}
	if state.ApprovalMode != "plan-only" {
		t.Errorf("ApprovalMode = %q, want %q", state.ApprovalMode, "plan-only")
	}
	if state.CmdRegistry == nil {
		t.Error("CmdRegistry should not be nil")
	}
	if state.SkillStore == nil {
		t.Error("SkillStore should not be nil")
	}
	if state.ToolRegistry == nil {
		t.Error("ToolRegistry should not be nil")
	}
	if state.MCPRegistry == nil {
		t.Error("MCPRegistry should not be nil")
	}
	if state.MCPClient == nil {
		t.Error("MCPClient should not be nil")
	}
	if state.WorkDir == "" {
		t.Error("WorkDir should be resolved, not empty")
	}

	// CmdRegistry should have default commands registered.
	if _, ok := state.CmdRegistry.Get("/help"); !ok {
		t.Error("CmdRegistry should have /help registered")
	}
}

func TestNewAppState_LoadsMCPServersForCommandRegistry(t *testing.T) {
	cfg := config.Default()
	cfg.MCPServers = map[string]any{
		"fs": map[string]any{
			"transport": "stdio",
			"command":   "fs-mcp",
			"allowed_tools": []any{
				"read_*",
			},
			"denied_tools": []any{
				"delete_*",
			},
			"enabled": true,
		},
	}
	state := NewAppState(cfg, "test")

	if _, ok := state.MCPRegistry.GetServer("fs"); !ok {
		t.Fatal("MCPRegistry missing configured server fs")
	}
	if _, err := state.MCPClient.CallTool(t.Context(), "fs", "delete_file", nil); err == nil || !strings.Contains(err.Error(), "denied by permission profile") {
		t.Fatalf("MCPClient did not load configured denylist, err = %v", err)
	}

	cmd, ok := state.CmdRegistry.Get("/mcp")
	if !ok {
		t.Fatal("registry missing /mcp command")
	}

	output := captureStderr(func() {
		if err := cmd.Execute([]string{"list"}, state.CommandContext()); err != nil {
			t.Fatalf("Execute(/mcp list) error: %v", err)
		}
	})

	if !strings.Contains(output, "[fs]") {
		t.Fatalf("expected /mcp list to include configured server, got:\n%s", output)
	}
	if strings.Contains(output, "No MCP servers connected.") {
		t.Fatalf("/mcp list should not report empty registry, got:\n%s", output)
	}
	if strings.Contains(output, "status: connected") {
		t.Fatalf("configured but not connected server must not be shown as connected:\n%s", output)
	}
	if !strings.Contains(output, "status: disconnected") {
		t.Fatalf("expected disconnected runtime status, got:\n%s", output)
	}
}

func TestNewAppState_LoadsStructuredMCPPermissions(t *testing.T) {
	cfg := config.Default()
	cfg.MCP.Servers = []config.MCPServer{{
		Name:         "fs",
		Transport:    "stdio",
		Command:      "fs-mcp",
		AllowedTools: []string{"read_*"},
		DeniedTools:  []string{"delete_*"},
	}}

	state := NewAppState(cfg, "test")

	server, ok := state.MCPRegistry.GetServer("fs")
	if !ok {
		t.Fatal("MCPRegistry missing configured server fs")
	}
	if len(server.AllowedTools) != 1 || server.AllowedTools[0] != "read_*" {
		t.Fatalf("AllowedTools = %#v, want [read_*]", server.AllowedTools)
	}
	if len(server.DeniedTools) != 1 || server.DeniedTools[0] != "delete_*" {
		t.Fatalf("DeniedTools = %#v, want [delete_*]", server.DeniedTools)
	}
	if _, err := state.MCPClient.CallTool(t.Context(), "fs", "delete_file", nil); err == nil || !strings.Contains(err.Error(), "denied by permission profile") {
		t.Fatalf("MCPClient did not load structured denylist, err = %v", err)
	}
}

func TestAppStateClearHistory(t *testing.T) {
	cfg := config.Default()
	state := NewAppState(cfg, "test")

	state.AddMessage(types.Message{Role: "user", Content: "hello"})
	state.AddMessage(types.Message{Role: "assistant", Content: "hi"})

	if len(state.History()) != 2 {
		t.Fatalf("History() = %d messages, want 2", len(state.History()))
	}

	state.ClearHistory()

	if len(state.History()) != 0 {
		t.Errorf("History() after clear = %d messages, want 0", len(state.History()))
	}
}

func TestAppStateHistory(t *testing.T) {
	cfg := config.Default()
	state := NewAppState(cfg, "test")

	msg := types.Message{Role: "user", Content: "test"}
	state.AddMessage(msg)

	history := state.History()
	if len(history) != 1 {
		t.Fatalf("History() = %d messages, want 1", len(history))
	}
	if history[0].Content != "test" {
		t.Errorf("History()[0].Content = %q, want %q", history[0].Content, "test")
	}

	// History() should return a copy, not a reference.
	history[0].Content = "modified"
	if state.Messages[0].Content != "test" {
		t.Error("History() should return a copy; modifying it should not affect state")
	}
}

func TestAppStateSetWorkDir(t *testing.T) {
	oldOverride := workDirOverride
	defer func() { workDirOverride = oldOverride }()

	cfg := config.Default()
	state := NewAppState(cfg, "test")

	original := state.WorkDir
	state.SetWorkDir("/new/path")

	if state.WorkDir != "/new/path" {
		t.Errorf("WorkDir = %q, want %q", state.WorkDir, "/new/path")
	}
	if state.PreviousDir != original {
		t.Errorf("PreviousDir = %q, want %q", state.PreviousDir, original)
	}
	if workDirOverride != "/new/path" {
		t.Errorf("workDirOverride = %q, want %q", workDirOverride, "/new/path")
	}

	// Second SetWorkDir should update PreviousDir to the old WorkDir.
	state.SetWorkDir("/another/path")
	if state.PreviousDir != "/new/path" {
		t.Errorf("PreviousDir after second set = %q, want %q", state.PreviousDir, "/new/path")
	}
}

func TestAppStateCommandContext(t *testing.T) {
	cfg := config.Default()
	state := NewAppState(cfg, "test")
	state.LineReader = &mockLineReader{line: "test"}

	ctx := state.CommandContext()

	if ctx.Cfg != cfg {
		t.Error("CommandContext().Cfg should be the same as state.Cfg")
	}
	if ctx.History != &state.Messages {
		t.Error("CommandContext().History should point to state.Messages")
	}
	if ctx.Store != state.SkillStore {
		t.Error("CommandContext().Store should be state.SkillStore")
	}
	if ctx.ForceSkills != &state.ForceSkills {
		t.Error("CommandContext().ForceSkills should point to state.ForceSkills")
	}
	if ctx.PreviousDir != &state.PreviousDir {
		t.Error("CommandContext().PreviousDir should point to state.PreviousDir")
	}
	if ctx.LineReader != state.LineReader {
		t.Error("CommandContext().LineReader should be state.LineReader")
	}
	if ctx.State != state {
		t.Error("CommandContext().State should be the same AppState")
	}
}

func TestDirUpdatesAppState(t *testing.T) {
	dir := t.TempDir()

	oldOverride := workDirOverride
	defer func() { workDirOverride = oldOverride }()
	workDirOverride = ""

	absDir, _ := filepath.Abs(dir)

	cfg := config.Default()
	cfg.TrustedDirs = []string{absDir} // pre-trust so checkTrust passes without stdin
	state := NewAppState(cfg, "test")
	state.LineReader = &mockLineReader{}
	originalWorkDir := state.WorkDir

	// Execute /dir via the registry.
	cmd, ok := state.CmdRegistry.Get("/dir")
	if !ok {
		t.Fatal("registry missing /dir command")
	}

	ctx := state.CommandContext()

	captureStderr(func() {
		if err := cmd.Execute([]string{dir}, ctx); err != nil {
			t.Fatalf("Execute(/dir) error: %v", err)
		}
	})

	// Both workDirOverride and state.WorkDir must be updated.
	if workDirOverride != absDir {
		t.Errorf("workDirOverride = %q, want %q", workDirOverride, absDir)
	}
	if state.WorkDir != absDir {
		t.Errorf("state.WorkDir = %q, want %q", state.WorkDir, absDir)
	}

	// PreviousDir should be the old WorkDir (via CommandContext pointer).
	if state.PreviousDir != originalWorkDir {
		t.Errorf("state.PreviousDir = %q, want %q", state.PreviousDir, originalWorkDir)
	}
}
