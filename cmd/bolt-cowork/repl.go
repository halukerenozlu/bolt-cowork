package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/halukerenozlu/bolt-cowork/internal/agent"
	"github.com/halukerenozlu/bolt-cowork/internal/config"
	"github.com/halukerenozlu/bolt-cowork/internal/sandbox"
	"github.com/halukerenozlu/bolt-cowork/internal/skill"
	"github.com/halukerenozlu/bolt-cowork/pkg/types"
	"gopkg.in/yaml.v3"
)

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

// initSkillStore creates and loads a skill store from config or defaults.
// It returns the store and any informational warnings from loading.
func initSkillStore(cfg *config.Config) (*skill.Store, []string) {
	store := skill.NewStore()
	var warnings []string
	// Bundled skills are always loaded first; filesystem skills override them.
	if sub, err := fs.Sub(embeddedSkillsFS, "skills"); err == nil {
		if err := store.LoadEmbedded(sub); err != nil {
			warnings = append(warnings, fmt.Sprintf("warning: embedded skill loading error: %v", err))
		}
	}
	skillDirs := cfg.Skills.Dirs
	if len(skillDirs) == 0 {
		workDir := resolveWorkDir(cfg)
		absDir, err := filepath.Abs(workDir)
		if err != nil {
			absDir = workDir
		}
		skillDirs = skillDefaultDirs(absDir)
	}
	warnings = append(warnings, store.LoadAll(skillDirs)...)
	return store, warnings
}

// relOrAbs returns a relative path from the current working directory if
// possible; falls back to absPath when the conversion fails.
func relOrAbs(absPath string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return absPath
	}
	rel, err := filepath.Rel(cwd, absPath)
	if err != nil {
		return absPath
	}
	return rel
}

// handleSlashCommand processes REPL slash commands via the registry.
// Returns true if the REPL should exit.
func handleSlashCommand(input string, registry *CommandRegistry, ctx *CommandContext) bool {
	trimmed := strings.TrimSpace(input)
	parts := strings.Fields(trimmed)
	cmdName := strings.ToLower(parts[0])

	cmd, ok := registry.Get(cmdName)
	if !ok {
		suggestSlashCommand(cmdName, registry.Names())
		return false
	}

	// Build args: for /dir preserve original case; others get raw parts[1:].
	args := parts[1:]

	if err := cmd.Execute(args, ctx); err != nil {
		if errors.Is(err, errExitRequested) {
			return true
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
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
	fmt.Fprintln(os.Stderr, "  /skill create    Create a new custom skill interactively")
	fmt.Fprintln(os.Stderr, "  /use <name>      Activate skill for next command")
}

// isValidSkillName reports whether name is a valid skill identifier:
// starts with a lowercase letter and contains only lowercase letters,
// digits, and hyphens.
func isValidSkillName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if r < 'a' || r > 'z' {
				return false
			}
		} else {
			if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-') {
				return false
			}
		}
	}
	return true
}

