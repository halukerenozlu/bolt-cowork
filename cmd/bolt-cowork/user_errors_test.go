package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestExtractLikelyPathFromCommand(t *testing.T) {
	got := extractLikelyPathFromCommand("merhaba.txt dosyasinda ne yaziyor")
	if got != "merhaba.txt" {
		t.Fatalf("extractLikelyPathFromCommand = %q, want %q", got, "merhaba.txt")
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
