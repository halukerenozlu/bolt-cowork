package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/halukerenozlu/bolt-cowork/internal/mcp"
	"github.com/halukerenozlu/bolt-cowork/internal/provider"
	"github.com/halukerenozlu/bolt-cowork/internal/sandbox"
)

// mcpStep returns a call_mcp_tool Step for use in approval tests.
func mcpStep(serverName, toolName string, args map[string]any) Step {
	return Step{
		Action:      ActionCallMCPTool,
		Description: "call MCP tool",
		ServerName:  serverName,
		ToolName:    toolName,
		Args:        args,
	}
}

// setupMCPAgent creates an Agent with a single call_mcp_tool step in the plan,
// a pre-configured MCPCaller, and the given approver/mode.
func setupMCPAgent(
	t *testing.T,
	step Step,
	caller *mockMCPCaller,
	approver Approver,
	mode ApprovalMode,
) *Agent {
	t.Helper()
	dir := t.TempDir()
	sb, err := sandbox.New(dir)
	if err != nil {
		t.Fatalf("sandbox.New: %v", err)
	}
	planJSON := makePlanJSON([]Step{step})
	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: planJSON},
	})
	ag := New(chain, sb, approver, mode, nil, nil)
	ag.SetMCPCaller(caller)
	registry := mcp.NewToolRegistry()
	registry.AddTools(step.ServerName, []mcp.Tool{{Name: step.ToolName}})
	ag.SetMCPToolRegistry(registry)
	return ag
}

// defaultCaller returns a mockMCPCaller that yields a single text result.
func defaultCaller(text string) *mockMCPCaller {
	return &mockMCPCaller{
		result: &mcp.CallToolResult{
			Content: []mcp.ToolResultContent{{Type: "text", Text: text}},
		},
	}
}

// --- mcpApprovalItems unit tests ---

func TestMCPApprovalItems_Basic(t *testing.T) {
	items := mcpApprovalItems("my-srv", "my-tool", map[string]any{"k": "v"})
	if len(items) != 3 {
		t.Fatalf("len(items) = %d, want 3", len(items))
	}
	if !strings.Contains(items[0], "my-srv") {
		t.Errorf("items[0] = %q, want server name", items[0])
	}
	if !strings.Contains(items[1], "my-tool") {
		t.Errorf("items[1] = %q, want tool name", items[1])
	}
	if !strings.Contains(items[2], "v") {
		t.Errorf("items[2] = %q, want arg value", items[2])
	}
}

func TestMCPApprovalItems_NilArgs(t *testing.T) {
	items := mcpApprovalItems("srv", "tool", nil)
	if len(items) != 3 {
		t.Fatalf("len(items) = %d, want 3", len(items))
	}
	if !strings.Contains(items[2], "{}") {
		t.Errorf("items[2] = %q, want empty JSON object", items[2])
	}
}

// --- 4-mode approval gate tests ---

// TestApproval_MCP_FullMode verifies that full mode requests approval for
// plan, execute, and result, and that the execute approval contains
// server/tool/args details.
func TestApproval_MCP_FullMode(t *testing.T) {
	step := mcpStep("srv", "greet", map[string]any{"name": "bolt"})
	approver := &sequenceApprover{
		decisions: []Decision{Approve, Approve, Approve}, // plan, execute, result
	}
	ag := setupMCPAgent(t, step, defaultCaller("ok"), approver, ApprovalFull)

	result, err := ag.Run(context.Background(), "call greet")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Success {
		t.Error("Result.Success = false, want true")
	}

	// full mode: plan(0) + execute(1) + result(2) = 3 total
	if len(approver.calls) != 3 {
		t.Fatalf("approval calls = %d, want 3", len(approver.calls))
	}

	execCall := approver.calls[1]
	if execCall.Stage != "execute" {
		t.Errorf("calls[1].Stage = %q, want execute", execCall.Stage)
	}
	if execCall.Description != "MCP Tool Çağrısı" {
		t.Errorf("calls[1].Description = %q, want MCP Tool Çağrısı", execCall.Description)
	}
	if !execCall.Dangerous {
		t.Error("calls[1].Dangerous = false, want true")
	}
	if len(execCall.Items) < 3 {
		t.Fatalf("calls[1].Items count = %d, want >= 3", len(execCall.Items))
	}
	if !strings.Contains(execCall.Items[0], "srv") {
		t.Errorf("Items[0] = %q, want server name", execCall.Items[0])
	}
	if !strings.Contains(execCall.Items[1], "greet") {
		t.Errorf("Items[1] = %q, want tool name", execCall.Items[1])
	}
	if !strings.Contains(execCall.Items[2], "bolt") {
		t.Errorf("Items[2] = %q, want arg value bolt", execCall.Items[2])
	}
}

// TestApproval_MCP_PlanOnlyMode verifies that plan-only mode only requests
// approval for the plan stage; the MCP tool executes without a prompt.
func TestApproval_MCP_PlanOnlyMode(t *testing.T) {
	step := mcpStep("srv", "tool", nil)
	approver := &sequenceApprover{
		decisions: []Decision{Approve}, // plan only
	}
	ag := setupMCPAgent(t, step, defaultCaller("done"), approver, ApprovalPlanOnly)

	result, err := ag.Run(context.Background(), "run tool")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Success {
		t.Error("Result.Success = false, want true")
	}

	// plan-only: only 1 approval (plan stage)
	if len(approver.calls) != 1 {
		t.Errorf("approval calls = %d, want 1 (plan only)", len(approver.calls))
	}
	if approver.calls[0].Stage != "plan" {
		t.Errorf("calls[0].Stage = %q, want plan", approver.calls[0].Stage)
	}
}

