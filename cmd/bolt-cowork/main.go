package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/halukerenozlu/bolt-cowork/internal/agent"
	"github.com/halukerenozlu/bolt-cowork/internal/config"
	"github.com/halukerenozlu/bolt-cowork/internal/provider"
	"github.com/halukerenozlu/bolt-cowork/internal/sandbox"
)

const version = "0.1.0"

var (
	dirFlag      = flag.String("dir", ".", "Working directory for the agent")
	providerFlag = flag.String("provider", "", "Override default provider (openai, anthropic)")
	approvalFlag = flag.String("approval", "", "Approval mode: full, plan-only, dangerous-only, none")
	configFlag   = flag.String("config", "", "Path to config file (default: ~/.bolt-cowork/config.yaml)")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: bolt-cowork [flags] [command]\n")
		fmt.Fprintf(os.Stderr, "       bolt-cowork init\n\n")
		fmt.Fprintf(os.Stderr, "If no command is given, enters interactive REPL mode.\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  bolt-cowork --dir ./workspace \"list files\"\n")
		fmt.Fprintf(os.Stderr, "  bolt-cowork --provider openai --approval none \"create README.md\"\n")
		fmt.Fprintf(os.Stderr, "  bolt-cowork init\n")
	}
	flag.Parse()

	args := flag.Args()

	// Handle "init" subcommand before loading config.
	if len(args) > 0 && args[0] == "init" {
		if _, err := runInit(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// No arguments → REPL mode (auto-init if config doesn't exist).
	if len(args) == 0 {
		cfg, err := loadOrInit()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		applyFlagOverrides(cfg)
		if err := cfg.Validate(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid config: %v\n", err)
			os.Exit(1)
		}
		if err := runREPL(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Load config for single command mode.
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	applyFlagOverrides(cfg)

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid config: %v\n", err)
		os.Exit(1)
	}

	// Single command mode.
	command := strings.Join(args, " ")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\nInterrupted.")
		cancel()
	}()
	defer signal.Stop(sigCh)

	if err := run(ctx, cfg, command, bufio.NewReader(os.Stdin)); err != nil {
		var rejErr *agent.RejectedError
		if errors.As(err, &rejErr) {
			switch rejErr.Stage {
			case "plan":
				fmt.Fprintln(os.Stderr, "Plan reddedildi.")
				return // exit 0 — henüz iş yapılmadı
			case "execute":
				fmt.Fprintln(os.Stderr, "Yürütme durduruldu.")
				os.Exit(1) // kısmen iş yapılmış olabilir
			case "result":
				fmt.Fprintln(os.Stderr, "Sonuç reddedildi.")
				os.Exit(1) // iş yapıldı ama kullanıcı kabul etmedi
			}
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func loadConfig() (*config.Config, error) {
	if *configFlag != "" {
		return config.LoadFile(*configFlag)
	}
	return config.Load()
}

// configExists reports whether the config file exists on disk.
func configExists() bool {
	path, err := configFilePath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// loadOrInit loads existing config or runs init wizard if no config exists.
func loadOrInit() (*config.Config, error) {
	if configExists() {
		return loadConfig()
	}

	fmt.Fprintln(os.Stderr, "No config found. Starting setup wizard...")
	fmt.Fprintln(os.Stderr)
	cfg, err := runInit()
	if err != nil {
		return nil, err
	}
	fmt.Fprintln(os.Stderr)
	return cfg, nil
}

// applyFlagOverrides applies CLI flag values to the config.
func applyFlagOverrides(cfg *config.Config) {
	if *providerFlag != "" {
		cfg.DefaultProvider = *providerFlag
	}
	if *approvalFlag != "" {
		cfg.ApprovalMode = *approvalFlag
	}
}

func run(ctx context.Context, cfg *config.Config, command string, reader *bufio.Reader) error {
	// Resolve working directory.
	workDir := resolveWorkDir(cfg)
	absDir, err := filepath.Abs(workDir)
	if err != nil {
		return fmt.Errorf("resolve directory: %w", err)
	}

	var sbOpts []sandbox.Option
	if len(cfg.Sandbox.DeniedPatterns) > 0 {
		sbOpts = append(sbOpts, sandbox.WithDeniedPatterns(cfg.Sandbox.DeniedPatterns...))
	}

	sb, err := sandbox.New(absDir, sbOpts...)
	if err != nil {
		return fmt.Errorf("create sandbox: %w", err)
	}

	// Build provider chain.
	providers := buildProviders(cfg)
	if len(providers) == 0 {
		return fmt.Errorf("no providers configured — set API keys in config or environment")
	}

	chain := provider.NewFallbackChain(providers, provider.WithOnFallback(func(from, to provider.LLMProvider) {
		fmt.Fprintf(os.Stderr, "Provider %s unavailable, falling back to %s\n", from.Name(), to.Name())
	}))

	// Create CLI approver.
	approver := &CLIApprover{reader: reader}

	// Create and run agent.
	mode := agent.ApprovalMode(cfg.ApprovalMode)
	ag := agent.New(chain, sb, approver, mode)

	fmt.Fprintf(os.Stderr, "bolt-cowork %s | dir: %s | provider: %s | approval: %s\n",
		version, absDir, cfg.DefaultProvider, cfg.ApprovalMode)
	fmt.Fprintf(os.Stderr, "Command: %s\n\n", command)

	result, err := ag.Run(ctx, command)
	if err != nil {
		return err
	}

	fmt.Println("\nTask completed successfully.")
	for i, sr := range result.StepResults {
		fmt.Printf("  %d. %s\n", i+1, sr)
	}

	return nil
}

// buildProviders creates LLM providers from the config fallback chain.
func buildProviders(cfg *config.Config) []provider.LLMProvider {
	var providers []provider.LLMProvider

	for _, entry := range cfg.FallbackChain {
		pc, ok := cfg.Providers[entry.Provider]
		if !ok {
			continue
		}
		p := createProvider(entry.Provider, pc.APIKey, entry.Model)
		if p != nil {
			providers = append(providers, p)
		}
	}

	// If fallback chain is empty, use default provider.
	if len(providers) == 0 {
		if pc, ok := cfg.Providers[cfg.DefaultProvider]; ok && len(pc.Models) > 0 {
			p := createProvider(cfg.DefaultProvider, pc.APIKey, pc.Models[0])
			if p != nil {
				providers = append(providers, p)
			}
		}
	}

	return providers
}

func createProvider(name, apiKey, model string) provider.LLMProvider {
	switch name {
	case "openai":
		return provider.NewOpenAI(apiKey, model)
	case "anthropic":
		return provider.NewAnthropic(apiKey, model)
	default:
		fmt.Fprintf(os.Stderr, "Warning: unknown provider %q, skipping\n", name)
		return nil
	}
}

// resolveWorkDir determines the working directory. If --dir was explicitly
// provided it takes priority. Otherwise, the first entry in
// config.sandbox.allowed_dirs is used. Falls back to "." if neither is set.
func resolveWorkDir(cfg *config.Config) string {
	if flagExplicitlySet("dir") {
		return *dirFlag
	}
	if len(cfg.Sandbox.AllowedDirs) > 0 && cfg.Sandbox.AllowedDirs[0] != "" {
		return cfg.Sandbox.AllowedDirs[0]
	}
	return "."
}

// flagExplicitlySet reports whether a flag was provided on the command line.
func flagExplicitlySet(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

// CLIApprover implements agent.Approver with interactive stdin/stderr prompts.
type CLIApprover struct {
	reader *bufio.Reader
}

func (c *CLIApprover) RequestApproval(_ context.Context, req agent.ApprovalRequest) (agent.Decision, error) {
	// Print request details.
	fmt.Fprintf(os.Stderr, "\n--- %s approval ---\n", strings.ToUpper(req.Stage))
	if req.Dangerous {
		fmt.Fprintln(os.Stderr, "[DANGEROUS]")
	}
	fmt.Fprintf(os.Stderr, "%s\n", req.Description)
	for _, item := range req.Items {
		fmt.Fprintf(os.Stderr, "  - %s\n", item)
	}

	// Show options based on stage.
	for {
		switch req.Stage {
		case "plan":
			fmt.Fprint(os.Stderr, "[a]pprove / [r]eject / re[v]ise: ")
		case "execute":
			fmt.Fprint(os.Stderr, "[a]pprove / approve a[l]l / [r]eject: ")
		default:
			fmt.Fprint(os.Stderr, "[a]ccept / [r]eject: ")
		}

		input, err := c.reader.ReadString('\n')
		if err != nil {
			return agent.Reject, fmt.Errorf("read input: %w", err)
		}
		input = strings.TrimSpace(strings.ToLower(input))

		switch input {
		case "a":
			return agent.Approve, nil
		case "r":
			return agent.Reject, nil
		case "v":
			if req.Stage == "plan" {
				return agent.Revise, nil
			}
		case "l":
			if req.Stage == "execute" {
				return agent.ApproveAll, nil
			}
		}

		fmt.Fprintln(os.Stderr, "Invalid input, try again.")
	}
}
