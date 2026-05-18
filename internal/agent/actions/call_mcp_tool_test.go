package actions_test

import (
	"context"
	"errors"
	"testing"

	"github.com/halukerenozlu/bolt-cowork/internal/agent/actions"
	"github.com/halukerenozlu/bolt-cowork/internal/mcp"
)

// mockCaller is a test double for actions.MCPCaller.
type mockCaller struct {
	result *mcp.CallToolResult
	err    error
}

func (m *mockCaller) CallTool(_ context.Context, _, _ string, _ map[string]any) (*mcp.CallToolResult, error) {
	return m.result, m.err
}

func TestCallMCPToolAction_Type(t *testing.T) {
	a := &actions.CallMCPToolAction{ServerName: "srv", ToolName: "tool"}
	if got := a.Type(); got != "call_mcp_tool" {
		t.Errorf("Type() = %q, want %q", got, "call_mcp_tool")
	}
}

func TestCallMCPToolAction_IsDangerous(t *testing.T) {
	a := &actions.CallMCPToolAction{}
	if !a.IsDangerous() {
		t.Error("IsDangerous() = false, want true")
	}
}

func TestCallMCPToolAction_Summary(t *testing.T) {
	a := &actions.CallMCPToolAction{ServerName: "my-server", ToolName: "my-tool"}
	want := "MCP tool çağrısı: my-server/my-tool"
	if got := a.Summary(); got != want {
		t.Errorf("Summary() = %q, want %q", got, want)
	}
}

func TestCallMCPToolAction_Execute_Success(t *testing.T) {
	caller := &mockCaller{
		result: &mcp.CallToolResult{
			Content: []mcp.ToolResultContent{
				{Type: "text", Text: "hello"},
				{Type: "text", Text: " world"},
			},
			IsError: false,
		},
	}
	a := &actions.CallMCPToolAction{
		ServerName: "srv",
		ToolName:   "greet",
		Args:       map[string]any{"name": "bolt"},
	}

	res, err := a.Execute(context.Background(), caller)
	if err != nil {
		t.Fatalf("Execute() unexpected Go error: %v", err)
	}
	if res.Error != "" {
		t.Errorf("Execute() Error = %q, want empty", res.Error)
	}
	if want := "hello world"; res.Output != want {
		t.Errorf("Execute() Output = %q, want %q", res.Output, want)
	}
}

func TestCallMCPToolAction_Execute_ToolError(t *testing.T) {
	// IsError=true: the tool itself signals failure via content, not a Go error.
	caller := &mockCaller{
		result: &mcp.CallToolResult{
			Content: []mcp.ToolResultContent{
				{Type: "text", Text: "permission denied"},
			},
			IsError: true,
		},
	}
	a := &actions.CallMCPToolAction{ServerName: "srv", ToolName: "restricted"}

	res, err := a.Execute(context.Background(), caller)
	if err != nil {
		t.Fatalf("Execute() unexpected Go error: %v", err)
	}
	if res.Output != "" {
		t.Errorf("Execute() Output = %q, want empty on tool error", res.Output)
	}
	if res.Error != "permission denied" {
		t.Errorf("Execute() Error = %q, want %q", res.Error, "permission denied")
	}
}

func TestCallMCPToolAction_Execute_Error(t *testing.T) {
	caller := &mockCaller{err: errors.New("connection refused")}
	a := &actions.CallMCPToolAction{ServerName: "srv", ToolName: "fail"}

	res, err := a.Execute(context.Background(), caller)
	if err != nil {
		t.Fatalf("Execute() returned unexpected Go error: %v", err)
	}
	if res.Error == "" {
		t.Error("Execute() Error is empty, want non-empty")
	}
	if res.Output != "" {
		t.Errorf("Execute() Output = %q, want empty on error", res.Output)
	}
}

func TestCallMCPToolAction_Execute_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately before the call

	// A real MCPCaller propagates context cancellation as an error.
	caller := &mockCaller{err: ctx.Err()}
	a := &actions.CallMCPToolAction{ServerName: "srv", ToolName: "tool"}

	res, err := a.Execute(ctx, caller)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want context.Canceled", err)
	}
	if res != (actions.ActionResult{}) {
		t.Errorf("Execute() result = %+v, want zero value", res)
	}
}
