package agent

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/halukerenozlu/bolt-cowork/internal/mcp"
	"github.com/halukerenozlu/bolt-cowork/internal/sandbox"
)

type mockMCPCaller struct {
	result     *mcp.CallToolResult
	err        error
	called     bool
	serverName string
	toolName   string
	args       map[string]any
}

func (m *mockMCPCaller) CallTool(_ context.Context, serverName, toolName string, args map[string]any) (*mcp.CallToolResult, error) {
	m.called = true
	m.serverName = serverName
	m.toolName = toolName
	m.args = args
	return m.result, m.err
}

func newTestExecutor(t *testing.T) *Executor {
	t.Helper()
	dir := t.TempDir()
	sb, err := sandbox.New(dir)
	if err != nil {
		t.Fatalf("sandbox.New: %v", err)
	}
	return NewExecutor(sb)
}

func newTestExecutorWithMCP(t *testing.T, caller *mockMCPCaller) *Executor {
	t.Helper()
	dir := t.TempDir()
	sb, err := sandbox.New(dir)
	if err != nil {
		t.Fatalf("sandbox.New: %v", err)
	}
	registry := mcp.NewToolRegistry()
	registry.AddTools("srv", []mcp.Tool{{Name: "tool"}})
	return NewExecutor(sb, WithMCPCaller(caller), WithMCPToolRegistry(registry))
}

func TestProtectedPath_EnvFile(t *testing.T) {
	exec := newTestExecutor(t)
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action:  ActionWrite,
		Path:    ".env",
		Content: "SECRET=123",
	})
	if err == nil {
		t.Fatal("expected error for writing .env, got nil")
	}
	if !strings.Contains(err.Error(), "protected file") {
		t.Errorf("expected 'protected file' in error, got: %v", err)
	}
}

func TestProtectedPath_ConfigYaml(t *testing.T) {
	exec := newTestExecutor(t)
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action:  ActionWrite,
		Path:    ".bolt-cowork/config.yaml",
		Content: "provider: openai",
	})
	if err == nil {
		t.Fatal("expected error for writing .bolt-cowork/config.yaml, got nil")
	}
	if !strings.Contains(err.Error(), "protected file") {
		t.Errorf("expected 'protected file' in error, got: %v", err)
	}
}

func TestExecutor_CallMCPTool_Success(t *testing.T) {
	caller := &mockMCPCaller{
		result: &mcp.CallToolResult{
			Content: []mcp.ToolResultContent{{Type: "text", Text: "tool output"}},
		},
	}
	exec := newTestExecutorWithMCP(t, caller)

	result, err := exec.ExecuteStep(context.Background(), Step{
		Action:     ActionCallMCPTool,
		ServerName: "srv",
		ToolName:   "tool",
		Args:       map[string]any{"path": "file.txt"},
	})
	if err != nil {
		t.Fatalf("ExecuteStep call_mcp_tool: %v", err)
	}
	if result != "tool output" {
		t.Errorf("result = %q, want %q", result, "tool output")
	}
	if caller.serverName != "srv" || caller.toolName != "tool" {
		t.Errorf("called %s/%s, want srv/tool", caller.serverName, caller.toolName)
	}
	if caller.args["path"] != "file.txt" {
		t.Errorf("args[path] = %v, want file.txt", caller.args["path"])
	}
}

func TestExecutor_CallMCPTool_UnknownToolRejected(t *testing.T) {
	caller := &mockMCPCaller{
		result: &mcp.CallToolResult{
			Content: []mcp.ToolResultContent{{Type: "text", Text: "should not be called"}},
		},
	}
	dir := t.TempDir()
	sb, err := sandbox.New(dir)
	if err != nil {
		t.Fatalf("sandbox.New: %v", err)
	}
	registry := mcp.NewToolRegistry()
	registry.AddTools("srv", []mcp.Tool{{Name: "allowed"}})
	exec := NewExecutor(sb, WithMCPCaller(caller), WithMCPToolRegistry(registry))

	_, err = exec.ExecuteStep(context.Background(), Step{
		Action:     ActionCallMCPTool,
		ServerName: "srv",
		ToolName:   "missing",
	})
	if err == nil {
		t.Fatal("expected error for unregistered MCP tool")
	}
	if !strings.Contains(err.Error(), "mcp: tool not found in registry: srv/missing") {
		t.Errorf("error = %q, want not found registry error", err)
	}
	if caller.called {
		t.Fatal("MCP caller was invoked for an unregistered tool")
	}
}

