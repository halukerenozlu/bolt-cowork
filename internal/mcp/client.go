package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
)

// ErrConnectionClosed is returned by in-flight calls when the server
// connection is closed before a response is received.
var ErrConnectionClosed = errors.New("connection closed")

// Client manages JSON-RPC connections to one or more named MCP servers.
// Each server is identified by a unique name and backed by a Transport.
// Client is safe for concurrent use from multiple goroutines.
type Client struct {
	mu             sync.Mutex
	conns          map[string]*serverConn
	gen            IDGenerator
	tools          *ToolRegistry
	resources      *ResourceRegistry
	notifications  *NotificationRegistry
	profiles       map[string]PermissionProfile
	resourcesStale atomic.Bool
	toolsStale     atomic.Bool
}

// serverConn holds per-server connection state.
type serverConn struct {
	transport     Transport
	pending       *PendingRegistry
	notifications *NotificationRegistry
	cancel        context.CancelFunc // stops the background reader goroutine
}

// NewClient creates an empty Client with no server connections.
func NewClient() *Client {
	c := &Client{
		conns:         make(map[string]*serverConn),
		tools:         NewToolRegistry(),
		resources:     NewResourceRegistry(),
		notifications: NewNotificationRegistry(),
		profiles:      make(map[string]PermissionProfile),
	}
	c.notifications.SetDefaultHandler(func(method string, params json.RawMessage) {
		log.Printf("mcp/client: unknown notification %q - discarding", method)
	})
	c.notifications.OnBuiltinNotification("notifications/resources/updated", func(method string, params json.RawMessage) {
		c.resourcesStale.Store(true)
	})
	c.notifications.OnBuiltinNotification("notifications/tools/list_changed", func(method string, params json.RawMessage) {
		c.toolsStale.Store(true)
	})
	return c
}

// SetPermissions stores the PermissionProfile for the named server.
// Any subsequent CallTool call for that server is checked against the profile
// before the request is sent over the wire. Calling SetPermissions with an
// empty profile (zero value) re-enables unrestricted access for the server.
func (c *Client) SetPermissions(serverName string, profile PermissionProfile) {
	c.mu.Lock()
	c.profiles[serverName] = profile
	c.mu.Unlock()
}

// LoadPermissions reads AllowedTools and DeniedTools from every ServerConfig
// in cfg and calls SetPermissions for each server that carries at least one
// permission rule. Servers with neither field set are left unrestricted.
//
// This method is intended to be called once after mcp.LoadConfig and before
// any tool calls, so that the config file's permission profiles are in effect
// for the lifetime of the Client.
func (c *Client) LoadPermissions(cfg *MCPConfig) {
	for _, srv := range cfg.Servers {
		if len(srv.AllowedTools) == 0 && len(srv.DeniedTools) == 0 {
			continue
		}
		c.SetPermissions(srv.Name, PermissionProfile{
			AllowedTools: srv.AllowedTools,
			DeniedTools:  srv.DeniedTools,
		})
	}
}

// Tools returns the ToolRegistry that holds all tools discovered via
// DiscoverTools. The registry is safe for concurrent use.
func (c *Client) Tools() *ToolRegistry {
	return c.tools
}

// Resources returns the ResourceRegistry populated by DiscoverResources.
// The registry is safe for concurrent use.
func (c *Client) Resources() *ResourceRegistry {
	return c.resources
}

// OnNotification registers a handler for server-to-client notifications.
func (c *Client) OnNotification(method string, handler NotificationHandler) {
	c.notifications.OnNotification(method, handler)
}

// ResourcesStale reports whether a resources/updated notification has arrived
// since the last successful DiscoverResources call.
func (c *Client) ResourcesStale() bool {
	return c.resourcesStale.Load()
}

// ToolsStale reports whether a tools/list_changed notification has arrived
// since the last successful DiscoverTools call.
func (c *Client) ToolsStale() bool {
	return c.toolsStale.Load()
}

// Connect attaches t as the transport for the server named name and starts
// a background goroutine that routes incoming responses to their callers.
// If a connection for name already exists it is replaced; the old transport
// is closed and its reader goroutine is stopped.
func (c *Client) Connect(name string, t Transport) {
	ctx, cancel := context.WithCancel(context.Background())
	conn := &serverConn{
		transport:     t,
		pending:       NewPendingRegistry(),
		notifications: c.notifications,
		cancel:        cancel,
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
		c.resources.RemoveServer(name)
		_ = old.transport.Close()
	}

	go conn.readLoop(ctx)
}

