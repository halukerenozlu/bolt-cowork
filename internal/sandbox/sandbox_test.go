package sandbox

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	t.Run("valid directory", func(t *testing.T) {
		dir := t.TempDir()
		sb, err := New(dir)
		if err != nil {
			t.Fatalf("New(%q) returned error: %v", dir, err)
		}
		if sb.Root() == "" {
			t.Fatal("Root() returned empty string")
		}
	})

	t.Run("nonexistent directory", func(t *testing.T) {
		_, err := New(filepath.Join(t.TempDir(), "nonexistent"))
		if err == nil {
			t.Fatal("expected error for nonexistent directory")
		}
	})

	t.Run("file instead of directory", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "file.txt")
		os.WriteFile(f, []byte("x"), 0644)
		_, err := New(f)
		if err == nil {
			t.Fatal("expected error when root is a file")
		}
	})

	t.Run("with options", func(t *testing.T) {
		dir1 := t.TempDir()
		dir2 := t.TempDir()
		sb, err := New(dir1,
			WithAllowedDirs(dir2),
			WithDeniedPatterns("*.env", "*.key"),
		)
		if err != nil {
			t.Fatalf("New with options returned error: %v", err)
		}
		// Should be able to write in both dirs.
		if err := sb.WriteFile(filepath.Join(dir1, "a.txt"), []byte("ok")); err != nil {
			t.Errorf("write to root dir failed: %v", err)
		}
		if err := sb.WriteFile(filepath.Join(dir2, "b.txt"), []byte("ok")); err != nil {
			t.Errorf("write to additional allowed dir failed: %v", err)
		}
	})
}

func TestValidatePath_Containment(t *testing.T) {
	tests := []struct {
		name    string
		pathFn  func(root string) string
		wantErr error
	}{
		{
			name:    "path inside root is allowed",
			pathFn:  func(root string) string { return filepath.Join(root, "file.txt") },
			wantErr: nil,
		},
		{
			name:    "root itself is allowed",
			pathFn:  func(root string) string { return root },
			wantErr: nil,
		},
		{
			name:    "deeply nested path inside root is allowed",
			pathFn:  func(root string) string { return filepath.Join(root, "a", "b", "c", "file.txt") },
			wantErr: nil,
		},
		{
			name:    "parent traversal is blocked",
			pathFn:  func(root string) string { return filepath.Join(root, "..", "escape.txt") },
			wantErr: ErrOutsideSandbox,
		},
		{
			name:    "absolute path outside root is blocked",
			pathFn:  func(_ string) string { return filepath.Join(os.TempDir(), "other", "file.txt") },
			wantErr: ErrOutsideSandbox,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			sb, err := New(root)
			if err != nil {
				t.Fatalf("New(%q): %v", root, err)
			}

			path := tt.pathFn(root)

			// For paths inside root, create the file so validatePath can resolve it.
			if tt.wantErr == nil {
				dir := filepath.Dir(path)
				os.MkdirAll(dir, 0755)
				os.WriteFile(path, []byte("test"), 0644)
			}

			err = sb.validatePath(path)
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("expected error %v, got nil", tt.wantErr)
				} else if !errors.Is(err, tt.wantErr) {
					t.Errorf("expected error %v, got: %v", tt.wantErr, err)
				}
			}
		})
	}
}

