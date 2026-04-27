package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/chzyer/readline"
	"github.com/halukerenozlu/bolt-cowork/internal/agent"
	"github.com/halukerenozlu/bolt-cowork/internal/config"
	"github.com/halukerenozlu/bolt-cowork/internal/provider"
	"github.com/halukerenozlu/bolt-cowork/internal/sandbox"
	"github.com/halukerenozlu/bolt-cowork/internal/skill"
	"github.com/halukerenozlu/bolt-cowork/pkg/types"
)

var version = "dev"

var (
	dirFlag      = flag.String("dir", ".", "Working directory for the agent")
	providerFlag = flag.String("provider", "", "Override default provider (openai, anthropic)")
	approvalFlag = flag.String("approval", "", "Approval mode: full, plan-only, dangerous-only, none")
	configFlag   = flag.String("config", "", "Path to config file (default: ~/.bolt-cowork/config.yaml)")
)

// lineReader abstracts line-oriented input. Both *bufio.Reader and
// *readline.Instance satisfy this via wrapper types.
type lineReader interface {
	// ReadLine reads a single line of visible input.
	ReadLine() (string, error)
	// ReadMasked reads a single line with echo disabled (for passwords/keys).
	ReadMasked(prompt string) (string, error)
}

// bufioLineReader wraps *bufio.Reader to satisfy lineReader.
type bufioLineReader struct {
	r *bufio.Reader
}

func (b *bufioLineReader) ReadLine() (string, error) {
	line, err := b.r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// ReadMasked uses the platform-specific readMasked function (term_*.go).
func (b *bufioLineReader) ReadMasked(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	return readMasked(b.r)
}

// readlineLineReader wraps *readline.Instance to satisfy lineReader.
// It temporarily overrides the prompt to an empty string for non-prompt reads,
// then restores it.
type readlineLineReader struct {
	rl *readline.Instance
}

func (r *readlineLineReader) ReadLine() (string, error) {
	saved := r.rl.Config.Prompt
	r.rl.SetPrompt("")
	defer r.rl.SetPrompt(saved)
	line, err := r.rl.Readline()
	if err == readline.ErrInterrupt {
		return "", errInterrupted
	}
	return line, err
}

// ReadMasked uses readline's built-in password mode (no echo).
func (r *readlineLineReader) ReadMasked(prompt string) (string, error) {
	pw, err := r.rl.ReadPassword(prompt)
	if err == readline.ErrInterrupt {
		return "", errInterrupted
	}
	return string(pw), err
}

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

	resolvedDir := resolveWorkDir(cfg)
	absDir, err := filepath.Abs(resolvedDir)
	if err != nil {
		absDir = resolvedDir
	}
	fmt.Fprintf(os.Stderr, "bolt-cowork %s | dir: %s | provider: %s | approval: %s\n",
		version, absDir, cfg.DefaultProvider, cfg.ApprovalMode)
	fmt.Fprintf(os.Stderr, "Command: %s\n\n", command)

	lr := &bufioLineReader{r: bufio.NewReader(os.Stdin)}
	if _, err := run(ctx, cfg, command, lr, nil, nil, nil); err != nil {
		var rejErr *agent.RejectedError
		if errors.As(err, &rejErr) {
			switch rejErr.Stage {
			case "plan":
				fmt.Fprintln(os.Stderr, "Plan rejected.")
				return // exit 0 — no work done yet
			case "execute":
				fmt.Fprintln(os.Stderr, "Execution stopped.")
				os.Exit(1) // partial work may have been done
			case "result":
				fmt.Fprintln(os.Stderr, "Result rejected.")
				os.Exit(1) // work done but user rejected outcome
			}
		}
		printRunError(err, command, cfg)
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

// skillDefaultDirs returns the default skill directory search order:
// 1. Built-in skills next to the executable
// 2. Global user skills (~/.bolt-cowork/skills/)
// 3. Project-local skills (./bolt-skills/)
func skillDefaultDirs(workDir string) []string {
	var dirs []string
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Join(filepath.Dir(exe), "skills"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".bolt-cowork", "skills"))
	}
	dirs = append(dirs, filepath.Join(workDir, "bolt-skills"))
	return dirs
}

