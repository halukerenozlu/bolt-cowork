package repl_test

import (
	"strings"
	"testing"

	"github.com/halukerenozlu/bolt-cowork/internal/mcp"
	"github.com/halukerenozlu/bolt-cowork/internal/repl"
)

// mockInspector is a test double for MCPInspector.
type mockInspector struct {
	servers []repl.ServerInfo
	tools   []repl.ToolInfo
}

func (m *mockInspector) Servers() []repl.ServerInfo { return m.servers }

func (m *mockInspector) ToolsByServer(name string) []repl.ToolInfo {
	var out []repl.ToolInfo
	for _, t := range m.tools {
		if t.ServerName == name {
			out = append(out, t)
		}
	}
	return out
}

func (m *mockInspector) AllTools() []repl.ToolInfo { return m.tools }

func run(args []string, inspector repl.MCPInspector) string {
	w := &strings.Builder{}
	_ = repl.HandleMCPCommand(args, inspector, w)
	return w.String()
}

// ─── /mcp list ───────────────────────────────────────────────────────────────

func TestMCPList_NoServers(t *testing.T) {
	out := run([]string{"list"}, &mockInspector{})
	if !strings.Contains(out, "No MCP servers connected.") {
		t.Errorf("expected empty message, got %q", out)
	}
}

func TestMCPList_NilInspector(t *testing.T) {
	out := run([]string{"list"}, nil)
	if !strings.Contains(out, "No MCP servers connected.") {
		t.Errorf("nil inspector should print empty message, got %q", out)
	}
}

func TestMCPList_TwoServers(t *testing.T) {
	inspector := &mockInspector{
		servers: []repl.ServerInfo{
			{Name: "alpha", Status: mcp.StatusConnected, URL: "http://alpha.local:8080"},
			{Name: "beta", Status: mcp.StatusDisconnected},
		},
	}
	out := run([]string{"list"}, inspector)

	cases := []struct {
		want    string
		missing bool
		label   string
	}{
		{want: "[alpha]", label: "alpha server header"},
		{want: "connected", label: "connected status"},
		{want: "http://alpha.local:8080", label: "alpha URL"},
		{want: "[beta]", label: "beta server header"},
		{want: "disconnected", label: "disconnected status"},
	}
	for _, tc := range cases {
		if !strings.Contains(out, tc.want) {
			t.Errorf("missing %s (%q) in output:\n%s", tc.label, tc.want, out)
		}
	}
}

func TestMCPList_ConnectionStatus(t *testing.T) {
	tests := []struct {
		name   string
		status mcp.ConnectionStatus
		want   string
	}{
		{name: "connected server", status: mcp.StatusConnected, want: "connected"},
		{name: "disconnected server", status: mcp.StatusDisconnected, want: "disconnected"},
		{name: "error server", status: mcp.StatusError, want: "error"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inspector := &mockInspector{
				servers: []repl.ServerInfo{{Name: "srv", Status: tc.status}},
			}
			out := run([]string{"list"}, inspector)
			if !strings.Contains(out, tc.want) {
				t.Errorf("expected %q in output, got: %q", tc.want, out)
			}
		})
	}
}

// ─── /mcp tools ──────────────────────────────────────────────────────────────

func TestMCPList_EnabledServerWithoutConnectionIsNotConnected(t *testing.T) {
	inspector := &mockInspector{
		servers: []repl.ServerInfo{{Name: "srv", Enabled: true, Status: mcp.StatusError}},
	}
	out := run([]string{"list"}, inspector)
	if strings.Contains(out, "status: connected") {
		t.Fatalf("enabled server with error status must not appear connected:\n%s", out)
	}
	if !strings.Contains(out, "status: error") {
		t.Fatalf("expected error status in output:\n%s", out)
	}
}

func TestMCPTools_NoTools(t *testing.T) {
	out := run([]string{"tools"}, &mockInspector{})
	if !strings.Contains(out, "No MCP tools available.") {
		t.Errorf("expected empty message, got %q", out)
	}
}

