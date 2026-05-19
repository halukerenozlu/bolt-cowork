package mcp_test

import (
	"testing"

	"github.com/halukerenozlu/bolt-cowork/internal/mcp"
)

func TestIsDangerousTool(t *testing.T) {
	tests := []struct {
		name       string
		toolName   string
		toolDesc   string
		wantDanger bool
	}{
		// Dangerous: keyword in name
		{name: "write in name", toolName: "write_file", toolDesc: "", wantDanger: true},
		{name: "delete in name", toolName: "delete_row", toolDesc: "", wantDanger: true},
		{name: "remove in name", toolName: "remove_tag", toolDesc: "", wantDanger: true},
		{name: "create in name", toolName: "create_user", toolDesc: "", wantDanger: true},
		{name: "update in name", toolName: "update_record", toolDesc: "", wantDanger: true},
		// Dangerous: keyword in description
		{name: "exec in description", toolName: "shell", toolDesc: "exec a command", wantDanger: true},
		{name: "run in description", toolName: "script", toolDesc: "run a script", wantDanger: true},
		{name: "modify in description", toolName: "config", toolDesc: "modify settings", wantDanger: true},
		{name: "drop in description", toolName: "db", toolDesc: "drop database", wantDanger: true},
		{name: "truncate in description", toolName: "db_op", toolDesc: "truncate table", wantDanger: true},
		{name: "shell in name", toolName: "shell", toolDesc: "opens a command interface", wantDanger: true},
		{name: "bash in name", toolName: "bash", toolDesc: "opens a command interface", wantDanger: true},
		{name: "patch in name", toolName: "apply_patch", toolDesc: "changes a file", wantDanger: true},
		{name: "apply in name", toolName: "apply_change", toolDesc: "changes state", wantDanger: true},
		{name: "send in name", toolName: "send_message", toolDesc: "sends a message", wantDanger: true},
		{name: "mail in name", toolName: "mail_user", toolDesc: "sends email", wantDanger: true},
		{name: "replace in name", toolName: "replace_text", toolDesc: "edits a file", wantDanger: true},
		{name: "filesystem in name", toolName: "filesystem", toolDesc: "accesses files", wantDanger: true},
		{name: "move in name", toolName: "move_file", toolDesc: "moves a file", wantDanger: true},
		{name: "rename in name", toolName: "rename_file", toolDesc: "renames a file", wantDanger: true},
		{name: "copy in name", toolName: "copy_file", toolDesc: "copies a file", wantDanger: true},
		{name: "upload in name", toolName: "upload_file", toolDesc: "uploads a file", wantDanger: true},
		{name: "deploy in name", toolName: "deploy_app", toolDesc: "deploys an app", wantDanger: true},
		{name: "install in name", toolName: "install_package", toolDesc: "installs a package", wantDanger: true},
		{name: "uninstall in name", toolName: "uninstall_package", toolDesc: "removes a package", wantDanger: true},
		{name: "kill in name", toolName: "kill_process", toolDesc: "kills a process", wantDanger: true},
		{name: "terminate in name", toolName: "terminate_process", toolDesc: "terminates a process", wantDanger: true},
		// Dangerous: case insensitive
		{name: "uppercase name", toolName: "DELETE_TABLE", toolDesc: "", wantDanger: true},
		{name: "uppercase description", toolName: "table_op", toolDesc: "WRITE data to table", wantDanger: true},
		// Safe: read-only tools
		{name: "list files", toolName: "list_files", toolDesc: "lists files in a directory", wantDanger: false},
		{name: "get status", toolName: "get_status", toolDesc: "returns current status", wantDanger: false},
		{name: "search docs", toolName: "search_docs", toolDesc: "search documentation", wantDanger: false},
		{name: "fetch url", toolName: "fetch_url", toolDesc: "retrieve content from a URL", wantDanger: false},
		{name: "read config", toolName: "read_config", toolDesc: "returns configuration values", wantDanger: false},
		// Edge cases
		{name: "empty name and desc", toolName: "", toolDesc: "", wantDanger: true},
		{name: "empty description", toolName: "get_weather", toolDesc: "", wantDanger: true},
		{name: "keyword only as substring boundary", toolName: "writer", toolDesc: "", wantDanger: true}, // "writer" contains "write"
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tool := mcp.MCPTool{Name: tc.toolName, Description: tc.toolDesc}
			got := mcp.IsDangerousTool(tool)
			if got != tc.wantDanger {
				t.Errorf("IsDangerousTool(name=%q, desc=%q) = %v, want %v",
					tc.toolName, tc.toolDesc, got, tc.wantDanger)
			}
		})
	}
}