func run(ctx context.Context, cfg *config.Config, command string, lr lineReader, history []types.Message, store *skill.Store, forceSkills []string) ([]types.Message, error) {
	// Resolve working directory.
	workDir := resolveWorkDir(cfg)
	absDir, err := filepath.Abs(workDir)
	if err != nil {
		return history, fmt.Errorf("resolve directory: %w", err)
	}

	var sbOpts []sandbox.Option
	if len(cfg.Sandbox.DeniedPatterns) > 0 {
		sbOpts = append(sbOpts, sandbox.WithDeniedPatterns(cfg.Sandbox.DeniedPatterns...))
	}
	if len(cfg.Sandbox.ReadOnlyDirs) > 0 {
		sbOpts = append(sbOpts, sandbox.WithReadOnlyDirs(cfg.Sandbox.ReadOnlyDirs...))
	}

	sb, err := sandbox.New(absDir, sbOpts...)
	if err != nil {
		return history, fmt.Errorf("create sandbox: %w", err)
	}

	// Build provider chain.
	providers := buildProviders(cfg)
	if len(providers) == 0 {
		return history, fmt.Errorf("no providers configured -- set API keys in config or environment")
	}

	chain := provider.NewFallbackChain(providers, provider.WithOnFallback(func(from, to provider.LLMProvider) {
		fmt.Fprintf(os.Stderr, "Provider %s unavailable, falling back to %s\n", from.Name(), to.Name())
	}))

	// Load skills if no store was provided.
	if store == nil {
		store = skill.NewStore()
		// Bundled skills are always loaded first; filesystem skills override them.
		if sub, err := fs.Sub(embeddedSkillsFS, "skills"); err == nil {
			if err := store.LoadEmbedded(sub); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: embedded skill loading error: %v\n", err)
			}
		}
		skillDirs := cfg.Skills.Dirs
		if len(skillDirs) == 0 {
			skillDirs = skillDefaultDirs(absDir)
		}
		if err := store.LoadAll(skillDirs); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: skill loading error: %v\n", err)
		}
	}

	// Create CLI approver.
	approver := &CLIApprover{lr: lr}

	// Create and run agent.
	mode := agent.ApprovalMode(cfg.ApprovalMode)
	ag := agent.New(chain, sb, approver, mode, store)
	ag.SetHistory(history)
	if len(forceSkills) > 0 {
		ag.SetForceSkills(forceSkills)
	}

	result, err := ag.Run(ctx, command)
	if err != nil {
		return ag.History(), err
	}

	fmt.Println("\nTask completed successfully.")
	for i, sr := range result.StepResults {
		fmt.Printf("  %d. %s\n", i+1, sr)
	}

	return ag.History(), nil
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
	case "gemini":
		return provider.NewGemini(apiKey, model)
	default:
		fmt.Fprintf(os.Stderr, "Warning: unknown provider %q, skipping\n", name)
		return nil
	}
}

// resolveWorkDir determines the working directory. Runtime override (from /dir)
// takes first priority, then --dir flag, then config.sandbox.allowed_dirs[0].
// Falls back to "." if none is set.
func resolveWorkDir(cfg *config.Config) string {
	if workDirOverride != "" {
		return workDirOverride
	}
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
	lr lineReader
}

// PromptRevision implements agent.RevisionPrompter. It reads a line of
// revision instructions from the user.
func (c *CLIApprover) PromptRevision(_ context.Context) (string, error) {
	fmt.Fprint(os.Stderr, "Revision instructions: ")
	input, err := c.lr.ReadLine()
	if err != nil {
		return "", fmt.Errorf("read revision: %w", err)
	}
	return strings.TrimSpace(input), nil
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

		input, err := c.lr.ReadLine()
		if err != nil {
			// EOF or interrupt during approval -> treat as cancellation.
			if errors.Is(err, io.EOF) || errors.Is(err, errInterrupted) {
				return agent.Reject, nil
			}
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

// SelectPath lets the user choose one candidate path before execution approval.
func (c *CLIApprover) SelectPath(_ context.Context, req agent.PathSelectionRequest) (string, error) {
	fmt.Fprintf(os.Stderr, "\n--- %s target selection ---\n", strings.ToUpper(req.Stage))
	fmt.Fprintf(os.Stderr, "Couldn't find %q directly. Select %s target:\n", req.OriginalPath, req.Action)
	for i, cand := range req.Candidates {
		kind := "file"
		if cand.IsDir {
			kind = "dir"
		}
		fmt.Fprintf(os.Stderr, "  %d. %s (%s)\n", i+1, cand.Path, kind)
	}

	for {
		fmt.Fprintf(os.Stderr, "Choose [1-%d] or [r]eject: ", len(req.Candidates))
		input, err := c.lr.ReadLine()
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, errInterrupted) {
				return "", nil
			}
			return "", fmt.Errorf("read input: %w", err)
		}

		input = strings.TrimSpace(strings.ToLower(input))
		if input == "r" {
			return "", nil
		}

		n, convErr := strconv.Atoi(input)
		if convErr == nil && n >= 1 && n <= len(req.Candidates) {
			return req.Candidates[n-1].Path, nil
		}

		fmt.Fprintln(os.Stderr, "Invalid input, try again.")
	}
}
