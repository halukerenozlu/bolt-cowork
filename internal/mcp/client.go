package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
)

// ErrConnectionClosed is returned by in-flight calls when the server
// connection is closed before a response is received.
var ErrConnectionClosed = errors.New("connection closed")

// Client manages JSON-RPC connections to one or more named MCP servers.
// Each server is identified by a unique name and backed by a Transport.
// Client is safe for concurrent use from multiple goroutines.
type Client struct {
	mu    sync.Mutex
	conns map[string]*serverConn
	gen   IDGenerator
	tools *ToolRegistry
}

// serverConn holds per-server connection state.
type serverConn struct {
	transport Transport
	pending   *PendingRegistry
	cancel    context.CancelFunc // stops the background reader goroutine
}

// NewClient creates an empty Client with no server connections.
func NewClient() *Client {
	return &Client{
		conns: make(map[string]*serverConn),
		tools: NewToolRegistry(),
	}
}

// Tools returns the ToolRegistry that holds all tools discovered via
// DiscoverTools. The registry is safe for concurrent use.
func (c *Client) Tools() *ToolRegistry {
	return c.tools
}

// Connect attaches t as the transport for the server named name and starts
// a background goroutine that routes incoming responses to their callers.
// If a connection for name already exists it is replaced; the old transport
// is closed and its reader goroutine is stopped.
func (c *Client) Connect(name string, t Transport) {
	ctx, cancel := context.WithCancel(context.Background())
	conn := &serverConn{
		transport: t,
		pending:   NewPendingRegistry(),
		cancel:    cancel,
	}

	c.mu.Lock()
	old, exists := c.conns[name]
	c.conns[name] = conn
	c.mu.Unlock()

	if exists {
		// Unblock any callers waiting on the old connection before tearing it
		// down so they receive a clear error instead of blocking indefinitely.
		old.pending.CloseAll(ErrConnectionClosed)
		old.cancel()
		c.tools.RemoveServer(name)
		_ = old.transport.Close()
	}

	go conn.readLoop(ctx)
}

// readLoop reads responses from the transport and routes them to waiting
// callers via the pending registry. It exits when ctx is cancelled or the
// transport is closed. On exit, all remaining in-flight callers are unblocked
// with ErrConnectionClosed so they never block indefinitely.
func (conn *serverConn) readLoop(ctx context.Context) {
	defer conn.pending.CloseAll(ErrConnectionClosed)

	for {
		raw, err := conn.transport.Receive(ctx)
		if err != nil {
			return
		}

		var resp Response
		if err := json.Unmarshal(raw, &resp); err != nil {
			// Malformed message — skip without terminating the loop.
			continue
		}

		if resp.ID.IsNull() {
			// Notification — no pending request to resolve.
			continue
		}

		conn.pending.Resolve(resp)
	}
}

// Disconnect closes the named server's transport, stops its reader goroutine,
// and removes the connection from the client. It is a no-op when name is not
// connected.
func (c *Client) Disconnect(name string) error {
	c.mu.Lock()
	conn, ok := c.conns[name]
	if ok {
		delete(c.conns, name)
	}
	c.mu.Unlock()

	if !ok {
		return nil
	}
	// Unblock in-flight callers before cancelling the context and closing the
	// transport, so they receive ErrConnectionClosed rather than blocking.
	conn.pending.CloseAll(ErrConnectionClosed)
	conn.cancel()
	c.tools.RemoveServer(name)
	return conn.transport.Close()
}

// Close disconnects all servers and releases all resources.
func (c *Client) Close() error {
	c.mu.Lock()
	names := make([]string, 0, len(c.conns))
	for name := range c.conns {
		names = append(names, name)
	}
	c.mu.Unlock()

	var firstErr error
	for _, name := range names {
		if err := c.Disconnect(name); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// call sends a JSON-RPC request to the named server and blocks until a
// matching response is received or ctx is cancelled.
func (c *Client) call(ctx context.Context, serverName, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	conn, ok := c.conns[serverName]
	c.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("mcp/client: no connection for server %q", serverName)
	}

	var rawParams json.RawMessage
	if params != nil {
		var err error
		rawParams, err = json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("mcp/client: marshal params: %w", err)
		}
	}

	id := c.gen.Next()
	req := NewRequest(id, method, rawParams)

	rawReq, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("mcp/client: marshal request: %w", err)
	}

	ch, err := conn.pending.Register(id)
	if err != nil {
		return nil, fmt.Errorf("mcp/client: register pending: %w", err)
	}
	// Cancel cleans up the registry entry if we exit before Resolve delivers.
	defer conn.pending.Cancel(id)

	if err := conn.transport.Send(ctx, rawReq); err != nil {
		return nil, fmt.Errorf("mcp/client: send %s: %w", method, err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp, ok := <-ch:
		if !ok {
			// Channel closed by CloseAll — return the connection-level error.
			if cerr := conn.pending.CloseErr(); cerr != nil {
				return nil, fmt.Errorf("mcp/client: %w", cerr)
			}
			return nil, fmt.Errorf("mcp/client: request cancelled")
		}
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	}
}

// listToolsResult is the decoded result payload of a tools/list response.
type listToolsResult struct {
	Tools []Tool `json:"tools"`
}

// ListTools requests the list of tools exposed by the named MCP server and
// returns them as raw wire-format Tool values.
func (c *Client) ListTools(ctx context.Context, serverName string) ([]Tool, error) {
	raw, err := c.call(ctx, serverName, "tools/list", nil)
	if err != nil {
		return nil, fmt.Errorf("mcp/client: ListTools %q: %w", serverName, err)
	}

	var result listToolsResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("mcp/client: ListTools decode: %w", err)
	}

	return result.Tools, nil
}

// DiscoverTools queries every connected server for its tool list and stores
// the results in the client's ToolRegistry. It continues after individual
// server failures so that a single unresponsive server does not block
// discovery of tools on the remaining servers.
//
// Any per-server error is logged and collected; if at least one server fails
// the combined error is returned after all servers have been queried.
// If all servers succeed, nil is returned.
func (c *Client) DiscoverTools(ctx context.Context) error {
	c.mu.Lock()
	names := make([]string, 0, len(c.conns))
	for name := range c.conns {
		names = append(names, name)
	}
	c.mu.Unlock()

	var errs []error
	for _, name := range names {
		tools, err := c.ListTools(ctx, name)
		if err != nil {
			log.Printf("mcp/client: DiscoverTools: server %q: %v", name, err)
			errs = append(errs, fmt.Errorf("%q: %w", name, err))
			continue
		}
		c.tools.ReplaceServerTools(name, tools)
	}

	if len(errs) > 0 {
		return fmt.Errorf("mcp/client: DiscoverTools: %w", errors.Join(errs...))
	}
	return nil
}

// callToolParams is the request body for tools/call.
type callToolParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// CallTool invokes toolName on the named MCP server with args and returns
// the tool's result. A nil args map is sent as an empty object.
func (c *Client) CallTool(ctx context.Context, serverName string, toolName string, args map[string]any) (*CallToolResult, error) {
	if args == nil {
		args = map[string]any{}
	}

	params := callToolParams{
		Name:      toolName,
		Arguments: args,
	}

	raw, err := c.call(ctx, serverName, "tools/call", params)
	if err != nil {
		return nil, fmt.Errorf("mcp/client: CallTool %q.%q: %w", serverName, toolName, err)
	}

	var result CallToolResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("mcp/client: CallTool decode: %w", err)
	}

	return &result, nil
}
