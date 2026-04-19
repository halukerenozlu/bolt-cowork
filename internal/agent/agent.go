package agent

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/halukerenozlu/bolt-cowork/internal/provider"
	"github.com/halukerenozlu/bolt-cowork/internal/sandbox"
)

const maxPlanIntentRetries = 3
const maxRevisions = 3

var deleteKeywords = []string{"sil", "delete", "remove", "kaldir", "yok et"}

// ErrRejected is returned when the user rejects an approval request.
var ErrRejected = errors.New("rejected by user")

// RejectedError is a stage-aware rejection error. It unwraps to ErrRejected
// so callers can use errors.Is(err, ErrRejected) while also inspecting Stage.
type RejectedError struct {
	Stage string // "plan", "execute", "result"
}

func (e *RejectedError) Error() string {
	return fmt.Sprintf("agent: %s stage: %s", e.Stage, ErrRejected)
}

func (e *RejectedError) Unwrap() error {
	return ErrRejected
}

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

	planningCommand := command
	intentRetries := 0
	revisionCount := 0

	for {
		plan, err := a.planner.CreatePlan(ctx, planningCommand, dirListing)
		if err != nil {
			return nil, fmt.Errorf("agent: create plan: %w", err)
		}

		if mismatch := validatePlanAgainstCommand(command, plan); mismatch != "" {
			intentRetries++
			if intentRetries >= maxPlanIntentRetries {
				return nil, fmt.Errorf("agent: plan does not match command intent after %d attempts: %s", intentRetries, mismatch)
			}
			planningCommand = command + "\n\nIMPORTANT: " + mismatch
			continue
		}
		intentRetries = 0

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
			if revisionCount >= maxRevisions {
				return nil, fmt.Errorf("agent: maximum revisions (%d) reached, please try a new command", maxRevisions)
			}
			revisionCount++
			// Collect revision feedback if the approver supports it.
			if rp, ok := a.approver.(RevisionPrompter); ok {
				feedback, promptErr := rp.PromptRevision(ctx)
				if promptErr != nil {
					return nil, fmt.Errorf("agent: read revision: %w", promptErr)
				}
				feedback = strings.TrimSpace(feedback)
				if feedback != "" {
					planningCommand = command + "\n\nRevision: " + feedback
					continue
				}
			}
			planningCommand = command
			continue // re-plan with original command
		case Reject:
			return nil, &RejectedError{Stage: "plan"}
		default:
			return nil, fmt.Errorf("agent: plan stage: unknown decision %d", decision)
		}
	}
}

func hasAction(plan *Plan, action StepAction) bool {
	for _, step := range plan.Steps {
		if step.Action == action {
			return true
		}
	}
	return false
}

func containsAny(s string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}

func normalizePathToken(s string) string {
	s = strings.TrimSpace(strings.Trim(s, `"'`))
	s = filepath.ToSlash(s)
	s = strings.TrimSuffix(s, "/")
	return strings.ToLower(s)
}

func samePathRef(path, token string) bool {
	p := normalizePathToken(path)
	t := normalizePathToken(token)
	if p == "" || t == "" {
		return false
	}
	return p == t || strings.HasSuffix(p, "/"+t)
}

func sourceDirForContentsIntent(command string) string {
	cmd := strings.ToLower(command)
	if !containsAny(cmd, []string{"içeri", "iceri", "contents"}) {
		return ""
	}

	words := strings.Fields(cmd)
	for i, w := range words {
		if !containsAny(w, []string{"içeri", "iceri", "contents"}) {
			continue
		}
		for j := i - 1; j >= 0 && j >= i-4; j-- {
			cand := strings.Trim(words[j], `"'.,;:!?()[]{}<>`)
			if cand == "" {
				continue
			}
			if isGrammarWord(cand) {
				continue
			}
			return cand
		}
	}
	return ""
}

func isGrammarWord(word string) bool {
	switch word {
	case "de", "da", "deki", "daki", "nin", "nun", "dosyasindaki", "klasorundeki":
		return true
	}
	return containsAny(word, []string{"klas", "folder", "directory"})
}

func isContentDeleteIntent(command string) bool {
	cmd := strings.ToLower(command)
	if !containsAny(cmd, deleteKeywords) {
		return false
	}
	return containsAny(cmd, []string{
		"içeri", "iceri", "content", "contents",
		"yazilan", "yazılan", "metin", "text", "satir", "satır",
	})
}

