package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/halukerenozlu/bolt-cowork/internal/config"
	"gopkg.in/yaml.v3"
)

// providerModels maps provider names to their available models.
var providerModels = map[string][]string{
	"anthropic": {"claude-sonnet-4-6", "claude-opus-4-6", "claude-haiku-4-5-20251001"},
	"openai":    {"gpt-4o", "gpt-4o-mini", "o3-mini"},
}

// providerEnvVars maps provider names to their API key environment variable.
var providerEnvVars = map[string]string{
	"anthropic": "ANTHROPIC_API_KEY",
	"openai":    "OPENAI_API_KEY",
}

// runInit runs the interactive setup wizard that creates config.yaml.
func runInit() error {
	reader := bufio.NewReader(os.Stdin)
	cfg := config.Default()
	cfg.Providers = make(map[string]config.ProviderConfig)

	fmt.Fprintln(os.Stderr, "bolt-cowork init — interactive setup")
	fmt.Fprintln(os.Stderr, "====================================")
	fmt.Fprintln(os.Stderr)

	// Step 1: Provider selection.
	fmt.Fprintln(os.Stderr, "Select provider:")
	fmt.Fprintln(os.Stderr, "  1) anthropic (default)")
	fmt.Fprintln(os.Stderr, "  2) openai")
	fmt.Fprintln(os.Stderr, "  3) both")
	fmt.Fprint(os.Stderr, "Choice [1]: ")

	choice, err := readLine(reader)
	if err != nil {
		return fmt.Errorf("init: read provider choice: %w", err)
	}
	choice = strings.TrimSpace(choice)
	if choice == "" {
		choice = "1"
	}

	// actualKeys stores the real API keys entered by the user so we can
	// show the env-var setup instructions at the end. The config file
	// itself only stores ${ENV_VAR} placeholders — never plaintext keys.
	actualKeys := make(map[string]string) // provider → key

	var selectedProviders []string
	switch choice {
	case "1":
		selectedProviders = []string{"anthropic"}
		cfg.DefaultProvider = "anthropic"
	case "2":
		selectedProviders = []string{"openai"}
		cfg.DefaultProvider = "openai"
	case "3":
		selectedProviders = []string{"anthropic", "openai"}
		cfg.DefaultProvider = "anthropic"
	default:
		return fmt.Errorf("init: invalid choice %q", choice)
	}

	// Step 2 & 3: API key + model for each provider.
	for _, prov := range selectedProviders {
		fmt.Fprintf(os.Stderr, "\n--- %s ---\n", prov)

		fmt.Fprintf(os.Stderr, "API key: ")
		apiKey, err := readMasked()
		if err != nil {
			return fmt.Errorf("init: read API key for %s: %w", prov, err)
		}
		apiKey = strings.TrimSpace(apiKey)
		if apiKey == "" {
			return fmt.Errorf("init: API key for %s cannot be empty", prov)
		}

		models := providerModels[prov]
		fmt.Fprintln(os.Stderr, "Select model:")
		for i, m := range models {
			suffix := ""
			if i == 0 {
				suffix = " (default)"
			}
			fmt.Fprintf(os.Stderr, "  %d) %s%s\n", i+1, m, suffix)
		}
		fmt.Fprint(os.Stderr, "Choice [1]: ")

		modelChoice, err := readLine(reader)
		if err != nil {
			return fmt.Errorf("init: read model choice for %s: %w", prov, err)
		}
		modelChoice = strings.TrimSpace(modelChoice)
		if modelChoice == "" {
			modelChoice = "1"
		}

		idx := 0
		if _, err := fmt.Sscanf(modelChoice, "%d", &idx); err != nil || idx < 1 || idx > len(models) {
			return fmt.Errorf("init: invalid model choice %q for %s", modelChoice, prov)
		}
		selectedModel := models[idx-1]

		actualKeys[prov] = apiKey
		envVar := providerEnvVars[prov]
		cfg.Providers[prov] = config.ProviderConfig{
			APIKey: "${" + envVar + "}",
			Models: []string{selectedModel},
		}
	}

	// Build fallback chain.
	cfg.FallbackChain = nil
	for _, prov := range selectedProviders {
		pc := cfg.Providers[prov]
		cfg.FallbackChain = append(cfg.FallbackChain, config.FallbackEntry{
			Provider: prov,
			Model:    pc.Models[0],
		})
	}

	// Step 4: Workspace directory.
	fmt.Fprintln(os.Stderr)
	fmt.Fprint(os.Stderr, "Workspace directory [.]: ")
	workDir, err := readLine(reader)
	if err != nil {
		return fmt.Errorf("init: read workspace dir: %w", err)
	}
	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		workDir = "."
	}
	absDir, err := filepath.Abs(workDir)
	if err != nil {
		return fmt.Errorf("init: resolve workspace dir: %w", err)
	}
	cfg.Sandbox.AllowedDirs = []string{absDir}

	// Step 5: Write config file.
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("init: resolve home directory: %w", err)
	}

	configDir := filepath.Join(home, ".bolt-cowork")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("init: create config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("init: marshal config: %w", err)
	}

	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("init: write config file: %w", err)
	}

	fmt.Fprintf(os.Stderr, "\nConfig written to %s\n", configPath)
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Set your API key(s) as environment variables:")
	for _, prov := range selectedProviders {
		envVar := providerEnvVars[prov]
		key := actualKeys[prov]
		fmt.Fprintf(os.Stderr, "  PowerShell:  $env:%s = '%s'\n", envVar, key)
		fmt.Fprintf(os.Stderr, "  bash/zsh:    export %s='%s'\n", envVar, key)
	}
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Run `bolt-cowork` to start interactive mode.")
	return nil
}

// readLine reads a single line from the reader.
func readLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
