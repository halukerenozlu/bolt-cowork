package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/halukerenozlu/bolt-cowork/internal/provider"
	"github.com/halukerenozlu/bolt-cowork/internal/sandbox"
	"github.com/halukerenozlu/bolt-cowork/internal/skill"
)

func TestE2E_SimpleFileCreate(t *testing.T) {
	dir := t.TempDir()
	planJSON := makePlanJSON([]Step{
		{Action: ActionWrite, Description: "create hello.txt", Path: "hello.txt", Content: "hi"},
	})

	ag, _ := setupAgent(t, planJSON, Approve, ApprovalNone)
	// Override sandbox to use our temp dir.
	sb, _ := sandbox.New(dir)
	ag.sandbox = sb
	ag.executor = NewExecutor(sb)

	result, err := ag.Run(context.Background(), "create hello.txt with content hi")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Success {
		t.Error("expected Success = true")
	}

	data, err := os.ReadFile(filepath.Join(dir, "hello.txt"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hi" {
		t.Errorf("file content = %q, want %q", data, "hi")
	}
}

func TestE2E_ReadThenWrite(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("key: value"), 0644)

	planJSON := makePlanJSON([]Step{
		{Action: ActionRead, Description: "read config.yaml", Path: "config.yaml"},
		{Action: ActionWrite, Description: "write output.txt", Path: "output.txt", Content: "processed"},
	})

	sb, _ := sandbox.New(dir)
	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: planJSON},
	})
	ag := New(chain, sb, &mockApprover{decision: Approve}, ApprovalNone, nil, nil)

	result, err := ag.Run(context.Background(), "read config and write output")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Success {
		t.Error("expected Success = true")
	}
	if len(result.StepResults) != 2 {
		t.Errorf("step results = %d, want 2", len(result.StepResults))
	}

	data, err := os.ReadFile(filepath.Join(dir, "output.txt"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "processed" {
		t.Errorf("output.txt = %q, want %q", data, "processed")
	}
}

func TestE2E_DangerousActionApproval(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "old-file.txt")
	os.WriteFile(target, []byte("old"), 0644)

	planJSON := makePlanJSON([]Step{
		{Action: ActionDelete, Description: "delete old-file.txt", Path: target},
	})

	sb, _ := sandbox.New(dir)
	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: planJSON},
	})
	approver := &mockApprover{decision: Approve}
	ag := New(chain, sb, approver, ApprovalDangerousOnly, nil, nil)

	result, err := ag.Run(context.Background(), "delete old-file.txt")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Success {
		t.Error("expected Success = true")
	}

	// File should be deleted.
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("file should have been deleted")
	}

	// Approver should have been called with Dangerous=true.
	if len(approver.calls) != 1 {
		t.Fatalf("approval calls = %d, want 1", len(approver.calls))
	}
	if !approver.calls[0].Dangerous {
		t.Error("expected Dangerous = true on approval request")
	}
}

func TestE2E_DangerousActionRejected(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "old-file.txt")
	os.WriteFile(target, []byte("old"), 0644)

	planJSON := makePlanJSON([]Step{
		{Action: ActionDelete, Description: "delete old-file.txt", Path: target},
	})

	sb, _ := sandbox.New(dir)
	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: planJSON},
	})
	approver := &mockApprover{decision: Reject}
	ag := New(chain, sb, approver, ApprovalDangerousOnly, nil, nil)

	_, err := ag.Run(context.Background(), "delete old-file.txt")
	if err == nil {
		t.Fatal("expected error when dangerous action is rejected")
	}
	if !errors.Is(err, ErrRejected) {
		t.Errorf("expected ErrRejected, got: %v", err)
	}
	var rejErr *RejectedError
	if !errors.As(err, &rejErr) {
		t.Fatalf("expected *RejectedError, got %T", err)
	}

	// File should still exist.
	if _, err := os.Stat(target); err != nil {
		t.Error("file should still exist after rejection")
	}
}

