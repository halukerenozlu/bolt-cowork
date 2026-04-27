package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chzyer/readline"
	"github.com/halukerenozlu/bolt-cowork/internal/agent"
	"github.com/halukerenozlu/bolt-cowork/internal/config"
	"github.com/halukerenozlu/bolt-cowork/internal/skill"
	"github.com/halukerenozlu/bolt-cowork/pkg/types"
	"gopkg.in/yaml.v3"
)

// signalCanceller manages Ctrl+C signal handling for command cancellation.
// It runs a goroutine that listens for os.Interrupt and calls the active
// cancel function if one is set. This keeps the REPL alive during Ctrl+C.
type signalCanceller struct {
	mu       sync.Mutex
	cancelFn context.CancelFunc
	sigCh    chan os.Signal
	done     chan struct{}
}

// newSignalCanceller creates and starts a signal canceller.
func newSignalCanceller() *signalCanceller {
	sc := &signalCanceller{
		sigCh: make(chan os.Signal, 1),
		done:  make(chan struct{}),
	}
	signal.Notify(sc.sigCh, os.Interrupt)
	go sc.run()
	return sc
}

// run listens for interrupt signals and cancels the active command.
func (sc *signalCanceller) run() {
	for {
		select {
		case <-sc.done:
			return
		case <-sc.sigCh:
			sc.mu.Lock()
			fn := sc.cancelFn
			sc.mu.Unlock()
			if fn != nil {
				fmt.Fprintln(os.Stderr, "\nCommand cancelled.")
				fn()
			}
		}
	}
}

// setCancel sets the active cancel function for the current command.
func (sc *signalCanceller) setCancel(fn context.CancelFunc) {
	sc.mu.Lock()
	sc.cancelFn = fn
	sc.mu.Unlock()
}

// clearCancel removes the active cancel function.
func (sc *signalCanceller) clearCancel() {
	sc.mu.Lock()
	sc.cancelFn = nil
	sc.mu.Unlock()
}

// stop stops the signal canceller goroutine and unregisters the signal.
func (sc *signalCanceller) stop() {
	signal.Stop(sc.sigCh)
	close(sc.done)
}

// modelAliases maps short names to full model IDs (Anthropic shortcuts).
var modelAliases = map[string]string{
	"haiku":  "claude-haiku-4-5-20251001",
	"sonnet": "claude-sonnet-4-6",
	"opus":   "claude-opus-4-6",
}

// detectProvider infers the provider from a model name.
func detectProvider(model string) string {
	switch {
	case strings.HasPrefix(model, "claude-") || model == "haiku" || model == "sonnet" || model == "opus":
		return "anthropic"
	case strings.HasPrefix(model, "gpt-") || strings.HasPrefix(model, "o3-") || strings.HasPrefix(model, "o1-"):
		return "openai"
	case strings.HasPrefix(model, "gemini-"):
		return "gemini"
	default:
		return ""
	}
}

// workDirOverride is set by /dir to override the working directory at runtime.
var workDirOverride string

// newReadlineCompleter builds a PrefixCompleter for slash commands.
func newReadlineCompleter() *readline.PrefixCompleter {
	return readline.NewPrefixCompleter(
		readline.PcItem("/help"),
		readline.PcItem("/quit"),
		readline.PcItem("/clear"),
		readline.PcItem("/model",
			readline.PcItem("haiku"),
			readline.PcItem("sonnet"),
			readline.PcItem("opus"),
			readline.PcItem("gpt-4o"),
			readline.PcItem("gpt-4o-mini"),
			readline.PcItem("gemini-2.5-pro"),
			readline.PcItem("gemini-2.5-flash"),
		),
		readline.PcItem("/key",
			readline.PcItem("set",
				readline.PcItem("anthropic"),
				readline.PcItem("openai"),
				readline.PcItem("gemini"),
			),
			readline.PcItem("help"),
			readline.PcItem("anthropic"),
			readline.PcItem("openai"),
			readline.PcItem("gemini"),
		),
		readline.PcItem("/config",
			readline.PcItem("show"),
			readline.PcItem("path"),
			readline.PcItem("reload"),
			readline.PcItem("set"),
			readline.PcItem("help"),
		),
		readline.PcItem("/dir"),
		readline.PcItem("/skills"),
		readline.PcItem("/skill"),
		readline.PcItem("/use"),
	)
}

