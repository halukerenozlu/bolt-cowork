package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/halukerenozlu/bolt-cowork/internal/sandbox"
)

// ErrPathTraversal is returned when a resolved path escapes the sandbox root.
var ErrPathTraversal = fmt.Errorf("path escapes sandbox root")

const maxReadLines = 200

// Executor runs plan steps using the sandbox.
type Executor struct {
	sandbox *sandbox.Sandbox
}

// NewExecutor creates an Executor backed by the given sandbox.
func NewExecutor(sb *sandbox.Sandbox) *Executor {
	return &Executor{sandbox: sb}
}

// resolvePath converts a relative path to an absolute path anchored at the
// sandbox root. Absolute paths outside the sandbox and traversal attempts
// (e.g. "../escape.txt") return an error. The function performs no prefix
// stripping; the LLM system prompt is responsible for returning sandbox-
// relative paths without repeating the root directory name.
// escapesRoot reports whether rel (from filepath.Rel) points outside the root.
// It distinguishes true ".." traversal from directory names that start with
// two dots (e.g. "..hidden").
func escapesRoot(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (e *Executor) resolvePath(p string) (string, error) {
	root := e.sandbox.Root()

	if filepath.IsAbs(p) {
		// Allow absolute paths only if they are inside the sandbox root.
		cleaned := filepath.Clean(p)
		rel, err := filepath.Rel(root, cleaned)
		if err != nil || escapesRoot(rel) {
			return "", fmt.Errorf("executor: %w: %q", ErrPathTraversal, p)
		}
		return cleaned, nil
	}

	joined := filepath.Join(root, p)
	cleaned := filepath.Clean(joined)

	// Verify the result is still under root (catches "../" traversals).
	rel, err := filepath.Rel(root, cleaned)
	if err != nil || escapesRoot(rel) {
		return "", fmt.Errorf("executor: %w: %q", ErrPathTraversal, p)
	}

	return cleaned, nil
}

// ExecuteStep runs a single step and returns a human-readable result.
func (e *Executor) ExecuteStep(_ context.Context, step Step) (string, error) {
	path, err := e.resolvePath(step.Path)
	if err != nil {
		return "", err
	}
	dest := ""
	if step.Destination != "" {
		dest, err = e.resolvePath(step.Destination)
		if err != nil {
			return "", err
		}
	}

	switch step.Action {
	case ActionRead:
		data, err := e.sandbox.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("executor: read %q: %w", step.Path, err)
		}
		content := string(data)
		lines := strings.SplitAfter(content, "\n")
		var preview string
		if len(lines) > maxReadLines {
			preview = strings.Join(lines[:maxReadLines], "") +
				fmt.Sprintf("\n[truncated - showing %d of %d lines]", maxReadLines, len(lines))
		} else {
			preview = content
		}
		return fmt.Sprintf("Read %q (%d bytes):\n%s", step.Path, len(data), preview), nil

	case ActionWrite:
		if step.Content == "" {
			return "", fmt.Errorf("executor: write %q: empty content - plan did not include file content", step.Path)
		}
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
		return fmt.Sprintf("Moved %q -> %q", step.Path, step.Destination), nil

	case ActionRename:
		if err := e.sandbox.RenameFile(path, dest); err != nil {
			return "", fmt.Errorf("executor: rename %q to %q: %w", step.Path, step.Destination, err)
		}
		return fmt.Sprintf("Renamed %q -> %q", step.Path, step.Destination), nil

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
		return "", fmt.Errorf("executor: unsupported action type: %s", step.Action)
	}
}