func TestExecutor_CallMCPTool_NotConfigured(t *testing.T) {
	exec := newTestExecutor(t)
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action:     ActionCallMCPTool,
		ServerName: "srv",
		ToolName:   "tool",
	})
	if err == nil {
		t.Fatal("expected error for missing MCP caller")
	}
	if !strings.Contains(err.Error(), "mcp not configured") {
		t.Errorf("error = %q, want mcp not configured", err)
	}
}

func TestProtectedPath_NormalFile(t *testing.T) {
	exec := newTestExecutor(t)
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action:  ActionWrite,
		Path:    "app.go",
		Content: "package main",
	})
	if err != nil {
		t.Errorf("expected no error for writing app.go, got: %v", err)
	}
}

func TestProtectedPath_CopyDest(t *testing.T) {
	exec := newTestExecutor(t)
	// Source check passes (app.go is not protected); destination .env must be blocked.
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action:      ActionCopy,
		Path:        "app.go",
		Destination: ".env",
	})
	if err == nil {
		t.Fatal("expected error when copying to .env, got nil")
	}
	if !strings.Contains(err.Error(), "protected file") {
		t.Errorf("expected 'protected file' in error, got: %v", err)
	}
}

func TestProtectedPath_MoveDest(t *testing.T) {
	exec := newTestExecutor(t)
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action:      ActionMove,
		Path:        "app.go",
		Destination: ".bolt-cowork/config.yaml",
	})
	if err == nil {
		t.Fatal("expected error when moving to .bolt-cowork/config.yaml, got nil")
	}
	if !strings.Contains(err.Error(), "protected file") {
		t.Errorf("expected 'protected file' in error, got: %v", err)
	}
}

func TestProtectedPath_RenameDest(t *testing.T) {
	exec := newTestExecutor(t)
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action:      ActionRename,
		Path:        "app.go",
		Destination: "secret.key",
	})
	if err == nil {
		t.Fatal("expected error when renaming to secret.key, got nil")
	}
	if !strings.Contains(err.Error(), "protected file") {
		t.Errorf("expected 'protected file' in error, got: %v", err)
	}
}

func TestProtectedPath_ReadDenied(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"ConfigYaml", ".bolt-cowork/config.yaml"},
		{"EnvFile", ".env.local"},
		{"PemKey", "server.pem"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := newTestExecutor(t)
			_, err := exec.ExecuteStep(context.Background(), Step{
				Action: ActionRead,
				Path:   tt.path,
			})
			if err == nil {
				t.Fatalf("expected error for reading %q, got nil", tt.path)
			}
			if !strings.Contains(err.Error(), "protected file") {
				t.Errorf("expected 'protected file' in error, got: %v", err)
			}
		})
	}
}

func TestProtectedPath_CopySourceProtected(t *testing.T) {
	exec := newTestExecutor(t)
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action:      ActionCopy,
		Path:        ".env",
		Destination: "safe.txt",
	})
	if err == nil {
		t.Fatal("expected error when copying from .env, got nil")
	}
	if !strings.Contains(err.Error(), "protected file") {
		t.Errorf("expected 'protected file' in error, got: %v", err)
	}
}

func TestProtectedPath_CopyDestProtected(t *testing.T) {
	exec := newTestExecutor(t)
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action:      ActionCopy,
		Path:        "safe.txt",
		Destination: ".ssh/authorized_keys",
	})
	if err == nil {
		t.Fatal("expected error when copying to .ssh/authorized_keys, got nil")
	}
	if !strings.Contains(err.Error(), "protected file") {
		t.Errorf("expected 'protected file' in error, got: %v", err)
	}
}

func newTestExecutorWithDir(t *testing.T, dir string) *Executor {
	t.Helper()
	sb, err := sandbox.New(dir)
	if err != nil {
		t.Fatalf("sandbox.New: %v", err)
	}
	return NewExecutor(sb)
}

