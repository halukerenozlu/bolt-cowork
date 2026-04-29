package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
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
			return "", fmt.Errorf("Access denied: %q escapes the workspace boundary: %w", p, ErrPathTraversal)
		}
		return cleaned, nil
	}

	joined := filepath.Join(root, p)
	cleaned := filepath.Clean(joined)

	// Verify the result is still under root (catches "../" traversals).
	rel, err := filepath.Rel(root, cleaned)
	if err != nil || escapesRoot(rel) {
		return "", fmt.Errorf("Access denied: %q escapes the workspace boundary: %w", p, ErrPathTraversal)
	}

	return cleaned, nil
}

// displayPath returns a workspace-relative path for user-facing messages.
// Falls back to absPath if conversion fails or path escapes root.
func displayPath(absPath, sandboxRoot string) string {
	rel, err := filepath.Rel(sandboxRoot, absPath)
	if err != nil || escapesRoot(rel) {
		return absPath
	}
	if rel == "." {
		return "."
	}
	return "./" + filepath.ToSlash(rel)
}

// friendlyError translates low-level sandbox/OS errors into user-readable messages.
// The original error is wrapped with %w so errors.Is/As chains remain intact.
func friendlyError(displayP, sandboxRoot string, err error) error {
	switch {
	case errors.Is(err, sandbox.ErrOutsideSandbox), errors.Is(err, sandbox.ErrSymlinkEscape):
		return fmt.Errorf("Access denied: %q is outside the workspace (allowed: %s): %w", displayP, sandboxRoot, err)
	case errors.Is(err, sandbox.ErrReadOnlyDir):
		return fmt.Errorf("Write denied: %q is in a read-only directory: %w", displayP, err)
	case errors.Is(err, sandbox.ErrDeniedPattern):
		return fmt.Errorf("Access denied: %q matches a restricted pattern: %w", displayP, err)
	case errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("File not found: %q: %w", displayP, err)
	case errors.Is(err, os.ErrPermission):
		return fmt.Errorf("Permission denied: cannot access %q. Check file permissions.: %w", displayP, err)
	default:
		return err
	}
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
			return "", friendlyError(displayPath(path, e.sandbox.Root()), e.sandbox.Root(), err)
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
		if sandbox.IsProtectedPath(path) {
			return "", fmt.Errorf("Protected file: %q cannot be modified by agent", step.Path)
		}
		if step.Content == "" {
			return "", fmt.Errorf("executor: write %q: empty content - plan did not include file content", step.Path)
		}
		if err := e.sandbox.WriteFile(path, []byte(step.Content)); err != nil {
			return "", friendlyError(displayPath(path, e.sandbox.Root()), e.sandbox.Root(), err)
		}
		return fmt.Sprintf("Wrote %q (%d bytes)", step.Path, len(step.Content)), nil

	case ActionDelete:
		if sandbox.IsProtectedPath(path) {
			return "", fmt.Errorf("Protected file: %q cannot be modified by agent", step.Path)
		}
		if err := e.sandbox.DeletePath(path, step.Recursive); err != nil {
			return "", friendlyError(displayPath(path, e.sandbox.Root()), e.sandbox.Root(), err)
		}
		return fmt.Sprintf("Deleted %q", step.Path), nil

	case ActionCopy:
		if sandbox.IsProtectedPath(dest) {
			return "", fmt.Errorf("Protected file: %q cannot be a destination", step.Destination)
		}
		if err := e.sandbox.CopyFile(path, dest); err != nil {
			return "", friendlyError(displayPath(path, e.sandbox.Root()), e.sandbox.Root(), err)
		}
		return fmt.Sprintf("Copied %q -> %q", step.Path, step.Destination), nil

	case ActionMkdir:
		existed := false
		if info, statErr := os.Stat(path); statErr == nil && info.IsDir() {
			existed = true
		}
		if err := e.sandbox.MkdirAll(path); err != nil {
			return "", friendlyError(displayPath(path, e.sandbox.Root()), e.sandbox.Root(), err)
		}
		if existed {
			return fmt.Sprintf("Directory %q already exists", step.Path), nil
		}
		return fmt.Sprintf("Created directory %q", step.Path), nil

	case ActionMove:
		if sandbox.IsProtectedPath(path) {
			return "", fmt.Errorf("Protected file: %q cannot be modified by agent", step.Path)
		}
		if sandbox.IsProtectedPath(dest) {
			return "", fmt.Errorf("Protected file: %q cannot be a destination", step.Destination)
		}
		if err := e.sandbox.MoveFile(path, dest); err != nil {
			return "", friendlyError(displayPath(path, e.sandbox.Root()), e.sandbox.Root(), err)
		}
		return fmt.Sprintf("Moved %q -> %q", step.Path, step.Destination), nil

	case ActionRename:
		if sandbox.IsProtectedPath(path) {
			return "", fmt.Errorf("Protected file: %q cannot be modified by agent", step.Path)
		}
		if sandbox.IsProtectedPath(dest) {
			return "", fmt.Errorf("Protected file: %q cannot be a destination", step.Destination)
		}
		if err := e.sandbox.RenameFile(path, dest); err != nil {
			return "", friendlyError(displayPath(path, e.sandbox.Root()), e.sandbox.Root(), err)
		}
		return fmt.Sprintf("Renamed %q -> %q", step.Path, step.Destination), nil

	case ActionList:
		entries, err := e.sandbox.ListDir(path)
		if err != nil {
			return "", friendlyError(displayPath(path, e.sandbox.Root()), e.sandbox.Root(), err)
		}
		names := make([]string, len(entries))
		for i, entry := range entries {
			names[i] = entry.Name()
		}
		return fmt.Sprintf("Listed %q: %s", step.Path, strings.Join(names, ", ")), nil

	default:
		return "", fmt.Errorf("Unsupported action type: %q. Supported: read, write, mkdir, copy, delete, move, rename, list", step.Action)
	}
}
