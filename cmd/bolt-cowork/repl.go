package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/halukerenozlu/bolt-cowork/internal/agent"
	"github.com/halukerenozlu/bolt-cowork/internal/config"
)

// modelAliases maps short names to full model IDs.
var modelAliases = map[string]string{
	"haiku":  "claude-haiku-4-5-20251001",
	"sonnet": "claude-sonnet-4-6",
	"opus":   "claude-opus-4-6",
}

// runREPL starts an interactive REPL session.
func runREPL(cfg *config.Config) error {
	workDir := resolveWorkDir(cfg)

	fmt.Fprintf(os.Stderr, "bolt-cowork %s | REPL mode\n", version)
	fmt.Fprintf(os.Stderr, "dir: %s | provider: %s | approval: %s\n",
		workDir, cfg.DefaultProvider, cfg.ApprovalMode)
	fmt.Fprintln(os.Stderr, "Type /help for commands, /quit to exit.")
	fmt.Fprintln(os.Stderr)

	reader := bufio.NewReader(os.Stdin)

	// Check if the active provider's API key is missing.
	if err := promptMissingAPIKey(cfg, reader); err != nil {
		return err
	}

	// Single signal handler for the entire REPL session.
	var (
		mu          sync.Mutex
		cancelCmd   context.CancelFunc
		lastCtrlC   time.Time // timestamp of last Ctrl+C at prompt
		interrupted bool      // set when signal fires at prompt
	)

	const ctrlCWindow = 3 * time.Second

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	go func() {
		for range sigCh {
			mu.Lock()
			fn := cancelCmd
			if fn != nil {
				// Command is running — cancel it.
				mu.Unlock()
				fmt.Fprintln(os.Stderr, "\nInterrupted.")
				fn()
				continue
			}

			// At prompt — double Ctrl+C within 3s exits.
			now := time.Now()
			if !lastCtrlC.IsZero() && now.Sub(lastCtrlC) < ctrlCWindow {
				mu.Unlock()
				fmt.Fprintln(os.Stderr, "\nGoodbye.")
				os.Exit(0)
			}
			lastCtrlC = now
			interrupted = true
			mu.Unlock()

			fmt.Fprintln(os.Stderr, "\nPress Ctrl+C again to quit, or type /quit.")
			fmt.Fprint(os.Stderr, "bolt-cowork> ")
		}
	}()

	for {
		fmt.Fprint(os.Stderr, "bolt-cowork> ")

		input, err := readREPLLine(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				fmt.Fprintln(os.Stderr, "\nGoodbye.")
				return nil
			}
			// Compatibility path: if a platform-specific reader returns
			// errInterrupted, apply the same double-press logic used by
			// the signal goroutine.
			if errors.Is(err, errInterrupted) {
				mu.Lock()
				now := time.Now()
				if !lastCtrlC.IsZero() && now.Sub(lastCtrlC) < ctrlCWindow {
					mu.Unlock()
					fmt.Fprintln(os.Stderr, "Goodbye.")
					return nil
				}
				lastCtrlC = now
				mu.Unlock()
				fmt.Fprintln(os.Stderr, "Press Ctrl+C again to quit, or type /quit.")
				continue
			}
			// Unknown read error — if the signal handler fired (fallback
			// for edge cases), retry instead of crashing.
			mu.Lock()
			wasInterrupted := interrupted
			interrupted = false
			mu.Unlock()
			if wasInterrupted {
				reader.Reset(os.Stdin)
				continue
			}
			return fmt.Errorf("repl: read input: %w", err)
		}

		// Successful input resets the Ctrl+C window.
		mu.Lock()
		lastCtrlC = time.Time{}
		interrupted = false
		mu.Unlock()

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		// Handle slash commands.
		if strings.HasPrefix(input, "/") {
			if handleSlashCommand(input, cfg, reader) {
				return nil // exit requested
			}
			continue
		}

		// Create a per-command cancellable context.
		ctx, cancel := context.WithCancel(context.Background())
		mu.Lock()
		cancelCmd = cancel
		mu.Unlock()

		// Run the command through the agent loop.
		if err := run(ctx, cfg, input, reader); err != nil {
			var rejErr *agent.RejectedError
			if errors.As(err, &rejErr) {
				switch rejErr.Stage {
				case "plan":
					fmt.Fprintln(os.Stderr, "Plan rejected.")
				case "execute":
					fmt.Fprintln(os.Stderr, "Execution stopped.")
				case "result":
					fmt.Fprintln(os.Stderr, "Result rejected.")
				}
			} else {
				printRunError(err, input, cfg)
			}
		}

		cancel()
		mu.Lock()
		cancelCmd = nil
		mu.Unlock()
		fmt.Fprintln(os.Stderr)
	}
}

