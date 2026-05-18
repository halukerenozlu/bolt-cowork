package actions

import (
	"context"
	"fmt"
	"strings"

	"github.com/halukerenozlu/bolt-cowork/internal/mcp"
)

// MCPCaller is the minimal interface for invoking tools on an MCP server.
// Using an interface instead of *mcp.Client avoids a direct dependency on the
// concrete client type and makes this action straightforward to test.
type MCPCaller interface {
	CallTool(ctx context.Context, serverName, toolName string, args map[string]any) (*mcp.CallToolResult, error)
}

// ActionResult holds the outcome of a single action execution.
type ActionResult struct {
	// Output contains the tool's response text on success.
	Output string
	// Error contains a human-readable error message on failure.
	Error string
}

// Action is the interface that all MCP-backed actions must implement.
type Action interface {
	Type() string
	Execute(ctx context.Context, client MCPCaller) (ActionResult, error)
	IsDangerous() bool
	Summary() string
}

// CallMCPToolAction calls a named tool on a named MCP server.
type CallMCPToolAction struct {
	ServerName string
	ToolName   string
	Args       map[string]any
}

// Type returns the stable string identifier for this action kind.
func (a *CallMCPToolAction) Type() string {
	return "call_mcp_tool"
}

// IsDangerous always returns true: MCP tool calls have side effects outside
// the local sandbox and therefore require explicit user approval.
func (a *CallMCPToolAction) IsDangerous() bool {
	return true
}

// Summary returns a short, human-readable description of the call.
func (a *CallMCPToolAction) Summary() string {
	return fmt.Sprintf("MCP tool çağrısı: %s/%s", a.ServerName, a.ToolName)
}

// Execute invokes the MCP tool and converts the result into an ActionResult.
// Transport or protocol errors are captured in ActionResult.Error so callers
// can surface them without a panic. Context cancellation returns a Go error,
// not an ActionResult.
func (a *CallMCPToolAction) Execute(ctx context.Context, client MCPCaller) (ActionResult, error) {
	if err := ctx.Err(); err != nil {
		return ActionResult{}, err
	}
	result, err := client.CallTool(ctx, a.ServerName, a.ToolName, a.Args)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ActionResult{}, ctxErr
		}
		return ActionResult{Error: err.Error()}, nil
	}

	// Concatenate all text content items into a single output string.
	var sb strings.Builder
	for _, c := range result.Content {
		if c.Type == "text" {
			sb.WriteString(c.Text)
		}
	}
	text := sb.String()

	if result.IsError {
		return ActionResult{Error: text}, nil
	}

	return ActionResult{Output: text}, nil
}
