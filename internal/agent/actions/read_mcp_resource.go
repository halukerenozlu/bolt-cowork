package actions

import (
	"context"
	"fmt"

	"github.com/halukerenozlu/bolt-cowork/internal/mcp"
)

// MCPResourceReader is the minimal interface for reading a resource from an
// MCP server. Using an interface instead of *mcp.Client avoids a direct
// dependency on the concrete client type and makes this action easy to test.
type MCPResourceReader interface {
	ReadResource(ctx context.Context, serverName, uri string) (*mcp.ResourceContents, error)
}

// ReadMCPResourceAction reads a single resource from a named MCP server.
type ReadMCPResourceAction struct {
	ServerName string
	URI        string
}

// Type returns the stable string identifier for this action kind.
func (a *ReadMCPResourceAction) Type() string {
	return "read_mcp_resource"
}

// IsDangerous returns false: reading a resource is a read-only operation with
// no side effects outside the local sandbox.
func (a *ReadMCPResourceAction) IsDangerous() bool {
	return false
}

// Summary returns a short, human-readable description of the read operation.
func (a *ReadMCPResourceAction) Summary() string {
	return fmt.Sprintf("Read MCP resource: %s/%s", a.ServerName, a.URI)
}

// Execute fetches the resource and returns its text content in ActionResult.Output.
func (a *ReadMCPResourceAction) Execute(ctx context.Context, reader MCPResourceReader) (ActionResult, error) {
	if err := ctx.Err(); err != nil {
		return ActionResult{}, err
	}
	contents, err := reader.ReadResource(ctx, a.ServerName, a.URI)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ActionResult{}, ctxErr
		}
		return ActionResult{}, fmt.Errorf("read_mcp_resource %s/%s: %w", a.ServerName, a.URI, err)
	}
	return ActionResult{Output: contents.Text}, nil
}