// historyFilePath returns the path for readline history storage.
func historyFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(home, ".bolt-cowork")
	_ = os.MkdirAll(dir, 0755)
	return filepath.Join(dir, "history")
}

// initSkillStore creates and loads a skill store from config or defaults.
func initSkillStore(cfg *config.Config) *skill.Store {
	store := skill.NewStore()
	skillDirs := cfg.Skills.Dirs
	if len(skillDirs) == 0 {
		workDir := resolveWorkDir(cfg)
		absDir, err := filepath.Abs(workDir)
		if err != nil {
			absDir = workDir
		}
		skillDirs = skillDefaultDirs(absDir)
	}
	if err := store.LoadAll(skillDirs); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: skill loading error: %v\n", err)
	}
	return store
}

// printBanner prints the ASCII logo and startup info to stderr.
func printBanner(cfg *config.Config) {
	workDir := resolveWorkDir(cfg)
	fmt.Fprintf(os.Stderr, "  \u2588\u2588\u2588\u2588\u2588\u2588\u2557  \u2588\u2588\u2588\u2588\u2588\u2588\u2557 \u2588\u2588\u2557  \u2588\u2588\u2588\u2588\u2588\u2588\u2588\u2557\n")
	fmt.Fprintf(os.Stderr, "  \u2588\u2588\u2554\u2550\u2550\u2588\u2588\u2557\u2588\u2588\u2554\u2550\u2550\u2550\u2588\u2588\u2557\u2588\u2588\u2551  \u2554\u2550\u2550\u2588\u2588\u2554\u2550\u2550\u2557\n")
	fmt.Fprintf(os.Stderr, "  \u2588\u2588\u2588\u2588\u2588\u2588\u2554\u255d\u2588\u2588\u2551   \u2588\u2588\u2551\u2588\u2588\u2551     \u2588\u2588\u2551       C o w o r k\n")
	fmt.Fprintf(os.Stderr, "  \u2588\u2588\u2554\u2550\u2550\u2588\u2588\u2557\u2588\u2588\u2551   \u2588\u2588\u2551\u2588\u2588\u2551     \u2588\u2588\u2551         v%s\n", version)
	fmt.Fprintf(os.Stderr, "  \u2588\u2588\u2588\u2588\u2588\u2588\u2554\u255d\u255a\u2588\u2588\u2588\u2588\u2588\u2588\u2554\u255d\u2588\u2588\u2588\u2588\u2588\u2588\u2557\u2588\u2588\u2551\n")
	fmt.Fprintf(os.Stderr, "  \u255a\u2550\u2550\u2550\u2550\u2550\u255d  \u255a\u2550\u2550\u2550\u2550\u2550\u255d \u255a\u2550\u2550\u2550\u2550\u2550\u2550\u255d\u255a\u2550\u255d    Native File Agent\n")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "  dir: %s | provider: %s | approval: %s\n",
		workDir, cfg.DefaultProvider, cfg.ApprovalMode)
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "  Type /help to get started")
	fmt.Fprintln(os.Stderr)
}

