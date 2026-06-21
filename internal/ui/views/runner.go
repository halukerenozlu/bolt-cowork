package views

import (
	"context"
	"time"

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

// ApprovalRequestEvent is emitted when the agent needs user approval.
// The agent goroutine blocks until a decision is sent to ResponseCh.
type ApprovalRequestEvent struct {
	Stage       string   // "skill", "plan", "execute", "result"
	Description string   // human-readable description
	Items       []string // step descriptions or tool details
	Dangerous   bool     // whether the operation is destructive
	ResponseCh  chan<- ApprovalResponse
}

func (ApprovalRequestEvent) isUIEvent() {}

// ApprovalResponse carries the user's decision back to the agent goroutine.
type ApprovalResponse struct {
	Approved bool // true = approve, false = reject
}

// AgentResult is returned by AgentRunner.Run after a single command completes.
type AgentResult struct {
	History []types.Message
	Err     error
}

// RuntimeModelChangedMsg tells the root App that future sessions must use the
// newly selected provider/model pair.
type RuntimeModelChangedMsg struct {
	Provider string
	Model    string
}

type SessionMessage struct {
	Role string
	Text string
}

type SessionSummary struct {
	ID        string
	Title     string
	UpdatedAt time.Time
	Active    bool
}

type SessionSnapshot struct {
	ID          string
	Title       string
	Provider    string
	Model       string
	Messages    []SessionMessage
	History     []types.Message
	TokenCount  int
	TokenBytes  int
	SessionCost float64
}

type SaveSessionMsg struct{ Snapshot SessionSnapshot }
type OpenSessionMsg struct{ ID string }
type CreateSessionMsg struct{ Title string }
type DeleteSessionMsg struct{ ID string }
type RenameSessionMsg struct {
	ID    string
	Title string
}

// AgentRunner wires the TUI session to the underlying agent.
// Constructed in main.go and threaded through App → Session.
type AgentRunner struct {
	// Run executes cmd. It calls onChunk with text as it becomes available and
	// onEvent with structured live updates (plan steps, step completions). Both
	// callbacks are optional (nil-safe). Run must be safe to call from a goroutine.
	Run func(ctx context.Context, cmd string, history []types.Message,
		onChunk func(string), onEvent func(UIEvent)) AgentResult

	Provider      string            // e.g. "anthropic"
	Model         string            // e.g. "claude-sonnet-4-6"
	Workspace     string            // absolute workspace path
	ApprovalMode  string            // e.g. "full", "plan-only", "dangerous-only", "none"
	LoadedSkills  []string          // names of skills loaded at startup
	SkillContents map[string]string // SKILL.md contents keyed by skill name
}
