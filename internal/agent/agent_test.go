package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/halukerenozlu/bolt-cowork/internal/provider"
	"github.com/halukerenozlu/bolt-cowork/internal/sandbox"
	"github.com/halukerenozlu/bolt-cowork/internal/skill"
	"github.com/halukerenozlu/bolt-cowork/pkg/types"
)

// --- Mocks ---

type mockApprover struct {
	decision Decision
	calls    []ApprovalRequest
}

func (m *mockApprover) RequestApproval(_ context.Context, req ApprovalRequest) (Decision, error) {
	m.calls = append(m.calls, req)
	return m.decision, nil
}

type selectingApprover struct {
	decision       Decision
	selectedPath   string
	calls          []ApprovalRequest
	selectionCalls int
	lastSelection  PathSelectionRequest
}

func (s *selectingApprover) RequestApproval(_ context.Context, req ApprovalRequest) (Decision, error) {
	s.calls = append(s.calls, req)
	return s.decision, nil
}

func (s *selectingApprover) SelectPath(_ context.Context, req PathSelectionRequest) (string, error) {
	s.selectionCalls++
	s.lastSelection = req
	return s.selectedPath, nil
}

type mockLLMProvider struct {
	name      string
	available bool
	response  string
	err       error
}

func (m *mockLLMProvider) Chat(_ context.Context, _ []types.Message, _ []provider.ToolSpec) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func (m *mockLLMProvider) StreamChat(_ context.Context, _ []types.Message) (<-chan string, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockLLMProvider) Name() string    { return m.name }
func (m *mockLLMProvider) Available() bool { return m.available }

// --- Helpers ---

func makePlanJSON(steps []Step) string {
	plan := Plan{Description: "test plan", Steps: steps}
	data, _ := json.Marshal(plan)
	return string(data)
}

func setupAgent(t *testing.T, llmResponse string, decision Decision, mode ApprovalMode) (*Agent, string) {
	t.Helper()
	dir := t.TempDir()

	sb, err := sandbox.New(dir)
	if err != nil {
		t.Fatalf("sandbox.New: %v", err)
	}

	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: llmResponse},
	})

	approver := &mockApprover{decision: decision}
	ag := New(chain, sb, approver, mode, nil)
	return ag, dir
}

func setupAgentWithApprover(t *testing.T, llmResponse string, approver Approver, mode ApprovalMode) (*Agent, string) {
	t.Helper()
	dir := t.TempDir()

	sb, err := sandbox.New(dir)
	if err != nil {
		t.Fatalf("sandbox.New: %v", err)
	}

	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: llmResponse},
	})

	ag := New(chain, sb, approver, mode, nil)
	return ag, dir
}

// --- shouldApprove Tests ---

func TestShouldApprove(t *testing.T) {
	tests := []struct {
		name      string
		mode      ApprovalMode
		stage     string
		dangerous bool
		want      bool
	}{
		{"full/skill", ApprovalFull, "skill", false, true},
		{"full/plan", ApprovalFull, "plan", false, true},
		{"full/execute", ApprovalFull, "execute", false, true},
		{"full/execute/dangerous", ApprovalFull, "execute", true, true},
		{"full/result", ApprovalFull, "result", false, true},

		{"plan-only/skill", ApprovalPlanOnly, "skill", false, false},
		{"plan-only/plan", ApprovalPlanOnly, "plan", false, true},
		{"plan-only/execute", ApprovalPlanOnly, "execute", false, false},
		{"plan-only/result", ApprovalPlanOnly, "result", false, false},

		{"dangerous-only/skill", ApprovalDangerousOnly, "skill", false, false},
		{"dangerous-only/plan", ApprovalDangerousOnly, "plan", false, false},
		{"dangerous-only/execute", ApprovalDangerousOnly, "execute", false, false},
		{"dangerous-only/execute/dangerous", ApprovalDangerousOnly, "execute", true, true},
		{"dangerous-only/result", ApprovalDangerousOnly, "result", false, false},

		{"none/skill", ApprovalNone, "skill", false, false},
		{"none/plan", ApprovalNone, "plan", false, false},
		{"none/execute", ApprovalNone, "execute", false, false},
		{"none/result", ApprovalNone, "result", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldApprove(tt.mode, tt.stage, tt.dangerous)
			if got != tt.want {
				t.Errorf("shouldApprove(%q, %q, %v) = %v, want %v",
					tt.mode, tt.stage, tt.dangerous, got, tt.want)
			}
		})
	}
}

// --- Approval Gate Tests ---

func TestAgent_FullMode_AllGates(t *testing.T) {
	dir := t.TempDir()
	sb, _ := sandbox.New(dir)

	// Use a write action so all three approval gates fire.
	planJSON := makePlanJSON([]Step{
		{Action: ActionWrite, Description: "write file", Path: filepath.Join(dir, "out.txt"), Content: "data"},
	})

	approver := &mockApprover{decision: Approve}
	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: planJSON},
	})
	ag := New(chain, sb, approver, ApprovalFull, nil)

	_, err := ag.Run(context.Background(), "write file")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// full mode: plan + execute + result = 3 approvals
	if len(approver.calls) != 3 {
		t.Errorf("approval calls = %d, want 3", len(approver.calls))
	}
	stages := []string{"plan", "execute", "result"}
	for i, want := range stages {
		if i < len(approver.calls) && approver.calls[i].Stage != want {
			t.Errorf("call[%d].Stage = %q, want %q", i, approver.calls[i].Stage, want)
		}
	}
}

func TestAgent_PlanOnlyMode(t *testing.T) {
	dir := t.TempDir()
	sb, _ := sandbox.New(dir)

	planJSON := makePlanJSON([]Step{
		{Action: ActionList, Description: "list root", Path: dir},
	})

	approver := &mockApprover{decision: Approve}
	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: planJSON},
	})
	ag := New(chain, sb, approver, ApprovalPlanOnly, nil)

	_, err := ag.Run(context.Background(), "list files")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(approver.calls) != 1 {
		t.Errorf("approval calls = %d, want 1 (plan only)", len(approver.calls))
	}
	if len(approver.calls) > 0 && approver.calls[0].Stage != "plan" {
		t.Errorf("call[0].Stage = %q, want %q", approver.calls[0].Stage, "plan")
	}
}

func TestAgent_DangerousOnlyMode(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "delete-me.txt"), []byte("bye"), 0644)

	sb, _ := sandbox.New(dir)
	planJSON := makePlanJSON([]Step{
		{Action: ActionList, Description: "list", Path: dir},
		{Action: ActionDelete, Description: "delete file", Path: filepath.Join(dir, "delete-me.txt")},
	})

	approver := &mockApprover{decision: Approve}
	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: planJSON},
	})
	ag := New(chain, sb, approver, ApprovalDangerousOnly, nil)

	_, err := ag.Run(context.Background(), "clean up")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Only the dangerous step should trigger approval
	if len(approver.calls) != 1 {
		t.Fatalf("approval calls = %d, want 1 (dangerous only)", len(approver.calls))
	}
	if !approver.calls[0].Dangerous {
		t.Error("expected dangerous flag on approval request")
	}
}

func TestAgent_NoneMode(t *testing.T) {
	dir := t.TempDir()
	sb, _ := sandbox.New(dir)

	planJSON := makePlanJSON([]Step{
		{Action: ActionList, Description: "list root", Path: dir},
	})

	approver := &mockApprover{decision: Approve}
	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: planJSON},
	})
	ag := New(chain, sb, approver, ApprovalNone, nil)

	_, err := ag.Run(context.Background(), "list files")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(approver.calls) != 0 {
		t.Errorf("approval calls = %d, want 0 (none mode)", len(approver.calls))
	}
}

func TestAgent_PlanRejected(t *testing.T) {
	planJSON := makePlanJSON([]Step{
		{Action: ActionList, Description: "list", Path: "."},
	})

	ag, _ := setupAgent(t, planJSON, Reject, ApprovalFull)

	_, err := ag.Run(context.Background(), "list files")
	if err == nil {
		t.Fatal("expected error when plan is rejected")
	}
	if !errors.Is(err, ErrRejected) {
		t.Errorf("expected ErrRejected, got: %v", err)
	}
	var rejErr *RejectedError
	if !errors.As(err, &rejErr) {
		t.Fatalf("expected *RejectedError, got %T", err)
	}
	if rejErr.Stage != "plan" {
		t.Errorf("Stage = %q, want %q", rejErr.Stage, "plan")
	}
}

func TestAgent_StepRejected(t *testing.T) {
	dir := t.TempDir()
	planJSON := makePlanJSON([]Step{
		{Action: ActionWrite, Description: "write file", Path: filepath.Join(dir, "out.txt"), Content: "data"},
	})

	// Use a custom approver that approves plan then rejects step.
	customApprover := &sequenceApprover{
		decisions: []Decision{Approve, Reject}, // plan=approve, step=reject
	}
	ag, _ := setupAgentWithApprover(t, planJSON, customApprover, ApprovalFull)

	_, err := ag.Run(context.Background(), "write file")
	if err == nil {
		t.Fatal("expected error when step is rejected")
	}
	if !errors.Is(err, ErrRejected) {
		t.Errorf("expected ErrRejected, got: %v", err)
	}
	var rejErr *RejectedError
	if !errors.As(err, &rejErr) {
		t.Fatalf("expected *RejectedError, got %T", err)
	}
	if rejErr.Stage != "execute" {
		t.Errorf("Stage = %q, want %q", rejErr.Stage, "execute")
	}
}

