package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the root configuration for Bolt Cowork.
type Config struct {
	DefaultProvider string                    `yaml:"default_provider"`
	Providers       map[string]ProviderConfig `yaml:"providers"`
	FallbackChain   []FallbackEntry           `yaml:"fallback_chain"`
	Sandbox         SandboxConfig             `yaml:"sandbox"`
	Skills          SkillsConfig              `yaml:"skills"`
	MCP             MCPConfig                 `yaml:"mcp"`
	MCPServers      map[string]any            `yaml:"mcp_servers"`
	ApprovalMode    string                    `yaml:"approval_mode"`
	MCPApprovalMode string                    `yaml:"mcp_approval_mode"`
	TrustedDirs     []string                  `yaml:"trusted_dirs,omitempty"`
}

// ProviderConfig holds settings for a single LLM provider.
type ProviderConfig struct {
	APIKey   string   `yaml:"api_key"`
	Endpoint string   `yaml:"endpoint,omitempty"`
	Models   []string `yaml:"models"`
}

// FallbackEntry represents one step in the fallback chain.
type FallbackEntry struct {
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`
}

// SandboxConfig holds file access restriction settings.
type SandboxConfig struct {
	AllowedDirs    []string `yaml:"allowed_dirs"`
	ReadOnlyDirs   []string `yaml:"read_only_dirs"`
	DeniedPatterns []string `yaml:"denied_patterns"`
}

// SkillsConfig holds skill directory settings.
type SkillsConfig struct {
	Dirs []string `yaml:"dirs"`
}

// MCPConfig holds MCP server settings.
type MCPConfig struct {
	Servers []MCPServer `yaml:"servers"`
}

// MCPServer represents a single MCP server definition.
type MCPServer struct {
	Name      string `yaml:"name"`
	Command   string `yaml:"command,omitempty"`
	Transport string `yaml:"transport,omitempty"`
	URL       string `yaml:"url,omitempty"`
}

// validApprovalModes lists all accepted approval mode values.
var validApprovalModes = map[string]bool{
	"full":           true,
	"plan-only":      true,
	"dangerous-only": true,
	"none":           true,
}

// envVarPattern matches ${VAR_NAME} placeholders.
var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// Default returns a Config with sensible defaults.
func Default() *Config {
	return &Config{
		DefaultProvider: "anthropic",
		ApprovalMode:    "full",
		MCPApprovalMode: "",
		Sandbox: SandboxConfig{
			DeniedPatterns: []string{"*.env", "*.key", ".ssh/*"},
		},
	}
}

// Load reads config from the default path (~/.bolt-cowork/config.yaml).
// If the file does not exist, returns Default().
func Load() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("config: resolve home directory: %w", err)
	}

	path := filepath.Join(home, ".bolt-cowork", "config.yaml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return Default(), nil
	}

	return LoadFile(path)
}

// LoadFile reads and parses config from a specific file path.
func LoadFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read file %q: %w", path, err)
	}

	// Expand environment variables before parsing.
	expanded := expandEnvVars(string(data))

	cfg := Default()
	if err := yaml.Unmarshal([]byte(expanded), cfg); err != nil {
		return nil, fmt.Errorf("config: parse yaml %q: %w", path, err)
	}

	// Expand tilde in path fields.
	for i, d := range cfg.Sandbox.AllowedDirs {
		cfg.Sandbox.AllowedDirs[i] = expandTilde(d)
	}
	for i, d := range cfg.Sandbox.ReadOnlyDirs {
		cfg.Sandbox.ReadOnlyDirs[i] = expandTilde(d)
	}
	for i, d := range cfg.Skills.Dirs {
		cfg.Skills.Dirs[i] = expandTilde(d)
	}
	for i, d := range cfg.TrustedDirs {
		cfg.TrustedDirs[i] = expandTilde(d)
	}

	// Apply defaults for empty fields.
	if cfg.ApprovalMode == "" {
		cfg.ApprovalMode = "full"
	}

	return cfg, nil
}

// expandTilde replaces a leading "~" or "~/" with the user's home directory.
func expandTilde(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~"+string(filepath.Separator)) {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[1:])
	}
	return path
}

// Validate checks the config for logical errors.
func (c *Config) Validate() error {
	if !validApprovalModes[c.ApprovalMode] {
		return fmt.Errorf("config: invalid approval_mode %q (valid: %s)",
			c.ApprovalMode, strings.Join(approvalModeList(), ", "))
	}
	if c.MCPApprovalMode != "" && !validApprovalModes[c.MCPApprovalMode] {
		return fmt.Errorf("config: invalid mcp_approval_mode %q (valid: %s)",
			c.MCPApprovalMode, strings.Join(approvalModeList(), ", "))
	}

	if c.DefaultProvider != "" && len(c.Providers) > 0 {
		if _, ok := c.Providers[c.DefaultProvider]; !ok {
			return fmt.Errorf("config: default_provider %q not found in providers",
				c.DefaultProvider)
		}
	}

	for i, entry := range c.FallbackChain {
		if len(c.Providers) > 0 {
			p, ok := c.Providers[entry.Provider]
			if !ok {
				return fmt.Errorf("config: fallback_chain[%d] references unknown provider %q",
					i, entry.Provider)
			}
			if !containsString(p.Models, entry.Model) {
				return fmt.Errorf("config: fallback_chain[%d] references unknown model %q for provider %q",
					i, entry.Model, entry.Provider)
			}
		}
	}

	return nil
}

// expandEnvVars replaces ${VAR} placeholders with their environment values.
// Unresolved variables become empty strings.
func expandEnvVars(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		varName := envVarPattern.FindStringSubmatch(match)[1]
		return os.Getenv(varName)
	})
}

func approvalModeList() []string {
	modes := make([]string, 0, len(validApprovalModes))
	for m := range validApprovalModes {
		modes = append(modes, m)
	}
	return modes
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
