package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	keyring "github.com/zalando/go-keyring"
	"gopkg.in/yaml.v3"
)

// KeyringService is the service name used for all keyring operations.
const KeyringService = "bolt-cowork"

// Config is the root configuration for Bolt Cowork.
type Config struct {
	DefaultProvider string                    `yaml:"default_provider"`
	Theme           string                    `yaml:"theme,omitempty"`
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
// APIKey is omitted from YAML output (stored in the system keyring instead).
type ProviderConfig struct {
	APIKey   string   `yaml:"-"`
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
	Name         string   `yaml:"name"`
	Command      string   `yaml:"command,omitempty"`
	Transport    string   `yaml:"transport,omitempty"`
	URL          string   `yaml:"url,omitempty"`
	AllowedTools []string `yaml:"allowed_tools,omitempty"`
	DeniedTools  []string `yaml:"denied_tools,omitempty"`
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

// removeHardcodedAPIKeyLines strips api_key: lines from YAML data whose
// values are literal strings (not ${...} env-var placeholders).
// Env-var based api_key lines are preserved unchanged.
func removeHardcodedAPIKeyLines(data []byte) []byte {
	lines := strings.Split(string(data), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "api_key:") {
			val := strings.TrimSpace(strings.TrimPrefix(trimmed, "api_key:"))
			if envVarPattern.MatchString(val) {
				out = append(out, line) // keep env-var placeholder
			}
			// else: drop the literal api_key line
		} else {
			out = append(out, line)
		}
	}
	return []byte(strings.Join(out, "\n"))
}

// defaultDeniedPatterns is the full set of security-sensitive file patterns
// that the sandbox blocks by default.
var defaultDeniedPatterns = []string{
	"*.env", ".env.*", "*.key", "*.pem", "*.p12", "*.pfx",
	"*.cer", "*.crt", ".ssh/*", "*.pub", "*.token", "*.secret",
	"*credentials*", "*secrets*", "*.kdbx", "*.keychain", "*.wallet",
	"Login Data", "cookies", "Cookies", ".git-credentials", ".netrc",
	"*.tfvars", "terraform.tfstate", "kubeconfig", ".kube/*",
	"*.rdp", "NTUSER.DAT", "SAM",
}

// Default returns a Config with sensible defaults.
func Default() *Config {
	return &Config{
		DefaultProvider: "anthropic",
		ApprovalMode:    "full",
		MCPApprovalMode: "",
		Theme:           "dark",
		Sandbox: SandboxConfig{
			DeniedPatterns: defaultDeniedPatterns,
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
// If the file contains hardcoded API keys (not env-var placeholders), they are
// automatically migrated to the system keyring and removed from the file.
// After loading, API keys from the keyring are populated into ProviderConfig.APIKey
// so that all existing code can continue reading cfg.Providers[name].APIKey.
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

	if cfg.ApprovalMode == "" {
		cfg.ApprovalMode = "full"
	}

	// Migrate any hardcoded API keys to the system keyring and remove them
	// from the config file. Uses the raw (unexpanded) data to detect env-var
	// placeholders that should remain in the file.
	migrateAPIKeys(data, cfg, path)

	// Since APIKey uses yaml:"-", env-var based api_key values from the YAML
	// are not deserialized automatically. Parse them from the expanded YAML.
	var expandedRaw struct {
		Providers map[string]struct {
			APIKey string `yaml:"api_key"`
		} `yaml:"providers"`
	}
	if err := yaml.Unmarshal([]byte(expanded), &expandedRaw); err == nil {
		for name, rp := range expandedRaw.Providers {
			if rp.APIKey != "" {
				if pc, ok := cfg.Providers[name]; ok && pc.APIKey == "" {
					pc.APIKey = rp.APIKey
					cfg.Providers[name] = pc
				}
			}
		}
	}

	// Populate ProviderConfig.APIKey from the keyring for runtime use.
	for name, pc := range cfg.Providers {
		if pc.APIKey == "" {
			if key := GetAPIKey(name); key != "" {
				pc.APIKey = key
				cfg.Providers[name] = pc
			}
		}
	}

	return cfg, nil
}

// migrateAPIKeys reads the raw (unexpanded) YAML, detects hardcoded API key
// values (not env-var placeholders), stores them in the system keyring, clears
// them from cfg, and rewrites the file without those fields.
// If the config file rewrite fails after keyring writes, the keyring entries
// are rolled back and an error is printed to stderr.
func migrateAPIKeys(rawData []byte, cfg *Config, path string) {
	var raw struct {
		Providers map[string]struct {
			APIKey string `yaml:"api_key"`
		} `yaml:"providers"`
	}
	if err := yaml.Unmarshal(rawData, &raw); err != nil {
		return
	}

	var migratedNames []string
	for name, rp := range raw.Providers {
		if rp.APIKey == "" || envVarPattern.MatchString(rp.APIKey) {
			continue
		}
		if err := SetAPIKey(name, rp.APIKey); err != nil {
			continue
		}
		migratedNames = append(migratedNames, name)
	}

	if len(migratedNames) == 0 {
		return
	}

	if err := os.WriteFile(path, removeHardcodedAPIKeyLines(rawData), 0600); err != nil {
		// Rollback: remove keys from keyring since plaintext removal failed.
		for _, name := range migratedNames {
			_ = DeleteAPIKey(name)
		}
		fmt.Fprintf(os.Stderr, "Warning: could not remove plaintext API keys from %s: %v\n", path, err)
		return
	}

	for _, name := range migratedNames {
		if pc, ok := cfg.Providers[name]; ok {
			pc.APIKey = ""
			cfg.Providers[name] = pc
		}
	}
}

// SetAPIKey stores the API key for provider in the system keyring.
// On Windows this uses Credential Manager; on macOS, Keychain;
// on Linux, the Secret Service (e.g. GNOME Keyring or KWallet).
func SetAPIKey(provider, key string) error {
	return keyring.Set(KeyringService, provider, key)
}

// GetAPIKey retrieves the API key for provider from the system keyring.
// Returns "" if the key is not found or if the keyring is unavailable.
func GetAPIKey(provider string) string {
	key, err := keyring.Get(KeyringService, provider)
	if err != nil {
		return ""
	}
	return key
}

// DeleteAPIKey removes the API key for provider from the system keyring.
func DeleteAPIKey(provider string) error {
	return keyring.Delete(KeyringService, provider)
}

// SaveFile writes cfg to path in YAML format, creating parent directories as
// needed. The file is written with 0600 permissions.
// API keys are never written to disk (defense-in-depth: cleared before marshal
// even though the yaml:"-" tag already prevents serialisation).
func SaveFile(cfg *Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("config: create directory: %w", err)
	}

	// Defense-in-depth: clear any runtime API keys so they never reach disk.
	cleaned := *cfg
	if len(cleaned.Providers) > 0 {
		cleaned.Providers = make(map[string]ProviderConfig, len(cfg.Providers))
		for name, pc := range cfg.Providers {
			pc.APIKey = ""
			cleaned.Providers[name] = pc
		}
	}

	data, err := yaml.Marshal(&cleaned)
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("config: write %q: %w", path, err)
	}
	return nil
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
