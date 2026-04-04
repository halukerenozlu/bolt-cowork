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

type mockLLMProvider struct {
	name      string
	available bool
	response  string
	err       error
}

func (m *mockLLMProvider) Chat(_ context.Context, _ []types.Message) (string, error) {
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
	ag := New(chain, sb, approver, mode)
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

	ag := New(chain, sb, approver, mode)
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
		{"full/plan", ApprovalFull, "plan", false, true},
		{"full/execute", ApprovalFull, "execute", false, true},
		{"full/execute/dangerous", ApprovalFull, "execute", true, true},
		{"full/result", ApprovalFull, "result", false, true},

		{"plan-only/plan", ApprovalPlanOnly, "plan", false, true},
		{"plan-only/execute", ApprovalPlanOnly, "execute", false, false},
		{"plan-only/result", ApprovalPlanOnly, "result", false, false},

		{"dangerous-only/plan", ApprovalDangerousOnly, "plan", false, false},
		{"dangerous-only/execute", ApprovalDangerousOnly, "execute", false, false},
		{"dangerous-only/execute/dangerous", ApprovalDangerousOnly, "execute", true, true},
		{"dangerous-only/result", ApprovalDangerousOnly, "result", false, false},

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

	planJSON := makePlanJSON([]Step{
		{Action: ActionList, Description: "list root", Path: dir},
	})

	approver := &mockApprover{decision: Approve}
	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: planJSON},
	})
	ag := New(chain, sb, approver, ApprovalFull)

	_, err := ag.Run(context.Background(), "list files")
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
	ag := New(chain, sb, approver, ApprovalPlanOnly)

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
	ag := New(chain, sb, approver, ApprovalDangerousOnly)

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
	ag := New(chain, sb, approver, ApprovalNone)

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
	planJSON := makePlanJSON([]Step{
		{Action: ActionList, Description: "list", Path: "."},
	})

	// Use a custom approver that approves plan then rejects step.
	customApprover := &sequenceApprover{
		decisions: []Decision{Approve, Reject}, // plan=approve, step=reject
	}
	ag, _ := setupAgentWithApprover(t, planJSON, customApprover, ApprovalFull)

	_, err := ag.Run(context.Background(), "list files")
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

	planJSON := makePlanJSON([]Step{
		{Action: ActionList, Description: "list root", Path: dir},
	})

	// Approve plan, approve step, reject result.
	approver := &sequenceApprover{
		decisions: []Decision{Approve, Approve, Reject},
	}
	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: planJSON},
	})
	ag := New(chain, sb, approver, ApprovalFull)

	_, err := ag.Run(context.Background(), "list files")
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
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0644)

	sb, _ := sandbox.New(dir)
	planJSON := makePlanJSON([]Step{
		{Action: ActionRead, Description: "read a", Path: filepath.Join(dir, "a.txt")},
		{Action: ActionRead, Description: "read b", Path: filepath.Join(dir, "b.txt")},
	})

	// Approve plan, ApproveAll on first step, then result should be auto-approved for remaining steps.
	approver := &sequenceApprover{
		decisions: []Decision{Approve, ApproveAll, Approve}, // plan, first-step=approveAll, result
	}
	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: planJSON},
	})
	ag := New(chain, sb, approver, ApprovalFull)

	result, err := ag.Run(context.Background(), "read files")
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

// --- Planner Tests ---

func TestPlanner_CreatePlan(t *testing.T) {
	planJSON := makePlanJSON([]Step{
		{Action: ActionRead, Description: "read file", Path: "test.txt"},
	})

	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: planJSON},
	})
	planner := NewPlanner(chain)

	plan, err := planner.CreatePlan(context.Background(), "read test.txt", "test.txt\n")
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

	_, err := planner.CreatePlan(context.Background(), "anything", "")
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

			plan, err := planner.CreatePlan(context.Background(), "list", "")
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

	plan, err := planner.CreatePlan(context.Background(), "list", "")
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

	_, err := planner.CreatePlan(context.Background(), "anything", "")
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
		{"write new file is not dangerous", Step{Action: ActionWrite, Path: filepath.Join(dir, "new.txt")}, false},
		{"write existing file is dangerous", Step{Action: ActionWrite, Path: existing}, true},
		{"move to non-existing dest is not dangerous", Step{Action: ActionMove, Path: existing, Destination: filepath.Join(dir, "moved.txt")}, false},
		{"move to existing dest is dangerous", Step{Action: ActionMove, Path: existing, Destination: existing}, true},
		{"rename to non-existing dest is not dangerous", Step{Action: ActionRename, Path: existing, Destination: filepath.Join(dir, "renamed.txt")}, false},
		{"rename to existing dest is dangerous", Step{Action: ActionRename, Path: existing, Destination: existing}, true},
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
	ag := New(chain, sb, approver, ApprovalNone)

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

	ag := New(chain, sb, &mockApprover{decision: Approve}, ApprovalNone)

	_, err := ag.Run(context.Background(), "do something")
	if err == nil {
		t.Fatal("expected error when provider fails")
	}
}
