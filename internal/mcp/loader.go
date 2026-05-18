package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// userHomeDir is the function used to resolve the current user's home
// directory. It is a package-level variable so tests can override it without
// touching the real filesystem.
var userHomeDir = os.UserHomeDir

// DefaultConfigPath returns the canonical location of the bolt-cowork MCP
// configuration file: ~/.bolt-cowork/mcp.json. If the home directory cannot
// be determined, it falls back to the relative path .bolt-cowork/mcp.json.
func DefaultConfigPath() string {
	home, err := userHomeDir()
	if err != nil {
		return filepath.Join(".bolt-cowork", "mcp.json")
	}
	return filepath.Join(home, ".bolt-cowork", "mcp.json")
}

// LoadConfig reads and parses an MCPConfig from the JSON file at path.
//
// If path begins with "~/", the tilde is expanded to the current user's home
// directory before the file is opened.
//
// If the file does not exist, LoadConfig returns an empty *MCPConfig and a
// nil error — a missing config is not treated as a failure.
func LoadConfig(path string) (*MCPConfig, error) {
	expanded, err := expandTilde(path)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(expanded)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &MCPConfig{}, nil
		}
		return nil, fmt.Errorf("mcp: read config %q: %w", path, err)
	}

	var cfg MCPConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("mcp: parse config %q: %w", path, err)
	}
	return &cfg, nil
}

// expandTilde replaces a leading "~/" in path with the current user's home
// directory. Paths that do not start with "~/" are returned unchanged.
func expandTilde(path string) (string, error) {
	if !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := userHomeDir()
	if err != nil {
		return "", fmt.Errorf("mcp: expand tilde in %q: %w", path, err)
	}
	return filepath.Join(home, path[2:]), nil
}
