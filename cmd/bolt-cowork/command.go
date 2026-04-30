package main

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/halukerenozlu/bolt-cowork/internal/config"
	"github.com/halukerenozlu/bolt-cowork/internal/skill"
	"github.com/halukerenozlu/bolt-cowork/pkg/types"
)

// errExitRequested is returned by a command's Execute to signal REPL exit.
var errExitRequested = errors.New("exit requested")

// CommandContext holds the shared state available to all slash commands.
type CommandContext struct {
	Cfg         *config.Config
	History     *[]types.Message
	Store       *skill.Store
	ForceSkills *[]string
	PreviousDir *string
	LineReader  lineReader
	State       *AppState // may be nil in tests or single-command mode
}

// SlashCommand describes a single slash command.
type SlashCommand struct {
	Name        string
	Description string
	Usage       string // e.g. "/mode [plan|build|strict|none]"
	Category    string // "General", "Config", "Skills", "Provider & Model", "Workspace"
	Hidden      bool
	Execute     func(args []string, ctx *CommandContext) error
}

// CommandRegistry is the central store for all slash commands.
type CommandRegistry struct {
	commands map[string]*SlashCommand
}

// NewCommandRegistry creates an empty registry.
func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{commands: make(map[string]*SlashCommand)}
}

// Register adds a command to the registry.
func (r *CommandRegistry) Register(cmd *SlashCommand) {
	r.commands[cmd.Name] = cmd
}

// Get returns the command for the given name, or false if not found.
func (r *CommandRegistry) Get(name string) (*SlashCommand, bool) {
	cmd, ok := r.commands[name]
	return cmd, ok
}

// All returns all registered commands sorted by name.
func (r *CommandRegistry) All() []*SlashCommand {
	cmds := make([]*SlashCommand, 0, len(r.commands))
	for _, cmd := range r.commands {
		cmds = append(cmds, cmd)
	}
	sort.Slice(cmds, func(i, j int) bool { return cmds[i].Name < cmds[j].Name })
	return cmds
}

// ByCategory returns commands grouped by category, sorted by name within each group.
func (r *CommandRegistry) ByCategory() map[string][]*SlashCommand {
	cats := make(map[string][]*SlashCommand)
	for _, cmd := range r.commands {
		cats[cmd.Category] = append(cats[cmd.Category], cmd)
	}
	for cat := range cats {
		sort.Slice(cats[cat], func(i, j int) bool { return cats[cat][i].Name < cats[cat][j].Name })
	}
	return cats
}