func TestAgent_ResultRejected(t *testing.T) {
	dir := t.TempDir()
	sb, _ := sandbox.New(dir)

	// Use a write action so execute approval fires.
	planJSON := makePlanJSON([]Step{
		{Action: ActionWrite, Description: "write file", Path: filepath.Join(dir, "out.txt"), Content: "data"},
	})

	// Approve plan, approve step, reject result.
	approver := &sequenceApprover{
		decisions: []Decision{Approve, Approve, Reject},
	}
	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: planJSON},
	})
	ag := New(chain, sb, approver, ApprovalFull, nil)

	_, err := ag.Run(context.Background(), "write file")
	if err == nil {
		t.Fatal("expected error when result is rejected")
	}
	if !errors.Is(err, ErrRejected) {
		t.Errorf("expected ErrRejected, got: %v", err)
	}
	var rejErr *RejectedError
	if !errors.As(err, &rejErr) {
		t.Fatalf("expected *RejectedError, got %T", err)
	}
	if rejErr.Stage != "result" {
		t.Errorf("Stage = %q, want %q", rejErr.Stage, "result")
	}
}

// sequenceApprover returns decisions in order.
type sequenceApprover struct {
	decisions []Decision
	calls     []ApprovalRequest
	index     int
}

func (s *sequenceApprover) RequestApproval(_ context.Context, req ApprovalRequest) (Decision, error) {
	s.calls = append(s.calls, req)
	if s.index >= len(s.decisions) {
		return Approve, nil
	}
	d := s.decisions[s.index]
	s.index++
	return d, nil
}

func TestAgent_ApproveAll(t *testing.T) {
	dir := t.TempDir()

	sb, _ := sandbox.New(dir)
	// Use write actions so execute-stage approval fires.
	planJSON := makePlanJSON([]Step{
		{Action: ActionWrite, Description: "write a", Path: filepath.Join(dir, "a.txt"), Content: "a"},
		{Action: ActionWrite, Description: "write b", Path: filepath.Join(dir, "b.txt"), Content: "b"},
	})

	// Approve plan, ApproveAll on first step, then result.
	approver := &sequenceApprover{
		decisions: []Decision{Approve, ApproveAll, Approve}, // plan, first-step=approveAll, result
	}
	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: planJSON},
	})
	ag := New(chain, sb, approver, ApprovalFull, nil)

	result, err := ag.Run(context.Background(), "write files")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Should have: plan + first step (ApproveAll) + result = 3 calls
	// Second step should NOT trigger approval because ApproveAll was used.
	if len(approver.calls) != 3 {
		t.Errorf("approval calls = %d, want 3 (plan + first-step + result)", len(approver.calls))
	}
	if len(result.StepResults) != 2 {
		t.Errorf("step results = %d, want 2", len(result.StepResults))
	}
}

func TestAgent_ReadOnlyAutoApprove_DangerousOnly(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0644)

	sb, _ := sandbox.New(dir)
	planJSON := makePlanJSON([]Step{
		{Action: ActionRead, Description: "read a.txt", Path: filepath.Join(dir, "a.txt")},
		{Action: ActionList, Description: "list dir", Path: dir},
		{Action: ActionWrite, Description: "write b.txt", Path: filepath.Join(dir, "b.txt"), Content: "new"},
	})

	// In dangerous-only mode: no plan/result approval.
	// Read and list are auto-approved. Write to new file is now dangerous
	// (all non-read actions are dangerous), so 1 approval call.
	approver := &sequenceApprover{
		decisions: []Decision{Approve},
	}
	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: planJSON},
	})
	ag := New(chain, sb, approver, ApprovalDangerousOnly, nil)

	result, err := ag.Run(context.Background(), "process")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.StepResults) != 3 {
		t.Fatalf("step results = %d, want 3", len(result.StepResults))
	}

	// Read and list results should have [auto] prefix.
	if !strings.HasPrefix(result.StepResults[0], "[auto]") {
		t.Errorf("read result missing [auto] prefix: %q", result.StepResults[0])
	}
	if !strings.HasPrefix(result.StepResults[1], "[auto]") {
		t.Errorf("list result missing [auto] prefix: %q", result.StepResults[1])
	}
	// Write result should NOT have [auto] prefix (it went through approval).
	if strings.HasPrefix(result.StepResults[2], "[auto]") {
		t.Errorf("write result should not have [auto] prefix: %q", result.StepResults[2])
	}

	// 1 approval call for the dangerous write step.
	if len(approver.calls) != 1 {
		t.Errorf("approval calls = %d, want 1", len(approver.calls))
	}
	if len(approver.calls) > 0 && !approver.calls[0].Dangerous {
		t.Error("write approval should have dangerous flag set")
	}
}

func TestAgent_FullMode_ApprovesReadOnly(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0644)

	sb, _ := sandbox.New(dir)
	planJSON := makePlanJSON([]Step{
		{Action: ActionRead, Description: "read a.txt", Path: filepath.Join(dir, "a.txt")},
		{Action: ActionList, Description: "list dir", Path: dir},
	})

	// In full mode: plan + read step + list step + result = 4 approvals.
	approver := &sequenceApprover{
		decisions: []Decision{Approve, Approve, Approve, Approve},
	}
	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: planJSON},
	})
	ag := New(chain, sb, approver, ApprovalFull, nil)

	result, err := ag.Run(context.Background(), "inspect")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.StepResults) != 2 {
		t.Fatalf("step results = %d, want 2", len(result.StepResults))
	}

	// In full mode, results should NOT have [auto] prefix.
	for i, sr := range result.StepResults {
		if strings.HasPrefix(sr, "[auto]") {
			t.Errorf("step %d should not have [auto] prefix in full mode: %q", i, sr)
		}
	}

	// All 4 gates: plan + read + list + result.
	if len(approver.calls) != 4 {
		t.Errorf("approval calls = %d, want 4 (plan + read + list + result)", len(approver.calls))
	}
}

// --- Planner Tests ---

func TestPlanner_CreatePlan(t *testing.T) {
	planJSON := makePlanJSON([]Step{
		{Action: ActionRead, Description: "read file", Path: "test.txt"},
	})

	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: planJSON},
	})
	planner := NewPlanner(chain)

	plan, err := planner.CreatePlan(context.Background(), "read test.txt", "test.txt\n", nil, nil)
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("steps count = %d, want 1", len(plan.Steps))
	}
	if plan.Steps[0].Action != ActionRead {
		t.Errorf("action = %q, want %q", plan.Steps[0].Action, ActionRead)
	}
}

func TestPlanner_InvalidJSON(t *testing.T) {
	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: "this is not json at all"},
	})
	planner := NewPlanner(chain)

	_, err := planner.CreatePlan(context.Background(), "anything", "", nil, nil)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestPlanner_JSONWithSurroundingText(t *testing.T) {
	inner := makePlanJSON([]Step{
		{Action: ActionList, Description: "list", Path: "."},
	})

	tests := []struct {
		name     string
		response string
	}{
		{"json in markdown fence", "Here is the plan:\n```json\n" + inner + "\n```\nHope this helps!"},
		{"json in plain fence", "Sure!\n```\n" + inner + "\n```"},
		{"json with surrounding text", "Here you go: " + inner + " Done."},
		{"json with leading newlines", "\n\n" + inner + "\n\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain := provider.NewFallbackChain([]provider.LLMProvider{
				&mockLLMProvider{name: "mock", available: true, response: tt.response},
			})
			planner := NewPlanner(chain)

			plan, err := planner.CreatePlan(context.Background(), "list", "", nil, nil)
			if err != nil {
				t.Fatalf("CreatePlan: %v", err)
			}
			if len(plan.Steps) != 1 {
				t.Errorf("steps = %d, want 1", len(plan.Steps))
			}
		})
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantOK  bool
		wantHas string // substring the result should contain
	}{
		{"fenced json block", "text\n```json\n{\"a\":1}\n```\nmore", true, `"a":1`},
		{"plain fenced block", "text\n```\n{\"a\":1}\n```\nmore", true, `"a":1`},
		{"braces fallback", "here is {\"a\":1} ok", true, `"a":1`},
		{"no json at all", "no json here", false, ""},
		{"plain fenced non-json", "text\n```\nhello world\n```\n", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := extractJSON(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("extractJSON ok = %v, want %v (got: %q)", ok, tt.wantOK, got)
			}
			if ok && !strings.Contains(got, tt.wantHas) {
				t.Errorf("extractJSON result %q doesn't contain %q", got, tt.wantHas)
			}
		})
	}
}