// ConnectAndInitialize attaches t as the transport for name, then performs the
// MCP initialize handshake for servers that require the initialized lifecycle.
func (c *Client) ConnectAndInitialize(ctx context.Context, name string, t Transport) (*InitializeResult, error) {
	c.Connect(name, t)
	result, err := c.Initialize(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("mcp/client: ConnectAndInitialize %q: %w", name, err)
	}
	return result, nil
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

		var envelope struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			// Malformed message — skip without terminating the loop.
			continue
		}

		if len(envelope.ID) == 0 {
			if envelope.Method == "" {
				log.Printf("mcp/client: notification without method - discarding")
				continue
			}
			conn.notifications.HandleNotification(envelope.Method, envelope.Params)
			continue
		}

		var resp Response
		if err := json.Unmarshal(raw, &resp); err != nil {
			// Malformed response — skip without terminating the loop.
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
	c.resources.RemoveServer(name)
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

// SendNotification sends an id-less JSON-RPC notification to serverName.
// No response is expected or registered in the pending request registry.
func (c *Client) SendNotification(ctx context.Context, serverName, method string, params any) error {
	c.mu.Lock()
	conn, ok := c.conns[serverName]
	c.mu.Unlock()

	if !ok {
		return fmt.Errorf("mcp/client: no connection for server %q", serverName)
	}

	var rawParams json.RawMessage
	if params != nil {
		var err error
		rawParams, err = json.Marshal(params)
		if err != nil {
			return fmt.Errorf("mcp/client: marshal notification params: %w", err)
		}
	}

	n := NewNotification(method, rawParams)
	raw, err := json.Marshal(n)
	if err != nil {
		return fmt.Errorf("mcp/client: marshal notification: %w", err)
	}
	if err := conn.transport.Send(ctx, raw); err != nil {
		return fmt.Errorf("mcp/client: send notification %s: %w", method, err)
	}
	return nil
}

// Initialize performs the MCP initialize handshake and then sends the
// notifications/initialized notification expected by MCP servers.
func (c *Client) Initialize(ctx context.Context, serverName string) (*InitializeResult, error) {
	params := InitializeParams{
		ProtocolVersion: "2024-11-05",
		ClientInfo:      ClientInfo{Name: "bolt-cowork", Version: "dev"},
	}
	raw, err := c.call(ctx, serverName, "initialize", params)
	if err != nil {
		return nil, fmt.Errorf("mcp/client: Initialize %q: %w", serverName, err)
	}

	var result InitializeResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("mcp/client: Initialize decode: %w", err)
	}
	if err := c.SendNotification(ctx, serverName, "notifications/initialized", nil); err != nil {
		return nil, fmt.Errorf("mcp/client: Initialize initialized notification: %w", err)
	}
	return &result, nil
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
	c.toolsStale.Store(false)
	return nil
}

// callToolParams is the request body for tools/call.
type callToolParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// CallTool invokes toolName on the named MCP server with args and returns
// the tool's result. A nil args map is sent as an empty object.
//
// If a PermissionProfile has been set for serverName via SetPermissions or
// LoadPermissions, the tool name is checked against that profile before any
// network I/O occurs. A denied or non-allowlisted tool returns an error
// immediately without sending a request over the wire.
func (c *Client) CallTool(ctx context.Context, serverName string, toolName string, args map[string]any) (*CallToolResult, error) {
	// Permission check: read profile under lock, then evaluate outside lock.
	c.mu.Lock()
	profile, hasProfile := c.profiles[serverName]
	c.mu.Unlock()

	if hasProfile {
		if ok, err := profile.IsAllowed(toolName); !ok {
			return nil, err
		}
	}

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

// listResourcesResult is the decoded result payload of a resources/list response.
type listResourcesResult struct {
	Resources []Resource `json:"resources"`
}

// DiscoverResources queries every connected server for its resource list and
// stores the results in the client's ResourceRegistry. It continues after
// individual server failures; any per-server error is collected and the
// combined error is returned after all servers have been queried.
func (c *Client) DiscoverResources(ctx context.Context) error {
	c.mu.Lock()
	names := make([]string, 0, len(c.conns))
	for name := range c.conns {
		names = append(names, name)
	}
	c.mu.Unlock()

	var errs []error
	for _, name := range names {
		raw, err := c.call(ctx, name, "resources/list", nil)
		if err != nil {
			log.Printf("mcp/client: DiscoverResources: server %q: %v", name, err)
			errs = append(errs, fmt.Errorf("%q: %w", name, err))
			continue
		}
		var result listResourcesResult
		if err := json.Unmarshal(raw, &result); err != nil {
			log.Printf("mcp/client: DiscoverResources: server %q decode: %v", name, err)
			errs = append(errs, fmt.Errorf("%q decode: %w", name, err))
			continue
		}
		c.resources.ReplaceServerResources(name, result.Resources)
	}

	if len(errs) > 0 {
		return fmt.Errorf("mcp/client: DiscoverResources: %w", errors.Join(errs...))
	}
	c.resourcesStale.Store(false)
	return nil
}

// readResourceParams is the request body for resources/read.
type readResourceParams struct {
	URI string `json:"uri"`
}

// readResourceResult is the decoded result payload of a resources/read response.
type readResourceResult struct {
	Contents []ResourceContents `json:"contents"`
}

// ReadResource calls resources/read on the named server and returns the
// resource contents for the given URI. An error is returned if the server
// is not connected, the RPC fails, or the response contains no contents.
func (c *Client) ReadResource(ctx context.Context, serverName, uri string) (*ResourceContents, error) {
	raw, err := c.call(ctx, serverName, "resources/read", readResourceParams{URI: uri})
	if err != nil {
		return nil, fmt.Errorf("mcp/client: ReadResource %q %q: %w", serverName, uri, err)
	}

	var result readResourceResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("mcp/client: ReadResource decode: %w", err)
	}
	if len(result.Contents) == 0 {
		return nil, fmt.Errorf("mcp/client: ReadResource %q %q: empty contents", serverName, uri)
	}
	contents := result.Contents[0]
	return &contents, nil
}
