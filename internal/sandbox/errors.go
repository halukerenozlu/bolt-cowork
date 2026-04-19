package sandbox

import "errors"

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
