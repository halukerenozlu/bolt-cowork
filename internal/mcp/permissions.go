package mcp

import (
	"fmt"
	"path/filepath"
)

// PermissionProfile defines which tools a server is allowed or denied to call.
// Both fields are optional; omitting both means all tools are permitted.
//
// Conflict rule: DeniedTools always wins. If a tool name matches an entry in
// both AllowedTools and DeniedTools it is blocked.
type PermissionProfile struct {
	// AllowedTools is a list of glob patterns (filepath.Match syntax) for
	// tool names that may be called. When non-empty, a tool must match at
	// least one pattern to be permitted.
	AllowedTools []string `json:"allowed_tools,omitempty"`

	// DeniedTools is a list of glob patterns for tool names that are always
	// blocked, regardless of AllowedTools.
	DeniedTools []string `json:"denied_tools,omitempty"`
}

// IsAllowed reports whether toolName is permitted by the profile.
//
// Evaluation order:
//  1. If toolName matches any DeniedTools pattern → blocked.
//  2. If AllowedTools is non-empty and toolName matches none of them → blocked.
//  3. Otherwise → allowed.
//
// Invalid deny patterns block the call; invalid allow patterns do not grant access.
func (p *PermissionProfile) IsAllowed(toolName string) (bool, error) {
	// Deny check has highest priority.
	for _, pattern := range p.DeniedTools {
		matched, err := filepath.Match(pattern, toolName)
		if err != nil {
			return false, fmt.Errorf("invalid denied tool pattern %q: %w", pattern, err)
		}
		if matched {
			return false, fmt.Errorf("tool %q is denied by permission profile", toolName)
		}
	}

	// Allowlist check: only enforced when the list is non-empty.
	if len(p.AllowedTools) > 0 {
		for _, pattern := range p.AllowedTools {
			matched, err := filepath.Match(pattern, toolName)
			if err != nil {
				continue
			}
			if matched {
				return true, nil
			}
		}
		return false, fmt.Errorf("tool %q is not in the allowlist", toolName)
	}

	return true, nil
}
