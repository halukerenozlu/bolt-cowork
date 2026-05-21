// Package testutil provides test helpers for the mcp package.
// This package is intended to be imported only from _test.go files.
package testutil

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/halukerenozlu/bolt-cowork/internal/mcp"
)

// FakeResource defines a resource for the mock server's resources/* handlers.
// A local type is used here because mcp.MCPResource carries registry fields
// (e.g. ServerName) that are not part of the wire protocol.
type FakeResource struct {
	URI         string
	Name        string
	Description string
	MimeType    string
	Content     string // text body returned by resources/read
}

// ToolHandler processes a tools/call invocation and returns a result.
type ToolHandler func(args map[string]any) (*mcp.CallToolResult, error)

// MockServerConfig configures an in-process mock MCP server.
type MockServerConfig struct {
	// Tools is the list of tools advertised by tools/list.
	Tools []mcp.Tool

	// Resources is the list of resources advertised by resources/list.
	// Included as Phase 2 preparation; not tested by Phase 1 e2e tests.
	Resources []FakeResource

	// Handlers maps tool names to custom call handlers.
	// If no handler is registered for a tool, the default echo handler is used,
	// which returns {"tool":"<name>","args":<args>} as a text content item.
	Handlers map[string]ToolHandler
}

// MockServer is an in-process mock MCP server that communicates via Go
// channels rather than stdio. It is designed for unit tests that need a
// realistic MCP server without subprocess overhead or Content-Length framing.
//
// Use NewMockServer to create and start the server. Pass the value returned
// by ClientTransport to mcp.Client.Connect.
type MockServer struct {
	cfg        MockServerConfig
	callCount  sync.Map // map[string]*int64
	toServer   chan []byte
	fromServer chan []byte
	done       chan struct{}
	closeOnce  sync.Once
	closed     atomic.Bool
}

// NewMockServer creates a MockServer with cfg and immediately starts its
// background request-handling goroutine.
func NewMockServer(cfg MockServerConfig) *MockServer {
	s := &MockServer{
		cfg:        cfg,
		toServer:   make(chan []byte, 32),
		fromServer: make(chan []byte, 32),
		done:       make(chan struct{}),
	}
	go s.serve()
	return s
}

// ClientTransport returns a mcp.Transport that is connected to this mock server.
// The returned transport uses buffered channels and does not perform
// Content-Length framing. Each call creates an independent transport instance.
func (s *MockServer) ClientTransport() mcp.Transport {
	return &mockServerTransport{
		server: s,
		done:   make(chan struct{}),
	}
}

// CallCount returns how many times toolName was dispatched to a handler
// (including the default echo handler).
func (s *MockServer) CallCount(toolName string) int {
	v, ok := s.callCount.Load(toolName)
	if !ok {
		return 0
	}
	return int(atomic.LoadInt64(v.(*int64)))
}

// Close shuts down the mock server. It is safe to call more than once.
func (s *MockServer) Close() {
	s.closeOnce.Do(func() {
		s.closed.Store(true)
		close(s.done)
	})
}

// serve is the background goroutine that reads requests from toServer and
// dispatches responses back through fromServer.
func (s *MockServer) serve() {
	for {
		select {
		case <-s.done:
			return
		case raw, ok := <-s.toServer:
			if !ok {
				return
			}
			s.dispatch(raw)
		}
	}
}

// dispatch parses raw as a JSON-RPC message and writes the appropriate
// response to fromServer. Notifications (null or absent id) are silently
// dropped per the JSON-RPC spec.
func (s *MockServer) dispatch(raw []byte) {
	var req mcp.Request
	if err := json.Unmarshal(raw, &req); err != nil {
		return // ignore malformed
	}

	// Notifications have no id — no response should be sent.
	if req.ID.IsNull() {
		return
	}

	var result json.RawMessage
	var rpcErr *mcp.RPCError

	switch req.Method {
	case "initialize":
		result, rpcErr = s.handleInitialize()
	case "tools/list":
		result, rpcErr = s.handleToolsList()
	case "tools/call":
		result, rpcErr = s.handleToolsCall(req.Params)
	case "resources/list":
		result, rpcErr = s.handleResourcesList()
	case "resources/read":
		result, rpcErr = s.handleResourcesRead(req.Params)
	default:
		rpcErr = &mcp.RPCError{
			Code:    mcp.CodeMethodNotFound,
			Message: fmt.Sprintf("method not found: %s", req.Method),
		}
	}

	var resp mcp.Response
	if rpcErr != nil {
		resp, _ = mcp.NewErrorResponse(req.ID, rpcErr)
	} else {
		resp = mcp.NewSuccessResponse(req.ID, result)
	}

	data, err := json.Marshal(resp)
	if err != nil {
		return
	}

	select {
	case s.fromServer <- data:
	case <-s.done:
	}
}

func (s *MockServer) handleInitialize() (json.RawMessage, *mcp.RPCError) {
	caps := mcp.ServerCapabilities{
		Tools: &mcp.ToolsCapability{},
	}
	if len(s.cfg.Resources) > 0 {
		caps.Resources = &mcp.ResourcesCapability{}
	}
	result := mcp.InitializeResult{
		ProtocolVersion: "2024-11-05",
		ServerInfo:      mcp.ServerInfo{Name: "mock-server", Version: "test"},
		Capabilities:    caps,
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, &mcp.RPCError{Code: mcp.CodeInternalError, Message: err.Error()}
	}
	return raw, nil
}

