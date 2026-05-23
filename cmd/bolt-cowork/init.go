package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/halukerenozlu/bolt-cowork/internal/config"
)

// providerModels maps provider names to their available models.
var providerModels = map[string][]string{
	"anthropic": {"claude-sonnet-4-6", "claude-opus-4-6", "claude-haiku-4-5-20251001"},
	"openai":    {"gpt-4o", "gpt-4o-mini", "o3-mini"},
	"gemini":    {"gemini-2.5-pro", "gemini-2.5-flash"},
}

// runInit runs the interactive setup wizard that creates config.yaml.
// Returns the newly created config on success.
func runInit() (*config.Config, error) {
	reader := bufio.NewReader(os.Stdin)
	cfg := config.Default()
	cfg.Providers = make(map[string]config.ProviderConfig)

	fmt.Fprintln(os.Stderr, "bolt-cowork init — interactive setup")
	fmt.Fprintln(os.Stderr, "====================================")
	fmt.Fprintln(os.Stderr)

	// Step 1: Provider selection.
	fmt.Fprintln(os.Stderr, "Select provider:")
	fmt.Fprintln(os.Stderr, "  1) Anthropic (Claude) (default)")
	fmt.Fprintln(os.Stderr, "  2) OpenAI (GPT)")
	fmt.Fprintln(os.Stderr, "  3) Google (Gemini)")
	fmt.Fprintln(os.Stderr, "  4) All")
	fmt.Fprint(os.Stderr, "Choice [1]: ")

	choice, err := readLine(reader)
	if err != nil {
		return nil, fmt.Errorf("init: read provider choice: %w", err)
	}
	choice = strings.TrimSpace(choice)
	if choice == "" {
		choice = "1"
	}

	var selectedProviders []string
	switch choice {
	case "1":
		selectedProviders = []string{"anthropic"}
		cfg.DefaultProvider = "anthropic"
	case "2":
		selectedProviders = []string{"openai"}
		cfg.DefaultProvider = "openai"
	case "3":
		selectedProviders = []string{"gemini"}
		cfg.DefaultProvider = "gemini"
	case "4":
		selectedProviders = []string{"anthropic", "openai", "gemini"}
		cfg.DefaultProvider = "anthropic"
	default:
		return nil, fmt.Errorf("init: invalid choice %q", choice)
	}

	// Step 2 & 3: API key + model for each provider.
	keyringUnavailable := false
	for _, prov := range selectedProviders {
		fmt.Fprintf(os.Stderr, "\n--- %s ---\n", prov)

		fmt.Fprintf(os.Stderr, "API key: ")
		apiKey, err := readMasked(reader)
		if err != nil {
			return nil, fmt.Errorf("init: read API key for %s: %w", prov, err)
		}
		apiKey = strings.TrimSpace(apiKey)
		if apiKey == "" {
			return nil, fmt.Errorf("init: API key for %s cannot be empty", prov)
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
			return nil, fmt.Errorf("init: read model choice for %s: %w", prov, err)
		}
		modelChoice = strings.TrimSpace(modelChoice)
		if modelChoice == "" {
			modelChoice = "1"
		}

		idx := 0
		if _, err := fmt.Sscanf(modelChoice, "%d", &idx); err != nil || idx < 1 || idx > len(models) {
			return nil, fmt.Errorf("init: invalid model choice %q for %s", modelChoice, prov)
		}
		selectedModel := models[idx-1]

		pc := config.ProviderConfig{
			Models: []string{selectedModel},
		}
		if err := config.SetAPIKey(prov, apiKey); err != nil {
			fmt.Fprintln(os.Stderr, keyringUnavailableMessage)
			keyringUnavailable = true
			pc.APIKey = apiKey
		}
		cfg.Providers[prov] = pc
	}
	if keyringUnavailable {
		setupTransientKeyWarning = keyringUnavailableMessage
	} else {
		setupTransientKeyWarning = ""
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
		return nil, fmt.Errorf("init: read workspace dir: %w", err)
	}
	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		workDir = "."
	}
	absDir, err := filepath.Abs(workDir)
	if err != nil {
		return nil, fmt.Errorf("init: resolve workspace dir: %w", err)
	}
	cfg.Sandbox.AllowedDirs = []string{absDir}

	// Step 5: Write config file.
	if err := saveConfigFile(cfg); err != nil {
		return nil, err
	}

	configPath, err := configFilePath()
	if err != nil {
		return nil, fmt.Errorf("init: %w", err)
	}
	fmt.Fprintf(os.Stderr, "\nConfig written to %s\n", configPath)
	return cfg, nil
}

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