// handleSkillCreateCommand runs an interactive prompt sequence that collects
// skill metadata, generates a SKILL.md template, writes it to the appropriate
// directory, and reloads the skill store.
func handleSkillCreateCommand(store *skill.Store, cfg *config.Config, lr lineReader) {
	if store == nil {
		fmt.Fprintln(os.Stderr, "no skill store available")
		return
	}
	if lr == nil {
		fmt.Fprintln(os.Stderr, "interactive input not available")
		return
	}

	fmt.Fprintln(os.Stderr, "Creating a new skill. Press Ctrl+C to cancel.")

	// Name — required, validated.
	rawName, err := lr.ReadLineWithPrompt("Skill name (lowercase letters, hyphens, digits): ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}
	name := strings.TrimSpace(rawName)
	if !isValidSkillName(name) {
		fmt.Fprintln(os.Stderr, "Error: name must start with a lowercase letter and contain only lowercase letters, digits, and hyphens (e.g. my-skill)")
		return
	}

	// Description — required.
	rawDesc, err := lr.ReadLineWithPrompt("Description: ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}
	desc := strings.TrimSpace(rawDesc)
	if desc == "" {
		fmt.Fprintln(os.Stderr, "Error: description is required")
		return
	}

	// Tags — optional, comma-separated.
	rawTags, err := lr.ReadLineWithPrompt("Tags (comma-separated, optional): ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}
	var tags []string
	for _, t := range strings.Split(rawTags, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			tags = append(tags, t)
		}
	}

	// Scope — project (default) or global.
	rawScope, err := lr.ReadLineWithPrompt("Scope [project/global] (default: project): ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}
	scope := strings.ToLower(strings.TrimSpace(rawScope))
	if scope == "" {
		scope = "project"
	}
	if scope != "project" && scope != "global" {
		fmt.Fprintln(os.Stderr, "Error: scope must be 'project' or 'global'")
		return
	}

	// Resolve target path: prefer cfg.Skills.Dirs, fall back to hardcoded defaults.
	home, _ := os.UserHomeDir()
	var targetDir string
	if scope == "global" {
		// Look for a home-prefixed entry in cfg.Skills.Dirs.
		for _, d := range cfg.Skills.Dirs {
			abs, err := filepath.Abs(d)
			if err != nil {
				abs = d
			}
			if home != "" && sandbox.IsUnderDir(home, abs) {
				targetDir = filepath.Join(abs, name)
				break
			}
		}
		if targetDir == "" {
			if home == "" {
				fmt.Fprintln(os.Stderr, "Error: cannot determine home directory")
				return
			}
			targetDir = filepath.Join(home, ".bolt-cowork", "skills", name)
		}
	} else {
		// Look for a non-home-prefixed entry in cfg.Skills.Dirs.
		for _, d := range cfg.Skills.Dirs {
			abs, err := filepath.Abs(d)
			if err != nil {
				abs = d
			}
			if home == "" || !sandbox.IsUnderDir(home, abs) {
				targetDir = filepath.Join(abs, name)
				break
			}
		}
		if targetDir == "" {
			workDir := resolveWorkDir(cfg)
			absWorkDir, err := filepath.Abs(workDir)
			if err != nil {
				absWorkDir = workDir
			}
			targetDir = filepath.Join(absWorkDir, "bolt-skills", name)
		}
	}
	targetFile := filepath.Join(targetDir, "SKILL.md")

	// Warn if file already exists and ask for confirmation.
	if _, err := os.Stat(targetFile); err == nil {
		rawConfirm, err := lr.ReadLineWithPrompt(fmt.Sprintf("File %s already exists. Overwrite? [y/N]: ", relOrAbs(targetFile)))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return
		}
		if strings.ToLower(strings.TrimSpace(rawConfirm)) != "y" {
			fmt.Fprintln(os.Stderr, "Cancelled.")
			return
		}
	}

	// Generate template and write.
	meta := skill.SkillMetadata{
		Name:        name,
		Description: desc,
		Tags:        tags,
	}
	content := skill.GenerateTemplate(meta)

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot create directory: %v\n", err)
		return
	}
	if err := os.WriteFile(targetFile, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot write skill file: %v\n", err)
		return
	}

	fmt.Fprintf(os.Stderr, "Skill %q created: %s\n", name, relOrAbs(targetFile))
	fmt.Fprintln(os.Stderr, "Edit the file to add your instructions, then use /skills to verify it loaded.")

	// Reload skill store so the new skill is immediately available.
	dirs := cfg.Skills.Dirs
	if len(dirs) == 0 {
		workDir := resolveWorkDir(cfg)
		absWorkDir, err2 := filepath.Abs(workDir)
		if err2 != nil {
			absWorkDir = workDir
		}
		dirs = skillDefaultDirs(absWorkDir)
	}
	for _, w := range store.LoadAll(dirs) {
		fmt.Fprintln(os.Stderr, w)
	}
	fmt.Fprintf(os.Stderr, "Skill %q loaded.\n", name)
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
		fmt.Fprintln(os.Stderr, "no skill store available")
		return
	}
	skills := store.GetAll()
	if len(skills) == 0 {
		fmt.Fprintln(os.Stderr, "no skills loaded")
		return
	}
	fmt.Fprintf(os.Stderr, "Loaded skills (%d):\n", len(skills))
	for _, sk := range skills {
		auto := " "
		if sk.Metadata.AutoTrigger {
			auto = "*"
		}
		fmt.Fprintf(os.Stderr, "  %s %-20s [%s] %s\n", auto, sk.Metadata.Name, sk.Scope, sk.Metadata.Description)
	}
	fmt.Fprintln(os.Stderr, "\n  * = auto_trigger enabled")
}

