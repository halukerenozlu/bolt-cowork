package sandbox

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestProtectedPath_ListContainsExpected(t *testing.T) {
	paths := []struct {
		path string
		want bool
	}{
		{".ssh/id_rsa", true},
		{".ssh/authorized_keys", true},
		{".ssh/a/b/c", true},
		{".ssh", true},
		{".gnupg/pubring.gpg", true},
		{".gnupg/secring.gpg", true},
		{".gnupg/private/key", true},
		{".gnupg", true},
		{".config/bolt-cowork/config.yaml", true},
		{".config/bolt-cowork/state.json", true},
		{".config/bolt-cowork/cache/state.json", true},
		{".config/bolt-cowork", true},
		{".env", true},
		{".env.local", true},
		{"secret.key", true},
		{"server.pem", true},
		{".bolt-cowork/config.yaml", true},
		{".bolt-cowork/mcp.json", true},
		{"/home/user/.bolt-cowork/mcp.json", true},
		{".cowork/sessions", true},
		{".cowork/sessions/abc.json", true},
		{"/workspace/.cowork/sessions/nested/abc.json", true},
		{"mcp.json", false}, // standalone file with same basename is not protected
		{".mcp.json", true},
		{".claude/settings.json", true},
		{".claude/hooks/pre-tool.sh", true},
		{".claude", true},
		{".git/config", true},
		// Not protected
		{"readme.txt", false},
		{"main.go", false},
		{"config.yaml", false},
	}

	for _, tt := range paths {
		t.Run(tt.path, func(t *testing.T) {
			got := IsProtectedPath(tt.path)
			if got != tt.want {
				t.Errorf("IsProtectedPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestProtectedPath_ReadDenied(t *testing.T) {
	dir := t.TempDir()

	// Create sandbox with denied patterns matching protected paths.
	sb, err := New(dir, WithDeniedPatterns(".ssh/*", ".gnupg/*", ".gitconfig"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Create files inside sandbox that match denied patterns.
	sshDir := filepath.Join(dir, ".ssh")
	os.MkdirAll(sshDir, 0755)
	os.WriteFile(filepath.Join(sshDir, "id_rsa"), []byte("private"), 0600)

	gnupgDir := filepath.Join(dir, ".gnupg")
	os.MkdirAll(gnupgDir, 0755)
	os.WriteFile(filepath.Join(gnupgDir, "secring.gpg"), []byte("keyring"), 0600)

	os.WriteFile(filepath.Join(dir, ".gitconfig"), []byte("[user]"), 0644)

	tests := []struct {
		name string
		path string
	}{
		{"ssh key", filepath.Join(sshDir, "id_rsa")},
		{"gnupg keyring", filepath.Join(gnupgDir, "secring.gpg")},
		{"gitconfig", filepath.Join(dir, ".gitconfig")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := sb.ReadFile(tt.path)
			if err == nil {
				t.Errorf("ReadFile(%q) should have been denied", tt.path)
			}
		})
	}
}

func TestProtectedPath_WriteDenied(t *testing.T) {
	dir := t.TempDir()
	sb, err := New(dir, WithDeniedPatterns("*.env", ".ssh/*"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	sshDir := filepath.Join(dir, ".ssh")
	os.MkdirAll(sshDir, 0755)

	tests := []struct {
		name string
		path string
	}{
		{"env file", filepath.Join(dir, ".env")},
		{"ssh authorized_keys", filepath.Join(sshDir, "authorized_keys")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sb.WriteFile(tt.path, []byte("data"))
			if err == nil {
				t.Errorf("WriteFile(%q) should have been denied", tt.path)
			}
		})
	}
}

func TestProtectedPath_DeleteDenied(t *testing.T) {
	dir := t.TempDir()
	sb, err := New(dir, WithDeniedPatterns("*.env", "*.key"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Create files first so delete has something to target.
	envFile := filepath.Join(dir, ".env")
	keyFile := filepath.Join(dir, "secret.key")
	os.WriteFile(envFile, []byte("SECRET=x"), 0644)
	os.WriteFile(keyFile, []byte("key"), 0644)

	tests := []struct {
		name string
		path string
	}{
		{"env file", envFile},
		{"key file", keyFile},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sb.DeleteFile(tt.path)
			if err == nil {
				t.Errorf("DeleteFile(%q) should have been denied", tt.path)
			}
		})
	}
}

func TestProtectedPath_TraversalBlocked(t *testing.T) {
	dir := t.TempDir()
	sb, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	traversals := []string{
		filepath.Join(dir, "..", "..", "etc", "passwd"),
		filepath.Join(dir, "..", "..", "..", ".ssh", "id_rsa"),
	}

	for _, p := range traversals {
		t.Run(p, func(t *testing.T) {
			_, err := sb.ReadFile(p)
			if err == nil {
				t.Errorf("ReadFile(%q) should have been blocked (traversal)", p)
			}
		})
	}
}

func TestProtectedPath_SymlinkBlocked(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink test not reliable on Windows")
	}

	dir := t.TempDir()
	outside := t.TempDir()
	secretFile := filepath.Join(outside, "secret.txt")
	os.WriteFile(secretFile, []byte("top-secret"), 0644)

	sb, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	link := filepath.Join(dir, "escape-link")
	if err := os.Symlink(secretFile, link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	_, err = sb.ReadFile(link)
	if err == nil {
		t.Error("ReadFile via symlink to outside should have been blocked")
	}
}

func TestProtectedPath_CaseInsensitive_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("case-insensitive protected path check is Windows-only")
	}

	cases := []struct {
		path string
		want bool
	}{
		{".SSH/id_rsa", true},
		{".Ssh/config", true},
		{".SSH", true},
		{".GNUPG/pubring.gpg", true},
		{".Env", true},
		{".ENV.LOCAL", true},
		{"SECRET.KEY", true},
		{".Config/Bolt-Cowork/config.yaml", true},
		{".CLAUDE/settings.json", true},
		// Still not protected
		{"readme.txt", false},
		{"main.go", false},
	}

	for _, tt := range cases {
		t.Run(tt.path, func(t *testing.T) {
			got := IsProtectedPath(tt.path)
			if got != tt.want {
				t.Errorf("IsProtectedPath(%q) = %v, want %v (Windows case-insensitive)", tt.path, got, tt.want)
			}
		})
	}
}

func TestProtectedPath_CaseSensitive_Unix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("case-sensitive protected path check is Unix-only")
	}

	// On Unix, .SSH (uppercase) is a different directory from .ssh (lowercase).
	// The protected list only contains lowercase entries, so uppercase should NOT match.
	cases := []struct {
		path string
		want bool
	}{
		{".SSH/id_rsa", false},
		{".SSH", false},
		{".GNUPG/pubring.gpg", false},
		{".ENV", false},
		// Lowercase still matches
		{".ssh/id_rsa", true},
		{".gnupg/pubring.gpg", true},
		{".env", true},
	}

	for _, tt := range cases {
		t.Run(tt.path, func(t *testing.T) {
			got := IsProtectedPath(tt.path)
			if got != tt.want {
				t.Errorf("IsProtectedPath(%q) = %v, want %v (Unix case-sensitive)", tt.path, got, tt.want)
			}
		})
	}
}

func TestProtectedPath_AllowedInsideSandbox(t *testing.T) {
	dir := t.TempDir()
	sb, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	readme := filepath.Join(dir, "readme.txt")
	if err := sb.WriteFile(readme, []byte("hello")); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	data, err := sb.ReadFile(readme)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("ReadFile = %q, want %q", data, "hello")
	}

	if err := sb.DeleteFile(readme); err != nil {
		t.Errorf("DeleteFile: %v", err)
	}
}