func TestPlanner_MarkdownFencedJSON(t *testing.T) {
	planJSON := "```json\n" + makePlanJSON([]Step{
		{Action: ActionList, Description: "list", Path: "."},
	}) + "\n```"

	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: planJSON},
	})
	planner := NewPlanner(chain)

	plan, err := planner.CreatePlan(context.Background(), "list", "", nil, nil)
	if err != nil {
		t.Fatalf("CreatePlan with markdown fences: %v", err)
	}
	if len(plan.Steps) != 1 {
		t.Errorf("steps = %d, want 1", len(plan.Steps))
	}
}

// --- sanitizeForLog Tests ---

func TestSanitizeForLog_Truncation(t *testing.T) {
	long := strings.Repeat("a", 300)
	got := sanitizeForLog(long)
	if len(got) > maxLogLen+len("...[truncated]") {
		t.Errorf("len = %d, expected at most %d", len(got), maxLogLen+len("...[truncated]"))
	}
	if !strings.HasSuffix(got, "...[truncated]") {
		t.Errorf("expected ...[truncated] suffix, got %q", got[len(got)-20:])
	}
}

func TestSanitizeForLog_ControlChars(t *testing.T) {
	input := "hello\x00world\rtest\ttab"
	got := sanitizeForLog(input)
	if strings.ContainsAny(got, "\x00\r\t") {
		t.Errorf("control chars not removed: %q", got)
	}
	// Newlines should be preserved.
	input2 := "line1\nline2"
	got2 := sanitizeForLog(input2)
	if !strings.Contains(got2, "\n") {
		t.Errorf("newline should be preserved: %q", got2)
	}
}

func TestSanitizeForLog_Short(t *testing.T) {
	got := sanitizeForLog("short")
	if got != "short" {
		t.Errorf("got %q, want %q", got, "short")
	}
}

func TestPlanner_InvalidJSON_ErrorSanitized(t *testing.T) {
	// Response with control chars and length > 200 — error message should be sanitized.
	longResponse := "not json " + strings.Repeat("x", 250) + "\x00\r"
	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: longResponse},
	})
	planner := NewPlanner(chain)

	_, err := planner.CreatePlan(context.Background(), "anything", "", nil, nil)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	errMsg := err.Error()
	if strings.Contains(errMsg, "\x00") || strings.Contains(errMsg, "\r") {
		t.Error("error message contains control characters")
	}
	if !strings.Contains(errMsg, "...[truncated]") {
		t.Error("long response should be truncated in error message")
	}
}

// --- Executor Tests ---

func TestExecutor_Read(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	os.WriteFile(path, []byte("hello"), 0644)

	sb, _ := sandbox.New(dir)
	exec := NewExecutor(sb)

	result, err := exec.ExecuteStep(context.Background(), Step{Action: ActionRead, Path: path})
	if err != nil {
		t.Fatalf("ExecuteStep read: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}
}

func TestExecutor_Write(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.txt")

	sb, _ := sandbox.New(dir)
	exec := NewExecutor(sb)

	_, err := exec.ExecuteStep(context.Background(), Step{
		Action: ActionWrite, Path: path, Content: "created",
	})
	if err != nil {
		t.Fatalf("ExecuteStep write: %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "created" {
		t.Errorf("file content = %q, want %q", data, "created")
	}
}

func TestExecutor_ReadTruncation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.txt")

	// Create a 201-line file.
	var sb2 strings.Builder
	for i := 1; i <= 201; i++ {
		fmt.Fprintf(&sb2, "line %d\n", i)
	}
	os.WriteFile(path, []byte(sb2.String()), 0644)

	sb, _ := sandbox.New(dir)
	exec := NewExecutor(sb)

	result, err := exec.ExecuteStep(context.Background(), Step{Action: ActionRead, Path: path})
	if err != nil {
		t.Fatalf("ExecuteStep read: %v", err)
	}
	if !strings.Contains(result, "[truncated") {
		t.Error("expected [truncated] marker in result for 201-line file")
	}
	if !strings.Contains(result, "line 200") {
		t.Error("expected line 200 in truncated result")
	}
	if strings.Contains(result, "line 201") {
		t.Error("line 201 should not appear in truncated result")
	}
}

func TestExecutor_WriteEmptyContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")

	sb, _ := sandbox.New(dir)
	exec := NewExecutor(sb)

	_, err := exec.ExecuteStep(context.Background(), Step{
		Action: ActionWrite, Path: path, Content: "",
	})
	if err == nil {
		t.Fatal("expected error for empty content write")
	}
	if !strings.Contains(err.Error(), "empty content") {
		t.Errorf("error = %q, want it to mention empty content", err)
	}
	// File should NOT have been created.
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Error("file should not exist after empty content write")
	}
}

func TestExecutor_Delete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "delete-me.txt")
	os.WriteFile(path, []byte("bye"), 0644)

	sb, _ := sandbox.New(dir)
	exec := NewExecutor(sb)

	_, err := exec.ExecuteStep(context.Background(), Step{Action: ActionDelete, Path: path})
	if err != nil {
		t.Fatalf("ExecuteStep delete: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file still exists after delete")
	}
}

func TestExecutor_Move(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	os.WriteFile(src, []byte("move me"), 0644)

	sb, _ := sandbox.New(dir)
	exec := NewExecutor(sb)

	_, err := exec.ExecuteStep(context.Background(), Step{
		Action: ActionMove, Path: src, Destination: dst,
	})
	if err != nil {
		t.Fatalf("ExecuteStep move: %v", err)
	}
	data, _ := os.ReadFile(dst)
	if string(data) != "move me" {
		t.Errorf("moved file content = %q, want %q", data, "move me")
	}
}

func TestExecutor_List(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0644)

	sb, _ := sandbox.New(dir)
	exec := NewExecutor(sb)

	result, err := exec.ExecuteStep(context.Background(), Step{Action: ActionList, Path: dir})
	if err != nil {
		t.Fatalf("ExecuteStep list: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty list result")
	}
}

// --- Executor Relative Path Tests ---

func TestExecutor_RelativePath_List(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)

	sb, _ := sandbox.New(dir)
	exec := NewExecutor(sb)

	// "." should resolve to sandbox root, not process cwd.
	result, err := exec.ExecuteStep(context.Background(), Step{Action: ActionList, Path: "."})
	if err != nil {
		t.Fatalf("ExecuteStep list '.': %v", err)
	}
	if !strings.Contains(result, "a.txt") {
		t.Errorf("expected a.txt in result, got: %s", result)
	}
}

func TestExecutor_RelativePath_Read(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello"), 0644)

	sb, _ := sandbox.New(dir)
	exec := NewExecutor(sb)

	result, err := exec.ExecuteStep(context.Background(), Step{Action: ActionRead, Path: "hello.txt"})
	if err != nil {
		t.Fatalf("ExecuteStep read relative: %v", err)
	}
	if !strings.Contains(result, "5 bytes") {
		t.Errorf("expected 5 bytes, got: %s", result)
	}
}

