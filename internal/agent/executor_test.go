package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/halukerenozlu/bolt-cowork/internal/sandbox"
)

func newTestExecutor(t *testing.T) *Executor {
	t.Helper()
	dir := t.TempDir()
	sb, err := sandbox.New(dir)
	if err != nil {
		t.Fatalf("sandbox.New: %v", err)
	}
	return NewExecutor(sb)
}

func TestProtectedPath_EnvFile(t *testing.T) {
	exec := newTestExecutor(t)
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action:  ActionWrite,
		Path:    ".env",
		Content: "SECRET=123",
	})
	if err == nil {
		t.Fatal("expected error for writing .env, got nil")
	}
	if !strings.Contains(err.Error(), "Protected file") {
		t.Errorf("expected 'Protected file' in error, got: %v", err)
	}
}

func TestProtectedPath_ConfigYaml(t *testing.T) {
	exec := newTestExecutor(t)
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action:  ActionWrite,
		Path:    ".bolt-cowork/config.yaml",
		Content: "provider: openai",
	})
	if err == nil {
		t.Fatal("expected error for writing .bolt-cowork/config.yaml, got nil")
	}
	if !strings.Contains(err.Error(), "Protected file") {
		t.Errorf("expected 'Protected file' in error, got: %v", err)
	}
}

func TestProtectedPath_NormalFile(t *testing.T) {
	exec := newTestExecutor(t)
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action:  ActionWrite,
		Path:    "app.go",
		Content: "package main",
	})
	if err != nil {
		t.Errorf("expected no error for writing app.go, got: %v", err)
	}
}

func TestProtectedPath_CopyDest(t *testing.T) {
	exec := newTestExecutor(t)
	// Source check passes (app.go is not protected); destination .env must be blocked.
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action:      ActionCopy,
		Path:        "app.go",
		Destination: ".env",
	})
	if err == nil {
		t.Fatal("expected error when copying to .env, got nil")
	}
	if !strings.Contains(err.Error(), "Protected file") {
		t.Errorf("expected 'Protected file' in error, got: %v", err)
	}
}

func TestProtectedPath_MoveDest(t *testing.T) {
	exec := newTestExecutor(t)
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action:      ActionMove,
		Path:        "app.go",
		Destination: ".bolt-cowork/config.yaml",
	})
	if err == nil {
		t.Fatal("expected error when moving to .bolt-cowork/config.yaml, got nil")
	}
	if !strings.Contains(err.Error(), "Protected file") {
		t.Errorf("expected 'Protected file' in error, got: %v", err)
	}
}

func TestProtectedPath_RenameDest(t *testing.T) {
	exec := newTestExecutor(t)
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action:      ActionRename,
		Path:        "app.go",
		Destination: "secret.key",
	})
	if err == nil {
		t.Fatal("expected error when renaming to secret.key, got nil")
	}
	if !strings.Contains(err.Error(), "Protected file") {
		t.Errorf("expected 'Protected file' in error, got: %v", err)
	}
}
