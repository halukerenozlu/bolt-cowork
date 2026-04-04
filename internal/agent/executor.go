package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/halukerenozlu/bolt-cowork/internal/sandbox"
)

// Executor runs plan steps using the sandbox.
type Executor struct {
	sandbox *sandbox.Sandbox
}

// NewExecutor creates an Executor backed by the given sandbox.
func NewExecutor(sb *sandbox.Sandbox) *Executor {
	return &Executor{sandbox: sb}
}

// resolvePath converts a potentially relative path to an absolute path
// anchored at the sandbox root. Paths that are already absolute are returned
// unchanged.
func (e *Executor) resolvePath(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(e.sandbox.Root(), p)
}

// ExecuteStep runs a single step and returns a human-readable result.
func (e *Executor) ExecuteStep(_ context.Context, step Step) (string, error) {
	path := e.resolvePath(step.Path)
	dest := e.resolvePath(step.Destination)

	switch step.Action {
	case ActionRead:
		data, err := e.sandbox.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("executor: read %q: %w", step.Path, err)
		}
		return fmt.Sprintf("Read %q (%d bytes)", step.Path, len(data)), nil

	case ActionWrite:
		if err := e.sandbox.WriteFile(path, []byte(step.Content)); err != nil {
			return "", fmt.Errorf("executor: write %q: %w", step.Path, err)
		}
		return fmt.Sprintf("Wrote %q (%d bytes)", step.Path, len(step.Content)), nil

	case ActionDelete:
		if err := e.sandbox.DeleteFile(path); err != nil {
			return "", fmt.Errorf("executor: delete %q: %w", step.Path, err)
		}
		return fmt.Sprintf("Deleted %q", step.Path), nil

	case ActionMove:
		if err := e.sandbox.MoveFile(path, dest); err != nil {
			return "", fmt.Errorf("executor: move %q to %q: %w", step.Path, step.Destination, err)
		}
		return fmt.Sprintf("Moved %q → %q", step.Path, step.Destination), nil

	case ActionRename:
		if err := e.sandbox.RenameFile(path, dest); err != nil {
			return "", fmt.Errorf("executor: rename %q to %q: %w", step.Path, step.Destination, err)
		}
		return fmt.Sprintf("Renamed %q → %q", step.Path, step.Destination), nil

	case ActionList:
		entries, err := e.sandbox.ListDir(path)
		if err != nil {
			return "", fmt.Errorf("executor: list %q: %w", step.Path, err)
		}
		names := make([]string, len(entries))
		for i, entry := range entries {
			names[i] = entry.Name()
		}
		return fmt.Sprintf("Listed %q: %s", step.Path, strings.Join(names, ", ")), nil

	default:
		return "", fmt.Errorf("executor: unknown action %q", step.Action)
	}
}
