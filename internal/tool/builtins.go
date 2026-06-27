package tool

import (
	"context"
	"fmt"

	"github.com/halukerenozlu/bolt-cowork/internal/sandbox"
)

// inputString extracts a string value from the input map.
// Returns "" if the key is missing or not a string.
func inputString(input map[string]any, key string) string {
	v, ok := input[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// inputBool extracts a bool value from the input map.
func inputBool(input map[string]any, key string) bool {
	v, ok := input[key]
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

// ---------- ReadTool ----------

// ReadTool reads file contents.
type ReadTool struct{ sb *sandbox.Sandbox }

func (t *ReadTool) Name() string        { return "read" }
func (t *ReadTool) Description() string { return "Read the contents of a file" }
func (t *ReadTool) IsReadOnly() bool    { return true }
func (t *ReadTool) IsDestructive() bool { return false }
func (t *ReadTool) InputSchema() map[string]any {
	return map[string]any{
		"path": "string",
	}
}

func (t *ReadTool) Call(_ context.Context, input map[string]any) (Result, error) {
	path := inputString(input, "path")
	if path == "" {
		return Result{Status: StatusFailed, Error: "missing required input: path"}, nil
	}
	data, err := t.sb.ReadFile(path)
	if err != nil {
		return Result{Status: StatusFailed, Error: err.Error(), Path: path}, nil
	}
	return Result{
		Status: StatusSuccess,
		Output: fmt.Sprintf("Read %q (%d bytes):\n%s", path, len(data), string(data)),
		Path:   path,
	}, nil
}

// ---------- WriteTool ----------

// WriteTool writes content to a file.
type WriteTool struct{ sb *sandbox.Sandbox }

func (t *WriteTool) Name() string        { return "write" }
func (t *WriteTool) Description() string { return "Write content to a file" }
func (t *WriteTool) IsReadOnly() bool    { return false }
func (t *WriteTool) IsDestructive() bool { return true }
func (t *WriteTool) InputSchema() map[string]any {
	return map[string]any{
		"path":    "string",
		"content": "string",
	}
}

func (t *WriteTool) Call(_ context.Context, input map[string]any) (Result, error) {
	path := inputString(input, "path")
	if path == "" {
		return Result{Status: StatusFailed, Error: "missing required input: path"}, nil
	}
	if sandbox.IsProtectedPath(path) {
		return Result{Status: StatusDenied, Error: fmt.Sprintf("Protected file: %q cannot be modified by agent", path), Path: path}, nil
	}
	content := inputString(input, "content")
	if content == "" {
		return Result{Status: StatusFailed, Error: "write requires non-empty content"}, nil
	}
	if err := t.sb.WriteFile(path, []byte(content)); err != nil {
		return Result{Status: StatusFailed, Error: err.Error(), Path: path}, nil
	}
	return Result{
		Status: StatusSuccess,
		Output: fmt.Sprintf("Wrote %q (%d bytes)", path, len(content)),
		Path:   path,
	}, nil
}

// ---------- DeleteTool ----------

// DeleteTool removes a file or directory.
type DeleteTool struct{ sb *sandbox.Sandbox }

func (t *DeleteTool) Name() string        { return "delete" }
func (t *DeleteTool) Description() string { return "Delete a file or directory" }
func (t *DeleteTool) IsReadOnly() bool    { return false }
func (t *DeleteTool) IsDestructive() bool { return true }
func (t *DeleteTool) InputSchema() map[string]any {
	return map[string]any{
		"path":      "string",
		"recursive": "boolean",
	}
}

func (t *DeleteTool) Call(_ context.Context, input map[string]any) (Result, error) {
	path := inputString(input, "path")
	if path == "" {
		return Result{Status: StatusFailed, Error: "missing required input: path"}, nil
	}
	if sandbox.IsProtectedPath(path) {
		return Result{Status: StatusDenied, Error: fmt.Sprintf("Protected file: %q cannot be modified by agent", path), Path: path}, nil
	}
	recursive := inputBool(input, "recursive")
	if err := t.sb.DeletePath(path, recursive); err != nil {
		return Result{Status: StatusFailed, Error: err.Error(), Path: path}, nil
	}
	return Result{
		Status: StatusSuccess,
		Output: fmt.Sprintf("Deleted %q", path),
		Path:   path,
	}, nil
}

// ---------- MkdirTool ----------

// MkdirTool creates a directory and any missing parents.
type MkdirTool struct{ sb *sandbox.Sandbox }

func (t *MkdirTool) Name() string        { return "mkdir" }
func (t *MkdirTool) Description() string { return "Create a directory (and parents)" }
func (t *MkdirTool) IsReadOnly() bool    { return false }
func (t *MkdirTool) IsDestructive() bool { return false }
func (t *MkdirTool) InputSchema() map[string]any {
	return map[string]any{
		"path": "string",
	}
}

func (t *MkdirTool) Call(_ context.Context, input map[string]any) (Result, error) {
	path := inputString(input, "path")
	if path == "" {
		return Result{Status: StatusFailed, Error: "missing required input: path"}, nil
	}
	if err := t.sb.MkdirAll(path); err != nil {
		return Result{Status: StatusFailed, Error: err.Error(), Path: path}, nil
	}
	return Result{
		Status: StatusSuccess,
		Output: fmt.Sprintf("Created directory %q", path),
		Path:   path,
	}, nil
}

// ---------- CopyTool ----------

// CopyTool copies a file to a new location.
type CopyTool struct{ sb *sandbox.Sandbox }

func (t *CopyTool) Name() string        { return "copy" }
func (t *CopyTool) Description() string { return "Copy a file to a new location" }
func (t *CopyTool) IsReadOnly() bool    { return false }
func (t *CopyTool) IsDestructive() bool { return false }
func (t *CopyTool) InputSchema() map[string]any {
	return map[string]any{
		"path":        "string",
		"destination": "string",
	}
}

func (t *CopyTool) Call(_ context.Context, input map[string]any) (Result, error) {
	path := inputString(input, "path")
	if path == "" {
		return Result{Status: StatusFailed, Error: "missing required input: path"}, nil
	}
	dest := inputString(input, "destination")
	if dest == "" {
		return Result{Status: StatusFailed, Error: "missing required input: destination"}, nil
	}
	if sandbox.IsProtectedPath(dest) {
		return Result{Status: StatusDenied, Error: fmt.Sprintf("Protected file: %q cannot be a destination", dest), Path: dest}, nil
	}
	if err := t.sb.CopyFile(path, dest); err != nil {
		return Result{Status: StatusFailed, Error: err.Error(), Path: path}, nil
	}
	return Result{
		Status: StatusSuccess,
		Output: fmt.Sprintf("Copied %q -> %q", path, dest),
		Path:   path,
	}, nil
}

// ---------- MoveTool ----------

// MoveTool moves a file to a new location.
type MoveTool struct{ sb *sandbox.Sandbox }

func (t *MoveTool) Name() string        { return "move" }
func (t *MoveTool) Description() string { return "Move a file to a new location" }
func (t *MoveTool) IsReadOnly() bool    { return false }
func (t *MoveTool) IsDestructive() bool { return true }
func (t *MoveTool) InputSchema() map[string]any {
	return map[string]any{
		"path":        "string",
		"destination": "string",
	}
}

func (t *MoveTool) Call(_ context.Context, input map[string]any) (Result, error) {
	path := inputString(input, "path")
	if path == "" {
		return Result{Status: StatusFailed, Error: "missing required input: path"}, nil
	}
	dest := inputString(input, "destination")
	if dest == "" {
		return Result{Status: StatusFailed, Error: "missing required input: destination"}, nil
	}
	if sandbox.IsProtectedPath(path) {
		return Result{Status: StatusDenied, Error: fmt.Sprintf("Protected file: %q cannot be modified by agent", path), Path: path}, nil
	}
	if sandbox.IsProtectedPath(dest) {
		return Result{Status: StatusDenied, Error: fmt.Sprintf("Protected file: %q cannot be a destination", dest), Path: dest}, nil
	}
	if err := t.sb.MoveFile(path, dest); err != nil {
		return Result{Status: StatusFailed, Error: err.Error(), Path: path}, nil
	}
	return Result{
		Status: StatusSuccess,
		Output: fmt.Sprintf("Moved %q -> %q", path, dest),
		Path:   path,
	}, nil
}

// ---------- RenameTool ----------

// RenameTool renames a file (same filesystem).
type RenameTool struct{ sb *sandbox.Sandbox }

func (t *RenameTool) Name() string        { return "rename" }
func (t *RenameTool) Description() string { return "Rename a file or directory" }
func (t *RenameTool) IsReadOnly() bool    { return false }
func (t *RenameTool) IsDestructive() bool { return true }
func (t *RenameTool) InputSchema() map[string]any {
	return map[string]any{
		"path":        "string",
		"destination": "string",
	}
}

func (t *RenameTool) Call(_ context.Context, input map[string]any) (Result, error) {
	path := inputString(input, "path")
	if path == "" {
		return Result{Status: StatusFailed, Error: "missing required input: path"}, nil
	}
	dest := inputString(input, "destination")
	if dest == "" {
		return Result{Status: StatusFailed, Error: "missing required input: destination"}, nil
	}
	if sandbox.IsProtectedPath(path) {
		return Result{Status: StatusDenied, Error: fmt.Sprintf("Protected file: %q cannot be modified by agent", path), Path: path}, nil
	}
	if sandbox.IsProtectedPath(dest) {
		return Result{Status: StatusDenied, Error: fmt.Sprintf("Protected file: %q cannot be a destination", dest), Path: dest}, nil
	}
	if err := t.sb.RenameFile(path, dest); err != nil {
		return Result{Status: StatusFailed, Error: err.Error(), Path: path}, nil
	}
	return Result{
		Status: StatusSuccess,
		Output: fmt.Sprintf("Renamed %q -> %q", path, dest),
		Path:   path,
	}, nil
}

// ---------- ListTool ----------

// ListTool lists directory entries.
type ListTool struct{ sb *sandbox.Sandbox }

func (t *ListTool) Name() string        { return "list" }
func (t *ListTool) Description() string { return "List directory contents" }
func (t *ListTool) IsReadOnly() bool    { return true }
func (t *ListTool) IsDestructive() bool { return false }
func (t *ListTool) InputSchema() map[string]any {
	return map[string]any{
		"path": "string",
	}
}

func (t *ListTool) Call(_ context.Context, input map[string]any) (Result, error) {
	path := inputString(input, "path")
	if path == "" {
		return Result{Status: StatusFailed, Error: "missing required input: path"}, nil
	}
	entries, err := t.sb.ListDir(path)
	if err != nil {
		return Result{Status: StatusFailed, Error: err.Error(), Path: path}, nil
	}
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name()
		if entry.IsDir() {
			names[i] += "/"
		}
	}
	return Result{
		Status: StatusSuccess,
		Output: FormatListOutput(path, names),
		Path:   path,
	}, nil
}
