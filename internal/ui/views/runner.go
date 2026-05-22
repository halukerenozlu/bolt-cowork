package views

import (
	"context"

	"github.com/halukerenozlu/bolt-cowork/pkg/types"
)

// UIEvent carries structured live-update data from the agent to the TUI.
// Implementations: PlanReadyEvent, StepStartEvent, StepDoneEvent, PermWarnEvent.
type UIEvent interface{ isUIEvent() }

// PlanReadyEvent is emitted once when the agent has finalised its execution plan.
type PlanReadyEvent struct {
	Steps []string // step descriptions in order
}

func (PlanReadyEvent) isUIEvent() {}

// StepStartEvent is emitted just before a plan step begins executing.
type StepStartEvent struct {
	Index  int    // 0-based step index
	Action string // step action type: "read", "write", "call_mcp_tool", etc.
	Desc   string // step description from the planner
}

func (StepStartEvent) isUIEvent() {}

// StepDoneEvent is emitted after each plan step completes (success or failure).
type StepDoneEvent struct {
	Index  int    // 0-based step index
	Action string // step action type: "read", "write", "call_mcp_tool", etc.
	Info   string // executor result string; for MCP: "server/tool: <output>"
	Err    error  // nil on success
}

func (StepDoneEvent) isUIEvent() {}

// PermWarnEvent is emitted when a dangerous action is auto-approved.
type PermWarnEvent struct {
	Warning string // e.g. "execute: delete workspace/old.txt"
}

func (PermWarnEvent) isUIEvent() {}

// AgentResult is returned by AgentRunner.Run after a single command completes.
type AgentResult struct {
	History []types.Message
	Err     error
}

// AgentRunner wires the TUI session to the underlying agent.
// Constructed in main.go and threaded through App → Session.
type AgentRunner struct {
	// Run executes cmd. It calls onChunk with text as it becomes available and
	// onEvent with structured live updates (plan steps, step completions). Both
	// callbacks are optional (nil-safe). Run must be safe to call from a goroutine.
	Run func(ctx context.Context, cmd string, history []types.Message,
		onChunk func(string), onEvent func(UIEvent)) AgentResult

	Provider     string   // e.g. "anthropic"
	Model        string   // e.g. "claude-sonnet-4-6"
	Workspace    string   // absolute workspace path
	ApprovalMode string   // e.g. "full", "plan-only", "dangerous-only", "none"
	LoadedSkills []string // names of skills loaded at startup
}