func TestExecutor_RelativePath_Write(t *testing.T) {
	dir := t.TempDir()

	sb, _ := sandbox.New(dir)
	exec := NewExecutor(sb)

	_, err := exec.ExecuteStep(context.Background(), Step{
		Action: ActionWrite, Path: "new.txt", Content: "created",
	})
	if err != nil {
		t.Fatalf("ExecuteStep write relative: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "new.txt"))
	if string(data) != "created" {
		t.Errorf("file content = %q, want %q", data, "created")
	}
}

func TestExecutor_RelativePath_Move(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "src.txt"), []byte("move me"), 0644)

	sb, _ := sandbox.New(dir)
	exec := NewExecutor(sb)

	_, err := exec.ExecuteStep(context.Background(), Step{
		Action: ActionMove, Path: "src.txt", Destination: "dst.txt",
	})
	if err != nil {
		t.Fatalf("ExecuteStep move relative: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "dst.txt"))
	if string(data) != "move me" {
		t.Errorf("moved file content = %q, want %q", data, "move me")
	}
}

func TestExecutor_RelativePath_Delete(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "del.txt"), []byte("bye"), 0644)

	sb, _ := sandbox.New(dir)
	exec := NewExecutor(sb)

	_, err := exec.ExecuteStep(context.Background(), Step{Action: ActionDelete, Path: "del.txt"})
	if err != nil {
		t.Fatalf("ExecuteStep delete relative: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "del.txt")); !os.IsNotExist(err) {
		t.Error("file still exists after delete")
	}
}

func TestExecutor_RelativePath_Rename(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "old.txt"), []byte("data"), 0644)

	sb, _ := sandbox.New(dir)
	exec := NewExecutor(sb)

	_, err := exec.ExecuteStep(context.Background(), Step{
		Action: ActionRename, Path: "old.txt", Destination: "new.txt",
	})
	if err != nil {
		t.Fatalf("ExecuteStep rename relative: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "new.txt"))
	if string(data) != "data" {
		t.Errorf("renamed file content = %q, want %q", data, "data")
	}
}

func TestExecutor_UnsupportedAction(t *testing.T) {
	dir := t.TempDir()
	sb, _ := sandbox.New(dir)
	exec := NewExecutor(sb)

	_, err := exec.ExecuteStep(context.Background(), Step{
		Action: StepAction("explode"), Path: "file.txt",
	})
	if err == nil {
		t.Fatal("expected error for unsupported action")
	}
	if !strings.Contains(err.Error(), "Unsupported action type") {
		t.Errorf("error = %q, want it to mention Unsupported action type", err)
	}
}

// --- resolvePath Tests ---

func TestResolvePath(t *testing.T) {
	root := t.TempDir()

	sb, err := sandbox.New(root)
	if err != nil {
		t.Fatalf("sandbox.New: %v", err)
	}
	exec := NewExecutor(sb)

	tests := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		{
			name: "bare filename",
			path: "file.txt",
			want: filepath.Join(root, "file.txt"),
		},
		{
			name: "subdirectory path",
			path: "sub/file.txt",
			want: filepath.Join(root, "sub", "file.txt"),
		},
		{
			name: "path with sandbox dir name as subdir",
			path: filepath.Base(root) + "/file.txt",
			want: filepath.Join(root, filepath.Base(root), "file.txt"),
		},
		{
			name:    "path traversal with ..",
			path:    "../escape.txt",
			wantErr: true,
		},
		{
			name:    "absolute path outside sandbox",
			path:    filepath.Join(os.TempDir(), "outside", "escape.txt"),
			wantErr: true,
		},
		{
			name: "absolute path inside sandbox",
			path: filepath.Join(root, "inside.txt"),
			want: filepath.Join(root, "inside.txt"),
		},
		{
			name: "dot-dot prefixed dir name is allowed",
			path: "..hidden/file.txt",
			want: filepath.Join(root, "..hidden", "file.txt"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := exec.resolvePath(tt.path)
			if tt.wantErr {
				if err == nil {
					t.Errorf("resolvePath(%q) = %q, want error", tt.path, got)
				}
				if !errors.Is(err, ErrPathTraversal) {
					t.Errorf("resolvePath(%q) error = %v, want ErrPathTraversal", tt.path, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolvePath(%q) unexpected error: %v", tt.path, err)
			}
			if got != tt.want {
				t.Errorf("resolvePath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// --- isDangerous Tests ---

func TestIsDangerous(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "existing.txt")
	os.WriteFile(existing, []byte("data"), 0644)

	sb, _ := sandbox.New(dir)

	tests := []struct {
		name string
		step Step
		want bool
	}{
		{"delete is always dangerous", Step{Action: ActionDelete, Path: existing}, true},
		{"write new file is dangerous", Step{Action: ActionWrite, Path: filepath.Join(dir, "new.txt")}, true},
		{"write existing file is dangerous", Step{Action: ActionWrite, Path: existing}, true},
		{"move is always dangerous", Step{Action: ActionMove, Path: existing, Destination: filepath.Join(dir, "moved.txt")}, true},
		{"rename is always dangerous", Step{Action: ActionRename, Path: existing, Destination: filepath.Join(dir, "renamed.txt")}, true},
		{"read is not dangerous", Step{Action: ActionRead, Path: existing}, false},
		{"list is not dangerous", Step{Action: ActionList, Path: dir}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDangerous(tt.step, sb)
			if got != tt.want {
				t.Errorf("isDangerous(%q) = %v, want %v", tt.step.Action, got, tt.want)
			}
		})
	}
}

// --- Integration Tests ---

func TestAgent_FullRun(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello world"), 0644)

	planJSON := makePlanJSON([]Step{
		{Action: ActionRead, Description: "read hello.txt", Path: filepath.Join(dir, "hello.txt")},
		{Action: ActionWrite, Description: "write output.txt", Path: filepath.Join(dir, "output.txt"), Content: "done"},
	})

	sb, _ := sandbox.New(dir)
	approver := &mockApprover{decision: Approve}
	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: planJSON},
	})
	ag := New(chain, sb, approver, ApprovalNone, nil)

	result, err := ag.Run(context.Background(), "process files")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Success {
		t.Error("expected Success = true")
	}
	if len(result.StepResults) != 2 {
		t.Errorf("step results = %d, want 2", len(result.StepResults))
	}

	// Verify output file was created.
	data, _ := os.ReadFile(filepath.Join(dir, "output.txt"))
	if string(data) != "done" {
		t.Errorf("output.txt = %q, want %q", data, "done")
	}
}

func TestAgent_ProviderError(t *testing.T) {
	dir := t.TempDir()
	sb, _ := sandbox.New(dir)

	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, err: fmt.Errorf("LLM error")},
	})

	ag := New(chain, sb, &mockApprover{decision: Approve}, ApprovalNone, nil)

	_, err := ag.Run(context.Background(), "do something")
	if err == nil {
		t.Fatal("expected error when provider fails")
	}
}

// --- Executor Tests for New Actions ---

func TestExecutor_Copy(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	os.WriteFile(src, []byte("copy me"), 0644)

	sb, _ := sandbox.New(dir)
	exec := NewExecutor(sb)

	result, err := exec.ExecuteStep(context.Background(), Step{
		Action: ActionCopy, Path: src, Destination: filepath.Join(dir, "dst.txt"),
	})
	if err != nil {
		t.Fatalf("ExecuteStep copy: %v", err)
	}
	if !strings.Contains(result, "Copied") {
		t.Errorf("result = %q, want Copied", result)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "dst.txt"))
	if string(data) != "copy me" {
		t.Errorf("copy content = %q, want %q", data, "copy me")
	}
}

func TestExecutor_Mkdir(t *testing.T) {
	dir := t.TempDir()

	sb, _ := sandbox.New(dir)
	exec := NewExecutor(sb)

	result, err := exec.ExecuteStep(context.Background(), Step{
		Action: ActionMkdir, Path: filepath.Join(dir, "newdir", "sub"),
	})
	if err != nil {
		t.Fatalf("ExecuteStep mkdir: %v", err)
	}
	if !strings.Contains(result, "Created directory") {
		t.Errorf("result = %q, want Created directory", result)
	}
	info, err := os.Stat(filepath.Join(dir, "newdir", "sub"))
	if err != nil {
		t.Fatalf("dir should exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory")
	}
}

func TestExecutor_Mkdir_AlreadyExists(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "existing")
	os.MkdirAll(target, 0755)

	sb, _ := sandbox.New(dir)
	exec := NewExecutor(sb)

	result, err := exec.ExecuteStep(context.Background(), Step{
		Action: ActionMkdir, Path: target,
	})
	if err != nil {
		t.Fatalf("ExecuteStep mkdir existing: %v", err)
	}
	if !strings.Contains(result, "already exists") {
		t.Errorf("result = %q, want message indicating existing directory", result)
	}
}

func TestExecutor_DeleteRecursive(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "subdir")
	os.MkdirAll(subdir, 0755)
	os.WriteFile(filepath.Join(subdir, "child.txt"), []byte("x"), 0644)

	sb, _ := sandbox.New(dir)
	exec := NewExecutor(sb)

	// Without recursive should fail.
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action: ActionDelete, Path: subdir, Recursive: false,
	})
	if err == nil {
		t.Fatal("expected error deleting non-empty dir without recursive")
	}

	// With recursive should succeed.
	_, err = exec.ExecuteStep(context.Background(), Step{
		Action: ActionDelete, Path: subdir, Recursive: true,
	})
	if err != nil {
		t.Fatalf("ExecuteStep delete recursive: %v", err)
	}
	if _, statErr := os.Stat(subdir); !os.IsNotExist(statErr) {
		t.Error("directory still exists after recursive delete")
	}
}

// --- isDangerous Tests for New Actions ---

func TestIsDangerous_NewActions(t *testing.T) {
	dir := t.TempDir()
	sb, _ := sandbox.New(dir)

	tests := []struct {
		name string
		step Step
		want bool
	}{
		{"copy is always dangerous", Step{Action: ActionCopy, Path: "a.txt", Destination: "b.txt"}, true},
		{"mkdir is dangerous", Step{Action: ActionMkdir, Path: "newdir"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDangerous(tt.step, sb)
			if got != tt.want {
				t.Errorf("isDangerous(%q) = %v, want %v", tt.step.Action, got, tt.want)
			}
		})
	}
}

func TestValidatePlanAgainstCommand_DeleteIntent(t *testing.T) {
	plan := &Plan{
		Description: "wrong plan",
		Steps: []Step{
			{Action: ActionList, Description: "list root", Path: "."},
		},
	}

	mismatch := validatePlanAgainstCommand("test7 klasörünü sil", plan)
	if mismatch == "" {
		t.Fatal("expected mismatch for delete command without delete step")
	}

	okPlan := &Plan{
		Description: "delete plan",
		Steps: []Step{
			{Action: ActionDelete, Description: "delete test7", Path: "test7"},
		},
	}
	if mismatch := validatePlanAgainstCommand("test7 klasörünü sil", okPlan); mismatch != "" {
		t.Fatalf("unexpected mismatch for valid delete plan: %s", mismatch)
	}
}