func TestProtectedPath_ReadViaSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}
	dir := t.TempDir()

	// Create the protected target file.
	envPath := filepath.Join(dir, ".env.local")
	if err := os.WriteFile(envPath, []byte("SECRET=abc"), 0644); err != nil {
		t.Fatal(err)
	}
	// Create a symlink that points to the protected file.
	linkPath := filepath.Join(dir, "safe.txt")
	if err := os.Symlink(envPath, linkPath); err != nil {
		t.Fatal(err)
	}

	exec := newTestExecutorWithDir(t, dir)
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action: ActionRead,
		Path:   "safe.txt",
	})
	if err == nil {
		t.Fatal("expected error reading symlink to .env.local, got nil")
	}
	if !strings.Contains(err.Error(), "protected file") {
		t.Errorf("expected 'protected file' in error, got: %v", err)
	}
}

func TestProtectedPath_CopySourceViaSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}
	dir := t.TempDir()

	// Create the protected target file.
	keyPath := filepath.Join(dir, "server.pem")
	if err := os.WriteFile(keyPath, []byte("-----BEGIN CERT-----"), 0644); err != nil {
		t.Fatal(err)
	}
	// Symlink alias.txt -> server.pem
	linkPath := filepath.Join(dir, "alias.txt")
	if err := os.Symlink(keyPath, linkPath); err != nil {
		t.Fatal(err)
	}

	exec := newTestExecutorWithDir(t, dir)
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action:      ActionCopy,
		Path:        "alias.txt",
		Destination: "output.txt",
	})
	if err == nil {
		t.Fatal("expected error copying symlink to server.pem, got nil")
	}
	if !strings.Contains(err.Error(), "protected file") {
		t.Errorf("expected 'protected file' in error, got: %v", err)
	}
}

func TestProtectedPath_CopyIntoDirBypass(t *testing.T) {
	dir := t.TempDir()

	// Create source file.
	srcPath := filepath.Join(dir, "authorized_keys")
	if err := os.WriteFile(srcPath, []byte("ssh-rsa AAAA..."), 0644); err != nil {
		t.Fatal(err)
	}
	// Create .ssh directory as destination.
	sshDir := filepath.Join(dir, ".ssh")
	if err := os.MkdirAll(sshDir, 0755); err != nil {
		t.Fatal(err)
	}

	exec := newTestExecutorWithDir(t, dir)
	// Copy authorized_keys into .ssh/ — final path is .ssh/authorized_keys which
	// matches ".ssh/*" in protectedPaths.
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action:      ActionCopy,
		Path:        "authorized_keys",
		Destination: ".ssh",
	})
	if err == nil {
		t.Fatal("expected error when copying into .ssh directory, got nil")
	}
	if !strings.Contains(err.Error(), "protected file") {
		t.Errorf("expected 'protected file' in error, got: %v", err)
	}
}

func TestProtectedPath_CopyViaDirSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}
	dir := t.TempDir()

	// Create source file.
	srcPath := filepath.Join(dir, "authorized_keys")
	if err := os.WriteFile(srcPath, []byte("ssh-rsa AAAA..."), 0644); err != nil {
		t.Fatal(err)
	}
	// Create actual .ssh directory.
	sshDir := filepath.Join(dir, ".ssh")
	if err := os.MkdirAll(sshDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Create a symlink safe_dir -> .ssh.
	linkDir := filepath.Join(dir, "safe_dir")
	if err := os.Symlink(sshDir, linkDir); err != nil {
		t.Fatal(err)
	}

	exec := newTestExecutorWithDir(t, dir)
	// Copy authorized_keys into safe_dir (which is really .ssh).
	// Final resolved path: .ssh/authorized_keys — must be blocked.
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action:      ActionCopy,
		Path:        "authorized_keys",
		Destination: "safe_dir",
	})
	if err == nil {
		t.Fatal("expected error when copying into symlinked .ssh directory, got nil")
	}
	if !strings.Contains(err.Error(), "Protected") {
		t.Errorf("expected 'Protected' in error, got: %v", err)
	}
}