// promptMissingAPIKey checks if the active provider's API key is empty and
// offers to set it interactively. If set, it saves the config to disk.
func promptMissingAPIKey(cfg *config.Config, reader *bufio.Reader) error {
	provName := activeProvider(cfg)
	if provName == "" {
		return nil
	}
	pc, ok := cfg.Providers[provName]
	if !ok || pc.APIKey != "" {
		return nil
	}

	fmt.Fprintf(os.Stderr, "%s API key not found. Would you like to enter it now? [y/n]: ", provName)
	answer, err := readLine(reader)
	if err != nil {
		return fmt.Errorf("repl: read answer: %w", err)
	}
	answer = strings.TrimSpace(strings.ToLower(answer))

	if answer != "y" && answer != "yes" {
		fmt.Fprintln(os.Stderr, "Warning: no API key configured. Commands will fail until a key is set (/key set).")
		fmt.Fprintln(os.Stderr)
		return nil
	}

	fmt.Fprintf(os.Stderr, "%s API key: ", provName)
	apiKey, err := readMasked(reader)
	if err != nil {
		return fmt.Errorf("repl: read API key: %w", err)
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "Warning: empty key entered. Commands will fail until a key is set (/key set).")
		fmt.Fprintln(os.Stderr)
		return nil
	}

	pc.APIKey = apiKey
	cfg.Providers[provName] = pc

	if err := saveConfigFile(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not save config: %v\n", err)
	} else {
		fmt.Fprintln(os.Stderr, "API key saved.")
	}
	fmt.Fprintln(os.Stderr)
	return nil
}

// handleSlashCommand processes REPL slash commands.
// Returns true if the REPL should exit.
func handleSlashCommand(input string, cfg *config.Config, reader *bufio.Reader) bool {
	parts := strings.Fields(strings.ToLower(strings.TrimSpace(input)))
	cmd := parts[0]

	switch cmd {
	case "/quit":
		fmt.Fprintln(os.Stderr, "Goodbye.")
		return true
	case "/help":
		fmt.Fprintln(os.Stderr, "Commands:")
		fmt.Fprintln(os.Stderr, "  /help             — show this help")
		fmt.Fprintln(os.Stderr, "  /model            — show current model")
		fmt.Fprintln(os.Stderr, "  /model <name>     — switch model (haiku, sonnet, opus)")
		fmt.Fprintln(os.Stderr, "  /key              — show active provider's API key (masked)")
		fmt.Fprintln(os.Stderr, "  /key <provider>   — show a provider's API key (masked)")
		fmt.Fprintln(os.Stderr, "  /key set          — set active provider's API key")
		fmt.Fprintln(os.Stderr, "  /key set <prov>   — set a provider's API key")
		fmt.Fprintln(os.Stderr, "  /quit             — exit REPL")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Type any other text to send a command to the agent.")
	case "/model":
		handleModelCommand(parts[1:], cfg)
	case "/key":
		handleKeyCommand(parts[1:], cfg, reader)
	default:
		suggestSlashCommand(cmd)
	}

	return false
}

// activeProvider returns the name of the provider that will be used next.
func activeProvider(cfg *config.Config) string {
	if len(cfg.FallbackChain) > 0 {
		return cfg.FallbackChain[0].Provider
	}
	return cfg.DefaultProvider
}

// activeModel returns the model that will be used for the next run() call.
func activeModel(cfg *config.Config) string {
	if len(cfg.FallbackChain) > 0 {
		return cfg.FallbackChain[0].Model
	}
	if pc, ok := cfg.Providers[cfg.DefaultProvider]; ok && len(pc.Models) > 0 {
		return pc.Models[0]
	}
	return "(unknown)"
}