// TestApproval_MCP_DangerousOnlyMode verifies that dangerous-only mode
// requests approval for the MCP step (dangerous) but auto-approves read-only steps.
func TestApproval_MCP_DangerousOnlyMode(t *testing.T) {
	dir := t.TempDir()
	sb, err := sandbox.New(dir)
	if err != nil {
		t.Fatalf("sandbox.New: %v", err)
	}

	// Plan: list (read-only, auto-approved) + call_mcp_tool (dangerous, needs approval).
	planJSON := makePlanJSON([]Step{
		{Action: ActionList, Description: "list dir", Path: dir},
		mcpStep("srv", "tool", nil),
	})
	approver := &sequenceApprover{
		decisions: []Decision{Approve}, // one approval: the MCP step
	}
	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: planJSON},
	})
	ag := New(chain, sb, approver, ApprovalDangerousOnly, nil, nil)
	ag.SetMCPCaller(defaultCaller("result"))
	registry := mcp.NewToolRegistry()
	registry.AddTools("srv", []mcp.Tool{{Name: "tool"}})
	ag.SetMCPToolRegistry(registry)

	result, err := ag.Run(context.Background(), "do stuff")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.StepResults) != 2 {
		t.Fatalf("StepResults count = %d, want 2", len(result.StepResults))
	}

	// List step is auto-approved → has [auto] prefix.
	if !strings.HasPrefix(result.StepResults[0], "[auto]") {
		t.Errorf("list result missing [auto] prefix: %q", result.StepResults[0])
	}
	// MCP step went through approval → no [auto] prefix.
	if strings.HasPrefix(result.StepResults[1], "[auto]") {
		t.Errorf("MCP result should not have [auto] prefix: %q", result.StepResults[1])
	}

	// Exactly one approval call: the MCP step.
	if len(approver.calls) != 1 {
		t.Fatalf("approval calls = %d, want 1 (dangerous MCP only)", len(approver.calls))
	}
	if approver.calls[0].Stage != "execute" {
		t.Errorf("calls[0].Stage = %q, want execute", approver.calls[0].Stage)
	}
	if !approver.calls[0].Dangerous {
		t.Error("MCP approval request Dangerous = false, want true")
	}
}

// TestApproval_MCP_NoneMode verifies that none mode never requests approval,
// even for the dangerous MCP step.
func TestApproval_MCP_NoneMode(t *testing.T) {
	step := mcpStep("srv", "tool", nil)
	approver := &mockApprover{decision: Approve}
	ag := setupMCPAgent(t, step, defaultCaller("out"), approver, ApprovalNone)

	result, err := ag.Run(context.Background(), "run tool")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Success {
		t.Error("Result.Success = false, want true")
	}

	// none mode: zero approval calls
	if len(approver.calls) != 0 {
		t.Errorf("approval calls = %d, want 0 (none mode)", len(approver.calls))
	}
}

// TestApproval_MCP_UserRejects verifies that when the user rejects the MCP
// step approval, the agent returns ErrRejected at the execute stage.
func TestApproval_MCP_UserRejects(t *testing.T) {
	step := mcpStep("srv", "tool", nil)
	// Plan approved, execute rejected.
	approver := &sequenceApprover{
		decisions: []Decision{Approve, Reject},
	}
	ag := setupMCPAgent(t, step, defaultCaller("never"), approver, ApprovalFull)

	_, err := ag.Run(context.Background(), "call tool")
	if err == nil {
		t.Fatal("expected error when MCP step is rejected, got nil")
	}
	if !errors.Is(err, ErrRejected) {
		t.Errorf("error = %v, want ErrRejected", err)
	}
	var rejErr *RejectedError
	if !errors.As(err, &rejErr) {
		t.Fatalf("expected *RejectedError, got %T", err)
	}
	if rejErr.Stage != "execute" {
		t.Errorf("Stage = %q, want execute", rejErr.Stage)
	}
}

// --- End-to-end integration test ---

// TestMCPToolCall_EndToEnd tests the full pipeline:
// plan → (no approval) → executor calls MCPCaller → result returned.
func TestMCPToolCall_EndToEnd(t *testing.T) {
	step := mcpStep("weather-srv", "get_forecast", map[string]any{"city": "Istanbul"})
	caller := defaultCaller("Sunny, 28°C")
	ag := setupMCPAgent(t, step, caller, &mockApprover{decision: Approve}, ApprovalNone)

	result, err := ag.Run(context.Background(), "get weather forecast")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Success {
		t.Error("Result.Success = false, want true")
	}
	if len(result.StepResults) != 1 {
		t.Fatalf("StepResults count = %d, want 1", len(result.StepResults))
	}
	if result.StepResults[0] != "Sunny, 28°C" {
		t.Errorf("StepResults[0] = %q, want %q", result.StepResults[0], "Sunny, 28°C")
	}

	// Verify the caller received the correct arguments.
	if caller.serverName != "weather-srv" {
		t.Errorf("called server = %q, want weather-srv", caller.serverName)
	}
	if caller.toolName != "get_forecast" {
		t.Errorf("called tool = %q, want get_forecast", caller.toolName)
	}
	if caller.args["city"] != "Istanbul" {
		t.Errorf("called args[city] = %v, want Istanbul", caller.args["city"])
	}
}
