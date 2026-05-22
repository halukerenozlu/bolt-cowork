package views

import (
	"context"

	"github.com/halukerenozlu/bolt-cowork/pkg/types"
)

// UIEvent carries structured live-update data from the agent to the TUI.
// Implementations: PlanReadyEvent, StepDoneEvent.
type UIEvent interface{ isUIEvent() }

// PlanReadyEvent is emitted once when the agent has finalised its execution plan.
type PlanReadyEvent struct {
	Steps []string // step descriptions in order
}

func (PlanReadyEvent) isUIEvent() {}

// StepDoneEvent is emitted after each plan step completes (success or failure).
type StepDoneEvent struct {
	Index int   // 0-based step index
	Info  string // executor result string, e.g. `Read "README.md" (2048 bytes)`
	Err   error  // nil on success
}

func (StepDoneEvent) isUIEvent() {}

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

	Provider  string // e.g. "anthropic"
	Model     string // e.g. "claude-sonnet-4-6"
	Workspace string // absolute workspace path
}
