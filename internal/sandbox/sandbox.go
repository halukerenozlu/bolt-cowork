package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Sandbox restricts file operations to a set of allowed directories
// and blocks access to paths matching denied patterns.
type Sandbox struct {
	root           string
	allowedDirs    []string
	deniedPatterns []string
}

// Option configures a Sandbox.
type Option func(*Sandbox)

// WithAllowedDirs adds additional directories the sandbox may access.
func WithAllowedDirs(dirs ...string) Option {
	return func(s *Sandbox) {
		s.allowedDirs = append(s.allowedDirs, dirs...)
	}
}

// WithDeniedPatterns adds glob patterns that block access to matching paths.
func WithDeniedPatterns(patterns ...string) Option {
	return func(s *Sandbox) {
		s.deniedPatterns = append(s.deniedPatterns, patterns...)
	}
}

// New creates a Sandbox rooted at the given directory.
// The directory must exist. Options can add additional allowed directories
// and denied patterns.
func New(dir string, opts ...Option) (*Sandbox, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("sandbox: resolve root path: %w", err)
	}

	info, err := os.Stat(absDir)
	if err != nil {
		return nil, fmt.Errorf("sandbox: stat root dir: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("sandbox: root path is not a directory: %s", absDir)
	}

	resolved, err := filepath.EvalSymlinks(absDir)
	if err != nil {
		return nil, fmt.Errorf("sandbox: resolve root symlinks: %w", err)
	}

	s := &Sandbox{
		root:        resolved,
		allowedDirs: []string{resolved},
	}

	for _, opt := range opts {
		opt(s)
	}

	// Resolve additional allowed dirs added by options.
	resolvedDirs := []string{resolved}
	for _, d := range s.allowedDirs[1:] {
		ad, err := filepath.Abs(d)
		if err != nil {
			return nil, fmt.Errorf("sandbox: resolve allowed dir %q: %w", d, err)
		}
		rd, err := filepath.EvalSymlinks(ad)
		if err != nil {
			return nil, fmt.Errorf("sandbox: resolve allowed dir symlinks %q: %w", d, err)
		}
		resolvedDirs = append(resolvedDirs, rd)
	}
	s.allowedDirs = resolvedDirs

	return s, nil
}

// Root returns the sandbox root directory (absolute, symlink-resolved).
func (s *Sandbox) Root() string {
	return s.root
}

// validatePath checks whether the given path is allowed.
func (s *Sandbox) validatePath(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("sandbox: resolve path: %w", err)
	}

	if s.matchesDeniedPattern(absPath) {
		return fmt.Errorf("sandbox: access denied for %q: %w", absPath, ErrDeniedPattern)
	}

	resolved, err := s.resolvePath(absPath)
	if err != nil {
		return err
	}

	if !s.isWithinAllowed(resolved) {
		return fmt.Errorf("sandbox: access denied for %q: %w", path, ErrOutsideSandbox)
	}

	return nil
}

// validateNewPath checks a path that may not exist yet (for WriteFile).
func (s *Sandbox) validateNewPath(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("sandbox: resolve path: %w", err)
	}

	if s.matchesDeniedPattern(absPath) {
		return fmt.Errorf("sandbox: access denied for %q: %w", absPath, ErrDeniedPattern)
	}

	resolved, err := s.resolveNewPath(absPath)
	if err != nil {
		return err
	}

	if !s.isWithinAllowed(resolved) {
		return fmt.Errorf("sandbox: access denied for %q: %w", path, ErrOutsideSandbox)
	}

	return nil
}

// resolvePath resolves symlinks for an existing path.
func (s *Sandbox) resolvePath(absPath string) (string, error) {
	resolved, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return s.resolveNewPath(absPath)
		}
		return "", fmt.Errorf("sandbox: resolve symlinks: %w", err)
	}

	if resolved != absPath && !s.isWithinAllowed(resolved) {
		return "", fmt.Errorf("sandbox: symlink %q escapes to %q: %w", absPath, resolved, ErrSymlinkEscape)
	}

	return resolved, nil
}

// resolveNewPath resolves the parent directory for a path that may not exist yet.
// If the parent directory does not exist either, falls back to the cleaned absolute path.
func (s *Sandbox) resolveNewPath(absPath string) (string, error) {
	dir := filepath.Dir(absPath)
	base := filepath.Base(absPath)

	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		if os.IsNotExist(err) {
			// Parent dir doesn't exist — use cleaned path for containment check.
			return filepath.Clean(absPath), nil
		}
		return "", fmt.Errorf("sandbox: resolve parent dir: %w", err)
	}

	return filepath.Join(resolvedDir, base), nil
}