func TestDeniedPatterns(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		file     string
		wantErr  error
	}{
		{
			name:     "blocks .env files",
			patterns: []string{"*.env"},
			file:     ".env",
			wantErr:  ErrDeniedPattern,
		},
		{
			name:     "blocks production.env",
			patterns: []string{"*.env"},
			file:     "production.env",
			wantErr:  ErrDeniedPattern,
		},
		{
			name:     "blocks .key files",
			patterns: []string{"*.key"},
			file:     "secret.key",
			wantErr:  ErrDeniedPattern,
		},
		{
			name:     "blocks .ssh contents",
			patterns: []string{".ssh/*"},
			file:     filepath.Join(".ssh", "id_rsa"),
			wantErr:  ErrDeniedPattern,
		},
		{
			name:     "allows normal txt file",
			patterns: []string{"*.env", "*.key"},
			file:     "readme.txt",
			wantErr:  nil,
		},
		{
			name:     "allows .envrc (not .env)",
			patterns: []string{"*.env"},
			file:     ".envrc",
			wantErr:  nil,
		},
		{
			name:     "no patterns allows everything",
			patterns: nil,
			file:     "secret.env",
			wantErr:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			sb, err := New(root, WithDeniedPatterns(tt.patterns...))
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			path := filepath.Join(root, tt.file)
			os.MkdirAll(filepath.Dir(path), 0755)
			os.WriteFile(path, []byte("data"), 0644)

			_, readErr := sb.ReadFile(path)
			if tt.wantErr == nil {
				if readErr != nil {
					t.Errorf("expected no error, got: %v", readErr)
				}
			} else {
				if readErr == nil {
					t.Errorf("expected error %v, got nil", tt.wantErr)
				} else if !errors.Is(readErr, tt.wantErr) {
					t.Errorf("expected error %v, got: %v", tt.wantErr, readErr)
				}
			}
		})
	}
}

func TestReadFile(t *testing.T) {
	root := t.TempDir()
	sb, _ := New(root)

	content := []byte("hello sandbox")
	path := filepath.Join(root, "test.txt")
	os.WriteFile(path, content, 0644)

	data, err := sb.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("ReadFile = %q, want %q", data, content)
	}
}

func TestReadFile_OutsideSandbox(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	sb, _ := New(root)

	path := filepath.Join(outside, "secret.txt")
	os.WriteFile(path, []byte("secret"), 0644)

	_, err := sb.ReadFile(path)
	if err == nil {
		t.Fatal("expected error reading outside sandbox")
	}
	if !errors.Is(err, ErrOutsideSandbox) {
		t.Errorf("expected ErrOutsideSandbox, got: %v", err)
	}
}

func TestOpenRead_EnforcesSandboxBoundary(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	insidePath := filepath.Join(root, "inside.txt")
	outsidePath := filepath.Join(outside, "outside.txt")
	if err := os.WriteFile(insidePath, []byte("inside"), 0o600); err != nil {
		t.Fatalf("write inside fixture: %v", err)
	}
	if err := os.WriteFile(outsidePath, []byte("outside"), 0o600); err != nil {
		t.Fatalf("write outside fixture: %v", err)
	}
	sb, err := New(root)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{name: "inside", path: insidePath},
		{name: "outside", path: outsidePath, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := sb.OpenRead(tt.path)
			if tt.wantErr {
				if err == nil {
					file.Close()
					t.Fatal("OpenRead() error = nil, want sandbox rejection")
				}
				return
			}
			if err != nil {
				t.Fatalf("OpenRead() error = %v", err)
			}
			defer file.Close()
		})
	}
}

func TestWriteFile(t *testing.T) {
	root := t.TempDir()
	sb, _ := New(root)

	path := filepath.Join(root, "new.txt")
	err := sb.WriteFile(path, []byte("new content"))
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "new content" {
		t.Errorf("file content = %q, want %q", data, "new content")
	}
}

func TestWriteFile_OutsideSandbox(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	sb, _ := New(root)

	err := sb.WriteFile(filepath.Join(outside, "hack.txt"), []byte("bad"))
	if err == nil {
		t.Fatal("expected error writing outside sandbox")
	}
	if !errors.Is(err, ErrOutsideSandbox) {
		t.Errorf("expected ErrOutsideSandbox, got: %v", err)
	}
}

func TestDeleteFile(t *testing.T) {
	root := t.TempDir()
	sb, _ := New(root)

	path := filepath.Join(root, "deleteme.txt")
	os.WriteFile(path, []byte("bye"), 0644)

	if err := sb.DeleteFile(path); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file still exists after delete")
	}
}