// runREPL starts an interactive REPL session.
func runREPL(cfg *config.Config) error {
	// Pre-readline: check API key using bufio (readline not yet active).
	bufReader := bufio.NewReader(os.Stdin)
	if err := promptMissingAPIKey(cfg, bufReader); err != nil {
		return err
	}

	// Try to create a readline instance; fall back to bufio if it fails
	// (e.g. piped stdin). Logo is only shown in interactive (readline) mode.
	rl, rlErr := readline.NewEx(&readline.Config{
		Prompt:            "bolt-cowork> ",
		HistoryFile:       historyFilePath(),
		AutoComplete:      newReadlineCompleter(),
		InterruptPrompt:   "^C",
		EOFPrompt:         "exit",
		HistorySearchFold: true,
		Stderr:            os.Stderr,
		Stdout:            os.Stderr,
	})

	if rlErr != nil {
		// Readline failed to init -- fall back to the old bufio loop.
		lr := &bufioLineReader{r: bufReader}
		return runREPLFallback(cfg, lr)
	}
	defer rl.Close()

	// Show logo only in interactive (readline) mode, not when stdin is piped.
	printBanner(cfg)

	// All interactive reads now go through readline.
	lr := &readlineLineReader{rl: rl}

	// Load skills once for the session.
	skillStore := initSkillStore(cfg)

	// Signal-based cancellation for mid-run Ctrl+C (when readline is not
	// active). Readline intercepts Ctrl+C only when Readline() is blocking;
	// during run() execution we need the OS signal handler instead.
	sc := newSignalCanceller()
	defer sc.stop()

	// Track Ctrl+C double-press for exit.
	var (
		lastCtrlC   time.Time
		history     []types.Message
		forceSkills []string
	)
	const ctrlCWindow = 3 * time.Second

	for {
		line, err := rl.Readline()
		if err != nil {
			if err == readline.ErrInterrupt {
				// At prompt -- double Ctrl+C logic.
				now := time.Now()
				if !lastCtrlC.IsZero() && now.Sub(lastCtrlC) < ctrlCWindow {
					fmt.Fprintln(os.Stderr, "Goodbye.")
					return nil
				}
				lastCtrlC = now
				fmt.Fprintln(os.Stderr, "Press Ctrl+C again to quit, or type /quit.")
				continue
			}
			if err == io.EOF {
				fmt.Fprintln(os.Stderr, "\nGoodbye.")
				return nil
			}
			return fmt.Errorf("repl: read input: %w", err)
		}

		// Successful input resets the Ctrl+C window.
		lastCtrlC = time.Time{}

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}

		// Handle slash commands.
		if strings.HasPrefix(input, "/") {
			if handleSlashCommand(input, cfg, lr, &history, skillStore, &forceSkills) {
				return nil // exit requested
			}
			continue
		}

		// Create a per-command cancellable context.
		ctx, cancel := context.WithCancel(context.Background())
		sc.setCancel(cancel)

		// Run the command through the agent loop.
		newHistory, err := run(ctx, cfg, input, lr, history, skillStore, forceSkills)
		forceSkills = nil // one-shot: clear after use
		history = newHistory
		if err != nil {
			if ctx.Err() == context.Canceled {
				// Already printed "Command cancelled." in signal handler.
			} else {
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
		}

		cancel()
		sc.clearCancel()
		fmt.Fprintln(os.Stderr)
	}
}