func TestProtectedPath_WriteViaSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}
	dir := t.TempDir()

	// Create target protected file.
	envPath := filepath.Join(dir, ".env.local")
	if err := os.WriteFile(envPath, []byte("OLD_SECRET=1"), 0644); err != nil {
		t.Fatal(err)
	}
	// Symlink safe.txt -> .env.local
	linkPath := filepath.Join(dir, "safe.txt")
	if err := os.Symlink(envPath, linkPath); err != nil {
		t.Fatal(err)
	}

	exec := newTestExecutorWithDir(t, dir)
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action:  ActionWrite,
		Path:    "safe.txt",
		Content: "NEW_SECRET=2",
	})
	if err == nil {
		t.Fatal("expected error writing via symlink to .env.local, got nil")
	}
	if !strings.Contains(err.Error(), "protected file") {
		t.Errorf("expected 'protected file' in error, got: %v", err)
	}
}

func TestProtectedPath_WriteViaDirSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}
	dir := t.TempDir()

	// Create actual .ssh directory.
	sshDir := filepath.Join(dir, ".ssh")
	if err := os.MkdirAll(sshDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Symlink safe_dir -> .ssh
	linkDir := filepath.Join(dir, "safe_dir")
	if err := os.Symlink(sshDir, linkDir); err != nil {
		t.Fatal(err)
	}

	exec := newTestExecutorWithDir(t, dir)
	// Write to safe_dir/authorized_keys — resolves to .ssh/authorized_keys.
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action:  ActionWrite,
		Path:    "safe_dir/authorized_keys",
		Content: "ssh-rsa AAAA...",
	})
	if err == nil {
		t.Fatal("expected error writing via symlinked .ssh directory, got nil")
	}
	if !strings.Contains(err.Error(), "protected file") {
		t.Errorf("expected 'protected file' in error, got: %v", err)
	}
}

func TestProtectedPath_MoveDestViaSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}
	dir := t.TempDir()

	// Create source file.
	srcPath := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(srcPath, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	// Create target protected file that the symlink points to.
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("SECRET=x"), 0644); err != nil {
		t.Fatal(err)
	}
	// Symlink alias.txt -> .env
	linkPath := filepath.Join(dir, "alias.txt")
	if err := os.Symlink(envPath, linkPath); err != nil {
		t.Fatal(err)
	}

	exec := newTestExecutorWithDir(t, dir)
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action:      ActionMove,
		Path:        "data.txt",
		Destination: "alias.txt",
	})
	if err == nil {
		t.Fatal("expected error moving to symlink pointing at .env, got nil")
	}
	if !strings.Contains(err.Error(), "protected file") {
		t.Errorf("expected 'protected file' in error, got: %v", err)
	}
}

func TestProtectedPath_DeleteViaSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}
	dir := t.TempDir()

	// Create protected target.
	keyPath := filepath.Join(dir, "server.pem")
	if err := os.WriteFile(keyPath, []byte("-----BEGIN KEY-----"), 0644); err != nil {
		t.Fatal(err)
	}
	// Symlink alias.txt -> server.pem
	linkPath := filepath.Join(dir, "alias.txt")
	if err := os.Symlink(keyPath, linkPath); err != nil {
		t.Fatal(err)
	}

	exec := newTestExecutorWithDir(t, dir)
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action: ActionDelete,
		Path:   "alias.txt",
	})
	if err == nil {
		t.Fatal("expected error deleting symlink to server.pem, got nil")
	}
	if !strings.Contains(err.Error(), "protected file") {
		t.Errorf("expected 'protected file' in error, got: %v", err)
	}
}

func TestProtectedPath_MkdirDirect(t *testing.T) {
	exec := newTestExecutor(t)
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action: ActionMkdir,
		Path:   ".ssh/newdir",
	})
	if err == nil {
		t.Fatal("expected error for mkdir .ssh/newdir, got nil")
	}
	if !strings.Contains(err.Error(), "protected file") {
		t.Errorf("expected 'protected file' in error, got: %v", err)
	}
}

