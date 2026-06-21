package session

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/halukerenozlu/bolt-cowork/pkg/types"
)

func TestStore_SessionLifecycle(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 6, 21, 19, 48, 0, 0, time.UTC)
	store := NewStore(filepath.Join(root, ".cowork", "sessions"), func() time.Time { return now })

	record, err := store.Create("20 MB'dan büyük dosyaları listele", "anthropic", "claude-haiku-4-5-20251001")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	record.History = []types.Message{{Role: types.RoleUser, Content: "20 MB'dan büyük dosyaları listele"}}
	record.Messages = []DisplayMessage{{Role: "user", Text: "20 MB'dan büyük dosyaları listele"}}
	if err := store.Save(record); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := store.Load(record.ID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Model != "claude-haiku-4-5-20251001" || len(loaded.Messages) != 1 {
		t.Fatalf("loaded record = %#v", loaded)
	}

	summaries, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(summaries) != 1 || summaries[0].Title != "20 MB'dan büyük dosyaları listele" {
		t.Fatalf("summaries = %#v", summaries)
	}

	if err := store.Rename(record.ID, "Büyük dosyalar"); err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	renamed, err := store.Load(record.ID)
	if err != nil {
		t.Fatalf("Load() after rename error = %v", err)
	}
	if renamed.Title != "Büyük dosyalar" {
		t.Fatalf("Title = %q, want Büyük dosyalar", renamed.Title)
	}

	if err := store.Delete(record.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := store.Load(record.ID); err == nil {
		t.Fatal("Load() after Delete() error = nil")
	}
}

func TestStore_RejectsSymlinkedSessionDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated privileges on Windows")
	}
	root := t.TempDir()
	outside := t.TempDir()
	cowork := filepath.Join(root, ".cowork")
	if err := os.MkdirAll(cowork, 0o700); err != nil {
		t.Fatalf("create .cowork: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(cowork, "sessions")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	store := NewStore(filepath.Join(cowork, "sessions"), time.Now)
	if _, err := store.Create("unsafe", "anthropic", "model"); err == nil {
		t.Fatal("Create() through symlinked sessions directory error = nil")
	}
}

func TestStore_RejectsInvalidSessionIDs(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions"), time.Now)
	tests := []struct {
		name string
		id   string
	}{
		{name: "parent traversal", id: "../config"},
		{name: "path separator", id: "a/b"},
		{name: "windows separator", id: `a\b`},
		{name: "empty", id: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := store.Load(tt.id); err == nil {
				t.Fatalf("Load(%q) error = nil", tt.id)
			}
			if err := store.Delete(tt.id); err == nil {
				t.Fatalf("Delete(%q) error = nil", tt.id)
			}
		})
	}
}
