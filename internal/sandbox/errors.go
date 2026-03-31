package sandbox

import "errors"

var (
	// ErrOutsideSandbox is returned when a path resolves outside allowed directories.
	ErrOutsideSandbox = errors.New("path is outside sandbox boundary")

	// ErrDeniedPattern is returned when a path matches a denied pattern.
	ErrDeniedPattern = errors.New("path matches a denied pattern")

	// ErrSymlinkEscape is returned when a symlink target resolves outside the sandbox.
	ErrSymlinkEscape = errors.New("symlink target escapes sandbox boundary")
)
