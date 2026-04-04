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

	// Single signal handler for the entire REPL session.
	// cancelCmd is non-nil only while a command is running.
	var (
		mu        sync.Mutex
		cancelCmd context.CancelFunc
	)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	go func() {
		for range sigCh {
			mu.Lock()
			fn := cancelCmd
			mu.Unlock()
			if fn != nil {
				fmt.Fprintln(os.Stderr, "\nInterrupted.")
				fn()
			}
			// If no command is running (prompt), the signal is ignored
			// and the user stays in the REPL.
		}
	}()

	for {
		fmt.Fprint(os.Stderr, "bolt-cowork> ")

		input, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				fmt.Fprintln(os.Stderr, "\nGoodbye.")
				return nil
			}
			return fmt.Errorf("repl: read input: %w", err)
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		// Handle slash commands.
		if strings.HasPrefix(input, "/") {
			if handleSlashCommand(input, cfg) {
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
					fmt.Fprintln(os.Stderr, "Plan reddedildi.")
				case "execute":
					fmt.Fprintln(os.Stderr, "Yürütme durduruldu.")
				case "result":
					fmt.Fprintln(os.Stderr, "Sonuç reddedildi.")
				}
			} else {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			}
		}

		cancel()
		mu.Lock()
		cancelCmd = nil
		mu.Unlock()
		fmt.Fprintln(os.Stderr)
	}
}

// handleSlashCommand processes REPL slash commands.
// Returns true if the REPL should exit.
func handleSlashCommand(input string, cfg *config.Config) bool {
	parts := strings.Fields(strings.ToLower(strings.TrimSpace(input)))
	cmd := parts[0]

	switch cmd {
	case "/quit", "/exit":
		fmt.Fprintln(os.Stderr, "Goodbye.")
		return true
	case "/help":
		fmt.Fprintln(os.Stderr, "Commands:")
		fmt.Fprintln(os.Stderr, "  /help          — show this help")
		fmt.Fprintln(os.Stderr, "  /model         — show current model")
		fmt.Fprintln(os.Stderr, "  /model <name>  — switch model (haiku, sonnet, opus)")
		fmt.Fprintln(os.Stderr, "  /quit          — exit REPL")
		fmt.Fprintln(os.Stderr, "  /exit          — exit REPL")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Type any other text to send a command to the agent.")
	case "/model":
		handleModelCommand(parts[1:], cfg)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s (type /help for available commands)\n", cmd)
	}

	return false
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