func TestValidatePlanAgainstCommand_ContentIntent(t *testing.T) {
	// Should reject moving/copying the source directory itself when user asks
	// to operate on its contents.
	badPlan := &Plan{
		Description: "move dir itself",
		Steps: []Step{
			{Action: ActionMkdir, Description: "create test7", Path: "test7"},
			{Action: ActionMove, Description: "move test4", Path: "test4", Destination: "test7/test4"},
		},
	}

	mismatch := validatePlanAgainstCommand("test4 deki içeriği test7 klasörüne taşı", badPlan)
	if mismatch == "" {
		t.Fatal("expected mismatch for content-intent command that moves source directory itself")
	}

	goodPlan := &Plan{
		Description: "move contents",
		Steps: []Step{
			{Action: ActionMkdir, Description: "create test7", Path: "test7"},
			{Action: ActionMove, Description: "move file", Path: "test4/merhaba.txt", Destination: "test7/merhaba.txt"},
		},
	}
	if mismatch := validatePlanAgainstCommand("test4 deki içeriği test7 klasörüne taşı", goodPlan); mismatch != "" {
		t.Fatalf("unexpected mismatch for valid content move plan: %s", mismatch)
	}
}

func TestValidatePlanAgainstCommand_ContentDeleteIntent_NoDeleteRequired(t *testing.T) {
	plan := &Plan{
		Description: "mixed plan",
		Steps: []Step{
			{Action: ActionMkdir, Description: "create test8", Path: "test8"},
			{Action: ActionCopy, Description: "copy file", Path: "test4/merhaba.txt", Destination: "test8/gul.txt"},
		},
	}

	command := "test8 klasoru olustur ve merhaba.txt dosyasindaki yazilanlari sil"
	if mismatch := validatePlanAgainstCommand(command, plan); mismatch != "" {
		t.Fatalf("unexpected mismatch for content-delete intent without explicit delete action: %s", mismatch)
	}
}

func TestExecuteStage_DeleteMissingPath_SelectsCandidateBeforeApproval(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "a", "test8"), 0755); err != nil {
		t.Fatalf("mkdir a/test8: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "b", "test8"), 0755); err != nil {
		t.Fatalf("mkdir b/test8: %v", err)
	}

	sb, err := sandbox.New(dir)
	if err != nil {
		t.Fatalf("sandbox.New: %v", err)
	}

	approver := &selectingApprover{
		decision:     Approve,
		selectedPath: "b/test8",
	}

	ag := &Agent{
		sandbox:  sb,
		approver: approver,
		mode:     ApprovalFull,
		executor: NewExecutor(sb),
	}

	plan := &Plan{
		Description: "delete test8",
		Steps: []Step{
			{Action: ActionDelete, Description: "delete test8", Path: "test8", Recursive: true},
		},
	}

	results, err := ag.executeStage(context.Background(), plan)
	if err != nil {
		t.Fatalf("executeStage: %v", err)
	}
	if len(results) != 1 || !strings.Contains(results[0], `"b/test8"`) {
		t.Fatalf("results = %v, want delete result for selected path", results)
	}
	if approver.selectionCalls != 1 {
		t.Fatalf("selectionCalls = %d, want 1", approver.selectionCalls)
	}
	if len(approver.calls) != 1 || approver.calls[0].Stage != "execute" {
		t.Fatalf("approval calls = %+v, want one execute approval after selection", approver.calls)
	}
	if _, err := os.Stat(filepath.Join(dir, "a", "test8")); err != nil {
		t.Fatalf("a/test8 should still exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "b", "test8")); !os.IsNotExist(err) {
		t.Fatalf("b/test8 should be deleted, stat err = %v", err)
	}
}

func TestExecuteStage_DeleteMissingPath_SingleCandidateStillRequiresSelection(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "x", "test8"), 0755); err != nil {
		t.Fatalf("mkdir x/test8: %v", err)
	}

	sb, err := sandbox.New(dir)
	if err != nil {
		t.Fatalf("sandbox.New: %v", err)
	}

	approver := &selectingApprover{
		decision:     Approve,
		selectedPath: "x/test8",
	}

	ag := &Agent{
		sandbox:  sb,
		approver: approver,
		mode:     ApprovalFull,
		executor: NewExecutor(sb),
	}

	plan := &Plan{
		Description: "delete test8",
		Steps: []Step{
			{Action: ActionDelete, Description: "delete test8", Path: "test8", Recursive: true},
		},
	}

	results, err := ag.executeStage(context.Background(), plan)
	if err != nil {
		t.Fatalf("executeStage: %v", err)
	}
	if len(results) != 1 || !strings.Contains(results[0], `"x/test8"`) {
		t.Fatalf("results = %v, want delete result for selected single candidate", results)
	}
	if approver.selectionCalls != 1 {
		t.Fatalf("selectionCalls = %d, want 1 (single candidate must still require explicit selection)", approver.selectionCalls)
	}
	if len(approver.calls) != 1 || approver.calls[0].Stage != "execute" {
		t.Fatalf("approval calls = %+v, want one execute approval after selection", approver.calls)
	}
	if _, err := os.Stat(filepath.Join(dir, "x", "test8")); !os.IsNotExist(err) {
		t.Fatalf("x/test8 should be deleted, stat err = %v", err)
	}
}

func TestExecuteStage_DeleteMissingPath_CandidateWithoutSelectorFails(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "x", "test8"), 0755); err != nil {
		t.Fatalf("mkdir x/test8: %v", err)
	}

	sb, err := sandbox.New(dir)
	if err != nil {
		t.Fatalf("sandbox.New: %v", err)
	}

	approver := &mockApprover{decision: Approve}
	ag := &Agent{
		sandbox:  sb,
		approver: approver,
		mode:     ApprovalFull,
		executor: NewExecutor(sb),
	}

	plan := &Plan{
		Steps: []Step{
			{Action: ActionDelete, Description: "delete missing", Path: "test8", Recursive: true},
		},
	}

	_, err = ag.executeStage(context.Background(), plan)
	if err == nil {
		t.Fatal("expected error for delete candidate without selector")
	}
	if !strings.Contains(err.Error(), "requires explicit path selection") {
		t.Fatalf("error = %v, want explicit path selection message", err)
	}
	if len(approver.calls) != 0 {
		t.Fatalf("approval calls = %d, want 0 (should fail before execute approval)", len(approver.calls))
	}
}

func TestExecuteStage_DeleteMissingPath_NoCandidateSkipsApproval(t *testing.T) {
	dir := t.TempDir()
	sb, err := sandbox.New(dir)
	if err != nil {
		t.Fatalf("sandbox.New: %v", err)
	}

	approver := &mockApprover{decision: Approve}
	ag := &Agent{
		sandbox:  sb,
		approver: approver,
		mode:     ApprovalFull,
		executor: NewExecutor(sb),
	}

	plan := &Plan{
		Steps: []Step{
			{Action: ActionDelete, Description: "delete missing", Path: "test8", Recursive: true},
		},
	}

	_, err = ag.executeStage(context.Background(), plan)
	if err == nil {
		t.Fatal("expected error for missing delete target")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error = %v, want os.ErrNotExist", err)
	}
	if len(approver.calls) != 0 {
		t.Fatalf("approval calls = %d, want 0 (should fail before execute approval)", len(approver.calls))
	}
}

// --- Revision Tests ---

// recordingLLMProvider captures the messages sent to it and returns a
// configurable response.
type recordingLLMProvider struct {
	name      string
	available bool
	response  string
	messages  [][]types.Message // one entry per Chat call
}

func (r *recordingLLMProvider) Chat(_ context.Context, msgs []types.Message, _ []provider.ToolSpec) (string, error) {
	r.messages = append(r.messages, msgs)
	return r.response, nil
}

func (r *recordingLLMProvider) StreamChat(_ context.Context, _ []types.Message) (<-chan string, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *recordingLLMProvider) Name() string    { return r.name }
func (r *recordingLLMProvider) Available() bool { return r.available }

// revisingApprover returns Revise for the first N plan approvals, then
// Approve. It implements RevisionPrompter and returns preset feedback texts.
type revisingApprover struct {
	revisionsLeft int
	feedbacks     []string // feedbacks to return, popped in order
	feedbackIndex int
	calls         []ApprovalRequest
}

func (r *revisingApprover) RequestApproval(_ context.Context, req ApprovalRequest) (Decision, error) {
	r.calls = append(r.calls, req)
	if req.Stage == "plan" && r.revisionsLeft > 0 {
		r.revisionsLeft--
		return Revise, nil
	}
	return Approve, nil
}

func (r *revisingApprover) PromptRevision(_ context.Context) (string, error) {
	if r.feedbackIndex < len(r.feedbacks) {
		fb := r.feedbacks[r.feedbackIndex]
		r.feedbackIndex++
		return fb, nil
	}
	return "", nil
}