// runREPLFallback is the old bufio-based REPL loop used when readline is
// unavailable (piped stdin, etc.). All input goes through the single lr.
func runREPLFallback(cfg *config.Config, lr lineReader) error {
	var (
		lastCtrlC   time.Time
		history     []types.Message
		forceSkills []string
	)
	const ctrlCWindow = 3 * time.Second

	skillStore := initSkillStore(cfg)

	sc := newSignalCanceller()
	defer sc.stop()

	for {
		fmt.Fprint(os.Stderr, "bolt-cowork> ")

		input, err := lr.ReadLine()
		if err != nil {
			if errors.Is(err, io.EOF) {
				fmt.Fprintln(os.Stderr, "\nGoodbye.")
				return nil
			}
			if errors.Is(err, errInterrupted) {
				now := time.Now()
				if !lastCtrlC.IsZero() && now.Sub(lastCtrlC) < ctrlCWindow {
					fmt.Fprintln(os.Stderr, "Goodbye.")
					return nil
				}
				lastCtrlC = now
				fmt.Fprintln(os.Stderr, "Press Ctrl+C again to quit, or type /quit.")
				continue
			}
			return fmt.Errorf("repl: read input: %w", err)
		}

		lastCtrlC = time.Time{}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		if strings.HasPrefix(input, "/") {
			if handleSlashCommand(input, cfg, lr, &history, skillStore, &forceSkills) {
				return nil
			}
			continue
		}

		ctx, cancel := context.WithCancel(context.Background())
		sc.setCancel(cancel)

		newHistory, err := run(ctx, cfg, input, lr, history, skillStore, forceSkills)
		forceSkills = nil // one-shot: clear after use
		history = newHistory
		if err != nil {
			if ctx.Err() == context.Canceled {
				// Already printed "Command cancelled." in signal handler.
			} else {
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
		}

		cancel()
		sc.clearCancel()
		fmt.Fprintln(os.Stderr)
	}
}

// promptMissingAPIKey checks if the active provider's API key is empty and
// offers to set it interactively. If set, it saves the config to disk.
// Called before readline is initialized, so uses bufio.Reader directly.
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
func handleSlashCommand(input string, cfg *config.Config, lr lineReader, history *[]types.Message, store *skill.Store, forceSkills *[]string) bool {
	trimmed := strings.TrimSpace(input)
	parts := strings.Fields(trimmed)
	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "/quit":
		fmt.Fprintln(os.Stderr, "Goodbye.")
		return true
	case "/clear":
		*history = nil
		fmt.Fprintln(os.Stderr, "Conversation history cleared.")
	case "/help":
		fmt.Fprintln(os.Stderr, "Commands:")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "  General:")
		fmt.Fprintln(os.Stderr, "    /help              Show this help")
		fmt.Fprintln(os.Stderr, "    /clear             Clear conversation history")
		fmt.Fprintln(os.Stderr, "    /quit              Exit bolt-cowork")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "  Config:")
		fmt.Fprintln(os.Stderr, "    /config            Show current configuration")
		fmt.Fprintln(os.Stderr, "    /config show       Show current configuration")
		fmt.Fprintln(os.Stderr, "    /config path       Show config file path")
		fmt.Fprintln(os.Stderr, "    /config reload     Reload config from disk")
		fmt.Fprintln(os.Stderr, "    /config set        Set a config value (planned)")
		fmt.Fprintln(os.Stderr, "    /config help       Show config subcommands")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "  Skills:")
		fmt.Fprintln(os.Stderr, "    /skills            List all loaded skills")
		fmt.Fprintln(os.Stderr, "    /skill <name>      Show skill details")
		fmt.Fprintln(os.Stderr, "    /use <name>        Activate skill for next command")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "  Provider & Model:")
		fmt.Fprintln(os.Stderr, "    /model             Show current model")
		fmt.Fprintln(os.Stderr, "    /model <name>      Switch model (haiku, sonnet, opus, gpt-4o, gemini-2.5-pro, ...)")
		fmt.Fprintln(os.Stderr, "    /key               Show active provider's API key")
		fmt.Fprintln(os.Stderr, "    /key <provider>    Show a provider's API key (masked)")
		fmt.Fprintln(os.Stderr, "    /key set           Set active provider's API key")
		fmt.Fprintln(os.Stderr, "    /key set <prov>    Set a provider's API key")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "  Workspace:")
		fmt.Fprintln(os.Stderr, "    /dir               Show working directory")
		fmt.Fprintln(os.Stderr, "    /dir <path>        Change working directory")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Type any other text to send a command to the agent.")
	case "/model":
		handleModelCommand(lowerArgs(parts[1:]), cfg)
	case "/key":
		handleKeyCommand(lowerArgs(parts[1:]), cfg, lr)
	case "/config":
		handleConfigCommand(lowerArgs(parts[1:]), cfg)
	case "/dir":
		// /dir preserves original case for path argument.
		handleDirCommand(parts[1:], cfg)
	case "/skills":
		handleSkillsCommand(store)
	case "/skill":
		handleSkillCommand(parts[1:], store)
	case "/use":
		handleUseCommand(parts[1:], store, forceSkills)
	default:
		suggestSlashCommand(cmd)
	}

	return false
}