func TestDeleteFile_OutsideSandbox(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	sb, _ := New(root)

	path := filepath.Join(outside, "keep.txt")
	os.WriteFile(path, []byte("safe"), 0644)

	err := sb.DeleteFile(path)
	if err == nil {
		t.Fatal("expected error deleting outside sandbox")
	}
	if !errors.Is(err, ErrOutsideSandbox) {
		t.Errorf("expected ErrOutsideSandbox, got: %v", err)
	}
}

func TestRenameFile(t *testing.T) {
	root := t.TempDir()
	sb, _ := New(root)

	src := filepath.Join(root, "old.txt")
	dst := filepath.Join(root, "new.txt")
	os.WriteFile(src, []byte("data"), 0644)

	if err := sb.RenameFile(src, dst); err != nil {
		t.Fatalf("RenameFile: %v", err)
	}

	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("source file still exists after rename")
	}
	data, _ := os.ReadFile(dst)
	if string(data) != "data" {
		t.Errorf("renamed file content = %q, want %q", data, "data")
	}
}

func TestRenameFile_DstOutsideSandbox(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	sb, _ := New(root)

	src := filepath.Join(root, "file.txt")
	os.WriteFile(src, []byte("data"), 0644)

	err := sb.RenameFile(src, filepath.Join(outside, "escaped.txt"))
	if err == nil {
		t.Fatal("expected error renaming to outside sandbox")
	}
	if !errors.Is(err, ErrOutsideSandbox) {
		t.Errorf("expected ErrOutsideSandbox, got: %v", err)
	}
}

func TestMoveFile(t *testing.T) {
	root := t.TempDir()
	sb, _ := New(root)

	src := filepath.Join(root, "src.txt")
	dst := filepath.Join(root, "dst.txt")
	os.WriteFile(src, []byte("move me"), 0644)

	if err := sb.MoveFile(src, dst); err != nil {
		t.Fatalf("MoveFile: %v", err)
	}

	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("source file still exists after move")
	}
	data, _ := os.ReadFile(dst)
	if string(data) != "move me" {
		t.Errorf("moved file content = %q, want %q", data, "move me")
	}
}

func TestListDir(t *testing.T) {
	root := t.TempDir()
	sb, _ := New(root)

	os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(root, "b.txt"), []byte("b"), 0644)
	os.Mkdir(filepath.Join(root, "subdir"), 0755)

	entries, err := sb.ListDir(root)
	if err != nil {
		t.Fatalf("ListDir: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("ListDir returned %d entries, want 3", len(entries))
	}
}

func TestListDir_OutsideSandbox(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	sb, _ := New(root)

	_, err := sb.ListDir(outside)
	if err == nil {
		t.Fatal("expected error listing outside sandbox")
	}
	if !errors.Is(err, ErrOutsideSandbox) {
		t.Errorf("expected ErrOutsideSandbox, got: %v", err)
	}
}

func TestFileInfo(t *testing.T) {
	root := t.TempDir()
	sb, _ := New(root)

	path := filepath.Join(root, "info.txt")
	os.WriteFile(path, []byte("hello"), 0644)

	info, err := sb.FileInfo(path)
	if err != nil {
		t.Fatalf("FileInfo: %v", err)
	}
	if info.Name() != "info.txt" {
		t.Errorf("FileInfo.Name() = %q, want %q", info.Name(), "info.txt")
	}
	if info.Size() != 5 {
		t.Errorf("FileInfo.Size() = %d, want 5", info.Size())
	}
}

func TestSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests require elevated privileges on Windows")
	}

	root := t.TempDir()
	outside := t.TempDir()
	sb, _ := New(root)

	// Create a file outside sandbox.
	outsideFile := filepath.Join(outside, "secret.txt")
	os.WriteFile(outsideFile, []byte("secret"), 0644)

	// Create a symlink inside sandbox pointing outside.
	link := filepath.Join(root, "escape-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	_, err := sb.ReadFile(filepath.Join(link, "secret.txt"))
	if err == nil {
		t.Fatal("expected error reading through symlink escape")
	}
	if !errors.Is(err, ErrSymlinkEscape) && !errors.Is(err, ErrOutsideSandbox) {
		t.Errorf("expected ErrSymlinkEscape or ErrOutsideSandbox, got: %v", err)
	}
}

