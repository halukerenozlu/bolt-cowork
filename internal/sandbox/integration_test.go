//go:build integration

package sandbox

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// fixtureDir returns the absolute path to the sample-go-project fixture.
// Go tests always run with the working directory set to the package directory,
// so the relative path from internal/sandbox/ is stable.
func fixtureDir(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs("../../testdata/fixtures/sample-go-project")
	if err != nil {
		t.Fatalf("fixtureDir: resolve path: %v", err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("fixtureDir: fixture not found at %s: %v", abs, err)
	}
	return abs
}

// copyFixture copies the sample-go-project fixture into a fresh t.TempDir().
// Tests that write, delete, or rename files must use this instead of fixtureDir
// to avoid modifying the committed fixture.
func copyFixture(t *testing.T) string {
	t.Helper()
	src := fixtureDir(t)
	dst := t.TempDir()
	if err := copyDir(src, dst); err != nil {
		t.Fatalf("copyFixture: %v", err)
	}
	return dst
}

// copyDir recursively copies src into dst (dst must already exist).
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0644)
	})
}

// --- a) Read allowed files from fixture (read-only, no copy needed) ---

func TestIntegration_ReadAllowedFiles(t *testing.T) {
	root := fixtureDir(t)
	sb, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	cases := []struct{ name, rel string }{
		{"go.mod", "go.mod"},
		{"README.md", "README.md"},
		{"cmd/main.go", filepath.Join("cmd", "main.go")},
		{"internal/handler/handler.go", filepath.Join("internal", "handler", "handler.go")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := sb.ReadFile(filepath.Join(root, tc.rel))
			if err != nil {
				t.Errorf("ReadFile(%q) unexpected error: %v", tc.rel, err)
			}
			if len(data) == 0 {
				t.Errorf("ReadFile(%q) returned empty content", tc.rel)
			}
		})
	}
}

// --- b) Denied patterns block sensitive files ---

