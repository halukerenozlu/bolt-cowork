package mcp

import "strings"

// MCPApprovalMode controls when the agent pauses to confirm MCP tool calls.
type MCPApprovalMode string

const (
	// MCPApprovalFull prompts the user before every MCP tool call.
	MCPApprovalFull MCPApprovalMode = "full"

	// MCPApprovalPlanOnly prompts only during the planning phase.
	MCPApprovalPlanOnly MCPApprovalMode = "plan-only"

// MCPApprovalDangerousOnly prompts only for tools whose metadata is missing or
// contains a keyword associated with destructive or state-changing operations.
	MCPApprovalDangerousOnly MCPApprovalMode = "dangerous-only"

	// MCPApprovalNone never prompts; all MCP tool calls execute automatically.
	MCPApprovalNone MCPApprovalMode = "none"
)

// dangerousToolKeywords are matched (case-insensitively) against a tool's
// Name and Description to decide whether it may have destructive side effects.
var dangerousToolKeywords = []string{
	"write", "delete", "remove", "exec", "run",
	"create", "modify", "update", "drop", "truncate",
	"shell", "bash", "patch", "apply", "send", "mail",
	"replace", "filesystem", "move", "rename", "copy",
	"upload", "deploy", "install", "uninstall", "kill", "terminate",
}

// IsDangerousTool returns true when the tool has no description or when its
// name or description contains at least one keyword associated with destructive
// or state-changing operations.
func IsDangerousTool(tool MCPTool) bool {
	if strings.TrimSpace(tool.Description) == "" {
		return true
	}
	haystack := strings.ToLower(tool.Name + " " + tool.Description)
	for _, kw := range dangerousToolKeywords {
		if strings.Contains(haystack, kw) {
			return true
		}
	}
	return false
}

// ShouldRequestApproval reports whether the user should be prompted for
// approval before the given MCP tool call proceeds.
//
// Parameters:
//   - tool:  the MCP tool about to be called
//   - phase: "planning" | "execution"
//   - mode:  the configured MCPApprovalMode
func ShouldRequestApproval(tool MCPTool, phase string, mode MCPApprovalMode) bool {
	switch mode {
	case MCPApprovalFull:
		return true
	case MCPApprovalPlanOnly:
		return phase == "planning"
	case MCPApprovalDangerousOnly:
		return phase == "execution" && IsDangerousTool(tool)
	case MCPApprovalNone:
		return false
	default:
		return true // safe default: always prompt on unknown mode
	}
}