func TestWriteFile_DeniedPattern(t *testing.T) {
	root := t.TempDir()
	sb, _ := New(root, WithDeniedPatterns("*.env"))

	err := sb.WriteFile(filepath.Join(root, "secrets.env"), []byte("bad"))
	if err == nil {
		t.Fatal("expected error writing denied pattern")
	}
	if !errors.Is(err, ErrDeniedPattern) {
		t.Errorf("expected ErrDeniedPattern, got: %v", err)
	}
}

// --- Read-Only Dir Tests ---

func TestReadOnlyDir_ReadAllowed(t *testing.T) {
	root := t.TempDir()
	roDir := filepath.Join(root, "readonly")
	os.MkdirAll(roDir, 0755)
	os.WriteFile(filepath.Join(roDir, "data.txt"), []byte("read me"), 0644)

	sb, err := New(root, WithReadOnlyDirs(roDir))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	data, err := sb.ReadFile(filepath.Join(roDir, "data.txt"))
	if err != nil {
		t.Fatalf("ReadFile in read-only dir should succeed: %v", err)
	}
	if string(data) != "read me" {
		t.Errorf("content = %q, want %q", data, "read me")
	}
}

func TestReadOnlyDir_ListAllowed(t *testing.T) {
	root := t.TempDir()
	roDir := filepath.Join(root, "readonly")
	os.MkdirAll(roDir, 0755)
	os.WriteFile(filepath.Join(roDir, "a.txt"), []byte("a"), 0644)

	sb, _ := New(root, WithReadOnlyDirs(roDir))

	entries, err := sb.ListDir(roDir)
	if err != nil {
		t.Fatalf("ListDir in read-only dir should succeed: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("entries = %d, want 1", len(entries))
	}
}

func TestReadOnlyDir_WriteBlocked(t *testing.T) {
	root := t.TempDir()
	roDir := filepath.Join(root, "readonly")
	os.MkdirAll(roDir, 0755)

	sb, _ := New(root, WithReadOnlyDirs(roDir))

	err := sb.WriteFile(filepath.Join(roDir, "new.txt"), []byte("bad"))
	if err == nil {
		t.Fatal("expected error writing to read-only dir")
	}
	if !errors.Is(err, ErrReadOnlyDir) {
		t.Errorf("expected ErrReadOnlyDir, got: %v", err)
	}
}

func TestReadOnlyDir_DeleteBlocked(t *testing.T) {
	root := t.TempDir()
	roDir := filepath.Join(root, "readonly")
	os.MkdirAll(roDir, 0755)
	f := filepath.Join(roDir, "keep.txt")
	os.WriteFile(f, []byte("data"), 0644)

	sb, _ := New(root, WithReadOnlyDirs(roDir))

	err := sb.DeleteFile(f)
	if err == nil {
		t.Fatal("expected error deleting from read-only dir")
	}
	if !errors.Is(err, ErrReadOnlyDir) {
		t.Errorf("expected ErrReadOnlyDir, got: %v", err)
	}
}

func TestReadOnlyDir_MoveDestBlocked(t *testing.T) {
	root := t.TempDir()
	roDir := filepath.Join(root, "readonly")
	os.MkdirAll(roDir, 0755)

	src := filepath.Join(root, "src.txt")
	os.WriteFile(src, []byte("data"), 0644)

	sb, _ := New(root, WithReadOnlyDirs(roDir))

	err := sb.MoveFile(src, filepath.Join(roDir, "moved.txt"))
	if err == nil {
		t.Fatal("expected error moving to read-only dir")
	}
	if !errors.Is(err, ErrReadOnlyDir) {
		t.Errorf("expected ErrReadOnlyDir, got: %v", err)
	}
}

func TestReadOnlyDir_CopyDestBlocked(t *testing.T) {
	root := t.TempDir()
	roDir := filepath.Join(root, "readonly")
	os.MkdirAll(roDir, 0755)

	src := filepath.Join(root, "src.txt")
	os.WriteFile(src, []byte("data"), 0644)

	sb, _ := New(root, WithReadOnlyDirs(roDir))

	err := sb.CopyFile(src, filepath.Join(roDir, "copied.txt"))
	if err == nil {
		t.Fatal("expected error copying to read-only dir")
	}
	if !errors.Is(err, ErrReadOnlyDir) {
		t.Errorf("expected ErrReadOnlyDir, got: %v", err)
	}
}

func TestReadOnlyDir_CopySrcAllowed(t *testing.T) {
	root := t.TempDir()
	roDir := filepath.Join(root, "readonly")
	os.MkdirAll(roDir, 0755)

	src := filepath.Join(roDir, "src.txt")
	os.WriteFile(src, []byte("copy me"), 0644)
	dst := filepath.Join(root, "copied.txt")

	sb, _ := New(root, WithReadOnlyDirs(roDir))

	err := sb.CopyFile(src, dst)
	if err != nil {
		t.Fatalf("CopyFile from read-only src to writable dst should succeed: %v", err)
	}
	data, _ := os.ReadFile(dst)
	if string(data) != "copy me" {
		t.Errorf("copy content = %q, want %q", data, "copy me")
	}
}

func TestReadOnlyDir_MkdirBlocked(t *testing.T) {
	root := t.TempDir()
	roDir := filepath.Join(root, "readonly")
	os.MkdirAll(roDir, 0755)

	sb, _ := New(root, WithReadOnlyDirs(roDir))

	err := sb.MkdirAll(filepath.Join(roDir, "subdir"))
	if err == nil {
		t.Fatal("expected error creating dir in read-only dir")
	}
	if !errors.Is(err, ErrReadOnlyDir) {
		t.Errorf("expected ErrReadOnlyDir, got: %v", err)
	}
}

// --- Read-only source move/rename regression tests ---

func TestReadOnlyDir_MoveSrcBlocked(t *testing.T) {
	root := t.TempDir()
	roDir := filepath.Join(root, "readonly")
	os.MkdirAll(roDir, 0755)
	src := filepath.Join(roDir, "file.txt")
	os.WriteFile(src, []byte("data"), 0644)

	sb, _ := New(root, WithReadOnlyDirs(roDir))

	err := sb.MoveFile(src, filepath.Join(root, "moved.txt"))
	if err == nil {
		t.Fatal("expected error moving from read-only dir")
	}
	if !errors.Is(err, ErrReadOnlyDir) {
		t.Errorf("expected ErrReadOnlyDir, got: %v", err)
	}
	// Source must still exist.
	if _, statErr := os.Stat(src); statErr != nil {
		t.Error("source file should still exist after blocked move")
	}
}

func TestReadOnlyDir_RenameSrcBlocked(t *testing.T) {
	root := t.TempDir()
	roDir := filepath.Join(root, "readonly")
	os.MkdirAll(roDir, 0755)
	src := filepath.Join(roDir, "file.txt")
	os.WriteFile(src, []byte("data"), 0644)

	sb, _ := New(root, WithReadOnlyDirs(roDir))

	err := sb.RenameFile(src, filepath.Join(root, "renamed.txt"))
	if err == nil {
		t.Fatal("expected error renaming from read-only dir")
	}
	if !errors.Is(err, ErrReadOnlyDir) {
		t.Errorf("expected ErrReadOnlyDir, got: %v", err)
	}
	// Source must still exist.
	if _, statErr := os.Stat(src); statErr != nil {
		t.Error("source file should still exist after blocked rename")
	}
}

// --- ..hidden dir name under read-only: write must be blocked ---

func TestReadOnlyDir_DotDotHiddenDirBlocked(t *testing.T) {
	root := t.TempDir()
	roDir := filepath.Join(root, "readonly")
	dotHidden := filepath.Join(roDir, "..hidden")
	os.MkdirAll(dotHidden, 0755)

	sb, _ := New(root, WithReadOnlyDirs(roDir))

	err := sb.WriteFile(filepath.Join(dotHidden, "file.txt"), []byte("bad"))
	if err == nil {
		t.Fatal("expected error writing to ..hidden dir under read-only")
	}
	if !errors.Is(err, ErrReadOnlyDir) {
		t.Errorf("expected ErrReadOnlyDir, got: %v", err)
	}
}

// --- DeletePath Tests ---

func TestDeletePath_File(t *testing.T) {
	root := t.TempDir()
	f := filepath.Join(root, "file.txt")
	os.WriteFile(f, []byte("bye"), 0644)

	sb, _ := New(root)
	if err := sb.DeletePath(f, false); err != nil {
		t.Fatalf("DeletePath file: %v", err)
	}
	if _, err := os.Stat(f); !os.IsNotExist(err) {
		t.Error("file still exists")
	}
}

func TestDeletePath_EmptyDir(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "emptydir")
	os.MkdirAll(dir, 0755)

	sb, _ := New(root)
	if err := sb.DeletePath(dir, false); err != nil {
		t.Fatalf("DeletePath empty dir: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("directory still exists")
	}
}

func TestDeletePath_NonEmptyDir_NoRecursive(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "notempty")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "child.txt"), []byte("x"), 0644)

	sb, _ := New(root)
	err := sb.DeletePath(dir, false)
	if err == nil {
		t.Fatal("expected error deleting non-empty dir without recursive")
	}
	if !errors.Is(err, ErrNotEmpty) {
		t.Errorf("expected ErrNotEmpty, got: %v", err)
	}
}