func TestPlanStage_RevisionFeedbackIncludedInCommand(t *testing.T) {
	dir := t.TempDir()
	sb, _ := sandbox.New(dir)

	planJSON := makePlanJSON([]Step{
		{Action: ActionList, Description: "list", Path: "."},
	})
	recorder := &recordingLLMProvider{
		name: "mock", available: true, response: planJSON,
	}
	chain := provider.NewFallbackChain([]provider.LLMProvider{recorder})

	approver := &revisingApprover{
		revisionsLeft: 1,
		feedbacks:     []string{"use verbose output"},
	}

	ag := New(chain, sb, approver, ApprovalFull, nil)

	// planStage is called internally by Run, but we test it directly.
	_, err := ag.planStage(context.Background(), "list files", nil)
	if err != nil {
		t.Fatalf("planStage: %v", err)
	}

	// Should have 2 Chat calls: first plan + revision re-plan.
	if len(recorder.messages) != 2 {
		t.Fatalf("Chat calls = %d, want 2", len(recorder.messages))
	}

	// The second call's user message should contain the revision feedback.
	secondUserMsg := recorder.messages[1][1].Content
	if !strings.Contains(secondUserMsg, "Revision: use verbose output") {
		t.Errorf("second plan command = %q, want it to contain revision feedback", secondUserMsg)
	}
}

func TestPlanStage_MaxRevisionsError(t *testing.T) {
	dir := t.TempDir()
	sb, _ := sandbox.New(dir)

	planJSON := makePlanJSON([]Step{
		{Action: ActionList, Description: "list", Path: "."},
	})
	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: planJSON},
	})

	approver := &revisingApprover{
		revisionsLeft: 10, // more than maxRevisions
		feedbacks:     []string{"try 1", "try 2", "try 3", "try 4"},
	}

	ag := New(chain, sb, approver, ApprovalFull, nil)

	_, err := ag.planStage(context.Background(), "list files", nil)
	if err == nil {
		t.Fatal("expected error after exceeding maxRevisions")
	}
	if !strings.Contains(err.Error(), "maximum revisions") {
		t.Errorf("error = %q, want it to mention maximum revisions", err)
	}
	// Flow: plan1->revise(1), plan2->revise(2), plan3->revise(3), plan4->revise->error.
	// The 4th plan is created, approval returns Revise, but the counter check
	// blocks it. So there are 4 plan approval calls total (3 successful revisions
	// + the 4th that triggers the error).
	planCalls := 0
	for _, c := range approver.calls {
		if c.Stage == "plan" {
			planCalls++
		}
	}
	if planCalls != 4 {
		t.Errorf("plan approval calls = %d, want 4 (maxRevisions + 1 for the blocked attempt)", planCalls)
	}
}

func TestPlanStage_EmptyRevisionFeedbackReplansWithOriginal(t *testing.T) {
	dir := t.TempDir()
	sb, _ := sandbox.New(dir)

	planJSON := makePlanJSON([]Step{
		{Action: ActionList, Description: "list", Path: "."},
	})
	recorder := &recordingLLMProvider{
		name: "mock", available: true, response: planJSON,
	}
	chain := provider.NewFallbackChain([]provider.LLMProvider{recorder})

	approver := &revisingApprover{
		revisionsLeft: 1,
		feedbacks:     []string{""}, // empty feedback
	}

	ag := New(chain, sb, approver, ApprovalFull, nil)

	_, err := ag.planStage(context.Background(), "list files", nil)
	if err != nil {
		t.Fatalf("planStage: %v", err)
	}

	if len(recorder.messages) != 2 {
		t.Fatalf("Chat calls = %d, want 2", len(recorder.messages))
	}

	// Empty feedback should re-plan with original command (no "Revision:" suffix).
	secondUserMsg := recorder.messages[1][1].Content
	if strings.Contains(secondUserMsg, "Revision:") {
		t.Errorf("second plan command should not contain Revision: with empty feedback, got %q", secondUserMsg)
	}
}

// --- Conversation History Tests ---

func TestAgent_ConversationHistory_AddsMessages(t *testing.T) {
	dir := t.TempDir()
	sb, _ := sandbox.New(dir)

	planJSON := makePlanJSON([]Step{
		{Action: ActionList, Description: "list", Path: dir},
	})

	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: planJSON},
	})
	ag := New(chain, sb, &mockApprover{decision: Approve}, ApprovalNone, nil)

	// Run first command.
	_, err := ag.Run(context.Background(), "first command")
	if err != nil {
		t.Fatalf("Run 1: %v", err)
	}

	hist := ag.History()
	// Should have user + assistant = 2 messages.
	if len(hist) != 2 {
		t.Fatalf("history len = %d, want 2", len(hist))
	}
	if hist[0].Role != types.RoleUser || hist[0].Content != "first command" {
		t.Errorf("hist[0] = %+v, want user/first command", hist[0])
	}
	if hist[1].Role != types.RoleAssistant {
		t.Errorf("hist[1].Role = %q, want assistant", hist[1].Role)
	}

	// Run second command.
	_, err = ag.Run(context.Background(), "second command")
	if err != nil {
		t.Fatalf("Run 2: %v", err)
	}

	hist = ag.History()
	// Should have 4 messages: user1, assistant1, user2, assistant2.
	if len(hist) != 4 {
		t.Fatalf("history len = %d, want 4", len(hist))
	}
	if hist[2].Role != types.RoleUser || hist[2].Content != "second command" {
		t.Errorf("hist[2] = %+v, want user/second command", hist[2])
	}
}

func TestAgent_ConversationHistory_MaxTurns(t *testing.T) {
	dir := t.TempDir()
	sb, _ := sandbox.New(dir)

	planJSON := makePlanJSON([]Step{
		{Action: ActionList, Description: "list", Path: dir},
	})

	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: planJSON},
	})
	ag := New(chain, sb, &mockApprover{decision: Approve}, ApprovalNone, nil)

	// Run 25 commands (exceeds maxConversationTurns=20).
	for i := 0; i < 25; i++ {
		_, err := ag.Run(context.Background(), fmt.Sprintf("command %d", i))
		if err != nil {
			t.Fatalf("Run %d: %v", i, err)
		}
	}

	hist := ag.History()
	// Should be capped at maxConversationTurns*2 = 40 messages.
	maxMessages := 20 * 2 // maxConversationTurns * 2
	if len(hist) > maxMessages {
		t.Errorf("history len = %d, want <= %d", len(hist), maxMessages)
	}
}

func TestAgent_ClearHistory(t *testing.T) {
	dir := t.TempDir()
	sb, _ := sandbox.New(dir)

	planJSON := makePlanJSON([]Step{
		{Action: ActionList, Description: "list", Path: dir},
	})

	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: planJSON},
	})
	ag := New(chain, sb, &mockApprover{decision: Approve}, ApprovalNone, nil)

	_, _ = ag.Run(context.Background(), "something")
	if len(ag.History()) == 0 {
		t.Fatal("expected non-empty history after run")
	}

	ag.ClearHistory()
	if len(ag.History()) != 0 {
		t.Errorf("history len = %d after clear, want 0", len(ag.History()))
	}
}

func TestAgent_SetHistory(t *testing.T) {
	dir := t.TempDir()
	sb, _ := sandbox.New(dir)

	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: makePlanJSON([]Step{
			{Action: ActionList, Description: "list", Path: dir},
		})},
	})
	ag := New(chain, sb, &mockApprover{decision: Approve}, ApprovalNone, nil)

	// Set external history.
	prev := []types.Message{
		{Role: types.RoleUser, Content: "prev question"},
		{Role: types.RoleAssistant, Content: "prev answer"},
	}
	ag.SetHistory(prev)

	hist := ag.History()
	if len(hist) != 2 {
		t.Fatalf("history len = %d, want 2", len(hist))
	}

	// Mutating original should not affect agent's copy.
	prev[0].Content = "modified"
	if ag.History()[0].Content != "prev question" {
		t.Error("SetHistory should copy, not reference")
	}
}

func TestAgent_EmptyStepsPlan(t *testing.T) {
	dir := t.TempDir()
	sb, _ := sandbox.New(dir)

	// LLM returns a plan with description but no steps (meta-question response).
	planJSON := makePlanJSON([]Step{})

	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: planJSON},
	})
	ag := New(chain, sb, &mockApprover{decision: Approve}, ApprovalNone, nil)

	result, err := ag.Run(context.Background(), "what did I ask?")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Success {
		t.Error("expected Success = true for empty-steps plan")
	}
	if len(result.StepResults) != 0 {
		t.Errorf("step results = %d, want 0", len(result.StepResults))
	}
}

// --- Skill Integration Tests ---

