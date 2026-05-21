package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockTransport is a controlled in-memory transport for unit tests.
// Send puts messages into sendCh; Receive reads from recvCh.
type mockTransport struct {
	sendCh    chan []byte
	recvCh    chan []byte
	closed    atomic.Bool
	closeOnce sync.Once
	done      chan struct{}
}

func newMockTransport() *mockTransport {
	return &mockTransport{
		sendCh: make(chan []byte, 16),
		recvCh: make(chan []byte, 16),
		done:   make(chan struct{}),
	}
}

func (m *mockTransport) Send(ctx context.Context, msg []byte) error {
	if m.closed.Load() {
		return ErrTransportClosed
	}
	cp := make([]byte, len(msg))
	copy(cp, msg)
	select {
	case m.sendCh <- cp:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-m.done:
		return ErrTransportClosed
	}
}

func (m *mockTransport) Receive(ctx context.Context) ([]byte, error) {
	if m.closed.Load() {
		return nil, ErrTransportClosed
	}
	select {
	case msg := <-m.recvCh:
		return msg, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-m.done:
		return nil, ErrTransportClosed
	}
}

func (m *mockTransport) Close() error {
	m.closeOnce.Do(func() {
		m.closed.Store(true)
		close(m.done)
	})
	return nil
}

// serveOnce reads one request from sendCh, calls handler, and puts the
// handler's response into recvCh. It runs synchronously so callers should
// invoke it in a goroutine when the client call happens concurrently.
func (m *mockTransport) serveOnce(t *testing.T, handler func(req Request) Response) {
	t.Helper()

	raw, ok := <-m.sendCh
	if !ok {
		t.Error("mockTransport: sendCh closed unexpectedly")
		return
	}

	var req Request
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Errorf("mockTransport: unmarshal request: %v", err)
		return
	}

	resp := handler(req)
	respRaw, err := json.Marshal(resp)
	if err != nil {
		t.Errorf("mockTransport: marshal response: %v", err)
		return
	}

	m.recvCh <- respRaw
}

// --- ListTools tests ---

func TestClient_ListTools(t *testing.T) {
	tests := []struct {
		name      string
		tools     []Tool
		wantNames []string
	}{
		{
			name:      "empty",
			tools:     []Tool{},
			wantNames: []string{},
		},
		{
			name: "single_tool",
			tools: []Tool{
				{Name: "read", Description: "Read a file", InputSchema: ToolSchema{Type: "object"}},
			},
			wantNames: []string{"read"},
		},
		{
			name: "multiple_tools",
			tools: []Tool{
				{Name: "read"},
				{Name: "write"},
				{Name: "delete"},
			},
			wantNames: []string{"read", "write", "delete"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mt := newMockTransport()
			c := NewClient()
			c.Connect("srv", mt)
			defer c.Close()

			go mt.serveOnce(t, func(req Request) Response {
				if req.Method != "tools/list" {
					t.Errorf("method = %q, want tools/list", req.Method)
				}
				result, _ := json.Marshal(listToolsResult{Tools: tt.tools})
				return NewSuccessResponse(req.ID, result)
			})

			got, err := c.ListTools(context.Background(), "srv")
			if err != nil {
				t.Fatalf("ListTools: %v", err)
			}
			if len(got) != len(tt.wantNames) {
				t.Fatalf("got %d tools, want %d", len(got), len(tt.wantNames))
			}
			for i, name := range tt.wantNames {
				if got[i].Name != name {
					t.Errorf("tools[%d].Name = %q, want %q", i, got[i].Name, name)
				}
			}
		})
	}
}

func TestClient_ListTools_UnknownServer(t *testing.T) {
	c := NewClient()
	defer c.Close()

	_, err := c.ListTools(context.Background(), "nope")
	if err == nil {
		t.Fatal("expected error for unknown server, got nil")
	}
}

func TestClient_ListTools_RPCError(t *testing.T) {
	mt := newMockTransport()
	c := NewClient()
	c.Connect("srv", mt)
	defer c.Close()

	go mt.serveOnce(t, func(req Request) Response {
		resp, _ := NewErrorResponse(req.ID, &RPCError{
			Code:    CodeMethodNotFound,
			Message: "method not found",
		})
		return resp
	})

	_, err := c.ListTools(context.Background(), "srv")
	if err == nil {
		t.Fatal("expected error from RPC error response, got nil")
	}
}