func TestDeletePath_NonEmptyDir_Recursive(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "notempty")
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "file.txt"), []byte("x"), 0644)

	sb, _ := New(root)
	if err := sb.DeletePath(dir, true); err != nil {
		t.Fatalf("DeletePath recursive: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("directory still exists after recursive delete")
	}
}

func TestDeletePath_ReadOnlyDir(t *testing.T) {
	root := t.TempDir()
	roDir := filepath.Join(root, "readonly")
	os.MkdirAll(roDir, 0755)
	f := filepath.Join(roDir, "keep.txt")
	os.WriteFile(f, []byte("data"), 0644)

	sb, _ := New(root, WithReadOnlyDirs(roDir))
	err := sb.DeletePath(f, false)
	if err == nil {
		t.Fatal("expected error deleting from read-only dir")
	}
	if !errors.Is(err, ErrReadOnlyDir) {
		t.Errorf("expected ErrReadOnlyDir, got: %v", err)
	}
}

func TestDeletePath_RecursiveStaysInSandbox(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	// Build a non-empty directory tree outside the sandbox.
	dir := filepath.Join(outside, "notempty")
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "file.txt"), []byte("x"), 0644)

	sb, _ := New(root)
	err := sb.DeletePath(dir, true)
	if err == nil {
		t.Fatal("expected error for recursive delete of path outside sandbox")
	}
	// Directory must still exist — sandbox boundary was enforced.
	if _, statErr := os.Stat(dir); statErr != nil {
		t.Error("directory should still exist after rejected delete")
	}
}