func validatePlanAgainstCommand(command string, plan *Plan) string {
	cmd := strings.ToLower(command)

	if containsAny(cmd, deleteKeywords) && !isContentDeleteIntent(cmd) && !hasAction(plan, ActionDelete) {
		return `the command asks to delete/remove; include at least one "delete" step`
	}
	if containsAny(cmd, []string{"kopyala", "copy"}) && !hasAction(plan, ActionCopy) {
		return `the command asks to copy; include at least one "copy" step`
	}
	if containsAny(cmd, []string{"taşı", "tasi", "move"}) && !(hasAction(plan, ActionMove) || hasAction(plan, ActionRename)) {
		return `the command asks to move; include at least one "move" (or "rename") step`
	}
	if containsAny(cmd, []string{"oluştur", "olustur", "create", "mkdir"}) &&
		containsAny(cmd, []string{"klas", "folder", "directory"}) &&
		!hasAction(plan, ActionMkdir) {
		return `the command asks to create a directory; include at least one "mkdir" step`
	}

	srcDir := sourceDirForContentsIntent(command)
	if srcDir != "" {
		for _, step := range plan.Steps {
			if (step.Action == ActionMove || step.Action == ActionCopy) && samePathRef(step.Path, srcDir) {
				return fmt.Sprintf("the command asks for contents of %q; do not move/copy the directory itself, operate on entries inside it", srcDir)
			}
		}
	}

	return ""
}

// isReadOnly returns true for actions that do not modify the filesystem.
func isReadOnly(action StepAction) bool {
	return action == ActionRead || action == ActionList
}

func (a *Agent) prepareDeleteStep(ctx context.Context, step Step) (Step, error) {
	if step.Action != ActionDelete {
		return step, nil
	}

	resolved, err := a.executor.resolvePath(step.Path)
	if err != nil {
		return step, err
	}
	if _, err := os.Stat(resolved); err == nil {
		return step, nil
	} else if !os.IsNotExist(err) {
		return step, fmt.Errorf("agent: stat delete target %q: %w", step.Path, err)
	}

	candidates, err := a.findDeleteCandidates(step.Path)
	if err != nil {
		return step, err
	}
	if len(candidates) == 0 {
		return step, &os.PathError{Op: "delete", Path: step.Path, Err: fs.ErrNotExist}
	}

	selector, ok := a.approver.(PathSelector)
	if !ok {
		paths := make([]string, 0, len(candidates))
		for _, cand := range candidates {
			paths = append(paths, cand.Path)
		}
		return step, fmt.Errorf("agent: delete target %q requires explicit path selection: %s", step.Path, strings.Join(paths, ", "))
	}

	selected, err := selector.SelectPath(ctx, PathSelectionRequest{
		Stage:        "execute",
		Action:       "delete",
		OriginalPath: step.Path,
		Candidates:   candidates,
	})
	if err != nil {
		return step, fmt.Errorf("agent: select delete target: %w", err)
	}
	if selected == "" {
		return step, &RejectedError{Stage: "execute"}
	}

	for _, cand := range candidates {
		if normalizePathToken(cand.Path) == normalizePathToken(selected) {
			step.Path = cand.Path
			return step, nil
		}
	}
	return step, fmt.Errorf("agent: selected delete target %q is not in candidates", selected)
}

func (a *Agent) findDeleteCandidates(path string) ([]PathCandidate, error) {
	base := filepath.Base(filepath.Clean(strings.TrimSpace(path)))
	if base == "" || base == "." || base == string(filepath.Separator) {
		return nil, nil
	}

	root := a.sandbox.Root()
	var candidates []PathCandidate
	err := filepath.WalkDir(root, func(current string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if !strings.EqualFold(d.Name(), base) {
			return nil
		}
		rel, err := filepath.Rel(root, current)
		if err != nil {
			return nil
		}
		candidates = append(candidates, PathCandidate{
			Path:  filepath.ToSlash(rel),
			IsDir: d.IsDir(),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("agent: search delete candidates for %q: %w", path, err)
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Path < candidates[j].Path
	})
	return candidates, nil
}

// executeStage runs each step with per-step approval.
// In "dangerous-only" mode, read-only actions (read, list) are auto-approved.
// In "full" mode, every step requires approval.
func (a *Agent) executeStage(ctx context.Context, plan *Plan) ([]string, error) {
	var results []string
	approveAll := false

	for i := range plan.Steps {
		step := plan.Steps[i]

		if step.Action == ActionDelete {
			var prepErr error
			step, prepErr = a.prepareDeleteStep(ctx, step)
			if prepErr != nil {
				return results, fmt.Errorf("agent: prepare delete step %q: %w", step.Description, prepErr)
			}
		}

		// In dangerous-only mode, skip approval for read-only operations.
		if a.mode == ApprovalDangerousOnly && isReadOnly(step.Action) {
			result, err := a.executor.ExecuteStep(ctx, step)
			if err != nil {
				return results, fmt.Errorf("agent: execute step %q: %w", step.Description, err)
			}
			results = append(results, "[auto] "+result)
			continue
		}

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
				return results, &RejectedError{Stage: "execute"}
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
		return &RejectedError{Stage: "result"}
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
	case ActionCopy:
		return true // creates new content
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
