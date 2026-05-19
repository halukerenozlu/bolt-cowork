package repl

import (
	"fmt"
	"io"
	"strings"

	"github.com/halukerenozlu/bolt-cowork/internal/mcp"
)

const descMaxLen = 80

// ServerInfo holds display-ready information about one MCP server.
type ServerInfo struct {
	Name    string
	Enabled bool
	Status  mcp.ConnectionStatus
	URL     string // non-empty for SSE transport servers
}

// ToolInfo holds display-ready information about one MCP tool.
type ToolInfo struct {
	Name        string
	Description string
	ServerName  string
}

// MCPInspector provides read-only access to the current MCP state.
// It is the only MCP dependency the command handlers require.
type MCPInspector interface {
	// Servers returns all registered servers sorted by name.
	Servers() []ServerInfo
	// ToolsByServer returns all tools for the named server, sorted by name.
	// Returns an empty slice when no tools are found for that server.
	ToolsByServer(serverName string) []ToolInfo
	// AllTools returns every tool from every server.
	AllTools() []ToolInfo
}

// RegistryInspector adapts *mcp.Registry to the MCPInspector interface.
type RegistryInspector struct {
	reg *mcp.Registry
}

// NewRegistryInspector wraps reg in a RegistryInspector.
// If reg is nil, all methods return empty slices.
func NewRegistryInspector(reg *mcp.Registry) *RegistryInspector {
	return &RegistryInspector{reg: reg}
}

// Servers returns all registered server configs as ServerInfo values.
func (ri *RegistryInspector) Servers() []ServerInfo {
	if ri.reg == nil {
		return nil
	}
	raw := ri.reg.Servers()
	out := make([]ServerInfo, len(raw))
	for i, s := range raw {
		out[i] = ServerInfo{Name: s.Name, Enabled: s.Enabled, Status: s.ConnectionStatus(), URL: s.URL}
	}
	return out
}

// ToolsByServer returns all tools provided by the named server.
func (ri *RegistryInspector) ToolsByServer(serverName string) []ToolInfo {
	if ri.reg == nil {
		return nil
	}
	raw := ri.reg.ToolsByServer(serverName)
	out := make([]ToolInfo, len(raw))
	for i, t := range raw {
		out[i] = ToolInfo{Name: t.Name, Description: t.Description, ServerName: t.ServerName}
	}
	return out
}

// AllTools returns all registered tools.
func (ri *RegistryInspector) AllTools() []ToolInfo {
	if ri.reg == nil {
		return nil
	}
	raw := ri.reg.Tools()
	out := make([]ToolInfo, len(raw))
	for i, t := range raw {
		out[i] = ToolInfo{Name: t.Name, Description: t.Description, ServerName: t.ServerName}
	}
	return out
}

// HandleMCPCommand dispatches /mcp subcommands.
// args is the slice of tokens after "/mcp" (e.g. ["list"] or ["tools", "my-server"]).
// Returns an error only for unexpected internal failures; user-facing messages
// are written directly to w.
func HandleMCPCommand(args []string, inspector MCPInspector, w io.Writer) error {
	if len(args) == 0 {
		fmt.Fprintln(w, "Usage: /mcp list | /mcp tools [server-name]")
		return nil
	}
	switch strings.ToLower(args[0]) {
	case "list":
		handleMCPList(inspector, w)
	case "tools":
		serverFilter := ""
		if len(args) > 1 {
			serverFilter = args[1]
		}
		handleMCPTools(serverFilter, inspector, w)
	default:
		fmt.Fprintln(w, "Unknown subcommand. Usage: /mcp list | /mcp tools [server-name]")
	}
	return nil
}

// handleMCPList lists all registered MCP servers with their connection status.
func handleMCPList(inspector MCPInspector, w io.Writer) {
	if inspector == nil {
		fmt.Fprintln(w, "No MCP servers connected.")
		return
	}
	servers := inspector.Servers()
	if len(servers) == 0 {
		fmt.Fprintln(w, "No MCP servers connected.")
		return
	}
	for _, s := range servers {
		status := s.Status
		if status == "" {
			status = mcp.StatusDisconnected
		}
		if s.URL != "" {
			fmt.Fprintf(w, "[%s]  status: %s  (url: %s)\n", s.Name, status, s.URL)
		} else {
			fmt.Fprintf(w, "[%s]  status: %s\n", s.Name, status)
		}
	}
}

// handleMCPTools lists tools grouped by server.
// If serverFilter is non-empty, only tools from that server are shown.
func handleMCPTools(serverFilter string, inspector MCPInspector, w io.Writer) {
	if inspector == nil {
		fmt.Fprintln(w, "No MCP tools available.")
		return
	}

	// TODO: add --json flag support for machine-readable output

	if serverFilter != "" {
		tools := inspector.ToolsByServer(serverFilter)
		if len(tools) == 0 {
			fmt.Fprintf(w, "No MCP tools available from server %q.\n", serverFilter)
			return
		}
		fmt.Fprintf(w, "[%s]\n", serverFilter)
		for _, t := range tools {
			printToolLine(t, w)
		}
		return
	}

	allTools := inspector.AllTools()
	if len(allTools) == 0 {
		fmt.Fprintln(w, "No MCP tools available.")
		return
	}

	// Group tools by server, preserving the order servers first appear.
	seen := make(map[string]bool)
	var order []string
	byServer := make(map[string][]ToolInfo)
	for _, t := range allTools {
		if !seen[t.ServerName] {
			seen[t.ServerName] = true
			order = append(order, t.ServerName)
		}
		byServer[t.ServerName] = append(byServer[t.ServerName], t)
	}
	for _, serverName := range order {
		fmt.Fprintf(w, "[%s]\n", serverName)
		for _, t := range byServer[serverName] {
			printToolLine(t, w)
		}
	}
}

// printToolLine writes a single "  • name  — description" line to w.
func printToolLine(t ToolInfo, w io.Writer) {
	desc := strings.TrimSpace(t.Description)
	if desc == "" {
		fmt.Fprintf(w, "  • %-20s  (no description)\n", t.Name)
		return
	}
	if len(desc) > descMaxLen {
		desc = desc[:descMaxLen-3] + "..."
	}
	fmt.Fprintf(w, "  • %-20s  — %s\n", t.Name, desc)
}