// --- CopyFile Tests ---

func TestCopyFile_HappyPath(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src.txt")
	dst := filepath.Join(root, "dst.txt")
	os.WriteFile(src, []byte("copy me"), 0644)

	sb, _ := New(root)
	if err := sb.CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile: %v", err)
	}

	data, _ := os.ReadFile(dst)
	if string(data) != "copy me" {
		t.Errorf("copy content = %q, want %q", data, "copy me")
	}
	// Source should still exist.
	if _, err := os.Stat(src); err != nil {
		t.Error("source should still exist after copy")
	}
}

func TestCopyFile_DestinationDirectory(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src.txt")
	dstDir := filepath.Join(root, "dest")
	os.WriteFile(src, []byte("copy me"), 0644)
	os.MkdirAll(dstDir, 0755)

	sb, _ := New(root)
	if err := sb.CopyFile(src, dstDir); err != nil {
		t.Fatalf("CopyFile to directory: %v", err)
	}

	dstFile := filepath.Join(dstDir, "src.txt")
	data, _ := os.ReadFile(dstFile)
	if string(data) != "copy me" {
		t.Errorf("copy content = %q, want %q", data, "copy me")
	}
}

func TestCopyFile_DestinationDirectory_TargetExists(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src.txt")
	dstDir := filepath.Join(root, "dest")
	os.WriteFile(src, []byte("copy me"), 0644)
	os.MkdirAll(dstDir, 0755)
	os.WriteFile(filepath.Join(dstDir, "src.txt"), []byte("existing"), 0644)

	sb, _ := New(root)
	err := sb.CopyFile(src, dstDir)
	if err == nil {
		t.Fatal("expected error when destination file in target directory exists")
	}
	if !errors.Is(err, ErrDestinationExists) {
		t.Errorf("expected ErrDestinationExists, got: %v", err)
	}
}

