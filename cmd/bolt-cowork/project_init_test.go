package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInitProject(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, dir string)
		check func(t *testing.T, dir string, err error)
	}{
		{
			name:  "fresh init creates expected files",
			setup: nil,
			check: func(t *testing.T, dir string, err error) {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				for _, rel := range []string{
					filepath.Join(".cowork", "config.json"),
					filepath.Join(".cowork", "keyset.json"),
					filepath.Join(".cowork", "sessions"),
				} {
					path := filepath.Join(dir, rel)
					if _, statErr := os.Stat(path); statErr != nil {
						t.Errorf("expected %s to exist: %v", rel, statErr)
					}
				}
			},
		},
		{
			name: "already initialized returns errAlreadyInitialized",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				if err := initProject(dir, false); err != nil {
					t.Fatalf("setup: first init failed: %v", err)
				}
			},
			check: func(t *testing.T, dir string, err error) {
				if !errors.Is(err, errAlreadyInitialized) {
					t.Fatalf("expected errAlreadyInitialized, got %v", err)
				}
			},
		},
		{
			name:  "config.json has correct format",
			setup: nil,
			check: func(t *testing.T, dir string, err error) {
				if err != nil {
					t.Fatalf("init failed: %v", err)
				}
				data, readErr := os.ReadFile(filepath.Join(dir, ".cowork", "config.json"))
				if readErr != nil {
					t.Fatalf("read config.json: %v", readErr)
				}
				var cfg coworkConfig
				if jsonErr := json.Unmarshal(data, &cfg); jsonErr != nil {
					t.Fatalf("unmarshal config.json: %v", jsonErr)
				}
				if cfg.Version != 1 {
					t.Errorf("version: want 1, got %d", cfg.Version)
				}
				if cfg.Name != "bolt-cowork" {
					t.Errorf("name: want %q, got %q", "bolt-cowork", cfg.Name)
				}
				if !cfg.Initialized {
					t.Error("initialized: want true, got false")
				}
			},
		},
		{
			name:  "keyset.json has correct format",
			setup: nil,
			check: func(t *testing.T, dir string, err error) {
				if err != nil {
					t.Fatalf("init failed: %v", err)
				}
				data, readErr := os.ReadFile(filepath.Join(dir, ".cowork", "keyset.json"))
				if readErr != nil {
					t.Fatalf("read keyset.json: %v", readErr)
				}
				var ks coworkKeyset
				if jsonErr := json.Unmarshal(data, &ks); jsonErr != nil {
					t.Fatalf("unmarshal keyset.json: %v", jsonErr)
				}
				if ks.Version != 1 {
					t.Errorf("version: want 1, got %d", ks.Version)
				}
				if ks.KeySet.ID != "bolt-cowork-default" {
					t.Errorf("keySet.id: want %q, got %q", "bolt-cowork-default", ks.KeySet.ID)
				}
				if len(ks.KeySet.Keys) != 0 {
					t.Errorf("keySet.keys: want empty, got %v", ks.KeySet.Keys)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.setup != nil {
				tt.setup(t, dir)
			}
			err := initProject(dir, false)
			tt.check(t, dir, err)
		})
	}
}

func TestInitProject_Force(t *testing.T) {
	dir := t.TempDir()

	if err := initProject(dir, true); err != nil {
		t.Fatalf("first init (force) failed: %v", err)
	}

	if err := initProject(dir, true); err != nil {
		t.Fatalf("second init (force) failed: %v", err)
	}
}
