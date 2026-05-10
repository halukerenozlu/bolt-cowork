package sandbox

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
)

var (
	// ErrOutsideSandbox is returned when a path resolves outside allowed directories.
	ErrOutsideSandbox = errors.New("path is outside sandbox boundary")

	// ErrDeniedPattern is returned when a path matches a denied pattern.
	ErrDeniedPattern = errors.New("path matches a denied pattern")

	// ErrSymlinkEscape is returned when a symlink target resolves outside the sandbox.
	ErrSymlinkEscape = errors.New("symlink target escapes sandbox boundary")

	// ErrReadOnlyDir is returned when a write operation targets a read-only directory.
	ErrReadOnlyDir = errors.New("path is in a read-only directory")

	// ErrNotEmpty is returned when deleting a non-empty directory without recursive flag.
	ErrNotEmpty = errors.New("directory not empty, set recursive: true to delete")

	// ErrDestinationExists is returned when a copy destination already exists.
	ErrDestinationExists = errors.New("destination already exists")
)

// WrapFSError wraps OS-level filesystem errors with user-friendly messages.
// Returns nil if err is nil.
func WrapFSError(op, path string, err error) error {
	if err == nil {
		return nil
	}
	if os.IsPermission(err) {
		return fmt.Errorf("%s %q: permission denied: %w", op, path, err)
	}
	if os.IsNotExist(err) {
		return fmt.Errorf("%s %q: file not found: %w", op, path, err)
	}
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("%s %q: already exists: %w", op, path, err)
	}
	if isFileLocked(err) {
		return fmt.Errorf("%s %q: file is locked by another process: %w", op, path, err)
	}
	return fmt.Errorf("%s %q: %w", op, path, err)
}

// isFileLocked checks for Windows file locking errors.
// On non-Windows, always returns false.
func isFileLocked(err error) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	return strings.Contains(err.Error(), "The process cannot access the file")
}
