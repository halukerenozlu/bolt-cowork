package sandbox

import (
	"path/filepath"
	"runtime"
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
	".bolt-cowork/mcp.json",
	".cowork/sessions",
	".cowork/sessions/*",
	".mcp.json",
	".claude",
	".claude/*",
	".git/config",
	".ssh",
	".ssh/*",
	".gnupg",
	".gnupg/*",
	".config/bolt-cowork",
	".config/bolt-cowork/*",
}

// IsProtectedPath reports whether path matches any entry in protectedPaths.
// path may be absolute or relative; only the name / suffix is matched.
// On Windows, matching is case-insensitive because the filesystem is.
func IsProtectedPath(path string) bool {
	pathSlash := filepath.ToSlash(path)
	base := filepath.Base(path)

	// On Windows, normalize to lowercase for case-insensitive matching.
	if runtime.GOOS == "windows" {
		pathSlash = strings.ToLower(pathSlash)
		base = strings.ToLower(base)
	}

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
			if strings.HasSuffix(patternSlash, "/*") {
				dirPattern := strings.TrimSuffix(patternSlash, "/*")
				for i := range parts {
					subPath := strings.Join(parts[i:], "/")
					if subPath == dirPattern || strings.HasPrefix(subPath, dirPattern+"/") {
						return true
					}
				}
			}
		}
	}
	return false
}
