package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/halukerenozlu/bolt-cowork/internal/agent"
	"github.com/halukerenozlu/bolt-cowork/internal/config"
)

func TestExtractLikelyPathFromCommand(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    string
	}{
		{name: "dot path", command: "merhaba.txt dosyasinda ne yaziyor", want: "merhaba.txt"},
		{name: "quoted slash path", command: `read "docs/guide.md"`, want: "docs/guide.md"},
		{name: "backslash path", command: `read docs\guide.md`, want: `docs\guide.md`},
		{name: "no path-like token", command: "show notes", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractLikelyPathFromCommand(tt.command)
			if got != tt.want {
				t.Fatalf("extractLikelyPathFromCommand = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPrettyPath_RelativeInsideRoot(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "test6", "merhaba.txt")
	got := prettyPath(p, root)
	if got != "test6/merhaba.txt" {
		t.Fatalf("prettyPath = %q, want %q", got, "test6/merhaba.txt")
	}
}

func TestPrettyPathOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")

	got := prettyPath(outside, root)
	if got != filepath.ToSlash(outside) {
		t.Fatalf("prettyPath outside root = %q, want %q", got, filepath.ToSlash(outside))
	}
}

func TestMissingPathFromError(t *testing.T) {
	pathErr := &os.PathError{Op: "open", Path: "missing.txt", Err: os.ErrNotExist}

	if got := missingPathFromError(pathErr); got != "missing.txt" {
		t.Fatalf("missingPathFromError = %q, want missing.txt", got)
	}
	if got := missingPathFromError(errors.New("plain error")); got != "" {
		t.Fatalf("missingPathFromError plain = %q, want empty", got)
	}
}

func TestFindPathSuggestions(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "test6"), 0755)
	_ = os.MkdirAll(filepath.Join(root, "docs"), 0755)
	_ = os.WriteFile(filepath.Join(root, "test6", "merhaba.txt"), []byte("a"), 0644)
	_ = os.WriteFile(filepath.Join(root, "docs", "merhaba.txt"), []byte("b"), 0644)
	_ = os.WriteFile(filepath.Join(root, "notes.txt"), []byte("c"), 0644)

	got := findPathSuggestions(root, "merhaba.txt", 5)
	if len(got) < 2 {
		t.Fatalf("findPathSuggestions returned %d result(s), want at least 2", len(got))
	}
	if !slices.Contains(got, "test6/merhaba.txt") {
		t.Fatalf("suggestions missing %q; got=%v", "test6/merhaba.txt", got)
	}
	if !slices.Contains(got, "docs/merhaba.txt") {
		t.Fatalf("suggestions missing %q; got=%v", "docs/merhaba.txt", got)
	}
}

func TestFindPathSuggestionsLimitsAndSkipsGit(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, ".git"), 0755)
	_ = os.MkdirAll(filepath.Join(root, "a"), 0755)
	_ = os.MkdirAll(filepath.Join(root, "b"), 0755)
	_ = os.WriteFile(filepath.Join(root, ".git", "target.txt"), []byte("git"), 0644)
	_ = os.WriteFile(filepath.Join(root, "a", "target.txt"), []byte("a"), 0644)
	_ = os.WriteFile(filepath.Join(root, "b", "target.txt"), []byte("b"), 0644)

	got := findPathSuggestions(root, "target.txt", 1)
	if len(got) != 1 {
		t.Fatalf("findPathSuggestions limit result len = %d, want 1: %v", len(got), got)
	}
	if strings.HasPrefix(got[0], ".git/") {
		t.Fatalf("findPathSuggestions should skip .git, got %v", got)
	}

	if got := findPathSuggestions("", "target.txt", 1); got != nil {
		t.Fatalf("empty root suggestions = %v, want nil", got)
	}
	if got := findPathSuggestions(root, "", 1); got != nil {
		t.Fatalf("empty target suggestions = %v, want nil", got)
	}
	if got := findPathSuggestions(root, "target.txt", 0); got != nil {
		t.Fatalf("zero limit suggestions = %v, want nil", got)
	}
}

func TestPrintFriendlyNotFoundError(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "docs"), 0755)
	_ = os.WriteFile(filepath.Join(root, "docs", "guide.md"), []byte("guide"), 0644)

	oldOverride := workDirOverride
	defer func() { workDirOverride = oldOverride }()
	workDirOverride = root

	cfg := config.Default()
	err := &os.PathError{Op: "open", Path: filepath.Join(root, "missing", "guide.md"), Err: os.ErrNotExist}
	output := captureStderr(func() {
		if !printFriendlyNotFoundError(err, "read guide.md", cfg) {
			t.Fatal("printFriendlyNotFoundError returned false for not-exist error")
		}
	})

	for _, want := range []string{"Couldn't find:", "Did you mean one of these?", "docs/guide.md"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
}

func TestPrintFriendlyNotFoundErrorFallbacks(t *testing.T) {
	root := t.TempDir()
	oldOverride := workDirOverride
	defer func() { workDirOverride = oldOverride }()
	workDirOverride = root

	cfg := config.Default()
	tests := []struct {
		name    string
		err     error
		command string
		want    string
	}{
		{
			name:    "command path fallback",
			err:     errors.New("wrapped: no such file or directory"),
			command: "read notes.txt",
			want:    `Couldn't find: "notes.txt"`,
		},
		{
			name:    "generic missing path guidance",
			err:     errors.New("wrapped: cannot find the file specified"),
			command: "show notes",
			want:    "Couldn't find the requested file or folder.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStderr(func() {
				if !printFriendlyNotFoundError(tt.err, tt.command, cfg) {
					t.Fatal("printFriendlyNotFoundError returned false")
				}
			})
			if !strings.Contains(output, tt.want) {
				t.Fatalf("output = %q, want to contain %q", output, tt.want)
			}
		})
	}

	output := captureStderr(func() {
		if printFriendlyNotFoundError(errors.New("permission denied"), "read notes.txt", cfg) {
			t.Fatal("printFriendlyNotFoundError returned true for non-missing error")
		}
	})
	if output != "" {
		t.Fatalf("non-missing error output = %q, want empty", output)
	}
}

func TestPrintRunErrorRedactsGenericError(t *testing.T) {
	redactor := agent.NewRedactor([]string{"secret-token"})

	output := captureStderr(func() {
		printRunError(errors.New("provider failed with secret-token"), "ask question", config.Default(), redactor)
	})

	if strings.Contains(output, "secret-token") {
		t.Fatalf("printRunError leaked secret: %q", output)
	}
	if !strings.Contains(output, "[REDACTED]") {
		t.Fatalf("printRunError output = %q, want redacted marker", output)
	}
}