// printConfigHelp prints the list of /config subcommands.
func printConfigHelp() {
	fmt.Fprintln(os.Stderr, "Config commands:")
	fmt.Fprintln(os.Stderr, "  /config show     Show current configuration")
	fmt.Fprintln(os.Stderr, "  /config path     Show config file path")
	fmt.Fprintln(os.Stderr, "  /config reload   Reload config from disk")
	fmt.Fprintln(os.Stderr, "  /config set      Set a config value (planned)")
	fmt.Fprintln(os.Stderr, "  /config help     Show this list")
}

// printSkillHelp prints the list of skill-related commands.
func printSkillHelp() {
	fmt.Fprintln(os.Stderr, "Skill commands:")
	fmt.Fprintln(os.Stderr, "  /skills          List all loaded skills")
	fmt.Fprintln(os.Stderr, "  /skill <name>    Show skill details")
	fmt.Fprintln(os.Stderr, "  /use <name>      Activate skill for next command")
}

// printKeyHelp prints the list of /key subcommands.
func printKeyHelp() {
	fmt.Fprintln(os.Stderr, "Key commands:")
	fmt.Fprintln(os.Stderr, "  /key              Show active provider's API key")
	fmt.Fprintln(os.Stderr, "  /key <provider>   Show a provider's API key")
	fmt.Fprintln(os.Stderr, "  /key set          Set active provider's API key")
	fmt.Fprintln(os.Stderr, "  /key set <prov>   Set a provider's API key")
}

// handleSkillsCommand lists all loaded skills.
func handleSkillsCommand(store *skill.Store) {
	if store == nil {
		fmt.Fprintln(os.Stderr, "No skill store available.")
		return
	}
	skills := store.GetAll()
	if len(skills) == 0 {
		fmt.Fprintln(os.Stderr, "No skills loaded.")
		return
	}
	fmt.Fprintf(os.Stderr, "Loaded skills (%d):\n", len(skills))
	for _, sk := range skills {
		auto := " "
		if sk.AutoTrigger {
			auto = "*"
		}
		fmt.Fprintf(os.Stderr, "  %s %-20s [%s] %s\n", auto, sk.Name, sk.Source, sk.Description)
	}
	fmt.Fprintln(os.Stderr, "\n  * = auto_trigger enabled")
}

// handleSkillCommand shows details for a specific skill, or lists skill commands if called with no args.
func handleSkillCommand(args []string, store *skill.Store) {
	if store == nil {
		fmt.Fprintln(os.Stderr, "No skill store available.")
		return
	}
	if len(args) == 0 {
		printSkillHelp()
		return
	}
	name := strings.ToLower(args[0])
	sk, err := store.GetByName(name)
	if err != nil {
		// Try levenshtein suggestion.
		all := store.GetAll()
		bestDist := 3
		bestName := ""
		for _, s := range all {
			d := agent.LevenshteinDistance(name, s.Name)
			if d < bestDist {
				bestDist = d
				bestName = s.Name
			}
		}
		if bestDist <= 2 {
			fmt.Fprintf(os.Stderr, "Skill %q not found. Did you mean %q?\n", name, bestName)
		} else {
			fmt.Fprintf(os.Stderr, "Skill %q not found.\n", name)
		}
		return
	}

	fmt.Fprintf(os.Stderr, "Name:         %s\n", sk.Name)
	fmt.Fprintf(os.Stderr, "Description:  %s\n", sk.Description)
	fmt.Fprintf(os.Stderr, "Source:       %s\n", sk.Source)
	fmt.Fprintf(os.Stderr, "AutoTrigger:  %v\n", sk.AutoTrigger)
	fmt.Fprintf(os.Stderr, "File:         %s\n", sk.FilePath)
	if sk.Content != "" {
		lines := strings.SplitN(sk.Content, "\n", 6)
		preview := lines
		if len(lines) > 5 {
			preview = lines[:5]
		}
		fmt.Fprintln(os.Stderr, "Content (first 5 lines):")
		for _, line := range preview {
			fmt.Fprintf(os.Stderr, "  %s\n", line)
		}
		if len(lines) > 5 {
			fmt.Fprintln(os.Stderr, "  ...")
		}
	}
	fmt.Fprintf(os.Stderr, "\nUse '/use %s' to activate for next command.\n", sk.Name)
}