func TestCopyFile_DestinationExists(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src.txt")
	dst := filepath.Join(root, "dst.txt")
	os.WriteFile(src, []byte("data"), 0644)
	os.WriteFile(dst, []byte("existing"), 0644)

	sb, _ := New(root)
	err := sb.CopyFile(src, dst)
	if err == nil {
		t.Fatal("expected error when destination exists")
	}
	if !errors.Is(err, ErrDestinationExists) {
		t.Errorf("expected ErrDestinationExists, got: %v", err)
	}
}

func TestCopyFile_DeniedPattern(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "data.txt")
	dst := filepath.Join(root, "secret.env")
	os.WriteFile(src, []byte("data"), 0644)

	sb, _ := New(root, WithDeniedPatterns("*.env"))
	err := sb.CopyFile(src, dst)
	if err == nil {
		t.Fatal("expected error copying to denied pattern")
	}
	if !errors.Is(err, ErrDeniedPattern) {
		t.Errorf("expected ErrDeniedPattern, got: %v", err)
	}
}

// --- MkdirAll Tests ---

func TestMkdirAll_HappyPath(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "a", "b", "c")

	sb, _ := New(root)
	if err := sb.MkdirAll(dir); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("dir should exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory")
	}
}

func TestMkdirAll_Idempotent(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "existing")
	os.MkdirAll(dir, 0755)

	sb, _ := New(root)
	if err := sb.MkdirAll(dir); err != nil {
		t.Fatalf("MkdirAll idempotent: %v", err)
	}
}

func TestMkdirAll_ReadOnlyDir(t *testing.T) {
	root := t.TempDir()
	roDir := filepath.Join(root, "readonly")
	os.MkdirAll(roDir, 0755)

	sb, _ := New(root, WithReadOnlyDirs(roDir))
	err := sb.MkdirAll(filepath.Join(roDir, "newdir"))
	if err == nil {
		t.Fatal("expected error creating dir in read-only dir")
	}
	if !errors.Is(err, ErrReadOnlyDir) {
		t.Errorf("expected ErrReadOnlyDir, got: %v", err)
	}
}

func TestMkdirAll_OutsideSandbox(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	sb, _ := New(root)
	err := sb.MkdirAll(filepath.Join(outside, "escape"))
	if err == nil {
		t.Fatal("expected error creating dir outside sandbox")
	}
	if !errors.Is(err, ErrOutsideSandbox) {
		t.Errorf("expected ErrOutsideSandbox, got: %v", err)
	}
}

// --- ..hidden dir name in allowed dirs ---

