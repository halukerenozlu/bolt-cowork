package sandbox

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
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