// Names returns a sorted list of all command names (for tab completion).
func (r *CommandRegistry) Names() []string {
	names := make([]string, 0, len(r.commands))
	for name := range r.commands {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// categoryOrder defines the display order for /help output.
var categoryOrder = []string{"General", "Config", "Skills", "Provider & Model", "Workspace"}

// RegisterDefaultCommands registers all built-in slash commands.
func RegisterDefaultCommands(r *CommandRegistry) {
	r.Register(&SlashCommand{
		Name:        "/help",
		Description: "Show available commands",
		Usage:       "/help",
		Category:    "General",
		Execute: func(args []string, ctx *CommandContext) error {
			printAutoHelp(r)
			return nil
		},
	})

	r.Register(&SlashCommand{
		Name:        "/clear",
		Description: "Clear conversation history",
		Usage:       "/clear",
		Category:    "General",
		Execute: func(args []string, ctx *CommandContext) error {
			*ctx.History = nil
			fmt.Fprintln(os.Stderr, "Conversation history cleared.")
			return nil
		},
	})

	r.Register(&SlashCommand{
		Name:        "/quit",
		Description: "Exit bolt-cowork",
		Usage:       "/quit",
		Category:    "General",
		Execute: func(args []string, ctx *CommandContext) error {
			fmt.Fprintln(os.Stderr, "Goodbye.")
			return errExitRequested
		},
	})

	r.Register(&SlashCommand{
		Name:        "/config",
		Description: "Show or manage configuration",
		Usage:       "/config [show|path|reload|set|help]",
		Category:    "Config",
		Execute: func(args []string, ctx *CommandContext) error {
			handleConfigCommand(lowerArgs(args), ctx.Cfg)
			return nil
		},
	})

	r.Register(&SlashCommand{
		Name:        "/mode",
		Description: "Set approval mode",
		Usage:       "/mode [plan|build|strict|none]",
		Category:    "Config",
		Execute: func(args []string, ctx *CommandContext) error {
			handleModeCommand(lowerArgs(args), ctx.Cfg)
			if ctx.State != nil {
				ctx.State.mu.Lock()
				ctx.State.ApprovalMode = ctx.Cfg.ApprovalMode
				ctx.State.mu.Unlock()
			}
			return nil
		},
	})

	r.Register(&SlashCommand{
		Name:        "/skills",
		Description: "List all loaded skills",
		Usage:       "/skills",
		Category:    "Skills",
		Execute: func(args []string, ctx *CommandContext) error {
			handleSkillsCommand(ctx.Store)
			return nil
		},
	})

	r.Register(&SlashCommand{
		Name:        "/skill",
		Description: "Show skill details",
		Usage:       "/skill <name>",
		Category:    "Skills",
		Execute: func(args []string, ctx *CommandContext) error {
			handleSkillCommand(args, ctx.Store)
			return nil
		},
	})

	r.Register(&SlashCommand{
		Name:        "/use",
		Description: "Activate skill for next command",
		Usage:       "/use <name>",
		Category:    "Skills",
		Execute: func(args []string, ctx *CommandContext) error {
			handleUseCommand(args, ctx.Store, ctx.ForceSkills)
			return nil
		},
	})

	r.Register(&SlashCommand{
		Name:        "/model",
		Description: "Show or switch model",
		Usage:       "/model [<name>]",
		Category:    "Provider & Model",
		Execute: func(args []string, ctx *CommandContext) error {
			handleModelCommand(lowerArgs(args), ctx.Cfg)
			return nil
		},
	})

	r.Register(&SlashCommand{
		Name:        "/key",
		Description: "Show or set API key",
		Usage:       "/key [set] [<provider>]",
		Category:    "Provider & Model",
		Execute: func(args []string, ctx *CommandContext) error {
			handleKeyCommand(lowerArgs(args), ctx.Cfg, ctx.LineReader)
			return nil
		},
	})

	r.Register(&SlashCommand{
		Name:        "/dir",
		Description: "Show or change workspace directory",
		Usage:       "/dir [path|-]",
		Category:    "Workspace",
		Execute: func(args []string, ctx *CommandContext) error {
			handleDirCommand(args, ctx.Cfg, ctx.History, ctx.Store, ctx.PreviousDir)
			// Sync workDirOverride back to AppState so state.WorkDir
			// stays consistent after handleDirCommand mutates the global.
			if ctx.State != nil && workDirOverride != "" {
				ctx.State.mu.Lock()
				ctx.State.WorkDir = workDirOverride
				ctx.State.mu.Unlock()
			}
			return nil
		},
	})

	r.Register(&SlashCommand{
		Name:        "/init",
		Description: "Initialize .cowork/ in working directory",
		Usage:       "/init [force]",
		Category:    "Workspace",
		Execute: func(args []string, ctx *CommandContext) error {
			if len(args) > 1 || (len(args) == 1 && strings.ToLower(args[0]) != "force") {
				fmt.Fprintln(os.Stderr, "Usage: /init [force]")
				return nil
			}
			force := len(args) == 1
			workDir := resolveWorkDir(ctx.Cfg)
			if err := initProject(workDir, force); err != nil {
				if errors.Is(err, errAlreadyInitialized) {
					fmt.Fprintln(os.Stderr, "Already initialized. Use /init force to reinitialize.")
				} else {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				}
			}
			return nil
		},
	})
}

// printAutoHelp generates /help output from the registry.
func printAutoHelp(r *CommandRegistry) {
	cats := r.ByCategory()
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr)

	for _, cat := range categoryOrder {
		cmds, ok := cats[cat]
		if !ok {
			continue
		}
		fmt.Fprintf(os.Stderr, "  %s:\n", cat)
		for _, cmd := range cmds {
			if cmd.Hidden {
				continue
			}
			fmt.Fprintf(os.Stderr, "    %-22s %s\n", cmd.Usage, cmd.Description)
		}
		fmt.Fprintln(os.Stderr)
	}

	fmt.Fprintln(os.Stderr, "Type any other text to send a command to the agent.")
}