func TestAgent_WithSkillStore(t *testing.T) {
	dir := t.TempDir()
	sb, _ := sandbox.New(dir)

	planJSON := makePlanJSON([]Step{
		{Action: ActionList, Description: "list", Path: dir},
	})

	recorder := &recordingLLMProvider{
		name: "mock", available: true, response: planJSON,
	}
	chain := provider.NewFallbackChain([]provider.LLMProvider{recorder})

	store := skill.NewStore()
	store.Upsert(&skill.Skill{
		Metadata: skill.SkillMetadata{
			Name:        "file-organizer",
			Description: "Organizes files by type",
			AutoTrigger: true,
		},
		Content: "Sort files into directories based on extension.",
	})

	ag := New(chain, sb, &mockApprover{decision: Approve}, ApprovalNone, store)

	_, err := ag.Run(context.Background(), "organize files in this directory")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// The system prompt sent to LLM should contain <active_skills>.
	if len(recorder.messages) == 0 {
		t.Fatal("expected at least one Chat call")
	}
	sysMsg := recorder.messages[0][0].Content
	if !strings.Contains(sysMsg, "## Skills") {
		t.Error("system prompt should contain skills section when skills match")
	}
	if !strings.Contains(sysMsg, "file-organizer") {
		t.Error("system prompt should contain matched skill name")
	}
}

func TestAgent_WithoutSkillStore(t *testing.T) {
	dir := t.TempDir()
	sb, _ := sandbox.New(dir)

	planJSON := makePlanJSON([]Step{
		{Action: ActionList, Description: "list", Path: dir},
	})

	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: planJSON},
	})

	// nil skill store should not cause errors.
	ag := New(chain, sb, &mockApprover{decision: Approve}, ApprovalNone, nil)

	result, err := ag.Run(context.Background(), "list files")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Success {
		t.Error("expected Success = true with nil skill store")
	}
}

func TestAgent_SkillMatchAndInject(t *testing.T) {
	dir := t.TempDir()
	sb, _ := sandbox.New(dir)

	planJSON := makePlanJSON([]Step{
		{Action: ActionList, Description: "list", Path: dir},
	})

	recorder := &recordingLLMProvider{
		name: "mock", available: true, response: planJSON,
	}
	chain := provider.NewFallbackChain([]provider.LLMProvider{recorder})

	store := skill.NewStore()
	store.Upsert(&skill.Skill{
		Metadata: skill.SkillMetadata{
			Name:        "summarizer",
			Description: "Summarizes documents and text",
			AutoTrigger: true,
		},
		Content: "Create a concise summary.",
	})
	store.Upsert(&skill.Skill{
		Metadata: skill.SkillMetadata{
			Name:        "file-organizer",
			Description: "Organizes files by type",
			AutoTrigger: true,
		},
		Content: "Sort files into directories.",
	})
	store.Upsert(&skill.Skill{
		Metadata: skill.SkillMetadata{
			Name:        "manual-only",
			Description: "Manual skill",
			AutoTrigger: false,
		},
		Content: "Should not appear.",
	})

	ag := New(chain, sb, &mockApprover{decision: Approve}, ApprovalNone, store)

	// "summarizes" is a token from the summarizer description, and it appears
	// as a substring in "summarizes" in the command.
	_, err := ag.Run(context.Background(), "summarizes this document")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(recorder.messages) == 0 {
		t.Fatal("expected at least one Chat call")
	}
	sysMsg := recorder.messages[0][0].Content
	if !strings.Contains(sysMsg, "## Skills") {
		t.Error("system prompt should contain skills section")
	}
	if !strings.Contains(sysMsg, "summarizer") {
		t.Error("system prompt should contain matched summarizer skill")
	}
	// file-organizer should NOT match "summarizes this document".
	if strings.Contains(sysMsg, "file-organizer") {
		t.Error("system prompt should NOT contain non-matching file-organizer skill")
	}
	// manual-only should NOT appear.
	if strings.Contains(sysMsg, "manual-only") {
		t.Error("system prompt should NOT contain AutoTrigger=false skill")
	}
}

func TestAgent_SkillApprovalGate_FullMode(t *testing.T) {
	dir := t.TempDir()
	sb, _ := sandbox.New(dir)

	planJSON := makePlanJSON([]Step{
		{Action: ActionList, Description: "list", Path: dir},
	})

	recorder := &recordingLLMProvider{
		name: "mock", available: true, response: planJSON,
	}
	chain := provider.NewFallbackChain([]provider.LLMProvider{recorder})

	store := skill.NewStore()
	store.Upsert(&skill.Skill{
		Metadata: skill.SkillMetadata{
			Name:        "file-organizer",
			Description: "Organizes files by type",
			AutoTrigger: true,
		},
		Content: "Sort files.",
	})

	approver := &sequenceApprover{
		// skill=approve, plan=approve, execute=approve, result=approve
		decisions: []Decision{Approve, Approve, Approve, Approve},
	}
	ag := New(chain, sb, approver, ApprovalFull, store)

	_, err := ag.Run(context.Background(), "organize files")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// First approval call should be skill stage.
	if len(approver.calls) < 1 {
		t.Fatal("expected at least one approval call")
	}
	if approver.calls[0].Stage != "skill" {
		t.Errorf("first approval stage = %q, want %q", approver.calls[0].Stage, "skill")
	}
	if len(approver.calls[0].Items) != 1 || approver.calls[0].Items[0] != "file-organizer" {
		t.Errorf("skill approval items = %v, want [file-organizer]", approver.calls[0].Items)
	}
}

func TestAgent_SkillApprovalGate_Rejected(t *testing.T) {
	dir := t.TempDir()
	sb, _ := sandbox.New(dir)

	planJSON := makePlanJSON([]Step{
		{Action: ActionList, Description: "list", Path: dir},
	})

	recorder := &recordingLLMProvider{
		name: "mock", available: true, response: planJSON,
	}
	chain := provider.NewFallbackChain([]provider.LLMProvider{recorder})

	store := skill.NewStore()
	store.Upsert(&skill.Skill{
		Metadata: skill.SkillMetadata{
			Name:        "file-organizer",
			Description: "Organizes files by type",
			AutoTrigger: true,
		},
		Content: "Sort files.",
	})

	approver := &sequenceApprover{
		// skill=reject, then plan+execute+result auto-approve
		decisions: []Decision{Reject, Approve, Approve, Approve},
	}
	ag := New(chain, sb, approver, ApprovalFull, store)

	_, err := ag.Run(context.Background(), "organize files")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// System prompt should NOT contain <active_skills> since skill was rejected.
	if len(recorder.messages) == 0 {
		t.Fatal("expected at least one Chat call")
	}
	sysMsg := recorder.messages[0][0].Content
	if strings.Contains(sysMsg, "<active_skills>") {
		t.Error("system prompt should NOT contain <active_skills> when skill is rejected")
	}
}

func TestAgent_SkillApprovalGate_NoneMode_Skipped(t *testing.T) {
	dir := t.TempDir()
	sb, _ := sandbox.New(dir)

	planJSON := makePlanJSON([]Step{
		{Action: ActionList, Description: "list", Path: dir},
	})

	recorder := &recordingLLMProvider{
		name: "mock", available: true, response: planJSON,
	}
	chain := provider.NewFallbackChain([]provider.LLMProvider{recorder})

	store := skill.NewStore()
	store.Upsert(&skill.Skill{
		Metadata: skill.SkillMetadata{
			Name:        "file-organizer",
			Description: "Organizes files by type",
			AutoTrigger: true,
		},
		Content: "Sort files.",
	})

	approver := &mockApprover{decision: Approve}
	ag := New(chain, sb, approver, ApprovalNone, store)

	_, err := ag.Run(context.Background(), "organize files")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// In none mode, no approval calls at all.
	if len(approver.calls) != 0 {
		t.Errorf("approval calls = %d, want 0 in none mode", len(approver.calls))
	}

	// But skills should still be injected.
	if len(recorder.messages) == 0 {
		t.Fatal("expected at least one Chat call")
	}
	sysMsg := recorder.messages[0][0].Content
	if !strings.Contains(sysMsg, "## Skills") {
		t.Error("system prompt should contain skills section in none mode (auto-approved)")
	}
}

// --- ForceSkills Tests ---

func TestAgent_ManualActivation(t *testing.T) {
	dir := t.TempDir()
	sb, _ := sandbox.New(dir)

	planJSON := makePlanJSON([]Step{
		{Action: ActionList, Description: "list", Path: dir},
	})

	recorder := &recordingLLMProvider{
		name: "mock", available: true, response: planJSON,
	}
	chain := provider.NewFallbackChain([]provider.LLMProvider{recorder})

	store := skill.NewStore()
	// manual-skill has AutoTrigger=false — normally won't match.
	store.Upsert(&skill.Skill{
		Metadata: skill.SkillMetadata{
			Name:        "manual-skill",
			Description: "Manual only skill",
			AutoTrigger: false,
		},
		Content: "Manual content.",
	})
	store.Upsert(&skill.Skill{
		Metadata: skill.SkillMetadata{
			Name:        "auto-skill",
			Description: "Organizes files by type",
			AutoTrigger: true,
		},
		Content: "Auto content.",
	})

	ag := New(chain, sb, &mockApprover{decision: Approve}, ApprovalNone, store)
	ag.SetForceSkills([]string{"manual-skill"})

	_, err := ag.Run(context.Background(), "do something unrelated")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(recorder.messages) == 0 {
		t.Fatal("expected at least one Chat call")
	}
	sysMsg := recorder.messages[0][0].Content
	// Forced skill should be injected.
	if !strings.Contains(sysMsg, "manual-skill") {
		t.Error("system prompt should contain forced manual-skill")
	}
	if !strings.Contains(sysMsg, "Manual content.") {
		t.Error("system prompt should contain forced skill content")
	}
	// auto-skill should NOT be injected (Match was skipped).
	if strings.Contains(sysMsg, "auto-skill") {
		t.Error("system prompt should NOT contain auto-skill when forceSkills is set")
	}
}

