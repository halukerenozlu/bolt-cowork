package agent

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/halukerenozlu/bolt-cowork/internal/agent/actions"
	"github.com/halukerenozlu/bolt-cowork/internal/mcp"
	"github.com/halukerenozlu/bolt-cowork/internal/sandbox"
	"github.com/halukerenozlu/bolt-cowork/internal/tool"
	"github.com/pdfcpu/pdfcpu/pkg/api"
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
const maxReadPreviewBytes = 64 << 10 // 64 KiB
const maxWriteContentBytes = 1 << 20 // 1 MB
const commandTimeout = 2 * time.Minute

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
		file, err := e.sandbox.OpenRead(path)
		if err != nil {
			return "", friendlyError(displayPath(path, e.sandbox.Root()), e.sandbox.Root(), err)
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			return "", fmt.Errorf("executor: stat open file %q: %w", step.Path, err)
		}
		data, err := io.ReadAll(io.LimitReader(file, maxReadPreviewBytes+1))
		if err != nil {
			return "", fmt.Errorf("executor: read preview %q: %w", step.Path, err)
		}
		if isBinaryContent(data) {
			contentType := http.DetectContentType(data)
			return fmt.Sprintf(
				"Binary file %q (%d bytes, %s); content omitted",
				step.Path, info.Size(), contentType,
			), nil
		}

		previewData := data
		truncatedBytes := info.Size() > maxReadPreviewBytes
		if len(previewData) > maxReadPreviewBytes {
			previewData = previewData[:maxReadPreviewBytes]
		}
		content := sanitizeTerminalText(string(previewData))
		lines := strings.SplitAfter(content, "\n")
		var preview string
		if len(lines) > maxReadLines {
			preview = strings.Join(lines[:maxReadLines], "") +
				fmt.Sprintf("\n[truncated - showing %d of %d lines]", maxReadLines, len(lines))
		} else {
			preview = content
		}
		if truncatedBytes {
			preview += fmt.Sprintf("\n[truncated - showing first %d of %d bytes]", maxReadPreviewBytes, info.Size())
		}
		return fmt.Sprintf("Read %q (%d bytes):\n%s", step.Path, info.Size(), preview), nil

	case ActionStat:
		if _, err := resolveAndCheckProtected(path); err != nil {
			return "", err
		}
		info, err := e.sandbox.FileInfo(path)
		if err != nil {
			return "", friendlyError(displayPath(path, e.sandbox.Root()), e.sandbox.Root(), err)
		}
		return fmt.Sprintf(
			"Stat %q: size=%d mode=%s modified=%s directory=%t",
			step.Path,
			info.Size(),
			info.Mode(),
			info.ModTime().UTC().Format("2006-01-02T15:04:05Z"),
			info.IsDir(),
		), nil

	case ActionHash:
		if _, err := resolveAndCheckProtected(path); err != nil {
			return "", err
		}
		file, err := e.sandbox.OpenRead(path)
		if err != nil {
			return "", friendlyError(displayPath(path, e.sandbox.Root()), e.sandbox.Root(), err)
		}
		defer file.Close()

		hash := sha256.New()
		if _, err := io.Copy(hash, file); err != nil {
			return "", fmt.Errorf("executor: hash %q: %w", step.Path, err)
		}
		return fmt.Sprintf("Hash %q: sha256=%x", step.Path, hash.Sum(nil)), nil

	case ActionWrite:
		if _, err := resolveAndCheckProtected(path); err != nil {
			return "", err
		}
		if len(step.Content) > maxWriteContentBytes {
			return "", fmt.Errorf("executor: write %q: content too large (%d bytes, max %d) - split into smaller files",
				step.Path, len(step.Content), maxWriteContentBytes)
		}
		parent := filepath.Dir(path)
		if parent != "." && parent != path {
			if err := e.sandbox.MkdirAll(parent); err != nil {
				return "", friendlyError(displayPath(parent, e.sandbox.Root()), e.sandbox.Root(), err)
			}
		}
		if err := e.sandbox.WriteFile(path, []byte(step.Content)); err != nil {
			return "", friendlyError(displayPath(path, e.sandbox.Root()), e.sandbox.Root(), err)
		}
		return fmt.Sprintf("Wrote %q (%d bytes)", step.Path, len(step.Content)), nil

	case ActionDelete:
		if _, err := resolveAndCheckProtected(path); err != nil {
			return "", err
		}
		if err := e.sandbox.DeletePath(path, true); err != nil {
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
			if entry.IsDir() {
				names[i] += "/"
			}
		}
		return tool.FormatListOutput(step.Path, names), nil

	case ActionRunCommand:
		if !commandAllowed(step.Command) {
			return "", fmt.Errorf("executor: command %q is not allowed; allowed commands: git, pandoc, soffice, libreoffice", step.Command)
		}
		for _, arg := range step.CommandArgs {
			if strings.Contains(arg, "..") {
				return "", fmt.Errorf("executor: command argument %q is not allowed", arg)
			}
		}
		cmdCtx, cancel := context.WithTimeout(ctx, commandTimeout)
		defer cancel()
		cmd := exec.CommandContext(cmdCtx, step.Command, step.CommandArgs...)
		cmd.Dir = e.sandbox.Root()
		output, runErr := cmd.CombinedOutput()
		text := sanitizeTerminalText(string(output))
		if len(text) > maxReadPreviewBytes {
			text = text[:maxReadPreviewBytes] + "\n[truncated]"
		}
		if runErr != nil {
			return "", fmt.Errorf("executor: command %q failed: %w\noutput: %s", step.Command, runErr, text)
		}
		return fmt.Sprintf("Ran %q %s:\n%s", step.Command, strings.Join(step.CommandArgs, " "), text), nil

	case ActionMergePDF:
		if len(step.Sources) < 2 {
			return "", fmt.Errorf("executor: merge_pdf requires at least 2 sources, got %d", len(step.Sources))
		}
		if dest == "" {
			return "", fmt.Errorf("executor: merge_pdf requires a destination path")
		}
		resolvedSources := make([]string, len(step.Sources))
		for i, src := range step.Sources {
			resolvedSrc, err := e.resolvePath(src)
			if err != nil {
				return "", err
			}
			if _, err := resolveAndCheckProtected(resolvedSrc); err != nil {
				return "", err
			}
			resolvedSources[i] = resolvedSrc
		}
		if _, err := resolveDestProtected(dest, resolvedSources[0]); err != nil {
			return "", err
		}
		if parent := filepath.Dir(dest); parent != "." && parent != dest {
			if err := e.sandbox.MkdirAll(parent); err != nil {
				return "", friendlyError(displayPath(parent, e.sandbox.Root()), e.sandbox.Root(), err)
			}
		}
		if err := api.MergeCreateFile(resolvedSources, dest, false, nil); err != nil {
			return "", fmt.Errorf("executor: merge_pdf %v -> %q: %w", step.Sources, step.Destination, err)
		}
		return fmt.Sprintf("Merged %d PDFs into %q", len(step.Sources), step.Destination), nil

	case ActionSplitPDF:
		if _, err := resolveAndCheckProtected(path); err != nil {
			return "", err
		}
		if dest == "" {
			return "", fmt.Errorf("executor: split_pdf requires a destination directory")
		}
		if _, err := resolveAndCheckProtected(dest); err != nil {
			return "", err
		}
		if err := e.sandbox.MkdirAll(dest); err != nil {
			return "", friendlyError(displayPath(dest, e.sandbox.Root()), e.sandbox.Root(), err)
		}
		span := step.Span
		if span <= 0 {
			span = 1
		}
		if err := api.SplitFile(path, dest, span, nil); err != nil {
			return "", fmt.Errorf("executor: split_pdf %q -> %q: %w", step.Path, step.Destination, err)
		}
		return fmt.Sprintf("Split %q into %q (%d page(s) per file)", step.Path, step.Destination, span), nil

	default:
		return "", fmt.Errorf("unsupported action type: %q, supported: read, write, mkdir, copy, delete, move, rename, list, stat, hash, call_mcp_tool, read_mcp_resource, run_command, merge_pdf, split_pdf", step.Action)
	}
}

// commandAllowed reports whether name is a bare executable name (no path
// separators) present in allowedCommands, so run_command can never be
// redirected to an LLM-supplied path on disk.
func commandAllowed(name string) bool {
	if strings.ContainsAny(name, `/\`) {
		return false
	}
	base := strings.TrimSuffix(strings.ToLower(name), ".exe")
	return allowedCommands[base]
}

func isBinaryContent(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	if !utf8.Valid(data) {
		return true
	}
	sample := data
	if len(sample) > 8<<10 {
		sample = sample[:8<<10]
	}
	return strings.IndexByte(string(sample), 0) >= 0
}

func sanitizeTerminalText(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	for _, r := range text {
		switch {
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, "\\x%02x", r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