// handleUseCommand activates a skill for the next command.
func handleUseCommand(args []string, store *skill.Store, forceSkills *[]string) {
	if store == nil {
		fmt.Fprintln(os.Stderr, "No skill store available.")
		return
	}
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: /use <skill-name>")
		return
	}
	name := strings.ToLower(args[0])
	sk, err := store.GetByName(name)
	if err != nil {
		all := store.GetAll()
		bestDist := 3
		bestName := ""
		for _, s := range all {
			d := agent.LevenshteinDistance(name, s.Name)
			if d < bestDist {
				bestDist = d
				bestName = s.Name
			}
		}
		if bestDist <= 2 {
			fmt.Fprintf(os.Stderr, "Skill %q not found. Did you mean %q?\n", name, bestName)
		} else {
			fmt.Fprintf(os.Stderr, "Skill %q not found.\n", name)
		}
		return
	}
	// Add to forceSkills (avoid duplicates).
	for _, existing := range *forceSkills {
		if existing == sk.Name {
			fmt.Fprintf(os.Stderr, "Skill %q already activated.\n", sk.Name)
			return
		}
	}
	*forceSkills = append(*forceSkills, sk.Name)
	fmt.Fprintf(os.Stderr, "Skill %q activated for next command.\n", sk.Name)
}

// lowerArgs lowercases each string in a slice.
func lowerArgs(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = strings.ToLower(a)
	}
	return out
}

// handleConfigCommand handles /config subcommands.
func handleConfigCommand(args []string, cfg *config.Config) {
	if len(args) == 0 {
		showMaskedConfig(cfg)
		return
	}

	switch args[0] {
	case "help":
		printConfigHelp()
	case "show":
		showMaskedConfig(cfg)
	case "path":
		path, err := configFilePath()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return
		}
		fmt.Fprintln(os.Stderr, path)
	case "reload":
		newCfg, err := loadConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reloading config: %v\n", err)
			return
		}
		if err := newCfg.Validate(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid config: %v\n", err)
			return
		}
		// Update in place so the pointer in runREPL stays valid.
		*cfg = *newCfg
		fmt.Fprintln(os.Stderr, "Config reloaded.")
	case "set":
		fmt.Fprintln(os.Stderr, "/config set: not yet implemented. Use /key set to change API keys.")
	default:
		fmt.Fprintf(os.Stderr, "Unknown subcommand %q. Available:\n  show, path, reload, set\n", args[0])
	}
}

// showMaskedConfig marshals the config to YAML with API keys masked.
func showMaskedConfig(cfg *config.Config) {
	// Make a shallow copy so we don't modify the live config.
	masked := *cfg
	masked.Providers = make(map[string]config.ProviderConfig, len(cfg.Providers))
	for name, pc := range cfg.Providers {
		pc.APIKey = maskKey(pc.APIKey)
		masked.Providers[name] = pc
	}

	data, err := yaml.Marshal(&masked)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling config: %v\n", err)
		return
	}
	fmt.Fprint(os.Stderr, string(data))
}