// sequentialMockProvider responds with successive entries from responses slice.
type sequentialMockProvider struct {
	responses []string
	callIdx   int
}

func (m *sequentialMockProvider) Chat(_ context.Context, _ []types.Message, _ []provider.ToolSpec) (string, error) {
	idx := m.callIdx
	if idx >= len(m.responses) {
		idx = len(m.responses) - 1
	}
	m.callIdx++
	return m.responses[idx], nil
}

func (m *sequentialMockProvider) StreamChat(_ context.Context, _ []types.Message) (<-chan string, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *sequentialMockProvider) Name() string    { return "sequential-mock" }
func (m *sequentialMockProvider) Available() bool { return true }

func TestAgent_ForceSkillsCleared(t *testing.T) {
	dir := t.TempDir()
	sb, _ := sandbox.New(dir)

	planJSON := makePlanJSON([]Step{
		{Action: ActionList, Description: "list", Path: dir},
	})

	recorder := &recordingLLMProvider{
		name: "mock", available: true, response: planJSON,
	}
	chain := provider.NewFallbackChain([]provider.LLMProvider{recorder})

	store := skill.NewStore()
	store.Upsert(&skill.Skill{
		Metadata: skill.SkillMetadata{
			Name:        "test-skill",
			Description: "Test skill",
			AutoTrigger: false,
		},
		Content: "Test content.",
	})

	ag := New(chain, sb, &mockApprover{decision: Approve}, ApprovalNone, store)
	ag.SetForceSkills([]string{"test-skill"})

	// First run: forced skill should be used.
	_, err := ag.Run(context.Background(), "first command")
	if err != nil {
		t.Fatalf("Run 1: %v", err)
	}

	// Second run: forceSkills should have been cleared.
	_, err = ag.Run(context.Background(), "second command")
	if err != nil {
		t.Fatalf("Run 2: %v", err)
	}

	// Second Chat call's system prompt should NOT contain test-skill.
	if len(recorder.messages) < 2 {
		t.Fatal("expected at least two Chat calls")
	}
	sysMsg2 := recorder.messages[1][0].Content
	if strings.Contains(sysMsg2, "test-skill") {
		t.Error("forceSkills should be cleared after first run")
	}
}

// --- dangerReason Tests ---

func TestDangerReason(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "exists.txt")
	os.WriteFile(existing, []byte("data"), 0644)

	sb, _ := sandbox.New(dir)

	tests := []struct {
		name    string
		step    Step
		wantSub string // substring expected in result; "" means result must be empty
	}{
		{"delete", Step{Action: ActionDelete, Path: existing}, "permanently removes"},
		{"write existing", Step{Action: ActionWrite, Path: existing}, "overwrites existing"},
		{"write new", Step{Action: ActionWrite, Path: filepath.Join(dir, "new.txt")}, "creates new"},
		{"move", Step{Action: ActionMove, Path: existing}, "relocates"},
		{"rename", Step{Action: ActionRename, Path: existing}, "renames"},
		{"copy", Step{Action: ActionCopy, Path: existing}, "copies"},
		{"mkdir", Step{Action: ActionMkdir, Path: filepath.Join(dir, "newdir")}, "creates new directory"},
		{"read", Step{Action: ActionRead, Path: existing}, ""},
		{"list", Step{Action: ActionList, Path: dir}, ""},
		// Paths outside sandbox must not trigger os.Stat; return generic reason.
		{"write outside sandbox (absolute)", Step{Action: ActionWrite, Path: filepath.Join(filepath.Dir(dir), "outside.txt")}, "writes to file"},
		{"write traversal (relative)", Step{Action: ActionWrite, Path: "../../etc/passwd"}, "writes to file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dangerReason(tt.step, sb)
			if tt.wantSub == "" {
				if got != "" {
					t.Errorf("dangerReason = %q, want empty string", got)
				}
				return
			}
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("dangerReason = %q, want it to contain %q", got, tt.wantSub)
			}
		})
	}
}

// --- displayPath Tests ---

func TestDisplayPath(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")

	outside := filepath.Join(filepath.Dir(root), "other.txt")

	tests := []struct {
		name    string
		absPath string
		want    string
	}{
		{"file in root", filepath.Join(root, "file.txt"), "./file.txt"},
		{"file in subdir", filepath.Join(sub, "file.txt"), "./sub/file.txt"},
		{"root itself", root, "."},
		{"outside root", outside, outside},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := displayPath(tt.absPath, root)
			if got != tt.want {
				t.Errorf("displayPath = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- Executor Friendly Error Tests ---

func TestExecutor_FriendlyError_OutsideSandbox(t *testing.T) {
	dir := t.TempDir()
	sb, _ := sandbox.New(dir)
	exec := NewExecutor(sb)

	outsidePath := filepath.Join(filepath.Dir(dir), "outside.txt")
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action: ActionRead,
		Path:   outsidePath,
	})
	if err == nil {
		t.Fatal("expected error for path outside sandbox")
	}
	if !strings.Contains(err.Error(), "Access denied") {
		t.Errorf("error = %q, want it to mention 'Access denied'", err)
	}
}

func TestExecutor_FriendlyError_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	sb, _ := sandbox.New(dir)
	exec := NewExecutor(sb)

	_, err := exec.ExecuteStep(context.Background(), Step{
		Action: ActionRead,
		Path:   "nonexistent.txt",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	if !strings.Contains(err.Error(), "File not found") {
		t.Errorf("error = %q, want it to mention 'File not found'", err)
	}
}

func TestExecutor_FriendlyError_ReadOnlyDir(t *testing.T) {
	dir := t.TempDir()
	roDir := filepath.Join(dir, "readonly")
	os.MkdirAll(roDir, 0755)

	sb, _ := sandbox.New(dir, sandbox.WithReadOnlyDirs(roDir))
	exec := NewExecutor(sb)

	_, err := exec.ExecuteStep(context.Background(), Step{
		Action:  ActionWrite,
		Path:    filepath.Join(roDir, "file.txt"),
		Content: "data",
	})
	if err == nil {
		t.Fatal("expected error for write to read-only directory")
	}
	if !strings.Contains(err.Error(), "Write denied") {
		t.Errorf("error = %q, want it to mention 'Write denied'", err)
	}
}

func TestAgent_UnsupportedAction(t *testing.T) {
	planJSON := makePlanJSON([]Step{
		{Action: "teleport", Description: "teleport something", Path: "x"},
	})
	ag, _ := setupAgent(t, planJSON, Approve, ApprovalNone)

	result, err := ag.Run(context.Background(), "teleport something")

	if err == nil {
		t.Fatal("expected error for unsupported action")
	}
	if result != nil && result.Success {
		t.Error("expected Success = false for unsupported action")
	}
	if !strings.Contains(err.Error(), "Unsupported action type") {
		t.Errorf("error = %q, want to contain 'Unsupported action type'", err)
	}
}

func TestAgent_ReviseFlow(t *testing.T) {
	dir := t.TempDir()
	sb, _ := sandbox.New(dir)

	plan1JSON := makePlanJSON([]Step{
		{Action: ActionList, Description: "first plan step", Path: "."},
	})
	plan2JSON := makePlanJSON([]Step{
		{Action: ActionList, Description: "revised plan step", Path: "."},
	})

	seqProvider := &sequentialMockProvider{responses: []string{plan1JSON, plan2JSON}}
	chain := provider.NewFallbackChain([]provider.LLMProvider{seqProvider})
	approver := &revisingApprover{
		revisionsLeft: 1,
		feedbacks:     []string{"use revised plan step instead"},
	}
	ag := New(chain, sb, approver, ApprovalFull, nil)

	result, err := ag.Run(context.Background(), "list files")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Count plan approval calls: should be 2 (first Revise, second Approve).
	planCalls := 0
	for _, c := range approver.calls {
		if c.Stage == "plan" {
			planCalls++
		}
	}
	if planCalls != 2 {
		t.Errorf("plan approval called %d times, want 2", planCalls)
	}

	// The executed plan should be the second (revised) plan.
	if result.Plan == nil || len(result.Plan.Steps) == 0 {
		t.Fatal("expected a plan with steps")
	}
	if result.Plan.Steps[0].Description != "revised plan step" {
		t.Errorf("executed step = %q, want %q", result.Plan.Steps[0].Description, "revised plan step")
	}
}
