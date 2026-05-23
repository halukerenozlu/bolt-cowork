package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

// ConfigPath returns the default config file path: ~/.bolt-cowork/config.yaml.
func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("config: resolve home directory: %w", err)
	}
	return filepath.Join(home, ".bolt-cowork", "config.yaml"), nil
}

// normalizePath returns the absolute, cleaned form of p.
func normalizePath(p string) (string, error) {
	return filepath.Abs(filepath.Clean(p))
}

// pathEqual compares two paths for equality.
// On Windows the comparison is case-insensitive; on other platforms it is exact.
func pathEqual(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// IsTrusted reports whether dir is trusted according to cfg.TrustedDirs.
// Only exact path matches are considered trusted; subdirectories of a trusted
// directory are NOT automatically trusted and require their own trust entry.
func IsTrusted(cfg *Config, dir string) bool {
	norm, err := normalizePath(dir)
	if err != nil {
		return false
	}
	for _, td := range cfg.TrustedDirs {
		normTD, err := normalizePath(td)
		if err != nil {
			continue
		}
		if pathEqual(norm, normTD) {
			return true
		}
	}
	return false
}

// AddTrustedDir persists dir to the config file at cfgPath's trusted_dirs list.
// The caller is responsible for providing the correct path (e.g. respecting --config).
func AddTrustedDir(dir, cfgPath string) error {
	return addTrustedDirToFile(dir, cfgPath)
}

// addTrustedDirToFile is the testable core of AddTrustedDir.
// It performs a round-trip via map[string]interface{} to preserve unexpanded
// environment variable references (e.g., ${OPENAI_API_KEY}) that would be
// destroyed if the *Config struct were re-marshalled.
func addTrustedDirToFile(dir, cfgPath string) error {
	norm, err := normalizePath(dir)
	if err != nil {
		return fmt.Errorf("config: normalize path %q: %w", dir, err)
	}

	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0700); err != nil {
		return fmt.Errorf("config: create config dir: %w", err)
	}

	// Read existing file (if any) into a raw map to preserve formatting/vars.
	var raw map[string]interface{}
	data, err := os.ReadFile(cfgPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("config: read %q: %w", cfgPath, err)
	}
	if len(data) > 0 {
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("config: parse %q: %w", cfgPath, err)
		}
	}
	if raw == nil {
		raw = make(map[string]interface{})
	}

	// Extract existing trusted_dirs slice.
	var existing []string
	if v, ok := raw["trusted_dirs"]; ok {
		if slice, ok := v.([]interface{}); ok {
			for _, item := range slice {
				if s, ok := item.(string); ok {
					existing = append(existing, s)
				}
			}
		}
	}

	// Duplicate check: skip if this exact (normalized) path is already present.
	for _, td := range existing {
		normTD, err := normalizePath(td)
		if err != nil {
			continue
		}
		if pathEqual(norm, normTD) {
			return nil
		}
	}

	// Append the new path and write back.
	existing = append(existing, norm)
	raw["trusted_dirs"] = existing

	out, err := yaml.Marshal(raw)
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}
	if err := os.WriteFile(cfgPath, out, 0600); err != nil {
		return fmt.Errorf("config: write %q: %w", cfgPath, err)
	}
	return nil
}