func TestIntegration_DeniedPatterns(t *testing.T) {
	root := fixtureDir(t)
	sb, err := New(root, WithDeniedPatterns("*.env", "*.key", ".git/config"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	cases := []struct {
		name    string
		rel     string
		wantErr error
	}{
		{
			name:    ".env denied by *.env pattern",
			rel:     ".env",
			wantErr: ErrDeniedPattern,
		},
		{
			name:    "secrets/api.key denied by *.key pattern",
			rel:     filepath.Join("secrets", "api.key"),
			wantErr: ErrDeniedPattern,
		},
		{
			name:    ".git/config denied by .git/config pattern",
			rel:     filepath.Join(".git", "config"),
			wantErr: ErrDeniedPattern,
		},
		{
			name:    ".gitignore is allowed (not matched by any pattern)",
			rel:     ".gitignore",
			wantErr: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := sb.ReadFile(filepath.Join(root, tc.rel))
			if tc.wantErr == nil {
				if err != nil {
					t.Errorf("ReadFile(%q) unexpected error: %v", tc.rel, err)
				}
				return
			}
			if err == nil {
				t.Errorf("ReadFile(%q) expected %v, got nil", tc.rel, tc.wantErr)
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("ReadFile(%q) = %v, want %v", tc.rel, err, tc.wantErr)
			}
		})
	}
}

// --- c) Write operations inside a project copy ---

func TestIntegration_WriteInProject(t *testing.T) {
	root := copyFixture(t)
	sb, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	t.Run("write new file in cmd/", func(t *testing.T) {
		path := filepath.Join(root, "cmd", "server.go")
		if err := sb.WriteFile(path, []byte("package main\n")); err != nil {
			t.Errorf("WriteFile cmd/server.go: %v", err)
		}
	})

	t.Run("write new file in docs/", func(t *testing.T) {
		path := filepath.Join(root, "docs", "api.md")
		if err := sb.WriteFile(path, []byte("# API\n")); err != nil {
			t.Errorf("WriteFile docs/api.md: %v", err)
		}
	})

	t.Run("delete README.md", func(t *testing.T) {
		path := filepath.Join(root, "README.md")
		if err := sb.DeleteFile(path); err != nil {
			t.Errorf("DeleteFile README.md: %v", err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Error("README.md still exists after delete")
		}
	})

	t.Run("rename cmd/main.go to cmd/app.go", func(t *testing.T) {
		src := filepath.Join(root, "cmd", "main.go")
		dst := filepath.Join(root, "cmd", "app.go")
		if err := sb.RenameFile(src, dst); err != nil {
			t.Errorf("RenameFile cmd/main.go -> cmd/app.go: %v", err)
		}
		if _, err := os.Stat(src); !os.IsNotExist(err) {
			t.Error("cmd/main.go still exists after rename")
		}
		if _, err := os.Stat(dst); err != nil {
			t.Errorf("cmd/app.go does not exist after rename: %v", err)
		}
	})
}

// --- d) Protected paths are denied for writes and deletes ---

func TestIntegration_ProtectedPaths(t *testing.T) {
	root := copyFixture(t)
	sb, err := New(root, WithDeniedPatterns("*.env", "*.key", ".git/config"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	t.Run("write .env denied", func(t *testing.T) {
		err := sb.WriteFile(filepath.Join(root, ".env"), []byte("BAD=1\n"))
		if err == nil {
			t.Fatal("expected error writing .env, got nil")
		}
		if !errors.Is(err, ErrDeniedPattern) {
			t.Errorf("expected ErrDeniedPattern, got: %v", err)
		}
	})

	t.Run("write .git/config denied", func(t *testing.T) {
		err := sb.WriteFile(filepath.Join(root, ".git", "config"), []byte("[bad]\n"))
		if err == nil {
			t.Fatal("expected error writing .git/config, got nil")
		}
		if !errors.Is(err, ErrDeniedPattern) {
			t.Errorf("expected ErrDeniedPattern, got: %v", err)
		}
	})

	t.Run("delete secrets/api.key denied", func(t *testing.T) {
		err := sb.DeleteFile(filepath.Join(root, "secrets", "api.key"))
		if err == nil {
			t.Fatal("expected error deleting secrets/api.key, got nil")
		}
		if !errors.Is(err, ErrDeniedPattern) {
			t.Errorf("expected ErrDeniedPattern, got: %v", err)
		}
	})

	// *.env matches filenames that end with ".env" (e.g. ".env", "production.env").
	// ".env.production" ends with ".production", so filepath.Match("*.env", ".env.production")
	// returns false — the write must succeed.
	t.Run("write .env.production allowed (*.env does not match it)", func(t *testing.T) {
		err := sb.WriteFile(filepath.Join(root, ".env.production"), []byte("PORT=8080\n"))
		if err != nil {
			t.Errorf("expected .env.production to be allowed, got: %v", err)
		}
	})
}

// --- e) Read-only directory enforcement ---

func TestIntegration_ReadOnlyDirectory(t *testing.T) {
	root := copyFixture(t)
	docsDir := filepath.Join(root, "docs")

	sb, err := New(root, WithReadOnlyDirs(docsDir))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	t.Run("read docs/guide.md allowed", func(t *testing.T) {
		data, err := sb.ReadFile(filepath.Join(docsDir, "guide.md"))
		if err != nil {
			t.Errorf("ReadFile docs/guide.md: %v", err)
		}
		if len(data) == 0 {
			t.Error("ReadFile docs/guide.md returned empty content")
		}
	})

	t.Run("write docs/new.md denied", func(t *testing.T) {
		err := sb.WriteFile(filepath.Join(docsDir, "new.md"), []byte("# New\n"))
		if err == nil {
			t.Fatal("expected ErrReadOnlyDir writing to docs/, got nil")
		}
		if !errors.Is(err, ErrReadOnlyDir) {
			t.Errorf("expected ErrReadOnlyDir, got: %v", err)
		}
	})

	t.Run("delete docs/guide.md denied", func(t *testing.T) {
		err := sb.DeleteFile(filepath.Join(docsDir, "guide.md"))
		if err == nil {
			t.Fatal("expected ErrReadOnlyDir deleting from docs/, got nil")
		}
		if !errors.Is(err, ErrReadOnlyDir) {
			t.Errorf("expected ErrReadOnlyDir, got: %v", err)
		}
	})

	t.Run("write cmd/new.go allowed (not read-only)", func(t *testing.T) {
		err := sb.WriteFile(filepath.Join(root, "cmd", "new.go"), []byte("package main\n"))
		if err != nil {
			t.Errorf("WriteFile cmd/new.go: %v", err)
		}
	})
}

// --- f) Nested traversal and path escape attempts ---

func TestIntegration_NestedTraversal(t *testing.T) {
	root := fixtureDir(t)
	sb, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	t.Run("read nested file allowed", func(t *testing.T) {
		path := filepath.Join(root, "internal", "handler", "handler.go")
		if _, err := sb.ReadFile(path); err != nil {
			t.Errorf("ReadFile internal/handler/handler.go: %v", err)
		}
	})

	t.Run("internal/../../etc/passwd blocked", func(t *testing.T) {
		// filepath.Join cleans the path: root/internal/../../etc/passwd -> <root-parent>/etc/passwd
		path := filepath.Join(root, "internal", "..", "..", "etc", "passwd")
		_, err := sb.ReadFile(path)
		if err == nil {
			t.Fatal("expected error for traversal path, got nil")
		}
		if !errors.Is(err, ErrOutsideSandbox) {
			t.Errorf("expected ErrOutsideSandbox, got: %v", err)
		}
	})

	t.Run("cmd/../../../outside blocked", func(t *testing.T) {
		path := filepath.Join(root, "cmd", "..", "..", "..", "outside")
		_, err := sb.ReadFile(path)
		if err == nil {
			t.Fatal("expected error for deep traversal, got nil")
		}
		if !errors.Is(err, ErrOutsideSandbox) {
			t.Errorf("expected ErrOutsideSandbox, got: %v", err)
		}
	})
}

// --- g) MkdirAll and CopyFile in a project copy ---

func TestIntegration_MkdirAndCopy(t *testing.T) {
	root := copyFixture(t)
	sb, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	t.Run("MkdirAll cmd/subpkg", func(t *testing.T) {
		dir := filepath.Join(root, "cmd", "subpkg")
		if err := sb.MkdirAll(dir); err != nil {
			t.Fatalf("MkdirAll cmd/subpkg: %v", err)
		}
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("cmd/subpkg does not exist: %v", err)
		}
		if !info.IsDir() {
			t.Error("cmd/subpkg is not a directory")
		}
	})

	t.Run("CopyFile cmd/main.go to cmd/subpkg/main.go", func(t *testing.T) {
		// Ensure subpkg exists (previous sub-test may have run).
		subpkg := filepath.Join(root, "cmd", "subpkg")
		os.MkdirAll(subpkg, 0755)

		src := filepath.Join(root, "cmd", "main.go")
		dst := filepath.Join(subpkg, "main.go")
		if err := sb.CopyFile(src, dst); err != nil {
			t.Fatalf("CopyFile cmd/main.go -> cmd/subpkg/main.go: %v", err)
		}
		if _, err := os.Stat(src); err != nil {
			t.Error("source cmd/main.go should still exist after copy")
		}
		if _, err := os.Stat(dst); err != nil {
			t.Errorf("destination cmd/subpkg/main.go not found: %v", err)
		}
	})

	t.Run("CopyFile to path outside sandbox blocked", func(t *testing.T) {
		outside := t.TempDir()
		src := filepath.Join(root, "cmd", "main.go")
		dst := filepath.Join(outside, "stolen.go")
		err := sb.CopyFile(src, dst)
		if err == nil {
			t.Fatal("expected error copying to outside sandbox, got nil")
		}
		if !errors.Is(err, ErrOutsideSandbox) {
			t.Errorf("expected ErrOutsideSandbox, got: %v", err)
		}
	})
}
