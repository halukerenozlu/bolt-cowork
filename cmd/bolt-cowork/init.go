package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/halukerenozlu/bolt-cowork/internal/config"
)

// providerModels aliases the central model catalog so setup and REPL commands
// cannot drift from the runtime provider registry.
var providerModels = config.DefaultModels

// configFilePath returns the config file path. If --config flag is set, uses
// that path; otherwise returns the default ~/.bolt-cowork/config.yaml.
func configFilePath() (string, error) {
	if *configFlag != "" {
		return *configFlag, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".bolt-cowork", "config.yaml"), nil
}

// saveConfigFile writes cfg to ~/.bolt-cowork/config.yaml via config.SaveFile,
// which ensures API keys are never written to disk.
func saveConfigFile(cfg *config.Config) error {
	path, err := configFilePath()
	if err != nil {
		return err
	}
	return config.SaveFile(cfg, path)
}

// readLine reads a single line from the reader.
func readLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
