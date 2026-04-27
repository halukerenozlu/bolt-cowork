package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var errAlreadyInitialized = errors.New("already initialized")

type coworkConfig struct {
	Version     int            `json:"version"`
	Name        string         `json:"name"`
	Initialized bool           `json:"initialized"`
	Settings    map[string]any `json:"settings"`
}

type keySetData struct {
	ID      string `json:"id"`
	Created string `json:"created"`
	Keys    []any  `json:"keys"`
}

type coworkKeyset struct {
	Version int        `json:"version"`
	KeySet  keySetData `json:"keySet"`
}

// writeJSON marshals v with indentation and writes it to path.
func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", filepath.Base(path), err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	return nil
}

// initProject creates a .cowork/ directory structure in workDir.
// If .cowork/ already exists and force is false, returns errAlreadyInitialized.
// If force is true, existing files are overwritten (extra user files are left intact).
func initProject(workDir string, force bool) error {
	coworkDir := filepath.Join(workDir, ".cowork")

	if _, err := os.Stat(coworkDir); err == nil && !force {
		return errAlreadyInitialized
	}

	if err := os.MkdirAll(coworkDir, 0755); err != nil {
		return fmt.Errorf("create .cowork/: %w", err)
	}

	sessionsDir := filepath.Join(coworkDir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		return fmt.Errorf("create .cowork/sessions/: %w", err)
	}

	cfg := coworkConfig{
		Version:     1,
		Name:        "bolt-cowork",
		Initialized: true,
		Settings:    map[string]any{},
	}
	if err := writeJSON(filepath.Join(coworkDir, "config.json"), cfg); err != nil {
		return err
	}

	ks := coworkKeyset{
		Version: 1,
		KeySet: keySetData{
			ID:      "bolt-cowork-default",
			Created: time.Now().UTC().Format(time.RFC3339),
			Keys:    []any{},
		},
	}
	if err := writeJSON(filepath.Join(coworkDir, "keyset.json"), ks); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Initialized bolt-cowork in .cowork/\n")
	fmt.Fprintf(os.Stderr, "  Created .cowork/config.json\n")
	fmt.Fprintf(os.Stderr, "  Created .cowork/keyset.json\n")
	fmt.Fprintf(os.Stderr, "  Created .cowork/sessions/\n")
	return nil
}
