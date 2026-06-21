package session

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestProjectKey_StableForSameWorkspace(t *testing.T) {
	dir := t.TempDir()
	a, err := ProjectKey(dir)
	if err != nil {
		t.Fatalf("ProjectKey() error = %v", err)
	}
	b, err := ProjectKey(dir)
	if err != nil {
		t.Fatalf("ProjectKey() error = %v", err)
	}
	if a != b {
		t.Fatalf("ProjectKey() not stable: %q != %q", a, b)
	}
	if len(a) != 16 {
		t.Fatalf("ProjectKey() length = %d, want 16", len(a))
	}
}

func TestProjectKey_DiffersForDifferentWorkspaces(t *testing.T) {
	a, err := ProjectKey(t.TempDir())
	if err != nil {
		t.Fatalf("ProjectKey() error = %v", err)
	}
	b, err := ProjectKey(t.TempDir())
	if err != nil {
		t.Fatalf("ProjectKey() error = %v", err)
	}
	if a == b {
		t.Fatalf("ProjectKey() collided for distinct workspaces: %q", a)
	}
}

func TestProjectKey_CaseInsensitiveOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("case-insensitive path comparison only applies on windows")
	}
	dir := t.TempDir()
	lower, err := ProjectKey(strings.ToLower(dir))
	if err != nil {
		t.Fatalf("ProjectKey() error = %v", err)
	}
	upper, err := ProjectKey(strings.ToUpper(dir))
	if err != nil {
		t.Fatalf("ProjectKey() error = %v", err)
	}
	if lower != upper {
		t.Fatalf("ProjectKey() differs by case on windows: %q != %q", lower, upper)
	}
}

func TestDirForWorkspace_JoinsHomeAndKey(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	key, err := ProjectKey(workspace)
	if err != nil {
		t.Fatalf("ProjectKey() error = %v", err)
	}
	dir, err := DirForWorkspace(home, workspace)
	if err != nil {
		t.Fatalf("DirForWorkspace() error = %v", err)
	}
	want := filepath.Join(home, ".bolt-cowork", "sessions", key)
	if dir != want {
		t.Fatalf("DirForWorkspace() = %q, want %q", dir, want)
	}
}
