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

// resolveAndCheckProtected resolves symlinks on path and checks the result
// against the protected-path list. If the path does not exist yet, it resolves
// the nearest existing ancestor and reconstructs the missing suffix from there.
func resolveAndCheckProtected(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		current := path
		parts := []string{filepath.Base(current)}
		for {
			dir := filepath.Dir(current)
			if dir == current {
				resolved = filepath.Join(parts...)
				break
			}
			if _, statErr := os.Stat(dir); statErr == nil {
				resolvedDir, evalErr := filepath.EvalSymlinks(dir)
				if evalErr != nil {
					return "", fmt.Errorf("resolve protected path ancestor %q: %w", dir, evalErr)
				}
				resolved = filepath.Join(append([]string{resolvedDir}, parts...)...)
				break
			}
			parts = append([]string{filepath.Base(dir)}, parts...)
			current = dir
		}
	}

	if sandbox.IsProtectedPath(resolved) {
		return "", fmt.Errorf("Protected file: %q cannot be accessed by agent", resolved)
	}
	return resolved, nil
}

// resolveDestProtected resolves the final destination path for file operations.
// When dest is an existing directory (possibly a symlink to one), the actual
// file written will be dest/basename(srcPath), so we resolve the directory
// first and check the joined result. For non-directory destinations, standard
// resolveAndCheckProtected logic applies.
func resolveDestProtected(dest, srcPath string) (string, error) {
	if dstInfo, err := os.Stat(dest); err == nil && dstInfo.IsDir() {
		resolvedDir, evalErr := filepath.EvalSymlinks(dest)
		if evalErr != nil {
			return "", fmt.Errorf("cannot resolve destination directory: %w", evalErr)
		}
		finalDest := filepath.Join(resolvedDir, filepath.Base(srcPath))
		if sandbox.IsProtectedPath(finalDest) {
			return "", fmt.Errorf("Protected file: %q cannot be accessed by agent", finalDest)
		}
		return finalDest, nil
	}
	return resolveAndCheckProtected(dest)
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
		if _, err := resolveAndCheckProtected(path); err != nil {
			return "", err
		}
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
		if _, err := resolveAndCheckProtected(path); err != nil {
			return "", err
		}
		if step.Content == "" {
			return "", fmt.Errorf("executor: write %q: empty content - plan did not include file content", step.Path)
		}
		if err := e.sandbox.WriteFile(path, []byte(step.Content)); err != nil {
			return "", friendlyError(displayPath(path, e.sandbox.Root()), e.sandbox.Root(), err)
		}
		return fmt.Sprintf("Wrote %q (%d bytes)", step.Path, len(step.Content)), nil

	case ActionDelete:
		if _, err := resolveAndCheckProtected(path); err != nil {
			return "", err
		}
		if err := e.sandbox.DeletePath(path, step.Recursive); err != nil {
			return "", friendlyError(displayPath(path, e.sandbox.Root()), e.sandbox.Root(), err)
		}
		return fmt.Sprintf("Deleted %q", step.Path), nil

	case ActionCopy:
		if _, err := resolveAndCheckProtected(path); err != nil {
			return "", err
		}
		if _, err := resolveDestProtected(dest, path); err != nil {
			return "", err
		}
		if err := e.sandbox.CopyFile(path, dest); err != nil {
			return "", friendlyError(displayPath(path, e.sandbox.Root()), e.sandbox.Root(), err)
		}
		return fmt.Sprintf("Copied %q -> %q", step.Path, step.Destination), nil

	case ActionMkdir:
		if _, err := resolveAndCheckProtected(path); err != nil {
			return "", err
		}
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
		if _, err := resolveAndCheckProtected(path); err != nil {
			return "", err
		}
		if _, err := resolveDestProtected(dest, path); err != nil {
			return "", err
		}
		if err := e.sandbox.MoveFile(path, dest); err != nil {
			return "", friendlyError(displayPath(path, e.sandbox.Root()), e.sandbox.Root(), err)
		}
		return fmt.Sprintf("Moved %q -> %q", step.Path, step.Destination), nil

	case ActionRename:
		if _, err := resolveAndCheckProtected(path); err != nil {
			return "", err
		}
		if _, err := resolveAndCheckProtected(dest); err != nil {
			return "", err
		}
		if err := e.sandbox.RenameFile(path, dest); err != nil {
			return "", friendlyError(displayPath(path, e.sandbox.Root()), e.sandbox.Root(), err)
		}
		return fmt.Sprintf("Renamed %q -> %q", step.Path, step.Destination), nil

	case ActionList:
		if _, err := resolveAndCheckProtected(path); err != nil {
			return "", err
		}
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
