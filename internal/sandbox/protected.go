package sandbox

import (
	"path/filepath"
	"strings"
)

// protectedPaths lists glob patterns for files the agent must never modify.
// Patterns without a slash are matched against the file's base name.
// Patterns with a slash are matched against trailing path segments.
var protectedPaths = []string{
	".env",
	".env.*",
	"*.key",
	"*.pem",
	".bolt-cowork/config.yaml",
	".mcp.json",
	".claude/*",
	".git/config",
	".ssh/*",
	".gnupg/*",
	".config/bolt-cowork/*",
}

// IsProtectedPath reports whether path matches any entry in protectedPaths.
// path may be absolute or relative; only the name / suffix is matched.
func IsProtectedPath(path string) bool {
	pathSlash := filepath.ToSlash(path)
	base := filepath.Base(path)

	for _, pattern := range protectedPaths {
		patternSlash := filepath.ToSlash(pattern)

		if !strings.Contains(patternSlash, "/") {
			// Base-name pattern (e.g. "*.key", ".env").
			if matched, _ := filepath.Match(patternSlash, base); matched {
				return true
			}
		} else {
			// Path-suffix pattern (e.g. ".bolt-cowork/config.yaml").
			parts := strings.Split(pathSlash, "/")
			patParts := strings.Split(patternSlash, "/")
			if len(parts) >= len(patParts) {
				suffix := strings.Join(parts[len(parts)-len(patParts):], "/")
				if matched, _ := filepath.Match(patternSlash, suffix); matched {
					return true
				}
			}
		}
	}
	return false
}