func TestMCPTools_NilInspector(t *testing.T) {
	out := run([]string{"tools"}, nil)
	if !strings.Contains(out, "No MCP tools available.") {
		t.Errorf("nil inspector should print empty message, got %q", out)
	}
}

func TestMCPTools_TwoServersGrouped(t *testing.T) {
	inspector := &mockInspector{
		tools: []repl.ToolInfo{
			{Name: "write_file", Description: "writes a file", ServerName: "fs-server"},
			{Name: "read_file", Description: "reads a file", ServerName: "fs-server"},
			{Name: "search", Description: "full text search", ServerName: "search-server"},
		},
	}
	out := run([]string{"tools"}, inspector)

	for _, want := range []string{"[fs-server]", "[search-server]", "write_file", "read_file", "search"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output:\n%s", want, out)
		}
	}

	// fs-server must appear before search-server (first-seen order).
	if strings.Index(out, "[fs-server]") >= strings.Index(out, "[search-server]") {
		t.Errorf("expected fs-server before search-server in:\n%s", out)
	}
}

func TestMCPTools_FilterByServer(t *testing.T) {
	inspector := &mockInspector{
		tools: []repl.ToolInfo{
			{Name: "write_file", Description: "writes a file", ServerName: "fs-server"},
			{Name: "search", Description: "full text search", ServerName: "search-server"},
		},
	}
	out := run([]string{"tools", "fs-server"}, inspector)

	if !strings.Contains(out, "write_file") {
		t.Errorf("expected write_file in filtered output:\n%s", out)
	}
	if strings.Contains(out, "search") {
		t.Errorf("search (wrong server) should be absent from filtered output:\n%s", out)
	}
	if !strings.Contains(out, "[fs-server]") {
		t.Errorf("expected server header in filtered output:\n%s", out)
	}
}

func TestMCPTools_FilterServerNoMatch(t *testing.T) {
	inspector := &mockInspector{
		tools: []repl.ToolInfo{
			{Name: "write_file", Description: "writes a file", ServerName: "fs-server"},
		},
	}
	out := run([]string{"tools", "missing-server"}, inspector)
	if !strings.Contains(out, "No MCP tools available") {
		t.Errorf("expected empty message for unknown server, got %q", out)
	}
}

func TestMCPTools_DescriptionTruncated(t *testing.T) {
	longDesc := strings.Repeat("x", 100) // exceeds descMaxLen (80)
	inspector := &mockInspector{
		tools: []repl.ToolInfo{
			{Name: "verbose_tool", Description: longDesc, ServerName: "srv"},
		},
	}
	out := run([]string{"tools"}, inspector)

	if strings.Contains(out, longDesc) {
		t.Errorf("expected description to be truncated, got full string in:\n%s", out)
	}
	if !strings.Contains(out, "...") {
		t.Errorf("expected ellipsis for truncated description in:\n%s", out)
	}
}

func TestMCPTools_NoDescription(t *testing.T) {
	inspector := &mockInspector{
		tools: []repl.ToolInfo{
			{Name: "mystery_tool", Description: "", ServerName: "srv"},
		},
	}
	out := run([]string{"tools"}, inspector)
	if !strings.Contains(out, "no description") {
		t.Errorf("expected '(no description)' for empty desc in:\n%s", out)
	}
}

// ─── Invalid subcommand / no args ─────────────────────────────────────────────

func TestMCPCommand_UnknownSubcommand(t *testing.T) {
	out := run([]string{"foo"}, &mockInspector{})
	if !strings.Contains(out, "Unknown subcommand") {
		t.Errorf("expected unknown subcommand message, got %q", out)
	}
	if !strings.Contains(out, "Usage:") {
		t.Errorf("expected usage hint in unknown subcommand message, got %q", out)
	}
}

func TestMCPCommand_NoArgs(t *testing.T) {
	out := run([]string{}, &mockInspector{})
	if !strings.Contains(out, "Usage:") {
		t.Errorf("expected usage message with no args, got %q", out)
	}
}