func TestIsWithinAllowed_DotDotHiddenName(t *testing.T) {
	root := t.TempDir()
	dotHidden := filepath.Join(root, "..hidden")
	os.MkdirAll(dotHidden, 0755)
	os.WriteFile(filepath.Join(dotHidden, "file.txt"), []byte("ok"), 0644)

	sb, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Reading a file under ..hidden should succeed (it's inside root).
	data, err := sb.ReadFile(filepath.Join(dotHidden, "file.txt"))
	if err != nil {
		t.Fatalf("ReadFile in ..hidden dir should succeed: %v", err)
	}
	if string(data) != "ok" {
		t.Errorf("content = %q, want %q", data, "ok")
	}

	// Writing should also succeed.
	if err := sb.WriteFile(filepath.Join(dotHidden, "new.txt"), []byte("new")); err != nil {
		t.Fatalf("WriteFile in ..hidden dir should succeed: %v", err)
	}
}

// --- Multiple Allowed Dirs ---

func TestMultipleAllowedDirs(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	sb, err := New(dir1, WithAllowedDirs(dir2))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Write to both dirs.
	if err := sb.WriteFile(filepath.Join(dir1, "a.txt"), []byte("a")); err != nil {
		t.Errorf("write to dir1: %v", err)
	}
	if err := sb.WriteFile(filepath.Join(dir2, "b.txt"), []byte("b")); err != nil {
		t.Errorf("write to dir2: %v", err)
	}

	// Read from both dirs.
	if _, err := sb.ReadFile(filepath.Join(dir1, "a.txt")); err != nil {
		t.Errorf("read from dir1: %v", err)
	}
	if _, err := sb.ReadFile(filepath.Join(dir2, "b.txt")); err != nil {
		t.Errorf("read from dir2: %v", err)
	}
}

// --- IsUnderDir Tests ---

func TestIsUnderDir(t *testing.T) {
	tests := []struct {
		parent string
		child  string
		want   bool
	}{
		{"/home/me", "/home/me/projects", true},
		// Key false-positive case: strings.HasPrefix would return true, IsUnderDir must return false.
		{"/home/me", "/home/me2/projects", false},
		// Self-reference: directory is "inside" itself.
		{"/home/me", "/home/me", true},
		// Completely different tree.
		{"/home/me", "/tmp/other", false},
		// Path traversal: /home/me/../other resolves to /home/other, not under /home/me.
		{"/home/me", "/home/me/../other", false},
	}
	for _, tt := range tests {
		got := IsUnderDir(tt.parent, tt.child)
		if got != tt.want {
			t.Errorf("IsUnderDir(%q, %q) = %v, want %v", tt.parent, tt.child, got, tt.want)
		}
	}
}

// --- WrapFSError Tests ---

func TestWrapFSError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantNil bool
		want    string // substring expected in error message
		wantErr error  // sentinel expected via errors.Is
	}{
		{
			name:    "nil error returns nil",
			err:     nil,
			wantNil: true,
		},
		{
			name:    "permission denied",
			err:     os.ErrPermission,
			want:    "permission denied",
			wantErr: os.ErrPermission,
		},
		{
			name:    "file not found",
			err:     os.ErrNotExist,
			want:    "file not found",
			wantErr: os.ErrNotExist,
		},
		{
			name:    "already exists",
			err:     os.ErrExist,
			want:    "already exists",
			wantErr: os.ErrExist,
		},
		{
			name: "generic error preserves message",
			err:  fmt.Errorf("disk quota exceeded"),
			want: "disk quota exceeded",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := WrapFSError("read", "f", tt.err)
			if tt.wantNil {
				if result != nil {
					t.Errorf("WrapFSError with nil error: got %v, want nil", result)
				}
				return
			}
			if result == nil {
				t.Fatal("expected non-nil error")
			}
			if !strings.Contains(result.Error(), tt.want) {
				t.Errorf("WrapFSError(%q): got %q, want to contain %q", tt.name, result.Error(), tt.want)
			}
			if tt.wantErr != nil && !errors.Is(result, tt.wantErr) {
				t.Errorf("WrapFSError(%q): errors.Is(%v) = false, want true (error chain broken)", tt.name, tt.wantErr)
			}
		})
	}
}