// handleSkillCommand shows details for a specific skill, or lists skill commands if called with no args.
func handleSkillCommand(args []string, store *skill.Store) {
	if store == nil {
		fmt.Fprintln(os.Stderr, "no skill store available")
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
			d := agent.LevenshteinDistance(name, s.Metadata.Name)
			if d < bestDist {
				bestDist = d
				bestName = s.Metadata.Name
			}
		}
		if bestDist <= 2 {
			fmt.Fprintf(os.Stderr, "skill %q not found, did you mean %q?\n", name, bestName)
		} else {
			fmt.Fprintf(os.Stderr, "skill %q not found\n", name)
		}
		return
	}

	fmt.Fprintf(os.Stderr, "Name:         %s\n", sk.Metadata.Name)
	fmt.Fprintf(os.Stderr, "Description:  %s\n", sk.Metadata.Description)
	fmt.Fprintf(os.Stderr, "Scope:        %s\n", sk.Scope)
	fmt.Fprintf(os.Stderr, "AutoTrigger:  %v\n", sk.Metadata.AutoTrigger)
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
	fmt.Fprintf(os.Stderr, "\nUse '/use %s' to activate for next command.\n", sk.Metadata.Name)
}

// handleUseCommand activates a skill for the next command.
func handleUseCommand(args []string, store *skill.Store, forceSkills *[]string) {
	if store == nil {
		fmt.Fprintln(os.Stderr, "no skill store available")
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
			d := agent.LevenshteinDistance(name, s.Metadata.Name)
			if d < bestDist {
				bestDist = d
				bestName = s.Metadata.Name
			}
		}
		if bestDist <= 2 {
			fmt.Fprintf(os.Stderr, "skill %q not found, did you mean %q?\n", name, bestName)
		} else {
			fmt.Fprintf(os.Stderr, "skill %q not found\n", name)
		}
		return
	}
	// Add to forceSkills (avoid duplicates).
	for _, existing := range *forceSkills {
		if existing == sk.Metadata.Name {
			fmt.Fprintf(os.Stderr, "Skill %q already activated.\n", sk.Metadata.Name)
			return
		}
	}
	*forceSkills = append(*forceSkills, sk.Metadata.Name)
	fmt.Fprintf(os.Stderr, "Skill %q activated for next command.\n", sk.Metadata.Name)
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
		fmt.Fprintln(os.Stderr, "config reloaded")
	case "set":
		fmt.Fprintln(os.Stderr, "/config set: not yet implemented, use /key set to change API keys")
	default:
		fmt.Fprintf(os.Stderr, "Unknown subcommand %q. Available:\n  show, path, reload, set\nRun /config for available subcommands.\n", args[0])
	}
}

// showMaskedConfig marshals the config to YAML with API keys masked.
func showMaskedConfig(cfg *config.Config) {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling config: %v\n", err)
		return
	}
	fmt.Fprint(os.Stderr, string(data))

	// API keys are stored in the keyring (yaml:"-"), show status separately.
	for name, pc := range cfg.Providers {
		masked := maskKey(pc.APIKey)
		fmt.Fprintf(os.Stderr, "# %s api_key: %s (keyring)\n", name, masked)
	}
}