func (s *MockServer) handleToolsList() (json.RawMessage, *mcp.RPCError) {
	type toolsListResult struct {
		Tools []mcp.Tool `json:"tools"`
	}
	raw, err := json.Marshal(toolsListResult{Tools: s.cfg.Tools})
	if err != nil {
		return nil, &mcp.RPCError{Code: mcp.CodeInternalError, Message: err.Error()}
	}
	return raw, nil
}

func (s *MockServer) handleToolsCall(params json.RawMessage) (json.RawMessage, *mcp.RPCError) {
	var p struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &mcp.RPCError{Code: mcp.CodeInvalidParams, Message: err.Error()}
	}

	// Increment call count atomically.
	v, _ := s.callCount.LoadOrStore(p.Name, new(int64))
	atomic.AddInt64(v.(*int64), 1)

	// Use a registered handler if available; otherwise fall back to echo.
	var toolResult *mcp.CallToolResult
	if s.cfg.Handlers != nil {
		if h, ok := s.cfg.Handlers[p.Name]; ok {
			var err error
			toolResult, err = h(p.Arguments)
			if err != nil {
				return nil, &mcp.RPCError{Code: mcp.CodeInternalError, Message: err.Error()}
			}
		}
	}
	if toolResult == nil {
		argsJSON, _ := json.Marshal(p.Arguments)
		text := fmt.Sprintf(`{"tool":%q,"args":%s}`, p.Name, argsJSON)
		toolResult = &mcp.CallToolResult{
			Content: []mcp.ToolResultContent{{Type: "text", Text: text}},
		}
	}

	raw, err := json.Marshal(toolResult)
	if err != nil {
		return nil, &mcp.RPCError{Code: mcp.CodeInternalError, Message: err.Error()}
	}
	return raw, nil
}

func (s *MockServer) handleResourcesList() (json.RawMessage, *mcp.RPCError) {
	type wireResource struct {
		URI         string `json:"uri"`
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
		MimeType    string `json:"mimeType,omitempty"`
	}
	type resourcesListResult struct {
		Resources []wireResource `json:"resources"`
	}
	list := make([]wireResource, len(s.cfg.Resources))
	for i, r := range s.cfg.Resources {
		list[i] = wireResource{
			URI:         r.URI,
			Name:        r.Name,
			Description: r.Description,
			MimeType:    r.MimeType,
		}
	}
	raw, err := json.Marshal(resourcesListResult{Resources: list})
	if err != nil {
		return nil, &mcp.RPCError{Code: mcp.CodeInternalError, Message: err.Error()}
	}
	return raw, nil
}

func (s *MockServer) handleResourcesRead(params json.RawMessage) (json.RawMessage, *mcp.RPCError) {
	var p struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &mcp.RPCError{Code: mcp.CodeInvalidParams, Message: err.Error()}
	}
	for _, r := range s.cfg.Resources {
		if r.URI != p.URI {
			continue
		}
		type contentItem struct {
			URI      string `json:"uri"`
			MimeType string `json:"mimeType,omitempty"`
			Text     string `json:"text"`
		}
		type resourceReadResult struct {
			Contents []contentItem `json:"contents"`
		}
		raw, err := json.Marshal(resourceReadResult{
			Contents: []contentItem{{URI: r.URI, MimeType: r.MimeType, Text: r.Content}},
		})
		if err != nil {
			return nil, &mcp.RPCError{Code: mcp.CodeInternalError, Message: err.Error()}
		}
		return raw, nil
	}
	return nil, &mcp.RPCError{
		Code:    mcp.CodeInvalidParams,
		Message: fmt.Sprintf("resource not found: %s", p.URI),
	}
}

// mockServerTransport implements mcp.Transport using the server's channels.
// Content-Length framing is intentionally absent: this transport bypasses
// the stdio layer entirely, communicating via buffered Go channels.
type mockServerTransport struct {
	server    *MockServer
	done      chan struct{}
	closeOnce sync.Once
	closed    atomic.Bool
}

// compile-time interface check
var _ mcp.Transport = (*mockServerTransport)(nil)

func (t *mockServerTransport) Send(ctx context.Context, msg []byte) error {
	if t.closed.Load() {
		return mcp.ErrTransportClosed
	}
	cp := make([]byte, len(msg))
	copy(cp, msg)
	select {
	case t.server.toServer <- cp:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-t.done:
		return mcp.ErrTransportClosed
	case <-t.server.done:
		return mcp.ErrTransportClosed
	}
}

func (t *mockServerTransport) Receive(ctx context.Context) ([]byte, error) {
	if t.closed.Load() {
		return nil, mcp.ErrTransportClosed
	}
	select {
	case msg, ok := <-t.server.fromServer:
		if !ok {
			return nil, mcp.ErrTransportClosed
		}
		return msg, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-t.done:
		return nil, mcp.ErrTransportClosed
	case <-t.server.done:
		return nil, mcp.ErrTransportClosed
	}
}

func (t *mockServerTransport) Close() error {
	t.closeOnce.Do(func() {
		t.closed.Store(true)
		close(t.done)
	})
	return nil
}