func TestE2E_PlanApprovalReject(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "should-not-exist.txt")

	planJSON := makePlanJSON([]Step{
		{Action: ActionWrite, Description: "write file", Path: target, Content: "nope"},
	})

	sb, _ := sandbox.New(dir)
	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: planJSON},
	})
	approver := &sequenceApprover{decisions: []Decision{Reject}}
	ag := New(chain, sb, approver, ApprovalFull, nil, nil)

	_, err := ag.Run(context.Background(), "write a file")
	if err == nil {
		t.Fatal("expected plan rejection error")
	}
	var rejErr *RejectedError
	if !errors.As(err, &rejErr) {
		t.Fatalf("expected *RejectedError, got %T", err)
	}
	if rejErr.Stage != "plan" {
		t.Errorf("rejected stage = %q, want %q", rejErr.Stage, "plan")
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("step should not execute after plan rejection, stat err = %v", statErr)
	}
	if len(approver.calls) != 1 || approver.calls[0].Stage != "plan" {
		t.Fatalf("approval calls = %#v, want one plan call", approver.calls)
	}
}

func TestE2E_ResultApproval(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "result.txt")

	planJSON := makePlanJSON([]Step{
		{Action: ActionWrite, Description: "write result", Path: target, Content: "ok"},
	})

	sb, _ := sandbox.New(dir)
	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: planJSON},
	})
	approver := &sequenceApprover{decisions: []Decision{Approve, Approve, Approve}}
	ag := New(chain, sb, approver, ApprovalFull, nil, nil)

	result, err := ag.Run(context.Background(), "write result")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Success {
		t.Fatal("expected success")
	}
	if len(result.StepResults) != 1 || !strings.Contains(result.StepResults[0], "Wrote") {
		t.Fatalf("step results = %#v, want visible write result", result.StepResults)
	}
	if len(approver.calls) != 3 {
		t.Fatalf("approval calls = %d, want 3", len(approver.calls))
	}
	if approver.calls[2].Stage != "result" {
		t.Errorf("final approval stage = %q, want result", approver.calls[2].Stage)
	}
	if len(approver.calls[2].Items) != 1 || !strings.Contains(approver.calls[2].Items[0], "Wrote") {
		t.Errorf("result approval items = %#v, want step result shown", approver.calls[2].Items)
	}
}

func TestE2E_ApproveAll_FullMode(t *testing.T) {
	dir := t.TempDir()
	planJSON := makePlanJSON([]Step{
		{Action: ActionWrite, Description: "write a", Path: filepath.Join(dir, "a.txt"), Content: "a"},
		{Action: ActionWrite, Description: "write b", Path: filepath.Join(dir, "b.txt"), Content: "b"},
		{Action: ActionWrite, Description: "write c", Path: filepath.Join(dir, "c.txt"), Content: "c"},
	})

	sb, _ := sandbox.New(dir)
	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: planJSON},
	})
	approver := &sequenceApprover{decisions: []Decision{Approve, ApproveAll, Approve}}
	ag := New(chain, sb, approver, ApprovalFull, nil, nil)

	result, err := ag.Run(context.Background(), "write files")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.StepResults) != 3 {
		t.Fatalf("step results = %d, want 3", len(result.StepResults))
	}
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s should exist after approve-all: %v", name, err)
		}
	}
	if len(approver.calls) != 3 {
		t.Fatalf("approval calls = %d, want plan + first execute + result", len(approver.calls))
	}
	if approver.calls[0].Stage != "plan" || approver.calls[1].Stage != "execute" || approver.calls[2].Stage != "result" {
		t.Fatalf("approval stages = %#v, want plan/execute/result", approver.calls)
	}
}