// handleDirCommand handles /dir subcommands.
func handleDirCommand(args []string, cfg *config.Config) {
	if len(args) == 0 {
		dir := resolveWorkDir(cfg)
		absDir, err := filepath.Abs(dir)
		if err != nil {
			absDir = dir
		}
		fmt.Fprintln(os.Stderr, absDir)
		return
	}

	newDir := strings.Join(args, " ")
	absDir, err := filepath.Abs(newDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid path: %v\n", err)
		return
	}

	info, err := os.Stat(absDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}
	if !info.IsDir() {
		fmt.Fprintf(os.Stderr, "Error: %s is not a directory\n", absDir)
		return
	}

	// Check if path is within allowed dirs (if configured).
	if len(cfg.Sandbox.AllowedDirs) > 0 {
		allowed := false
		for _, ad := range cfg.Sandbox.AllowedDirs {
			absAllowed, err := filepath.Abs(ad)
			if err != nil {
				continue
			}
			rel, err := filepath.Rel(absAllowed, absDir)
			if err != nil {
				continue
			}
			if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				allowed = true
				break
			}
		}
		if !allowed {
			fmt.Fprintf(os.Stderr, "Error: %s is outside allowed directories\n", absDir)
			return
		}
	}

	workDirOverride = absDir
	fmt.Fprintf(os.Stderr, "Working directory changed to %s\n", absDir)
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
// Supports cross-provider switching: /model gpt-4o → switches to openai,
// /model gemini-2.5-pro → switches to gemini, /model sonnet → anthropic.
func handleModelCommand(args []string, cfg *config.Config) {
	if len(args) == 0 {
		prov := activeProvider(cfg)
		fmt.Fprintf(os.Stderr, "Current model: %s (%s)\n", activeModel(cfg), prov)
		return
	}

	input := args[0]

	// Resolve alias (haiku/sonnet/opus) to full model name.
	fullModel := input
	if alias, ok := modelAliases[input]; ok {
		fullModel = alias
	}

	// Detect which provider this model belongs to.
	prov := detectProvider(input)
	if prov == "" {
		fmt.Fprintf(os.Stderr, "Unknown model %q. Available: haiku, sonnet, opus, gpt-4o, gpt-4o-mini, gemini-2.5-pro, gemini-2.5-flash\n", input)
		return
	}

	// Ensure provider exists in config.
	pc, ok := cfg.Providers[prov]
	if !ok {
		fmt.Fprintf(os.Stderr, "Warning: provider %q not configured. Add it with 'bolt-cowork init' or /key set %s.\n", prov, prov)
		return
	}

	// Ensure the model is in the provider's model list (add if not).
	if !containsString(pc.Models, fullModel) {
		pc.Models = append(pc.Models, fullModel)
		cfg.Providers[prov] = pc
	}

	// Update fallback chain.
	if len(cfg.FallbackChain) > 0 {
		cfg.FallbackChain[0].Provider = prov
		cfg.FallbackChain[0].Model = fullModel
	} else {
		cfg.FallbackChain = []config.FallbackEntry{
			{Provider: prov, Model: fullModel},
		}
	}
	cfg.DefaultProvider = prov

	fmt.Fprintf(os.Stderr, "Switched to %s/%s (session only)\n", prov, fullModel)
}

// containsString checks if a string exists in a slice.
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// handleKeyCommand handles /key subcommands.
func handleKeyCommand(args []string, cfg *config.Config, lr lineReader) {
	if len(args) == 0 {
		// Default: show active provider's key (backward-compatible).
		provName := activeProvider(cfg)
		if provName == "" {
			fmt.Fprintln(os.Stderr, "No provider configured.")
			return
		}
		pc, ok := cfg.Providers[provName]
		if !ok {
			fmt.Fprintf(os.Stderr, "Provider %q not found in config.\n", provName)
			return
		}
		handleKeyShow(provName, pc)
		return
	}

	if args[0] == "help" {
		printKeyHelp()
		return
	}

	// Parse: /key <provider>, /key set, /key set <provider>
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
		handleKeySet(provName, pc, cfg, lr)
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
func handleKeySet(provName string, pc config.ProviderConfig, cfg *config.Config, lr lineReader) {
	prompt := fmt.Sprintf("New %s API key: ", provName)
	apiKey, err := lr.ReadMasked(prompt)
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
var knownSlashCommands = []string{"/help", "/quit", "/model", "/key", "/config", "/dir", "/clear", "/skills", "/skill", "/use"}

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