// handleDirCommand handles /dir [path|-].
// history and store may be nil (e.g. in tests); previousDir may also be nil.
// trust is called to verify directory access before switching; use checkTrust
// in production and a stub in tests.
func handleDirCommand(args []string, cfg *config.Config, history *[]types.Message, store *skill.Store, previousDir *string, trust func(*config.Config, string) bool) {
	// /dir with no arguments: show current workspace.
	if len(args) == 0 {
		dir := resolveWorkDir(cfg)
		absDir, err := filepath.Abs(dir)
		if err != nil {
			absDir = dir
		}
		rel := relOrAbs(absDir)
		fmt.Fprintf(os.Stderr, "Current workspace: %s (%s)\n", rel, absDir)
		return
	}

	newDir := strings.Join(args, " ")

	// /dir - : go back to previous directory.
	if newDir == "-" {
		if previousDir == nil || *previousDir == "" {
			fmt.Fprintln(os.Stderr, "no previous directory")
			return
		}
		prev := *previousDir

		// Validate previousDir using the same checks as /dir <path>.
		info, err := os.Stat(prev)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				fmt.Fprintf(os.Stderr, "Directory not found: %q\n", prev)
			} else if errors.Is(err, os.ErrPermission) {
				fmt.Fprintf(os.Stderr, "Permission denied: %q\n", prev)
			} else {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			}
			return
		}
		if !info.IsDir() {
			fmt.Fprintf(os.Stderr, "%q is not a directory\n", prev)
			return
		}
		if len(cfg.Sandbox.AllowedDirs) > 0 {
			allowed := false
			for _, ad := range cfg.Sandbox.AllowedDirs {
				absAllowed, err := filepath.Abs(ad)
				if err != nil {
					continue
				}
				rel, err := filepath.Rel(absAllowed, prev)
				if err != nil {
					continue
				}
				if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
					allowed = true
					break
				}
			}
			if !allowed {
				fmt.Fprintln(os.Stderr, "Cannot switch back: previous workspace is no longer in allowed directories")
				return
			}
		}

		if !trust(cfg, prev) {
			fmt.Fprintln(os.Stderr, "Directory not trusted. Staying in current directory.")
			return
		}

		cur := workDirOverride
		if cur == "" {
			cur = resolveWorkDir(cfg)
			absC, err := filepath.Abs(cur)
			if err == nil {
				cur = absC
			}
		}
		workDirOverride = prev
		*previousDir = cur
		if history != nil {
			*history = nil
		}
		if store != nil {
			dirs := skillDefaultDirs(prev)
			for _, w := range store.LoadAll(dirs) {
				fmt.Fprintln(os.Stderr, w)
			}
		}
		fmt.Fprintf(os.Stderr, "Working directory changed to %s\n", relOrAbs(prev))
		return
	}

	// Expand tilde prefix to user home directory.
	if strings.HasPrefix(newDir, "~") {
		home, homeErr := os.UserHomeDir()
		if homeErr == nil {
			if newDir == "~" {
				newDir = home
			} else if strings.HasPrefix(newDir, "~/") || strings.HasPrefix(newDir, `~\`) {
				newDir = filepath.Join(home, newDir[2:])
			}
		}
	}

	// Resolve relative paths against the current workspace, not process cwd.
	var absDir string
	if filepath.IsAbs(newDir) {
		absDir = filepath.Clean(newDir)
	} else {
		currentDir := resolveWorkDir(cfg)
		absCurrentDir, err := filepath.Abs(currentDir)
		if err != nil {
			absCurrentDir = currentDir
		}
		absDir = filepath.Clean(filepath.Join(absCurrentDir, newDir))
	}

	info, err := os.Stat(absDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "Directory not found: %q\n", newDir)
		} else if errors.Is(err, os.ErrPermission) {
			fmt.Fprintf(os.Stderr, "Permission denied: %q\n", newDir)
		} else {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		return
	}
	if !info.IsDir() {
		fmt.Fprintf(os.Stderr, "%q is not a directory\n", newDir)
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

	if !trust(cfg, absDir) {
		fmt.Fprintln(os.Stderr, "Directory not trusted. Staying in current directory.")
		return
	}

	// Record current directory as previous before switching.
	if previousDir != nil {
		cur := workDirOverride
		if cur == "" {
			cur = resolveWorkDir(cfg)
			absC, err := filepath.Abs(cur)
			if err == nil {
				cur = absC
			}
		}
		*previousDir = cur
	}

	workDirOverride = absDir

	if history != nil {
		*history = nil
	}
	if store != nil {
		dirs := skillDefaultDirs(absDir)
		for _, w := range store.LoadAll(dirs) {
			fmt.Fprintln(os.Stderr, w)
		}
	}

	fmt.Fprintf(os.Stderr, "Working directory changed to %s\n", relOrAbs(absDir))
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
		fmt.Fprintf(os.Stderr, "Provider %q is not configured. To add it:\n", prov)
		fmt.Fprintf(os.Stderr, "  1. /key set %-10s  (set API key)\n", prov)
		fmt.Fprintf(os.Stderr, "  2. /model %-12s  (then switch model)\n", input)
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
			fmt.Fprintln(os.Stderr, "no provider configured")
			return
		}
		pc, ok := cfg.Providers[provName]
		if !ok {
			fmt.Fprintf(os.Stderr, "provider %q not found in config\n", provName)
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
		fmt.Fprintln(os.Stderr, "no provider configured")
		return
	}

	// For set operations, validate and auto-create provider if needed.
	if isSet {
		if _, supported := providerModels[provName]; !supported {
			fmt.Fprintf(os.Stderr, "Unknown provider %q. Supported: %s\n", provName, strings.Join(supportedProviderNames(), ", "))
			return
		}
		pc, ok := cfg.Providers[provName]
		var successMsg string
		if !ok {
			if cfg.Providers == nil {
				cfg.Providers = make(map[string]config.ProviderConfig)
			}
			pc = config.ProviderConfig{Models: append([]string(nil), providerModels[provName][:1]...)}
			if preset, exists := config.HostedPresets[provName]; exists {
				pc.Endpoint = preset.Endpoint
			}
			if len(cfg.Providers) == 0 {
				cfg.DefaultProvider = provName
				successMsg = fmt.Sprintf("Provider %q created with default settings. API key set.", provName)
			} else {
				successMsg = fmt.Sprintf("Provider %q added to config and API key set.", provName)
			}
		} else {
			successMsg = fmt.Sprintf("API key updated for %q.", provName)
		}
		handleKeySet(provName, pc, cfg, lr, successMsg)
		return
	}

	pc, ok := cfg.Providers[provName]
	if !ok {
		fmt.Fprintf(os.Stderr, "provider %q not found in config\n", provName)
		return
	}
	handleKeyShow(provName, pc)
}

func supportedProviderNames() []string {
	var names []string
	for _, name := range config.Default().GetProviders() {
		if len(providerModels[name]) > 0 {
			names = append(names, name)
		}
	}
	return names
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
// and saves it to disk. successMsg is printed on successful save.
func handleKeySet(provName string, pc config.ProviderConfig, cfg *config.Config, lr lineReader, successMsg string) {
	prompt := fmt.Sprintf("New %s API key: ", provName)
	apiKey, err := lr.ReadMasked(prompt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading key: %v\n", err)
		return
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "empty key, not changed")
		return
	}

	if err := config.SetAPIKey(provName, apiKey); err != nil {
		fmt.Fprintf(os.Stderr, "Error: could not store key in keyring: %v\n", err)
		return
	}
	pc.APIKey = apiKey
	cfg.Providers[provName] = pc
	cfgPath, err := configFilePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: key updated in session but could not determine config path: %v\n", err)
		return
	}
	if err := config.SaveFile(cfg, cfgPath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: key updated in session but could not save config: %v\n", err)
		return
	}
	fmt.Fprintln(os.Stderr, successMsg)
}

// modeAliases maps /mode short names to internal approval mode strings.
var modeAliases = map[string]string{
	"plan":   "plan-only",
	"build":  "dangerous-only",
	"strict": "full",
	"none":   "none",
}

// modeDescriptions maps internal approval modes to human-readable descriptions.
var modeDescriptions = map[string]string{
	"plan-only":      "approve plans before execution",
	"dangerous-only": "auto-approve safe actions, prompt for dangerous ones",
	"full":           "approve every step",
	"none":           "no approval prompts",
}

// handleModeCommand shows or changes the current approval mode.
func handleModeCommand(args []string, cfg *config.Config) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Current mode: %s\n", cfg.ApprovalMode)
		return
	}

	alias := strings.ToLower(args[0])
	mode, ok := modeAliases[alias]
	if !ok {
		fmt.Fprintf(os.Stderr, "Unknown mode %q. Available: plan, build, strict, none\n", alias)
		return
	}

	cfg.ApprovalMode = mode
	fmt.Fprintf(os.Stderr, "Mode changed to '%s' (%s)\n", alias, modeDescriptions[mode])
}

// maskKey returns "***...last8" for keys longer than 8 chars,
// or "***" for shorter keys.
func maskKey(key string) string {
	if len(key) <= 8 {
		return "***"
	}
	return "***..." + key[len(key)-8:]
}

// suggestSlashCommand prints an "Unknown command" message. If a known command
// is within Levenshtein distance <= 2, it suggests it with "Did you mean ...?".
func suggestSlashCommand(cmd string, known []string) {
	bestDist := 3 // threshold + 1
	bestCmd := ""
	for _, k := range known {
		d := agent.LevenshteinDistance(cmd, k)
		if d < bestDist {
			bestDist = d
			bestCmd = k
		}
	}
	if bestDist <= 2 {
		fmt.Fprintf(os.Stderr, "unknown command '%s', did you mean '%s'?\n", cmd, bestCmd)
	} else {
		fmt.Fprintln(os.Stderr, "unknown command, type /help for available commands")
	}
}