// isWithinAllowed checks if a resolved path is inside any allowed directory.
func (s *Sandbox) isWithinAllowed(resolved string) bool {
	for _, allowed := range s.allowedDirs {
		rel, err := filepath.Rel(allowed, resolved)
		if err != nil {
			continue
		}
		if !strings.HasPrefix(rel, "..") {
			return true
		}
	}
	return false
}

// matchesDeniedPattern checks if a path matches any denied pattern.
func (s *Sandbox) matchesDeniedPattern(absPath string) bool {
	base := filepath.Base(absPath)
	rel, _ := filepath.Rel(s.root, absPath)
	relSlash := filepath.ToSlash(rel)

	for _, pattern := range s.deniedPatterns {
		patternSlash := filepath.ToSlash(pattern)

		// Match against base name (handles *.env, *.key).
		if matched, _ := filepath.Match(patternSlash, base); matched {
			return true
		}

		// Match against full relative path.
		if matched, _ := filepath.Match(patternSlash, relSlash); matched {
			return true
		}

		// For directory patterns like ".ssh/*", check sub-paths.
		if strings.Contains(patternSlash, "/") {
			parts := strings.Split(relSlash, "/")
			for i := range parts {
				subPath := strings.Join(parts[i:], "/")
				if matched, _ := filepath.Match(patternSlash, subPath); matched {
					return true
				}
			}
		}
	}
	return false
}

// ReadFile reads the named file and returns its contents.
func (s *Sandbox) ReadFile(path string) ([]byte, error) {
	if err := s.validatePath(path); err != nil {
		return nil, fmt.Errorf("sandbox: read %q: %w", path, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("sandbox: read %q: %w", path, err)
	}
	return data, nil
}

// WriteFile writes data to the named file, creating it if necessary.
func (s *Sandbox) WriteFile(path string, data []byte) error {
	if err := s.validateNewPath(path); err != nil {
		return fmt.Errorf("sandbox: write %q: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("sandbox: write %q: %w", path, err)
	}
	return nil
}

// DeleteFile removes the named file.
func (s *Sandbox) DeleteFile(path string) error {
	if err := s.validatePath(path); err != nil {
		return fmt.Errorf("sandbox: delete %q: %w", path, err)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("sandbox: delete %q: %w", path, err)
	}
	return nil
}

// RenameFile renames a file from oldPath to newPath (same filesystem only).
func (s *Sandbox) RenameFile(oldPath, newPath string) error {
	if err := s.validatePath(oldPath); err != nil {
		return fmt.Errorf("sandbox: rename src %q: %w", oldPath, err)
	}
	if err := s.validateNewPath(newPath); err != nil {
		return fmt.Errorf("sandbox: rename dst %q: %w", newPath, err)
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		return fmt.Errorf("sandbox: rename %q to %q: %w", oldPath, newPath, err)
	}
	return nil
}

// MoveFile moves a file from src to dst, supporting cross-filesystem moves.
func (s *Sandbox) MoveFile(src, dst string) error {
	if err := s.validatePath(src); err != nil {
		return fmt.Errorf("sandbox: move src %q: %w", src, err)
	}
	if err := s.validateNewPath(dst); err != nil {
		return fmt.Errorf("sandbox: move dst %q: %w", dst, err)
	}

	// Try rename first (fast path, same filesystem).
	if err := os.Rename(src, dst); err == nil {
		return nil
	}

	// Fallback: read + write + delete for cross-filesystem moves.
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("sandbox: move read %q: %w", src, err)
	}
	if err := os.WriteFile(dst, data, 0644); err != nil {
		return fmt.Errorf("sandbox: move write %q: %w", dst, err)
	}
	if err := os.Remove(src); err != nil {
		return fmt.Errorf("sandbox: move remove src %q: %w", src, err)
	}
	return nil
}

// ListDir returns the entries in the named directory.
func (s *Sandbox) ListDir(path string) ([]os.DirEntry, error) {
	if err := s.validatePath(path); err != nil {
		return nil, fmt.Errorf("sandbox: list %q: %w", path, err)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("sandbox: list %q: %w", path, err)
	}
	return entries, nil
}

// FileInfo returns the os.FileInfo for the named file.
func (s *Sandbox) FileInfo(path string) (os.FileInfo, error) {
	if err := s.validatePath(path); err != nil {
		return nil, fmt.Errorf("sandbox: stat %q: %w", path, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("sandbox: stat %q: %w", path, err)
	}
	return info, nil
}
