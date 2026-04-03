package agent

import "context"

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
	Stage       string   // "plan", "execute", "result"
	Description string   // Human-readable description shown to the user.
	Items       []string // Plan steps or skill names.
	Dangerous   bool     // Whether the operation is destructive.
}

// Approver abstracts user interaction for approval gates.
type Approver interface {
	RequestApproval(ctx context.Context, req ApprovalRequest) (Decision, error)
}

// shouldApprove determines if approval is needed based on mode and context.
func shouldApprove(mode ApprovalMode, stage string, dangerous bool) bool {
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
