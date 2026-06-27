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

	tea "github.com/charmbracelet/bubbletea"
	"github.com/halukerenozlu/bolt-cowork/internal/agent"
	"github.com/halukerenozlu/bolt-cowork/internal/config"
	"github.com/halukerenozlu/bolt-cowork/internal/mcp"
	"github.com/halukerenozlu/bolt-cowork/internal/provider"
	"github.com/halukerenozlu/bolt-cowork/internal/sandbox"
	"github.com/halukerenozlu/bolt-cowork/internal/skill"
	"github.com/halukerenozlu/bolt-cowork/internal/ui"
	"github.com/halukerenozlu/bolt-cowork/internal/ui/views"
	"github.com/halukerenozlu/bolt-cowork/pkg/types"
	keyring "github.com/zalando/go-keyring"
)

var version = "dev"

const keyringUnavailableMessage = "Keyring unavailable. API key will be stored in memory for this session only."

var (
	dirFlag         = flag.String("dir", ".", "Working directory for the agent")
	providerFlag    = flag.String("provider", "", "Override default provider (openai, anthropic)")
	approvalFlag    = flag.String("approval", "", "Approval mode: full, plan-only, dangerous-only, none")
	mcpApprovalFlag = flag.String("mcp-approval", "", "MCP tool approval mode: full, plan-only, dangerous-only, none")
	configFlag      = flag.String("config", "", "Path to config file (default: ~/.bolt-cowork/config.yaml)")
	versionFlag     = flag.Bool("version", false, "Show version information")
)

var setupTransientKeyWarning string

