package mcp

import (
	"context"
	"strings"
	"testing"
)

func TestPermissionProfile_IsAllowed(t *testing.T) {
	tests := []struct {
		name        string
		profile     PermissionProfile
		toolName    string
		wantAllowed bool
		wantErrSub  string // substring expected in error message (empty = no error)
	}{
		// --- Empty profile: all tools allowed ---
		{
			name:        "empty profile allows any tool",
			profile:     PermissionProfile{},
			toolName:    "delete_file",
			wantAllowed: true,
		},

		// --- AllowedTools only ---
		{
			name:        "allowlist exact match",
			profile:     PermissionProfile{AllowedTools: []string{"read_file"}},
			toolName:    "read_file",
			wantAllowed: true,
		},
		{
			name:        "allowlist no match blocks tool",
			profile:     PermissionProfile{AllowedTools: []string{"read_file"}},
			toolName:    "write_file",
			wantAllowed: false,
			wantErrSub:  "not in the allowlist",
		},
		{
			name:        "allowlist wildcard match",
			profile:     PermissionProfile{AllowedTools: []string{"read_*"}},
			toolName:    "read_dir",
			wantAllowed: true,
		},
		{
			name:        "allowlist wildcard no match",
			profile:     PermissionProfile{AllowedTools: []string{"read_*"}},
			toolName:    "delete_file",
			wantAllowed: false,
			wantErrSub:  "not in the allowlist",
		},
		{
			name:        "allowlist star matches all",
			profile:     PermissionProfile{AllowedTools: []string{"*"}},
			toolName:    "anything",
			wantAllowed: true,
		},
		{
			name:        "invalid allowlist pattern blocks because no valid pattern matched",
			profile:     PermissionProfile{AllowedTools: []string{"[invalid"}},
			toolName:    "read_file",
			wantAllowed: false,
			wantErrSub:  "not in the allowlist",
		},

		// --- DeniedTools only ---
		{
			name:        "denylist exact match blocks tool",
			profile:     PermissionProfile{DeniedTools: []string{"delete_file"}},
			toolName:    "delete_file",
			wantAllowed: false,
			wantErrSub:  "denied by permission profile",
		},
		{
			name:        "denylist no match allows tool",
			profile:     PermissionProfile{DeniedTools: []string{"delete_file"}},
			toolName:    "read_file",
			wantAllowed: true,
		},
		{
			name:        "denylist wildcard blocks matching tool",
			profile:     PermissionProfile{DeniedTools: []string{"delete_*"}},
			toolName:    "delete_dir",
			wantAllowed: false,
			wantErrSub:  "denied by permission profile",
		},
		{
			name:        "denylist wildcard does not block non-matching tool",
			profile:     PermissionProfile{DeniedTools: []string{"delete_*"}},
			toolName:    "read_file",
			wantAllowed: true,
		},
		{
			name:        "denylist star blocks all tools",
			profile:     PermissionProfile{DeniedTools: []string{"*"}},
			toolName:    "read_file",
			wantAllowed: false,
			wantErrSub:  "denied by permission profile",
		},
		{
			name:        "invalid denylist pattern blocks tool",
			profile:     PermissionProfile{DeniedTools: []string{"[invalid"}},
			toolName:    "read_file",
			wantAllowed: false,
			wantErrSub:  "invalid denied tool pattern",
		},

		// --- Both fields: deny wins ---
		{
			name: "tool in both lists is blocked",
			profile: PermissionProfile{
				AllowedTools: []string{"read_*", "write_file"},
				DeniedTools:  []string{"write_file"},
			},
			toolName:    "write_file",
			wantAllowed: false,
			wantErrSub:  "denied by permission profile",
		},
		{
			name: "tool in allowlist only is permitted",
			profile: PermissionProfile{
				AllowedTools: []string{"read_*", "write_file"},
				DeniedTools:  []string{"delete_*"},
			},
			toolName:    "read_file",
			wantAllowed: true,
		},
		{
			name: "tool matching denylist wildcard blocked even though in allowlist",
			profile: PermissionProfile{
				AllowedTools: []string{"delete_*"},
				DeniedTools:  []string{"delete_*"},
			},
			toolName:    "delete_file",
			wantAllowed: false,
			wantErrSub:  "denied by permission profile",
		},
		{
			name: "tool not in allowlist and not denied is still blocked by allowlist",
			profile: PermissionProfile{
				AllowedTools: []string{"read_file"},
				DeniedTools:  []string{"delete_*"},
			},
			toolName:    "write_file",
			wantAllowed: false,
			wantErrSub:  "not in the allowlist",
		},

		// --- Error message contains tool name ---
		{
			name:        "error message includes tool name for denied",
			profile:     PermissionProfile{DeniedTools: []string{"bad_tool"}},
			toolName:    "bad_tool",
			wantAllowed: false,
			wantErrSub:  "bad_tool",
		},
		{
			name:        "error message includes tool name for not in allowlist",
			profile:     PermissionProfile{AllowedTools: []string{"good_tool"}},
			toolName:    "other_tool",
			wantAllowed: false,
			wantErrSub:  "other_tool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.profile.IsAllowed(tt.toolName)

			if got != tt.wantAllowed {
				t.Errorf("IsAllowed(%q) = %v, want %v", tt.toolName, got, tt.wantAllowed)
			}

			if tt.wantErrSub == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.wantErrSub)
				} else if !strings.Contains(err.Error(), tt.wantErrSub) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.wantErrSub)
				}
			}
		})
	}
}

