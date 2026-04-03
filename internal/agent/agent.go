package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/halukerenozlu/bolt-cowork/internal/provider"
	"github.com/halukerenozlu/bolt-cowork/internal/sandbox"
)

// ErrRejected is returned when the user rejects an approval request.
var ErrRejected = errors.New("rejected by user")

// Result holds the outcome of an agent run.
type Result struct {
	Success     bool
	Plan        *Plan
	StepResults []string
	Error       error
}

// Agent orchestrates the command → plan → approve → execute → report loop.
type Agent struct {
	chain    *provider.FallbackChain
	sandbox  *sandbox.Sandbox
	approver Approver
	mode     ApprovalMode
	planner  *Planner
	executor *Executor
}

// New creates an Agent with the given dependencies.
func New(chain *provider.FallbackChain, sb *sandbox.Sandbox, approver Approver, mode ApprovalMode) *Agent {
	return &Agent{
		chain:    chain,
		sandbox:  sb,
		approver: approver,
		mode:     mode,
		planner:  NewPlanner(chain),
		executor: NewExecutor(sb),
	}
}

// Run executes the full agent loop for a user command.
func (a *Agent) Run(ctx context.Context, command string) (*Result, error) {
	// Stage 1: Skill matching — skipped in v0.1.

	// Stage 2: Planning.
	plan, err := a.planStage(ctx, command)
	if err != nil {
		return nil, err
	}

	// Stage 3: Execution.
	stepResults, err := a.executeStage(ctx, plan)
	if err != nil {
		return &Result{Plan: plan, StepResults: stepResults, Error: err}, err
	}

	// Stage 4: Result approval.
	if err := a.resultStage(ctx, stepResults); err != nil {
		return &Result{Plan: plan, StepResults: stepResults, Error: err}, err
	}

	return &Result{
		Success:     true,
		Plan:        plan,
		StepResults: stepResults,
	}, nil
}

// planStage creates a plan and requests approval. Supports revision loop.
func (a *Agent) planStage(ctx context.Context, command string) (*Plan, error) {
	dirListing, err := a.buildDirListing()
	if err != nil {
		return nil, fmt.Errorf("agent: list directory: %w", err)
	}

	for {
		plan, err := a.planner.CreatePlan(ctx, command, dirListing)
		if err != nil {
			return nil, fmt.Errorf("agent: create plan: %w", err)
		}

		if !shouldApprove(a.mode, "plan", false) {
			return plan, nil
		}

		items := make([]string, len(plan.Steps))
		for i, s := range plan.Steps {
			items[i] = s.Description
		}

		decision, err := a.approver.RequestApproval(ctx, ApprovalRequest{
			Stage:       "plan",
			Description: plan.Description,
			Items:       items,
		})
		if err != nil {
			return nil, fmt.Errorf("agent: plan approval: %w", err)
		}

		switch decision {
		case Approve, ApproveAll:
			return plan, nil
		case Revise:
			continue // re-plan
		case Reject:
			return nil, fmt.Errorf("agent: plan stage: %w", ErrRejected)
		default:
			return nil, fmt.Errorf("agent: plan stage: unknown decision %d", decision)
		}
	}
}

// executeStage runs each step with per-step approval.
func (a *Agent) executeStage(ctx context.Context, plan *Plan) ([]string, error) {
	var results []string
	approveAll := false

	for _, step := range plan.Steps {
		dangerous := isDangerous(step, a.sandbox)
		if !approveAll && shouldApprove(a.mode, "execute", dangerous) {
			decision, err := a.approver.RequestApproval(ctx, ApprovalRequest{
				Stage:       "execute",
				Description: step.Description,
				Items:       []string{fmt.Sprintf("%s %s", step.Action, step.Path)},
				Dangerous:   dangerous,
			})
			if err != nil {
				return results, fmt.Errorf("agent: step approval: %w", err)
			}

			switch decision {
			case Approve:
				// continue to execute
			case ApproveAll:
				approveAll = true
			case Reject:
				return results, fmt.Errorf("agent: execute stage: %w", ErrRejected)
			default:
				return results, fmt.Errorf("agent: execute stage: unknown decision %d", decision)
			}
		}

		result, err := a.executor.ExecuteStep(ctx, step)
		if err != nil {
			return results, fmt.Errorf("agent: execute step %q: %w", step.Description, err)
		}
		results = append(results, result)
	}

	return results, nil
}

// resultStage requests final approval for the completed results.
func (a *Agent) resultStage(ctx context.Context, stepResults []string) error {
	if !shouldApprove(a.mode, "result", false) {
		return nil
	}

	decision, err := a.approver.RequestApproval(ctx, ApprovalRequest{
		Stage:       "result",
		Description: "Task completed.",
		Items:       stepResults,
	})
	if err != nil {
		return fmt.Errorf("agent: result approval: %w", err)
	}

	switch decision {
	case Approve, ApproveAll, Revise:
		return nil
	case Reject:
		return fmt.Errorf("agent: result stage: %w", ErrRejected)
	default:
		return fmt.Errorf("agent: result stage: unknown decision %d", decision)
	}
}

// isDangerous computes whether a step is destructive based on action type
// and filesystem state. This is computed server-side, never from LLM output.
func isDangerous(step Step, sb *sandbox.Sandbox) bool {
	switch step.Action {
	case ActionDelete:
		return true
	case ActionWrite:
		_, err := sb.FileInfo(step.Path)
		return err == nil // file exists = overwrite
	case ActionMove, ActionRename:
		_, err := sb.FileInfo(step.Destination)
		return err == nil // destination exists = overwrite
	default:
		return false
	}
}

// buildDirListing returns a text listing of the sandbox root directory.
func (a *Agent) buildDirListing() (string, error) {
	entries, err := a.sandbox.ListDir(a.sandbox.Root())
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	for _, entry := range entries {
		if entry.IsDir() {
			sb.WriteString(entry.Name() + "/\n")
		} else {
			sb.WriteString(entry.Name() + "\n")
		}
	}
	return sb.String(), nil
}