// handleModelCommand shows or switches the active model.
func handleModelCommand(args []string, cfg *config.Config) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Current model: %s\n", activeModel(cfg))
		return
	}

	alias := args[0]
	fullModel, ok := modelAliases[alias]
	if !ok {
		fmt.Fprintf(os.Stderr, "Unknown model %q. Available: haiku, sonnet, opus\n", alias)
		return
	}

	// Update the active (first) entry in the fallback chain.
	if len(cfg.FallbackChain) > 0 {
		if cfg.FallbackChain[0].Provider != "anthropic" {
			fmt.Fprintf(os.Stderr, "Active provider is %s, not anthropic. Cannot switch to %s.\n",
				cfg.FallbackChain[0].Provider, fullModel)
			return
		}
		cfg.FallbackChain[0].Model = fullModel
		fmt.Fprintf(os.Stderr, "Switched to %s (session only)\n", fullModel)
		return
	}

	// No fallback chain — update the default provider's model list.
	pc, ok := cfg.Providers["anthropic"]
	if !ok {
		fmt.Fprintln(os.Stderr, "No anthropic provider configured.")
		return
	}
	if cfg.DefaultProvider != "anthropic" {
		fmt.Fprintf(os.Stderr, "Active provider is %s, not anthropic. Cannot switch to %s.\n",
			cfg.DefaultProvider, fullModel)
		return
	}
	if len(pc.Models) > 0 {
		pc.Models[0] = fullModel
	} else {
		pc.Models = []string{fullModel}
	}
	cfg.Providers["anthropic"] = pc
	fmt.Fprintf(os.Stderr, "Switched to %s (session only)\n", fullModel)
}

// handleKeyCommand handles /key subcommands.
func handleKeyCommand(args []string, cfg *config.Config, reader *bufio.Reader) {
	// Parse: /key, /key <provider>, /key set, /key set <provider>
	isSet := len(args) > 0 && args[0] == "set"
	var provName string

	if isSet {
		if len(args) > 1 {
			provName = args[1]
		}
	} else {
		if len(args) > 0 {
			provName = args[0]
		}
	}

	if provName == "" {
		provName = activeProvider(cfg)
	}

	if provName == "" {
		fmt.Fprintln(os.Stderr, "No provider configured.")
		return
	}

	pc, ok := cfg.Providers[provName]
	if !ok {
		fmt.Fprintf(os.Stderr, "Provider %q not found in config.\n", provName)
		return
	}

	if isSet {
		handleKeySet(provName, pc, cfg, reader)
	} else {
		handleKeyShow(provName, pc)
	}
}

// handleKeyShow displays a masked version of the provider's API key.
func handleKeyShow(provName string, pc config.ProviderConfig) {
	key := pc.APIKey
	if key == "" {
		fmt.Fprintf(os.Stderr, "%s API key: (not set)\n", provName)
		return
	}

	masked := maskKey(key)
	fmt.Fprintf(os.Stderr, "%s API key: %s\n", provName, masked)
}

// handleKeySet prompts for a new API key, updates the in-memory config,
// and saves it to disk.
func handleKeySet(provName string, pc config.ProviderConfig, cfg *config.Config, reader *bufio.Reader) {
	fmt.Fprintf(os.Stderr, "New %s API key: ", provName)
	apiKey, err := readMasked(reader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading key: %v\n", err)
		return
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "Empty key, not changed.")
		return
	}

	pc.APIKey = apiKey
	cfg.Providers[provName] = pc

	if err := saveConfigFile(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: key updated in session but could not save config: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "%s API key updated and saved.\n", provName)
	}
}

// maskKey returns "***...last8" for keys longer than 8 chars,
// or "***" for shorter keys.
func maskKey(key string) string {
	if len(key) <= 8 {
		return "***"
	}
	return "***..." + key[len(key)-8:]
}

// knownSlashCommands lists all valid REPL slash commands.
var knownSlashCommands = []string{"/help", "/quit", "/model", "/key"}

// suggestSlashCommand prints an "Unknown command" message. If a known command
// is within Levenshtein distance <= 2, it suggests it with "Did you mean ...?".
func suggestSlashCommand(cmd string) {
	bestDist := 3 // threshold + 1
	bestCmd := ""
	for _, known := range knownSlashCommands {
		d := agent.LevenshteinDistance(cmd, known)
		if d < bestDist {
			bestDist = d
			bestCmd = known
		}
	}
	if bestDist <= 2 {
		fmt.Fprintf(os.Stderr, "Unknown command '%s'. Did you mean '%s'?\n", cmd, bestCmd)
	} else {
		fmt.Fprintln(os.Stderr, "Unknown command. Type /help for available commands.")
	}
}