// TestClient_LoadPermissions_CallToolEnforcement verifies that after
// LoadPermissions, CallTool is blocked for denied/not-allowlisted tools
// without sending any request over the transport.
func TestClient_LoadPermissions_CallToolEnforcement(t *testing.T) {
	tests := []struct {
		name      string
		servers   []ServerConfig
		server    string
		toolName  string
		wantBlock bool
		wantSub   string
	}{
		{
			name: "denied tool is blocked before transport call",
			servers: []ServerConfig{{
				Name:        "fs",
				DeniedTools: []string{"delete_*"},
			}},
			server:    "fs",
			toolName:  "delete_file",
			wantBlock: true,
			wantSub:   "denied by permission profile",
		},
		{
			name: "allowed tool passes through to transport",
			servers: []ServerConfig{{
				Name:         "fs",
				AllowedTools: []string{"read_*"},
			}},
			server:    "fs",
			toolName:  "read_file",
			wantBlock: false,
		},
		{
			name: "tool not in allowlist is blocked",
			servers: []ServerConfig{{
				Name:         "fs",
				AllowedTools: []string{"read_*"},
			}},
			server:    "fs",
			toolName:  "write_file",
			wantBlock: true,
			wantSub:   "not in the allowlist",
		},
		{
			name: "server with no permissions allows all tools",
			servers: []ServerConfig{{
				Name: "fs",
			}},
			server:    "fs",
			toolName:  "delete_file",
			wantBlock: false,
		},
		{
			name: "deny wins when tool in both lists",
			servers: []ServerConfig{{
				Name:         "fs",
				AllowedTools: []string{"write_file"},
				DeniedTools:  []string{"write_file"},
			}},
			server:    "fs",
			toolName:  "write_file",
			wantBlock: true,
			wantSub:   "denied by permission profile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mt := newMockTransport()
			c := NewClient()
			c.Connect(tt.server, mt)
			defer c.Close()

			cfg := &MCPConfig{Servers: tt.servers}
			c.LoadPermissions(cfg)

			if tt.wantBlock {
				// Blocked calls must return an error without touching the transport.
				_, err := c.CallTool(context.Background(), tt.server, tt.toolName, nil)
				if err == nil {
					t.Fatalf("CallTool(%q) expected error, got nil", tt.toolName)
				}
				if tt.wantSub != "" && !strings.Contains(err.Error(), tt.wantSub) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.wantSub)
				}
				// Verify no request was sent over the wire.
				select {
				case raw := <-mt.sendCh:
					t.Errorf("transport received a request despite permission block: %s", raw)
				default:
				}
				return
			}

			// Not blocked: the call should reach the transport (even if it times out).
			// We just verify no permission error occurs — a transport error is expected
			// since we don't serve the request.
			done := make(chan error, 1)
			go func() {
				_, err := c.CallTool(context.Background(), tt.server, tt.toolName, nil)
				done <- err
			}()

			// Drain the request from the transport to unblock the call.
			select {
			case <-mt.sendCh:
				// Good: the request reached the transport — permission check passed.
				mt.Close() // Cause the call to return with a transport error.
			case err := <-done:
				if err != nil && strings.Contains(err.Error(), "allowlist") {
					t.Errorf("unexpected allowlist error for permitted tool: %v", err)
				}
				if err != nil && strings.Contains(err.Error(), "denied") {
					t.Errorf("unexpected deny error for permitted tool: %v", err)
				}
			}
		})
	}
}
