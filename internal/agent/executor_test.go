package agent

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/halukerenozlu/bolt-cowork/internal/sandbox"
)

func newTestExecutor(t *testing.T) *Executor {
	t.Helper()
	dir := t.TempDir()
	sb, err := sandbox.New(dir)
	if err != nil {
		t.Fatalf("sandbox.New: %v", err)
	}
	return NewExecutor(sb)
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