func TestProtectedPath_MkdirViaSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}
	dir := t.TempDir()

	// Create actual .ssh directory.
	sshDir := filepath.Join(dir, ".ssh")
	if err := os.MkdirAll(sshDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Symlink safe_dir -> .ssh
	linkDir := filepath.Join(dir, "safe_dir")
	if err := os.Symlink(sshDir, linkDir); err != nil {
		t.Fatal(err)
	}

	exec := newTestExecutorWithDir(t, dir)
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action: ActionMkdir,
		Path:   "safe_dir/newdir",
	})
	if err == nil {
		t.Fatal("expected error for mkdir via symlinked .ssh directory, got nil")
	}
	if !strings.Contains(err.Error(), "protected file") {
		t.Errorf("expected 'protected file' in error, got: %v", err)
	}
}

func TestProtectedPath_ListDirect(t *testing.T) {
	dir := t.TempDir()
	sshDir := filepath.Join(dir, ".ssh")
	if err := os.MkdirAll(sshDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "id_rsa"), []byte("key"), 0600); err != nil {
		t.Fatal(err)
	}

	exec := newTestExecutorWithDir(t, dir)
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action: ActionList,
		Path:   ".ssh",
	})
	if err == nil {
		t.Fatal("expected error for list .ssh, got nil")
	}
	if !strings.Contains(err.Error(), "protected file") {
		t.Errorf("expected 'protected file' in error, got: %v", err)
	}
}

func TestProtectedPath_ListViaSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}
	dir := t.TempDir()

	sshDir := filepath.Join(dir, ".ssh")
	if err := os.MkdirAll(sshDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "id_rsa"), []byte("key"), 0600); err != nil {
		t.Fatal(err)
	}
	linkDir := filepath.Join(dir, "safe_dir")
	if err := os.Symlink(sshDir, linkDir); err != nil {
		t.Fatal(err)
	}

	exec := newTestExecutorWithDir(t, dir)
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action: ActionList,
		Path:   "safe_dir",
	})
	if err == nil {
		t.Fatal("expected error for list via symlinked .ssh directory, got nil")
	}
	if !strings.Contains(err.Error(), "protected file") {
		t.Errorf("expected 'protected file' in error, got: %v", err)
	}
}

func TestProtectedPath_MkdirDeepSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}
	dir := t.TempDir()

	sshDir := filepath.Join(dir, ".ssh")
	if err := os.MkdirAll(sshDir, 0755); err != nil {
		t.Fatal(err)
	}
	linkDir := filepath.Join(dir, "safe_dir")
	if err := os.Symlink(sshDir, linkDir); err != nil {
		t.Fatal(err)
	}

	exec := newTestExecutorWithDir(t, dir)
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action: ActionMkdir,
		Path:   "safe_dir/a/b/c",
	})
	if err == nil {
		t.Fatal("expected error for deep mkdir via symlinked .ssh directory, got nil")
	}
	if !strings.Contains(err.Error(), "protected file") {
		t.Errorf("expected 'protected file' in error, got: %v", err)
	}
}

func TestResolveAndCheckProtected_NonExistentParent(t *testing.T) {
	// Ensure no panic when both path and parent don't exist.
	resolved, err := resolveAndCheckProtected(filepath.Join(t.TempDir(), "no_such_dir", "file.txt"))
	if err != nil {
		t.Fatalf("unexpected error for non-protected path with non-existent parent: %v", err)
	}
	if resolved == "" {
		t.Fatal("resolved path should not be empty")
	}
}

func TestResolveAndCheckProtected_NonExistentParent_Protected(t *testing.T) {
	// Even with non-existent parent, a protected basename must be caught.
	_, err := resolveAndCheckProtected(filepath.Join(t.TempDir(), "no_such_dir", ".env"))
	if err == nil {
		t.Fatal("expected error for .env under non-existent parent, got nil")
	}
	if !strings.Contains(err.Error(), "protected file") {
		t.Errorf("expected 'protected file' in error, got: %v", err)
	}
}