func TestClient_ListTools_ContextCancelled(t *testing.T) {
	mt := newMockTransport()
	c := NewClient()
	c.Connect("srv", mt)
	defer c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancelled

	_, err := c.ListTools(ctx, "srv")
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// --- CallTool tests ---

func TestClient_CallTool(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		args     map[string]any
		result   CallToolResult
		wantText string
		isError  bool
	}{
		{
			name:     "text_result",
			toolName: "read",
			args:     map[string]any{"path": "/foo.txt"},
			result:   CallToolResult{Content: []ToolResultContent{{Type: "text", Text: "file contents"}}},
			wantText: "file contents",
		},
		{
			name:     "tool_signals_error",
			toolName: "read",
			args:     map[string]any{"path": "/missing.txt"},
			result:   CallToolResult{IsError: true, Content: []ToolResultContent{{Type: "text", Text: "not found"}}},
			wantText: "not found",
			isError:  true,
		},
		{
			name:     "nil_args_becomes_empty_object",
			toolName: "noop",
			args:     nil,
			result:   CallToolResult{Content: []ToolResultContent{{Type: "text", Text: "ok"}}},
			wantText: "ok",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mt := newMockTransport()
			c := NewClient()
			c.Connect("srv", mt)
			defer c.Close()

			go mt.serveOnce(t, func(req Request) Response {
				if req.Method != "tools/call" {
					t.Errorf("method = %q, want tools/call", req.Method)
				}

				var params callToolParams
				if err := json.Unmarshal(req.Params, &params); err != nil {
					t.Errorf("unmarshal params: %v", err)
				}
				if params.Name != tt.toolName {
					t.Errorf("params.Name = %q, want %q", params.Name, tt.toolName)
				}

				result, _ := json.Marshal(tt.result)
				return NewSuccessResponse(req.ID, result)
			})

			got, err := c.CallTool(context.Background(), "srv", tt.toolName, tt.args)
			if err != nil {
				t.Fatalf("CallTool: %v", err)
			}
			if len(got.Content) == 0 {
				t.Fatal("expected at least one content item")
			}
			if got.Content[0].Text != tt.wantText {
				t.Errorf("Content[0].Text = %q, want %q", got.Content[0].Text, tt.wantText)
			}
			if got.IsError != tt.isError {
				t.Errorf("IsError = %v, want %v", got.IsError, tt.isError)
			}
		})
	}
}

func TestClient_CallTool_UnknownServer(t *testing.T) {
	c := NewClient()
	defer c.Close()

	_, err := c.CallTool(context.Background(), "nope", "tool", nil)
	if err == nil {
		t.Fatal("expected error for unknown server, got nil")
	}
}

func TestClient_CallTool_RPCError(t *testing.T) {
	mt := newMockTransport()
	c := NewClient()
	c.Connect("srv", mt)
	defer c.Close()

	go mt.serveOnce(t, func(req Request) Response {
		resp, _ := NewErrorResponse(req.ID, &RPCError{
			Code:    CodeInvalidParams,
			Message: "invalid parameters",
		})
		return resp
	})

	_, err := c.CallTool(context.Background(), "srv", "tool", nil)
	if err == nil {
		t.Fatal("expected error from RPC error response, got nil")
	}
}

func TestClient_CallTool_NilArgsRequestBody(t *testing.T) {
	mt := newMockTransport()
	c := NewClient()
	c.Connect("srv", mt)
	defer c.Close()

	var capturedParams callToolParams

	go mt.serveOnce(t, func(req Request) Response {
		_ = json.Unmarshal(req.Params, &capturedParams)
		result, _ := json.Marshal(CallToolResult{
			Content: []ToolResultContent{{Type: "text", Text: "ok"}},
		})
		return NewSuccessResponse(req.ID, result)
	})

	_, err := c.CallTool(context.Background(), "srv", "noop", nil)
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	// nil args should arrive as an empty (non-null) arguments object.
	if capturedParams.Arguments == nil {
		t.Error("Arguments was nil; expected empty map")
	}
}