// lineReader abstracts line-oriented input for single-command mode and
// interactive slash-command prompts. bufioLineReader is the concrete
// implementation used at runtime.
type lineReader interface {
	// ReadLine reads a single line of visible input.
	ReadLine() (string, error)
	// ReadLineWithPrompt reads a single line, displaying prompt inline.
	ReadLineWithPrompt(prompt string) (string, error)
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

// ReadLineWithPrompt prints prompt to stderr and reads a line.
func (b *bufioLineReader) ReadLineWithPrompt(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	return b.ReadLine()
}

// ReadMasked uses the platform-specific readMasked function (term_*.go).
func (b *bufioLineReader) ReadMasked(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	return readMasked(b.r)
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

	if *versionFlag {
		fmt.Printf("bolt-cowork %s\n", version)
		return
	}

	args := flag.Args()

	// Handle "init" subcommand before loading config.
	if len(args) > 0 && args[0] == "init" {
		if _, err := runSetupTUI(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Validate --dir early: if explicitly set, directory must exist.
	if flagExplicitlySet("dir") {
		if info, err := os.Stat(*dirFlag); err != nil {
			if os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "Error: directory does not exist: %s\n", *dirFlag)
			} else {
				fmt.Fprintf(os.Stderr, "Error: cannot access directory %s: %v\n", *dirFlag, err)
			}
			os.Exit(1)
		} else if !info.IsDir() {
			fmt.Fprintf(os.Stderr, "Error: %s is not a directory\n", *dirFlag)
			os.Exit(1)
		}
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
		if !checkTrust(cfg, resolveWorkDir(cfg)) {
			if setupTransientKeyWarning != "" {
				fmt.Fprintln(os.Stderr, setupTransientKeyWarning)
			}
			os.Exit(0)
		}
		cfgPath, err := configFilePath()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		app := ui.New(cfg, version, buildTUIRunner(cfg), cfgPath)
		appErr := app.Run()
		if setupTransientKeyWarning != "" {
			fmt.Fprintln(os.Stderr, setupTransientKeyWarning)
		}
		if appErr != nil {
			fmt.Fprintln(os.Stderr, appErr)
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

	resolvedDir := resolveWorkDir(cfg)
	absDir, err := filepath.Abs(resolvedDir)
	if err != nil {
		absDir = resolvedDir
	}
	if !checkTrust(cfg, absDir) {
		os.Exit(0)
	}

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

	fmt.Fprintf(os.Stderr, "bolt-cowork %s | dir: %s | provider: %s | approval: %s\n",
		version, absDir, cfg.DefaultProvider, cfg.ApprovalMode)
	fmt.Fprintf(os.Stderr, "Command: %s\n\n", command)

	// Create redactor for single-command mode error output.
	var cmdSecrets []string
	for _, pc := range cfg.Providers {
		if pc.APIKey != "" {
			cmdSecrets = append(cmdSecrets, pc.APIKey)
		}
	}
	cmdRedactor := agent.NewRedactor(cmdSecrets)

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
		printRunError(err, command, cfg, cmdRedactor)
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

// loadOrInit loads existing config or launches the TUI setup wizard if none exists.
func loadOrInit() (*config.Config, error) {
	if configExists() {
		return loadConfig()
	}
	return runSetupTUI()
}

// runSetupTUI launches the interactive TUI setup wizard, saves the config and
// keyring entry on completion, then loads and returns the resulting config.
// Falls back to the CLI wizard if the TUI program fails to start.
func runSetupTUI() (*config.Config, error) {
	configPath, err := configFilePath()
	if err != nil {
		return nil, err
	}

	var transientProvider, transientAPIKey string
	saveFunc := func(provider, apiKey string) error {
		models, ok := providerModels[provider]
		if !ok {
			models = []string{"default"}
		}
		cfg := config.Default()
		cfg.DefaultProvider = provider
		pc := config.ProviderConfig{Models: append([]string(nil), models...)}
		if preset, ok := config.HostedPresets[provider]; ok {
			pc.Endpoint = preset.Endpoint
		}
		cfg.Providers = map[string]config.ProviderConfig{
			provider: pc,
		}
		cfg.FallbackChain = []config.FallbackEntry{
			{Provider: provider, Model: models[0]},
		}
		if err := config.SaveFile(cfg, configPath); err != nil {
			return err
		}
		if err := config.SetAPIKey(provider, apiKey); err != nil {
			transientProvider = provider
			transientAPIKey = apiKey
			return views.SetupWarningError{Message: keyringUnavailableMessage}
		}
		return nil
	}

	p := tea.NewProgram(views.NewSetup(saveFunc), tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("setup wizard could not start: %w", err)
	}

	setup, ok := finalModel.(views.Setup)
	if !ok || !setup.IsComplete() {
		return nil, fmt.Errorf("setup was cancelled")
	}

	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}
	if transientProvider != "" {
		pc := cfg.Providers[transientProvider]
		pc.APIKey = transientAPIKey
		cfg.Providers[transientProvider] = pc
		setupTransientKeyWarning = keyringUnavailableMessage
		fmt.Fprintln(os.Stderr, keyringUnavailableMessage)
	} else {
		setupTransientKeyWarning = ""
	}
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
	if *mcpApprovalFlag != "" {
		cfg.MCPApprovalMode = *mcpApprovalFlag
	}
}

// skillDefaultDirs returns the default skill directory search order:
// 1. Global user skills (~/.bolt-cowork/skills/)
// 2. Project-local skills (<workDir>/bolt-skills/)
// Bundled skills are loaded separately via LoadEmbedded from the binary's
// embedded FS, so the executable's directory is not included here.
func skillDefaultDirs(workDir string) []string {
	var dirs []string
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

	chain := provider.NewFallbackChain(providers, provider.WithOnFallback(func(from, to provider.LLMProvider, reason string) {
		fmt.Fprintf(os.Stderr, "Provider %s %s, falling back to %s\n", from.Name(), reason, to.Name())
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
		for _, w := range store.LoadAll(skillDirs) {
			fmt.Fprintln(os.Stderr, w)
		}
	}

	// Collect API key secrets and create redactor.
	var secrets []string
	for _, pc := range cfg.Providers {
		if pc.APIKey != "" {
			secrets = append(secrets, pc.APIKey)
		}
	}
	redactor := agent.NewRedactor(secrets)

	// Create spinner and CLI approver.
	spin := newSpinner(os.Stderr, "Planning...")
	approver := &CLIApprover{lr: lr, spinner: spin}

	// Create and run agent.
	mode := agent.ApprovalMode(cfg.ApprovalMode)
	ag := agent.New(chain, sb, approver, mode, store, redactor)
	if cfg.MCPApprovalMode != "" {
		ag.SetMCPApprovalMode(mcp.MCPApprovalMode(cfg.MCPApprovalMode))
	}
	ag.SetHistory(history)
	if len(forceSkills) > 0 {
		ag.SetForceSkills(forceSkills)
	}

	spin.Start()
	result, err := ag.Run(ctx, command)
	spin.Stop()
	if err != nil {
		return ag.History(), err
	}

	displayAgentResult(result)
	return ag.History(), nil
}

// displayAgentResult prints the outcome of an agent run to stdout/stderr.
// Zero-step results with a non-empty Description are conversational replies
// and should be shown as-is; empty plans show a generic warning.
func displayAgentResult(result *agent.Result) {
	if len(result.StepResults) == 0 {
		if result.Plan != nil && result.Plan.Description != "" {
			fmt.Println(result.Plan.Description)
		} else {
			fmt.Fprintln(os.Stderr, colorYellow("No actionable steps found. Try rephrasing your request."))
		}
		return
	}
	fmt.Println(colorGreen("\nTask completed successfully."))
	for i, sr := range result.StepResults {
		fmt.Printf("  %d. %s\n", i+1, sr)
	}
}

// buildProviders creates LLM providers from the config fallback chain.
func buildProviders(cfg *config.Config) []provider.LLMProvider {
	var providers []provider.LLMProvider

	for _, entry := range cfg.FallbackChain {
		pc, ok := cfg.Providers[entry.Provider]
		if !ok {
			continue
		}
		p := createProvider(entry.Provider, pc, entry.Model)
		if p != nil {
			providers = append(providers, p)
		}
	}

	// If fallback chain is empty, use default provider.
	if len(providers) == 0 {
		if pc, ok := cfg.Providers[cfg.DefaultProvider]; ok && len(pc.Models) > 0 {
			p := createProvider(cfg.DefaultProvider, pc, pc.Models[0])
			if p != nil {
				providers = append(providers, p)
			}
		}
	}

	return providers
}

func createProvider(name string, pc config.ProviderConfig, model string) provider.LLMProvider {
	switch name {
	case "openai":
		return provider.NewOpenAI(pc.APIKey, model)
	case "anthropic":
		return provider.NewAnthropic(pc.APIKey, model)
	case "gemini":
		return provider.NewGemini(pc.APIKey, model)
	default:
		endpoint := pc.Endpoint
		preset, isPreset := config.HostedPresets[name]
		if endpoint == "" && isPreset && preset.Endpoint != "" {
			endpoint = preset.Endpoint
		}
		if endpoint != "" {
			p := provider.NewCustom(name, endpoint, pc.APIKey, model)
			if isPreset && preset.RequiresAPIKey {
				p.SetRequiresAPIKey(true)
			}
			return p
		}
		fmt.Fprintf(os.Stderr, "Warning: unknown provider %q with no endpoint, skipping\n", name)
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
	lr      lineReader
	spinner *Spinner
}

// PromptRevision implements agent.RevisionPrompter. It reads a line of
// revision instructions from the user.
func (c *CLIApprover) PromptRevision(_ context.Context) (string, error) {
	input, err := c.lr.ReadLineWithPrompt("Revision instructions: ")
	if err != nil {
		return "", fmt.Errorf("read revision: %w", err)
	}
	return strings.TrimSpace(input), nil
}

func (c *CLIApprover) RequestApproval(_ context.Context, req agent.ApprovalRequest) (agent.Decision, error) {
	if c.spinner != nil {
		c.spinner.Stop()
	}
	// Print request details.
	fmt.Fprintf(os.Stderr, "\n--- %s approval ---\n", strings.ToUpper(req.Stage))
	if req.Dangerous {
		dangerLine := colorYellow("[DANGEROUS]")
		if req.DangerReason != "" {
			dangerLine += " - " + req.DangerReason
		}
		fmt.Fprintln(os.Stderr, dangerLine)
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

// tuiApprover is a mode-aware Approver used in TUI mode.
// When approval is required (based on mode), it sends an ApprovalRequestEvent
// to the TUI and blocks until the user responds via the response channel.
// When approval is not required, it auto-approves (notifying for dangerous ops).
type tuiApprover struct {
	mode       agent.ApprovalMode
	notify     func(string)
	onPermWarn func(string)
	onEvent    func(views.UIEvent)
}

func (t *tuiApprover) RequestApproval(ctx context.Context, req agent.ApprovalRequest) (agent.Decision, error) {
	if agent.ShouldApprove(t.mode, req.Stage, req.Dangerous) {
		if t.onEvent == nil {
			// No TUI event handler — fall back to auto-approve.
			return agent.Approve, nil
		}
		respCh := make(chan views.ApprovalResponse, 1)
		t.onEvent(views.ApprovalRequestEvent{
			Stage:       req.Stage,
			Description: req.Description,
			Items:       req.Items,
			Dangerous:   req.Dangerous,
			ResponseCh:  respCh,
		})
		// Block until the user responds or context is cancelled.
		select {
		case resp := <-respCh:
			switch views.ApprovalResponseLabel(respCh) {
			case "Approve":
				return agent.Approve, nil
			case "Approve all":
				return agent.ApproveAll, nil
			case "Revise":
				return agent.Revise, nil
			case "Reject":
				return agent.Reject, nil
			default:
				if resp.Approved {
					return agent.Approve, nil
				}
				return agent.Reject, nil
			}
		case <-ctx.Done():
			return agent.Reject, ctx.Err()
		}
	}

	// Auto-approve but notify for dangerous operations.
	if req.Dangerous {
		msg := fmt.Sprintf("[auto-approved] %s: %s", req.Stage, req.Description)
		if req.DangerReason != "" {
			msg += " ⚠ " + req.DangerReason
		}
		if t.notify != nil {
			t.notify(msg)
		}
		if t.onPermWarn != nil {
			warn := req.Stage + ": " + req.Description
			t.onPermWarn(warn)
		}
	}
	return agent.Approve, nil
}

// tuiRunResult is the internal result type for runTUI.
type tuiRunResult struct {
	Response string
	History  []types.Message
	Err      error
}

// runTUI executes one agent command for the TUI session. It reuses the
// provided skill store and redactor across calls. notify, if non-nil, is
// called by tuiApprover to surface auto-approved dangerous actions in the
// chat panel. onEvent, if non-nil, receives structured live-update events
// (plan steps, step completions) as the agent executes.
func runTUI(ctx context.Context, cfg *config.Config, command string, history []types.Message, store *skill.Store, redactor *agent.Redactor, notify func(string), onEvent func(views.UIEvent)) tuiRunResult {
	workDir := resolveWorkDir(cfg)
	absDir, err := filepath.Abs(workDir)
	if err != nil {
		absDir = workDir
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
		return tuiRunResult{Err: fmt.Errorf("create sandbox: %w", err)}
	}

	providers := buildProviders(cfg)
	if len(providers) == 0 {
		return tuiRunResult{Err: fmt.Errorf("no providers configured — set API keys in config or environment")}
	}

	chain := provider.NewFallbackChain(providers, provider.WithOnFallback(func(from, to provider.LLMProvider, reason string) {
		if onEvent != nil {
			onEvent(views.ProviderFallbackEvent{
				From:   from.Name(),
				To:     to.Name(),
				Reason: reason,
			})
		}
	}))

	mode := agent.ApprovalMode(cfg.ApprovalMode)
	approver := &tuiApprover{
		mode:   mode,
		notify: notify,
		onPermWarn: func(warn string) {
			if onEvent != nil {
				onEvent(views.PermWarnEvent{Warning: warn})
			}
		},
		onEvent: onEvent,
	}
	ag := agent.New(chain, sb, approver, mode, store, redactor)
	if cfg.MCPApprovalMode != "" {
		ag.SetMCPApprovalMode(mcp.MCPApprovalMode(cfg.MCPApprovalMode))
	}
	ag.SetHistory(history)

	// Wire live-update callbacks so the TUI receives plan steps, step starts,
	// and step completions as they happen, enabling the plan widget and right panel.
	if onEvent != nil {
		ag.SetPlanCallback(func(steps []string) {
			onEvent(views.PlanReadyEvent{Steps: steps})
		})
		ag.SetStepStartCallback(func(idx int, action, desc string) {
			onEvent(views.StepStartEvent{Index: idx, Action: action, Desc: desc})
		})
		ag.SetStepCallback(func(idx int, action, info string, err error) {
			onEvent(views.StepDoneEvent{Index: idx, Action: action, Info: info, Err: err})
		})
	}

	result, agErr := ag.Run(ctx, command)
	if agErr != nil {
		return tuiRunResult{History: ag.History(), Err: agErr}
	}

	// Step-level output is surfaced live via StepDoneEvent/execLog. Only
	// zero-step conversational plans use their description as the reply.
	resp := tuiResponseText(result)

	if onEvent != nil {
		if active := chain.LastActive(); active != nil {
			parts := strings.SplitN(active.Name(), "/", 2)
			p, m := parts[0], ""
			if len(parts) == 2 {
				m = parts[1]
			}
			onEvent(views.ProviderActiveEvent{Provider: p, Model: m})
		}
	}

	return tuiRunResult{
		Response: resp,
		History:  ag.History(),
	}
}

func tuiResponseText(result *agent.Result) string {
	if result == nil || result.Plan == nil || len(result.Plan.Steps) > 0 {
		return ""
	}
	return strings.TrimSpace(result.Plan.Description)
}

// buildTUIRunner constructs an AgentRunner for interactive TUI mode.
// The skill store is initialised once and reused across all user messages.
func buildTUIRunner(cfg *config.Config) views.AgentRunner {
	// Resolve display metadata.
	providerName := cfg.DefaultProvider
	modelName := ""
	if providerName != "" && len(cfg.FallbackChain) > 0 {
		providerName = cfg.FallbackChain[0].Provider
		modelName = cfg.FallbackChain[0].Model
	} else if pc, ok := cfg.Providers[cfg.DefaultProvider]; ok && len(pc.Models) > 0 {
		modelName = pc.Models[0]
	}

	workspace := resolveWorkDir(cfg)
	if abs, err := filepath.Abs(workspace); err == nil {
		workspace = abs
	}

	// Build skill store once for the session lifetime.
	store := skill.NewStore()
	if sub, err := fs.Sub(embeddedSkillsFS, "skills"); err == nil {
		_ = store.LoadEmbedded(sub) // ignore errors in TUI mode
	}
	skillDirs := cfg.Skills.Dirs
	if len(skillDirs) == 0 {
		skillDirs = skillDefaultDirs(workspace)
	}
	store.LoadAll(skillDirs) // warnings are discarded in TUI mode

	// Collect loaded skill names for the right panel SKILLS section.
	var loadedSkillNames []string
	loadedSkillContents := make(map[string]string)
	for _, sk := range store.GetAll() {
		loadedSkillNames = append(loadedSkillNames, sk.Metadata.Name)
		loadedSkillContents[sk.Metadata.Name] = sk.Content
	}

	// Collect API keys for redaction.
	var secrets []string
	for _, pc := range cfg.Providers {
		if pc.APIKey != "" {
			secrets = append(secrets, pc.APIKey)
		}
	}
	redactor := agent.NewRedactor(secrets)

	return views.AgentRunner{
		ConfigureProvider: func(name, apiKey string) {
			if cfg.Providers == nil {
				cfg.Providers = make(map[string]config.ProviderConfig)
			}
			pc := cfg.Providers[name]
			if preset, ok := config.HostedPresets[name]; ok && pc.Endpoint == "" {
				pc.Endpoint = preset.Endpoint
			}
			if len(pc.Models) == 0 {
				pc.Models = append([]string(nil), config.DefaultModels[name]...)
			}
			pc.APIKey = apiKey
			cfg.Providers[name] = pc
			redactor.AddSecret(apiKey)
		},
		PersistProviderKey: func(name, apiKey string) error {
			if err := config.SetAPIKey(name, apiKey); err != nil {
				return fmt.Errorf("store provider key: %w", err)
			}
			return nil
		},
		DeleteProviderKey: func(name string) error {
			if err := config.DeleteAPIKey(name); err != nil && !errors.Is(err, keyring.ErrNotFound) {
				return fmt.Errorf("remove provider key: %w", err)
			}
			return nil
		},
		HasStoredProviderKey: func(name string) bool {
			return config.GetAPIKey(name) != ""
		},
		HasEnvironmentProviderKey: func(name string) bool {
			return config.DetectEnvKey(name) != ""
		},
		VerifyProvider: func(ctx context.Context, name string) error {
			// Try existing chain first.
			for _, p := range buildProviders(cfg) {
				if p.Name() == name || strings.HasPrefix(p.Name(), name+"/") {
					return provider.VerifyProvider(ctx, p)
				}
			}
			// Provider not in chain — build one with fresh keyring key.
			pc := cfg.Providers[name]
			if pc.APIKey == "" {
				pc.APIKey = config.GetAPIKey(name)
			}
			if preset, ok := config.HostedPresets[name]; ok && pc.Endpoint == "" && preset.Endpoint != "" {
				pc.Endpoint = preset.Endpoint
			}
			model := ""
			if len(pc.Models) > 0 {
				model = pc.Models[0]
			} else if models := config.DefaultModels[name]; len(models) > 0 {
				model = models[0]
			}
			p := createProvider(name, pc, model)
			if p == nil {
				return fmt.Errorf("provider %q not found", name)
			}
			return provider.VerifyProvider(ctx, p)
		},
		DiscoverModels: func(ctx context.Context, name string) ([]string, error) {
			pc := cfg.Providers[name]
			if pc.APIKey == "" {
				pc.APIKey = config.GetAPIKey(name)
			}
			endpoint := pc.Endpoint
			if endpoint == "" {
				if preset, ok := config.HostedPresets[name]; ok && preset.Endpoint != "" {
					endpoint = preset.Endpoint
				}
			}
			if endpoint == "" {
				return nil, fmt.Errorf("no endpoint for %q", name)
			}
			return provider.DiscoverModels(ctx, endpoint, pc.APIKey)
		},
		Provider:      providerName,
		Model:         modelName,
		Workspace:     workspace,
		ApprovalMode:  cfg.ApprovalMode,
		LoadedSkills:  loadedSkillNames,
		SkillContents: loadedSkillContents,
		Run: func(ctx context.Context, cmd string, history []types.Message, onChunk func(string), onEvent func(views.UIEvent)) views.AgentResult {
			// Pass onChunk as the notify function so dangerous auto-approvals
			// are surfaced as system messages in the chat panel.
			r := runTUI(ctx, cfg, cmd, history, store, redactor, onChunk, onEvent)
			if r.Err == nil && r.Response != "" && onChunk != nil {
				onChunk(r.Response)
			}
			return views.AgentResult{
				History: r.History,
				Err:     r.Err,
			}
		},
	}
}

// checkTrust prompts the user for directory trust via a TUI modal if the
// directory is not yet in cfg.TrustedDirs. Returns true when execution should
// proceed, false when the user declined and the process should exit.
func checkTrust(cfg *config.Config, workDir string) bool {
	absDir, err := filepath.Abs(workDir)
	if err != nil {
		absDir = workDir
	}

	if config.IsTrusted(cfg, absDir) {
		return true
	}

	trustFunc := func(dir string) error {
		cfgPath, pathErr := configFilePath()
		if pathErr != nil {
			return pathErr
		}
		return config.AddTrustedDir(dir, cfgPath)
	}

	p := tea.NewProgram(views.NewTrustModal(absDir, trustFunc), tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Trust prompt could not start: %v\n", err)
		if promptTrustCLI(absDir, trustFunc) {
			cfg.TrustedDirs = append(cfg.TrustedDirs, absDir)
			return true
		}
		return false
	}

	modal, ok := finalModel.(views.TrustModal)
	if !ok || modal.IsDeclined() {
		fmt.Fprintln(os.Stderr, "Exiting.")
		return false
	}
	if modal.IsTrusted() {
		cfg.TrustedDirs = append(cfg.TrustedDirs, absDir)
		return true
	}

	fmt.Fprintln(os.Stderr, "Exiting.")
	return false
}

func promptTrustCLI(absDir string, trustFunc func(string) error) bool {
	reader := bufio.NewReader(os.Stdin)

	fmt.Fprintln(os.Stderr, "Trust this directory?")
	fmt.Fprintf(os.Stderr, "%s\n\n", absDir)
	fmt.Fprintln(os.Stderr, "bolt-cowork will be able to read, edit, and execute files here.")
	fmt.Fprint(os.Stderr, "Trust this folder? [y/N]: ")

	answer, err := readLine(reader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not read trust response: %v\n", err)
		return false
	}

	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes", "1":
		if err := trustFunc(absDir); err != nil {
			fmt.Fprintf(os.Stderr, "Could not save trusted directory: %v\n", err)
			return false
		}
		return true
	default:
		fmt.Fprintln(os.Stderr, "Exiting.")
		return false
	}
}
