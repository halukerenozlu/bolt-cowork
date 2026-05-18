// Package registry re-exports mcp.ToolRegistry so that callers can import
// internal/mcp/registry without importing the parent mcp package directly.
//
// The canonical implementation lives in internal/mcp/tool_registry.go.
// All methods (AddTools, GetTool, ListAll, RemoveServer, Clear) are inherited
// from mcp.ToolRegistry via the type alias and require no wrapper code.
package registry

import mcppkg "github.com/halukerenozlu/bolt-cowork/internal/mcp"

// ToolRegistry is an alias for mcp.ToolRegistry.
// It indexes MCP tools by name and groups them by originating server.
type ToolRegistry = mcppkg.ToolRegistry

// NewToolRegistry creates an empty ToolRegistry.
func NewToolRegistry() *ToolRegistry {
	return mcppkg.NewToolRegistry()
}