func TestClient_SendNotification_NoID(t *testing.T) {
	mt := newMockTransport()
	c := NewClient()
	c.Connect("srv", mt)
	defer c.Close()

	if err := c.SendNotification(context.Background(), "srv", "notifications/initialized", nil); err != nil {
		t.Fatalf("SendNotification: %v", err)
	}

	select {
	case raw := <-mt.sendCh:
		var msg map[string]any
		if err := json.Unmarshal(raw, &msg); err != nil {
			t.Fatalf("unmarshal notification: %v", err)
		}
		if _, ok := msg["id"]; ok {
			t.Fatalf("notification contains id field: %s", raw)
		}
		if msg["method"] != "notifications/initialized" {
			t.Fatalf("method = %v, want notifications/initialized", msg["method"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for notification send")
	}
}

func TestClient_ReadLoop_DispatchesNotification(t *testing.T) {
	mt := newMockTransport()
	c := NewClient()
	c.Connect("srv", mt)
	defer c.Close()

	got := make(chan string, 1)
	c.OnNotification("notifications/custom", func(method string, params json.RawMessage) {
		got <- method + ":" + string(params)
	})

	raw, err := json.Marshal(NewNotification("notifications/custom", json.RawMessage(`{"x":1}`)))
	if err != nil {
		t.Fatalf("marshal notification: %v", err)
	}
	mt.recvCh <- raw

	select {
	case value := <-got:
		want := `notifications/custom:{"x":1}`
		if value != want {
			t.Fatalf("handler value = %q, want %q", value, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for notification handler")
	}
}

func TestClient_StaleFlags(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		checkStale func(*Client) bool
		discover   func(context.Context, *Client) error
		serve      func(*mockTransport, *testing.T)
	}{
		{
			name:       "tools list changed",
			method:     "notifications/tools/list_changed",
			checkStale: (*Client).ToolsStale,
			discover: func(ctx context.Context, c *Client) error {
				return c.DiscoverTools(ctx)
			},
			serve: func(mt *mockTransport, t *testing.T) {
				t.Helper()
				mt.serveOnce(t, func(req Request) Response {
					return NewSuccessResponse(req.ID, toolsListResponse(t, []Tool{{Name: "read"}}))
				})
			},
		},
		{
			name:       "resources updated",
			method:     "notifications/resources/updated",
			checkStale: (*Client).ResourcesStale,
			discover: func(ctx context.Context, c *Client) error {
				return c.DiscoverResources(ctx)
			},
			serve: func(mt *mockTransport, t *testing.T) {
				t.Helper()
				mt.serveOnce(t, func(req Request) Response {
					result, _ := json.Marshal(listResourcesResult{Resources: []Resource{{URI: "file://a", Name: "A"}}})
					return NewSuccessResponse(req.ID, result)
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mt := newMockTransport()
			c := NewClient()
			c.Connect("srv", mt)
			defer c.Close()

			raw, err := json.Marshal(NewNotification(tt.method, nil))
			if err != nil {
				t.Fatalf("marshal notification: %v", err)
			}
			mt.recvCh <- raw

			deadline := time.After(2 * time.Second)
			for !tt.checkStale(c) {
				select {
				case <-deadline:
					t.Fatal("stale flag was not set")
				default:
					time.Sleep(10 * time.Millisecond)
				}
			}

			go tt.serve(mt, t)
			if err := tt.discover(context.Background(), c); err != nil {
				t.Fatalf("discover: %v", err)
			}
			if tt.checkStale(c) {
				t.Fatal("stale flag was not reset after successful discovery")
			}
		})
	}
}

func TestClient_BuiltinNotificationRunsWithUserHandler(t *testing.T) {
	mt := newMockTransport()
	c := NewClient()
	c.Connect("srv", mt)
	defer c.Close()

	gotUserHandler := make(chan struct{}, 1)
	c.OnNotification("notifications/resources/updated", func(method string, params json.RawMessage) {
		gotUserHandler <- struct{}{}
	})

	raw, err := json.Marshal(NewNotification("notifications/resources/updated", nil))
	if err != nil {
		t.Fatalf("marshal notification: %v", err)
	}
	mt.recvCh <- raw

	deadline := time.After(2 * time.Second)
	for !c.ResourcesStale() {
		select {
		case <-deadline:
			t.Fatal("builtin stale handler was not called")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	select {
	case <-gotUserHandler:
	case <-time.After(2 * time.Second):
		t.Fatal("user notification handler was not called")
	}
}

// --- Connection lifecycle tests ---

func TestClient_Connect_ReplacesOldConnection(t *testing.T) {
	c := NewClient()

	first := newMockTransport()
	c.Connect("srv", first)

	second := newMockTransport()
	c.Connect("srv", second) // should close first

	// first transport should be closed after being replaced.
	if !first.closed.Load() {
		t.Error("first transport was not closed after replacement")
	}

	c.Close()
}

func TestClient_Disconnect_Unknown(t *testing.T) {
	c := NewClient()
	if err := c.Disconnect("nope"); err != nil {
		t.Errorf("Disconnect on unknown server returned error: %v", err)
	}
}

func TestClient_Close_Empty(t *testing.T) {
	c := NewClient()
	if err := c.Close(); err != nil {
		t.Errorf("Close on empty client returned error: %v", err)
	}
}

// --- Blocking-bug regression tests ---

// TestClient_ListTools_UnblocksOnDisconnect verifies that an in-flight
// ListTools call made with context.Background() returns ErrConnectionClosed
// when Disconnect is called before the server replies.
func TestClient_ListTools_UnblocksOnDisconnect(t *testing.T) {
	mt := newMockTransport()
	c := NewClient()
	c.Connect("srv", mt)

	errCh := make(chan error, 1)
	go func() {
		_, err := c.ListTools(context.Background(), "srv")
		errCh <- err
	}()

	// Wait until the request has been sent (sitting in sendCh), confirming
	// the goroutine is now blocked inside the select waiting for a response.
	select {
	case <-mt.sendCh:
		// request received — now disconnect
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: ListTools did not send a request")
	}

	if err := c.Disconnect("srv"); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error after Disconnect, got nil")
		}
		if !errors.Is(err, ErrConnectionClosed) {
			t.Errorf("error = %v, want to wrap ErrConnectionClosed", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: ListTools did not unblock after Disconnect")
	}
}

// TestClient_ListTools_UnblocksOnTransportClose verifies that an in-flight
// ListTools call returns ErrConnectionClosed when the underlying transport
// closes spontaneously (e.g. the server process exits).
func TestClient_ListTools_UnblocksOnTransportClose(t *testing.T) {
	mt := newMockTransport()
	c := NewClient()
	c.Connect("srv", mt)
	defer c.Close()

	errCh := make(chan error, 1)
	go func() {
		_, err := c.ListTools(context.Background(), "srv")
		errCh <- err
	}()

	// Wait until the request has been sent.
	select {
	case <-mt.sendCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: ListTools did not send a request")
	}

	// Close the transport directly — simulates a server crash.
	if err := mt.Close(); err != nil {
		t.Fatalf("transport Close: %v", err)
	}

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error after transport close, got nil")
		}
		if !errors.Is(err, ErrConnectionClosed) {
			t.Errorf("error = %v, want to wrap ErrConnectionClosed", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: ListTools did not unblock after transport close")
	}
}

// TestClient_ListTools_UnblocksOnReplace verifies that replacing an existing
// connection via Connect unblocks callers on the old connection.
func TestClient_ListTools_UnblocksOnReplace(t *testing.T) {
	mt := newMockTransport()
	c := NewClient()
	c.Connect("srv", mt)
	defer c.Close()

	errCh := make(chan error, 1)
	go func() {
		_, err := c.ListTools(context.Background(), "srv")
		errCh <- err
	}()

	select {
	case <-mt.sendCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: ListTools did not send a request")
	}

	// Replace the connection — old callers must unblock.
	c.Connect("srv", newMockTransport())

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error after connection replacement, got nil")
		}
		if !errors.Is(err, ErrConnectionClosed) {
			t.Errorf("error = %v, want to wrap ErrConnectionClosed", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: ListTools did not unblock after connection replacement")
	}
}

// TestClient_ListTools_ClosedRegistryBeforeRegister covers the race where a
// caller has selected a connection, but that connection's pending registry is
// closed before the request can be registered.
func TestClient_ListTools_ClosedRegistryBeforeRegister(t *testing.T) {
	mt := newMockTransport()
	c := NewClient()
	c.Connect("srv", mt)
	defer c.Close()

	c.mu.Lock()
	conn := c.conns["srv"]
	c.mu.Unlock()
	conn.pending.CloseAll(ErrConnectionClosed)

	_, err := c.ListTools(context.Background(), "srv")
	if err == nil {
		t.Fatal("expected error from closed pending registry, got nil")
	}
	if !errors.Is(err, ErrConnectionClosed) {
		t.Errorf("error = %v, want to wrap ErrConnectionClosed", err)
	}
	select {
	case raw := <-mt.sendCh:
		t.Fatalf("request was sent despite closed pending registry: %s", raw)
	default:
	}
}

// TestClient_ConcurrentListTools fires multiple ListTools calls in parallel
// to verify the pending registry and ID generator handle concurrency correctly.
func TestClient_ConcurrentListTools(t *testing.T) {
	mt := newMockTransport()
	c := NewClient()
	c.Connect("srv", mt)
	defer c.Close()

	const n = 5

	// Start a server goroutine that handles all n requests.
	go func() {
		for i := 0; i < n; i++ {
			mt.serveOnce(t, func(req Request) Response {
				result, _ := json.Marshal(listToolsResult{
					Tools: []Tool{{Name: "tool"}},
				})
				return NewSuccessResponse(req.ID, result)
			})
		}
	}()

	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			_, err := c.ListTools(context.Background(), "srv")
			errs <- err
		}()
	}

	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Errorf("concurrent ListTools [%d]: %v", i, err)
		}
	}
}

// --- DiscoverTools tests ---

// toolsListResponse builds the JSON payload for a successful tools/list response.
func toolsListResponse(t *testing.T, tools []Tool) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(listToolsResult{Tools: tools})
	if err != nil {
		t.Fatalf("toolsListResponse: marshal: %v", err)
	}
	return raw
}

func TestClient_DiscoverTools_SingleServer(t *testing.T) {
	mt := newMockTransport()
	c := NewClient()
	c.Connect("srv", mt)
	defer c.Close()

	wantTools := []Tool{
		{Name: "read", Description: "Read a file"},
		{Name: "write", Description: "Write a file"},
	}

	go mt.serveOnce(t, func(req Request) Response {
		return NewSuccessResponse(req.ID, toolsListResponse(t, wantTools))
	})

	if err := c.DiscoverTools(context.Background()); err != nil {
		t.Fatalf("DiscoverTools: %v", err)
	}

	all := c.Tools().ListAll()
	got, ok := all["srv"]
	if !ok {
		t.Fatalf("ListAll: missing server %q", "srv")
	}
	if len(got) != len(wantTools) {
		t.Fatalf("ListAll[srv]: got %d tools, want %d", len(got), len(wantTools))
	}

	byName := make(map[string]Tool, len(got))
	for _, tool := range got {
		byName[tool.Name] = tool
	}
	for _, want := range wantTools {
		if got, ok := byName[want.Name]; !ok {
			t.Errorf("tool %q not found in registry", want.Name)
		} else if got.Description != want.Description {
			t.Errorf("tool %q: Description = %q, want %q", want.Name, got.Description, want.Description)
		}
	}
}

func TestClient_DiscoverTools_MultiServer(t *testing.T) {
	mt1 := newMockTransport()
	mt2 := newMockTransport()
	c := NewClient()
	c.Connect("fs", mt1)
	c.Connect("web", mt2)
	defer c.Close()

	fsTools := []Tool{{Name: "read"}, {Name: "write"}}
	webTools := []Tool{{Name: "fetch"}}

	// Both servers respond concurrently.
	go mt1.serveOnce(t, func(req Request) Response {
		return NewSuccessResponse(req.ID, toolsListResponse(t, fsTools))
	})
	go mt2.serveOnce(t, func(req Request) Response {
		return NewSuccessResponse(req.ID, toolsListResponse(t, webTools))
	})

	if err := c.DiscoverTools(context.Background()); err != nil {
		t.Fatalf("DiscoverTools: %v", err)
	}

	all := c.Tools().ListAll()
	if len(all["fs"]) != 2 {
		t.Errorf("registry[fs]: got %d tools, want 2", len(all["fs"]))
	}
	if len(all["web"]) != 1 {
		t.Errorf("registry[web]: got %d tools, want 1", len(all["web"]))
	}

	if _, _, ok := c.Tools().GetTool("read"); !ok {
		t.Error("GetTool(read): not found after DiscoverTools")
	}
	if _, _, ok := c.Tools().GetTool("fetch"); !ok {
		t.Error("GetTool(fetch): not found after DiscoverTools")
	}
}

func TestClient_DiscoverTools_PartialFailure(t *testing.T) {
	mt1 := newMockTransport()
	mt2 := newMockTransport()
	c := NewClient()
	c.Connect("good", mt1)
	c.Connect("bad", mt2)
	defer c.Close()

	goodTools := []Tool{{Name: "read"}}

	// good server responds successfully; bad server returns an RPC error.
	go mt1.serveOnce(t, func(req Request) Response {
		return NewSuccessResponse(req.ID, toolsListResponse(t, goodTools))
	})
	go mt2.serveOnce(t, func(req Request) Response {
		resp, _ := NewErrorResponse(req.ID, &RPCError{
			Code:    CodeInternalError,
			Message: "server unavailable",
		})
		return resp
	})

	err := c.DiscoverTools(context.Background())
	if err == nil {
		t.Fatal("DiscoverTools: expected error for partial failure, got nil")
	}

	// The good server's tools must be in the registry.
	if _, _, ok := c.Tools().GetTool("read"); !ok {
		t.Error("GetTool(read): not found despite good server succeeding")
	}
	// The bad server contributed no tools.
	if all := c.Tools().ListAll(); len(all["bad"]) != 0 {
		t.Errorf("registry[bad]: got %d tools, want 0", len(all["bad"]))
	}
}

func TestClient_DiscoverTools_RefreshRemovesStaleTools(t *testing.T) {
	mt := newMockTransport()
	c := NewClient()
	c.Connect("srv", mt)
	defer c.Close()

	go mt.serveOnce(t, func(req Request) Response {
		return NewSuccessResponse(req.ID, toolsListResponse(t, []Tool{{Name: "read"}, {Name: "write"}}))
	})
	if err := c.DiscoverTools(context.Background()); err != nil {
		t.Fatalf("first DiscoverTools: %v", err)
	}
	if _, _, ok := c.Tools().GetTool("write"); !ok {
		t.Fatal("GetTool(write): not found after first DiscoverTools")
	}

	go mt.serveOnce(t, func(req Request) Response {
		return NewSuccessResponse(req.ID, toolsListResponse(t, []Tool{{Name: "read"}}))
	})
	if err := c.DiscoverTools(context.Background()); err != nil {
		t.Fatalf("second DiscoverTools: %v", err)
	}

	if _, _, ok := c.Tools().GetTool("write"); ok {
		t.Error("stale write tool remained after refresh")
	}
	if _, srv, ok := c.Tools().GetTool("read"); !ok || srv != "srv" {
		t.Errorf("GetTool(read) = (_, %q, %v), want (_, srv, true)", srv, ok)
	}
	if all := c.Tools().ListAll(); len(all["srv"]) != 1 {
		t.Errorf("registry[srv]: got %d tools after refresh, want 1", len(all["srv"]))
	}
}

func TestClient_Disconnect_RemovesTools(t *testing.T) {
	mt := newMockTransport()
	c := NewClient()
	c.Connect("srv", mt)

	// Discover tools first.
	go mt.serveOnce(t, func(req Request) Response {
		return NewSuccessResponse(req.ID, toolsListResponse(t, []Tool{{Name: "read"}}))
	})
	if err := c.DiscoverTools(context.Background()); err != nil {
		t.Fatalf("DiscoverTools: %v", err)
	}
	if _, _, ok := c.Tools().GetTool("read"); !ok {
		t.Fatal("GetTool(read): not found before Disconnect")
	}

	// Disconnect must remove the server's tools from the registry.
	if err := c.Disconnect("srv"); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}

	if _, _, ok := c.Tools().GetTool("read"); ok {
		t.Error("GetTool(read): still present after Disconnect")
	}
	if all := c.Tools().ListAll(); len(all["srv"]) != 0 {
		t.Errorf("registry[srv]: got %d tools after Disconnect, want 0", len(all["srv"]))
	}
}

func TestClient_Connect_Replace_ClearsTools(t *testing.T) {
	mt := newMockTransport()
	c := NewClient()
	c.Connect("srv", mt)
	defer c.Close()

	go mt.serveOnce(t, func(req Request) Response {
		return NewSuccessResponse(req.ID, toolsListResponse(t, []Tool{{Name: "read"}}))
	})
	if err := c.DiscoverTools(context.Background()); err != nil {
		t.Fatalf("DiscoverTools: %v", err)
	}
	if _, _, ok := c.Tools().GetTool("read"); !ok {
		t.Fatal("GetTool(read): not found before connection replacement")
	}

	c.Connect("srv", newMockTransport())

	if _, _, ok := c.Tools().GetTool("read"); ok {
		t.Error("stale read tool remained after connection replacement")
	}
	if all := c.Tools().ListAll(); len(all["srv"]) != 0 {
		t.Errorf("registry[srv]: got %d tools after connection replacement, want 0", len(all["srv"]))
	}
}