func TestContainsADS(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"file.txt:hidden", true},
		{"safe.txt:.env", true},
		{`subdir\file.txt:stream`, true},
		// Drive letter is allowed
		{`C:\Users\test\file.txt`, false},
		{`D:\data\hello.txt`, false},
		// No colon at all
		{"normal-file.txt", false},
		{"subdir/path/to/file", false},
	}

	if runtime.GOOS != "windows" {
		// On non-Windows, containsADS always returns false.
		for _, tt := range tests {
			if containsADS(tt.path) {
				t.Errorf("containsADS(%q) = true on non-Windows, want false", tt.path)
			}
		}
		return
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := containsADS(tt.path)
			if got != tt.want {
				t.Errorf("containsADS(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestADS_Blocked_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("ADS check is Windows-only")
	}

	exec := newTestExecutor(t)
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action:  ActionWrite,
		Path:    "file.txt:hidden",
		Content: "secret data",
	})
	if err == nil {
		t.Fatal("expected error for ADS path file.txt:hidden, got nil")
	}
	if !strings.Contains(err.Error(), "alternate data stream") {
		t.Errorf("expected 'alternate data stream' in error, got: %v", err)
	}
}

func TestADS_CopyDest_Blocked_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("ADS check is Windows-only")
	}

	dir := t.TempDir()
	srcPath := filepath.Join(dir, "src.txt")
	os.WriteFile(srcPath, []byte("data"), 0644)

	exec := newTestExecutorWithDir(t, dir)
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action:      ActionCopy,
		Path:        "src.txt",
		Destination: "dst.txt:hidden",
	})
	if err == nil {
		t.Fatal("expected error for ADS destination dst.txt:hidden, got nil")
	}
	if !strings.Contains(err.Error(), "alternate data stream") {
		t.Errorf("expected 'alternate data stream' in error, got: %v", err)
	}
}

func TestADS_DriveLetterAllowed_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("ADS drive letter check is Windows-only")
	}

	// containsADS should return false for a normal drive-letter path.
	if containsADS(`C:\Users\test\file.txt`) {
		t.Error("containsADS should allow normal drive-letter paths")
	}
}

func TestADS_NotCheckedOnUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("this test verifies Unix behavior")
	}

	// On Unix, colons are valid in filenames.
	if containsADS("file:with:colons") {
		t.Error("containsADS should always return false on Unix")
	}
}

// --- P2-2: Reserved filename tests ---

func TestReservedFilename_CON_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("reserved filenames only apply on Windows")
	}
	exec := newTestExecutor(t)
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action:  ActionWrite,
		Path:    "CON",
		Content: "data",
	})
	if err == nil {
		t.Fatal("expected error for reserved filename CON, got nil")
	}
	if !strings.Contains(err.Error(), "reserved filename") {
		t.Errorf("expected 'reserved filename' in error, got: %v", err)
	}
}

func TestReservedFilename_NulWithExt_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("reserved filenames only apply on Windows")
	}
	exec := newTestExecutor(t)
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action:  ActionWrite,
		Path:    "NUL.txt",
		Content: "data",
	})
	if err == nil {
		t.Fatal("expected error for reserved filename NUL.txt, got nil")
	}
	if !strings.Contains(err.Error(), "reserved filename") {
		t.Errorf("expected 'reserved filename' in error, got: %v", err)
	}
}

func TestReservedFilename_NotCheckedOnUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("this test verifies Unix behavior")
	}
	if isReservedFilename("CON") {
		t.Error("isReservedFilename should always return false on Unix")
	}
}

// --- P2-3: Large content size limit tests ---

func TestWrite_LargeContent_SizeLimit(t *testing.T) {
	exec := newTestExecutor(t)
	largeContent := strings.Repeat("x", maxWriteContentBytes+1)
	_, err := exec.ExecuteStep(context.Background(), Step{
		Action:  ActionWrite,
		Path:    "huge.txt",
		Content: largeContent,
	})
	if err == nil {
		t.Fatal("expected error for oversized content, got nil")
	}
	if !strings.Contains(err.Error(), "content too large") {
		t.Errorf("expected 'content too large' in error, got: %v", err)
	}
}

func TestWrite_NormalContent_Allowed(t *testing.T) {
	exec := newTestExecutor(t)
	normalContent := strings.Repeat("x", 1024)
	result, err := exec.ExecuteStep(context.Background(), Step{
		Action:  ActionWrite,
		Path:    "normal.txt",
		Content: normalContent,
	})
	if err != nil {
		t.Fatalf("unexpected error for normal-size write: %v", err)
	}
	if !strings.Contains(result, "Wrote") {
		t.Errorf("expected 'Wrote' in result, got: %s", result)
	}
}