func TestE2E_MultiStepPlan(t *testing.T) {
	dir := t.TempDir()

	planJSON := makePlanJSON([]Step{
		{Action: ActionMkdir, Description: "create subdir", Path: filepath.Join(dir, "subdir")},
		{Action: ActionWrite, Description: "write file", Path: filepath.Join(dir, "subdir", "data.txt"), Content: "hello"},
		{Action: ActionList, Description: "list subdir", Path: filepath.Join(dir, "subdir")},
	})

	sb, _ := sandbox.New(dir)
	chain := provider.NewFallbackChain([]provider.LLMProvider{
		&mockLLMProvider{name: "mock", available: true, response: planJSON},
	})
	ag := New(chain, sb, &mockApprover{decision: Approve}, ApprovalNone, nil, nil)

	result, err := ag.Run(context.Background(), "create subdir, write file, list")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Success {
		t.Error("expected Success = true")
	}
	if len(result.StepResults) != 3 {
		t.Errorf("step results = %d, want 3", len(result.StepResults))
	}

	// Verify directory exists.
	info, err := os.Stat(filepath.Join(dir, "subdir"))
	if err != nil {
		t.Fatalf("subdir should exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("subdir should be a directory")
	}

	// Verify file exists with correct content.
	data, err := os.ReadFile(filepath.Join(dir, "subdir", "data.txt"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("file content = %q, want %q", data, "hello")
	}
}

func TestE2E_InvalidAction(t *testing.T) {
	planJSON := makePlanJSON([]Step{
		{Action: "explode", Description: "do something invalid", Path: "file.txt"},
	})

	ag, _ := setupAgent(t, planJSON, Approve, ApprovalNone)

	_, err := ag.Run(context.Background(), "explode file")
	if err == nil {
		t.Fatal("expected error for invalid action")
	}
	if !strings.Contains(err.Error(), "unsupported action") {
		t.Errorf("error = %q, want it to contain 'unsupported action'", err)
	}
}

func TestE2E_SkillInjection(t *testing.T) {
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
			Name:        "test-e2e-skill",
			Description: "Organize files into directories",
			AutoTrigger: true,
		},
		Content: "E2E test skill content.",
	})

	ag := New(chain, sb, &mockApprover{decision: Approve}, ApprovalNone, store, nil)

	result, err := ag.Run(context.Background(), "organize these files")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Success {
		t.Error("expected Success = true")
	}

	// Verify agent has skills store.
	if ag.Skills() == nil {
		t.Fatal("expected non-nil skills store")
	}

	// Verify skill was injected into system prompt.
	if len(recorder.messages) == 0 {
		t.Fatal("expected at least one Chat call")
	}
	sysMsg := recorder.messages[0][0].Content
	if !strings.Contains(sysMsg, "test-e2e-skill") {
		t.Error("system prompt should contain injected skill name")
	}
}

func TestE2E_SkillApprovalReject(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "skill-output.txt")

	planJSON := makePlanJSON([]Step{
		{Action: ActionWrite, Description: "write skill output", Path: target, Content: "nope"},
	})
	recorder := &recordingLLMProvider{
		name: "mock", available: true, response: planJSON,
	}
	chain := provider.NewFallbackChain([]provider.LLMProvider{recorder})

	store := skill.NewStore()
	store.Upsert(&skill.Skill{
		Metadata: skill.SkillMetadata{
			Name:        "test-e2e-skill",
			Description: "Organize files into directories",
			AutoTrigger: true,
		},
		Content: "E2E test skill content.",
	})

	sb, _ := sandbox.New(dir)
	approver := &sequenceApprover{decisions: []Decision{Approve, Reject}}
	ag := New(chain, sb, approver, ApprovalFull, store, nil)

	_, err := ag.Run(context.Background(), "organize these files")
	if err == nil {
		t.Fatal("expected plan rejection error")
	}
	var rejErr *RejectedError
	if !errors.As(err, &rejErr) {
		t.Fatalf("expected *RejectedError, got %T", err)
	}
	if rejErr.Stage != "plan" {
		t.Errorf("rejected stage = %q, want plan", rejErr.Stage)
	}
	if len(recorder.messages) == 0 {
		t.Fatal("expected planner Chat call")
	}
	sysMsg := recorder.messages[0][0].Content
	if !strings.Contains(sysMsg, "test-e2e-skill") || !strings.Contains(sysMsg, "E2E test skill content.") {
		t.Errorf("system prompt should contain injected skill, got: %s", sysMsg)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("step should not execute after plan rejection, stat err = %v", statErr)
	}
	if len(approver.calls) != 2 || approver.calls[0].Stage != "skill" || approver.calls[1].Stage != "plan" {
		t.Fatalf("approval calls = %#v, want skill then plan", approver.calls)
	}
}
