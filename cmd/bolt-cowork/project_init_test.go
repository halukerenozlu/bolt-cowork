package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInitProject_Fresh(t *testing.T) {
	dir := t.TempDir()

	if err := initProject(dir, false); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	for _, rel := range []string{
		filepath.Join(".cowork", "config.json"),
		filepath.Join(".cowork", "keyset.json"),
		filepath.Join(".cowork", "sessions"),
	} {
		path := filepath.Join(dir, rel)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected %s to exist: %v", rel, err)
		}
	}
}

func TestInitProject_AlreadyExists(t *testing.T) {
	dir := t.TempDir()

	if err := initProject(dir, false); err != nil {
		t.Fatalf("first init failed: %v", err)
	}

	err := initProject(dir, false)
	if !errors.Is(err, errAlreadyInitialized) {
		t.Fatalf("expected errAlreadyInitialized, got %v", err)
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

func TestInitProject_ConfigFormat(t *testing.T) {
	dir := t.TempDir()

	if err := initProject(dir, false); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".cowork", "config.json"))
	if err != nil {
		t.Fatalf("read config.json: %v", err)
	}

	var cfg coworkConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal config.json: %v", err)
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
}

func TestInitProject_KeysetFormat(t *testing.T) {
	dir := t.TempDir()

	if err := initProject(dir, false); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".cowork", "keyset.json"))
	if err != nil {
		t.Fatalf("read keyset.json: %v", err)
	}

	var ks coworkKeyset
	if err := json.Unmarshal(data, &ks); err != nil {
		t.Fatalf("unmarshal keyset.json: %v", err)
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
}
