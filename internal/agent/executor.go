package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/halukerenozlu/bolt-cowork/internal/agent/actions"
	"github.com/halukerenozlu/bolt-cowork/internal/mcp"
	"github.com/halukerenozlu/bolt-cowork/internal/sandbox"
)

// containsADS reports whether path contains a Windows NTFS Alternate Data
// Stream separator (colon) outside of the drive letter prefix (e.g. "C:").
// On non-Windows systems it always returns false because colons are valid
// filename characters on Unix.
func containsADS(path string) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	clean := path
	if len(clean) >= 2 && clean[1] == ':' {
		clean = clean[2:]
	}
	return strings.Contains(clean, ":")
}

// windowsReserved lists Windows reserved device names that cannot be used as
// filenames regardless of extension.
var windowsReserved = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true,
	"COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true,
	"LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// isReservedFilename reports whether the base name of path (without extension)
// is a Windows reserved device name. On non-Windows systems it always returns false.
func isReservedFilename(path string) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	base := filepath.Base(path)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	return windowsReserved[strings.ToUpper(name)]
}

// ErrPathTraversal is returned when a resolved path escapes the sandbox root.
var ErrPathTraversal = fmt.Errorf("path escapes sandbox root")

const maxReadLines = 200
const maxWriteContentBytes = 1 << 20 // 1 MB

// Executor runs plan steps using the sandbox.
type Executor struct {
	sandbox      *sandbox.Sandbox
	mcpCaller    actions.MCPCaller
	mcpReader    actions.MCPResourceReader
	toolRegistry *mcp.ToolRegistry
}

// ExecutorOption configures an Executor.
type ExecutorOption func(*Executor)

// WithMCPCaller configures the MCP caller used by call_mcp_tool steps.
func WithMCPCaller(caller actions.MCPCaller) ExecutorOption {
	return func(e *Executor) {
		e.mcpCaller = caller
	}
}

// WithMCPToolRegistry configures the registry used to validate MCP tool calls.
func WithMCPToolRegistry(registry *mcp.ToolRegistry) ExecutorOption {
	return func(e *Executor) {
		e.toolRegistry = registry
	}
}

// WithMCPResourceReader configures the reader used for read_mcp_resource steps.
func WithMCPResourceReader(reader actions.MCPResourceReader) ExecutorOption {
	return func(e *Executor) {
		e.mcpReader = reader
	}
}

// NewExecutor creates an Executor backed by the given sandbox.
func NewExecutor(sb *sandbox.Sandbox, opts ...ExecutorOption) *Executor {
	e := &Executor{sandbox: sb}
	for _, opt := range opts {
		opt(e)
	}
	return e
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
			return "", fmt.Errorf("access denied: %q escapes the workspace boundary: %w", p, ErrPathTraversal)
		}
		return cleaned, nil
	}

	joined := filepath.Join(root, p)
	cleaned := filepath.Clean(joined)

	// Verify the result is still under root (catches "../" traversals).
	rel, err := filepath.Rel(root, cleaned)
	if err != nil || escapesRoot(rel) {
		return "", fmt.Errorf("access denied: %q escapes the workspace boundary: %w", p, ErrPathTraversal)
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
		return fmt.Errorf("access denied: %q is outside the workspace (allowed: %s): %w", displayP, sandboxRoot, err)
	case errors.Is(err, sandbox.ErrReadOnlyDir):
		return fmt.Errorf("write denied: %q is in a read-only directory: %w", displayP, err)
	case errors.Is(err, sandbox.ErrDeniedPattern):
		return fmt.Errorf("access denied: %q matches a restricted pattern: %w", displayP, err)
	case errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("file not found: %q: %w", displayP, err)
	case errors.Is(err, os.ErrPermission):
		return fmt.Errorf("permission denied: cannot access %q, check file permissions: %w", displayP, err)
	default:
		return err
	}
}

// resolveAndCheckProtected resolves symlinks on path and checks the result
// against the protected-path list. If the path does not exist yet, it resolves
// the nearest existing ancestor and reconstructs the missing suffix from there.
func resolveAndCheckProtected(path string) (string, error) {
	if containsADS(path) {
		return "", fmt.Errorf("invalid path %q: alternate data streams are not allowed", path)
	}
	if isReservedFilename(path) {
		return "", fmt.Errorf("invalid path %q: cannot use Windows reserved filename", path)
	}

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
		return "", fmt.Errorf("protected file: %q cannot be accessed by agent", resolved)
	}
	return resolved, nil
}

// resolveDestProtected resolves the final destination path for file operations.
// When dest is an existing directory (possibly a symlink to one), the actual
// file written will be dest/basename(srcPath), so we resolve the directory
// first and check the joined result. For non-directory destinations, standard
// resolveAndCheckProtected logic applies.
func resolveDestProtected(dest, srcPath string) (string, error) {
	if containsADS(dest) {
		return "", fmt.Errorf("invalid path %q: alternate data streams are not allowed", dest)
	}

	if dstInfo, err := os.Stat(dest); err == nil && dstInfo.IsDir() {
		resolvedDir, evalErr := filepath.EvalSymlinks(dest)
		if evalErr != nil {
			return "", fmt.Errorf("cannot resolve destination directory: %w", evalErr)
		}
		finalDest := filepath.Join(resolvedDir, filepath.Base(srcPath))
		if sandbox.IsProtectedPath(finalDest) {
			return "", fmt.Errorf("protected file: %q cannot be accessed by agent", finalDest)
		}
		return finalDest, nil
	}
	return resolveAndCheckProtected(dest)
}

// ExecuteStep runs a single step and returns a human-readable result.
func (e *Executor) ExecuteStep(ctx context.Context, step Step) (string, error) {
	if step.Action == ActionCallMCPTool {
		if e.mcpCaller == nil {
			return "", fmt.Errorf("executor: mcp not configured")
		}
		if e.toolRegistry == nil {
			return "", fmt.Errorf("mcp: tool not found in registry: %s/%s", step.ServerName, step.ToolName)
		}
		if _, ok := e.toolRegistry.GetServerTool(step.ServerName, step.ToolName); !ok {
			return "", fmt.Errorf("mcp: tool not found in registry: %s/%s", step.ServerName, step.ToolName)
		}
		action := &actions.CallMCPToolAction{
			ServerName: step.ServerName,
			ToolName:   step.ToolName,
			Args:       step.Args,
		}
		result, err := action.Execute(ctx, e.mcpCaller)
		if err != nil {
			return "", err
		}
		if result.Error != "" {
			return "", fmt.Errorf("%s", result.Error)
		}
		return result.Output, nil
	}

	if step.Action == ActionReadMCPResource {
		if e.mcpReader == nil {
			return "", fmt.Errorf("executor: mcp resource reader not configured")
		}
		action := &actions.ReadMCPResourceAction{
			ServerName: step.ServerName,
			URI:        step.ResourceURI,
		}
		result, err := action.Execute(ctx, e.mcpReader)
		if err != nil {
			return "", err
		}
		if result.Error != "" {
			return "", fmt.Errorf("%s", result.Error)
		}
		return result.Output, nil
	}

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
		if len(step.Content) > maxWriteContentBytes {
			return "", fmt.Errorf("executor: write %q: content too large (%d bytes, max %d) - split into smaller files",
				step.Path, len(step.Content), maxWriteContentBytes)
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
		return "", fmt.Errorf("unsupported action type: %q, supported: read, write, mkdir, copy, delete, move, rename, list, call_mcp_tool, read_mcp_resource", step.Action)
	}
}