func TestShouldRequestApproval(t *testing.T) {
	safe := mcp.MCPTool{Name: "get_status", Description: "returns current status"}
	dangerous := mcp.MCPTool{Name: "delete_file", Description: "permanently deletes a file"}

	tests := []struct {
		name  string
		tool  mcp.MCPTool
		phase string
		mode  mcp.MCPApprovalMode
		want  bool
	}{
		// full: always prompt regardless of tool or phase
		{name: "full/planning/safe", tool: safe, phase: "planning", mode: mcp.MCPApprovalFull, want: true},
		{name: "full/planning/dangerous", tool: dangerous, phase: "planning", mode: mcp.MCPApprovalFull, want: true},
		{name: "full/execution/safe", tool: safe, phase: "execution", mode: mcp.MCPApprovalFull, want: true},
		{name: "full/execution/dangerous", tool: dangerous, phase: "execution", mode: mcp.MCPApprovalFull, want: true},

		// plan-only: prompt during planning, auto-execute during execution
		{name: "plan-only/planning/safe", tool: safe, phase: "planning", mode: mcp.MCPApprovalPlanOnly, want: true},
		{name: "plan-only/planning/dangerous", tool: dangerous, phase: "planning", mode: mcp.MCPApprovalPlanOnly, want: true},
		{name: "plan-only/execution/safe", tool: safe, phase: "execution", mode: mcp.MCPApprovalPlanOnly, want: false},
		{name: "plan-only/execution/dangerous", tool: dangerous, phase: "execution", mode: mcp.MCPApprovalPlanOnly, want: false},

		// dangerous-only: prompt only for dangerous tools during execution
		{name: "dangerous-only/planning/safe", tool: safe, phase: "planning", mode: mcp.MCPApprovalDangerousOnly, want: false},
		{name: "dangerous-only/planning/dangerous", tool: dangerous, phase: "planning", mode: mcp.MCPApprovalDangerousOnly, want: false},
		{name: "dangerous-only/execution/safe", tool: safe, phase: "execution", mode: mcp.MCPApprovalDangerousOnly, want: false},
		{name: "dangerous-only/execution/dangerous", tool: dangerous, phase: "execution", mode: mcp.MCPApprovalDangerousOnly, want: true},

		// none: never prompt
		{name: "none/planning/safe", tool: safe, phase: "planning", mode: mcp.MCPApprovalNone, want: false},
		{name: "none/planning/dangerous", tool: dangerous, phase: "planning", mode: mcp.MCPApprovalNone, want: false},
		{name: "none/execution/safe", tool: safe, phase: "execution", mode: mcp.MCPApprovalNone, want: false},
		{name: "none/execution/dangerous", tool: dangerous, phase: "execution", mode: mcp.MCPApprovalNone, want: false},

		// unknown mode: default to true (safe)
		{name: "unknown/execution/safe", tool: safe, phase: "execution", mode: mcp.MCPApprovalMode("unknown"), want: true},
		{name: "empty mode/execution/safe", tool: safe, phase: "execution", mode: mcp.MCPApprovalMode(""), want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mcp.ShouldRequestApproval(tc.tool, tc.phase, tc.mode)
			if got != tc.want {
				t.Errorf("ShouldRequestApproval(tool=%q, phase=%q, mode=%q) = %v, want %v",
					tc.tool.Name, tc.phase, tc.mode, got, tc.want)
			}
		})
	}
}
