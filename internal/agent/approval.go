package agent

import (
	"context"
	"encoding/json"
	"fmt"
)

// ApprovalMode controls when the agent pauses for user approval.
type ApprovalMode string

const (
	ApprovalFull          ApprovalMode = "full"
	ApprovalPlanOnly      ApprovalMode = "plan-only"
	ApprovalDangerousOnly ApprovalMode = "dangerous-only"
	ApprovalNone          ApprovalMode = "none"
)

// Decision represents the user's response to an approval request.
type Decision int

const (
	Approve    Decision = iota // Continue with the current action.
	Reject                     // Stop the agent.
	Revise                     // Re-create the plan (plan stage only).
	ApproveAll                 // Approve all remaining steps (execute stage only).
)

// ApprovalRequest holds context for an approval prompt.
type ApprovalRequest struct {
	Stage        string   // "plan", "execute", "result"
	Description  string   // Human-readable description shown to the user.
	Items        []string // Plan steps or skill names.
	Dangerous    bool     // Whether the operation is destructive.
	DangerReason string   // Short explanation of why the operation is dangerous.
}

// Approver abstracts user interaction for approval gates.
type Approver interface {
	RequestApproval(ctx context.Context, req ApprovalRequest) (Decision, error)
}

// PathCandidate is a selectable filesystem target.
type PathCandidate struct {
	Path  string // Path relative to sandbox root.
	IsDir bool
}

// PathSelectionRequest asks the user to disambiguate a missing path.
type PathSelectionRequest struct {
	Stage        string // usually "execute"
	Action       string // e.g. "delete"
	OriginalPath string // original unresolved user/planner path
	Candidates   []PathCandidate
}

// PathSelector is an optional extension for approvers that can pick one path.
// Returning an empty string means "reject/cancel".
type PathSelector interface {
	SelectPath(ctx context.Context, req PathSelectionRequest) (string, error)
}

// RevisionPrompter is an optional extension for approvers that can collect
// revision instructions from the user when they choose "Revise" in the plan stage.
type RevisionPrompter interface {
	PromptRevision(ctx context.Context) (string, error)
}

// mcpApprovalItems builds the Items slice for a call_mcp_tool approval request.
// Each element is a labelled field (Server, Tool, Args) so the user can see
// exactly what will be called before confirming.
func mcpApprovalItems(serverName, toolName string, args map[string]any) []string {
	argsJSON, err := json.Marshal(args)
	if err != nil || args == nil {
		argsJSON = []byte("{}")
	}
	return []string{
		fmt.Sprintf("Server : %s", serverName),
		fmt.Sprintf("Tool   : %s", toolName),
		fmt.Sprintf("Args   : %s", argsJSON),
	}
}

// ShouldApprove determines if approval is needed based on mode and context.
func ShouldApprove(mode ApprovalMode, stage string, dangerous bool) bool {
	switch mode {
	case ApprovalFull:
		return true
	case ApprovalPlanOnly:
		return stage == "plan"
	case ApprovalDangerousOnly:
		return stage == "execute" && dangerous
	case ApprovalNone:
		return false
	default:
		return true // default to full
	}
}
