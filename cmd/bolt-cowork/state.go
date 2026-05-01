package main

import (
	"path/filepath"
	"sync"

	"github.com/halukerenozlu/bolt-cowork/internal/agent"
	"github.com/halukerenozlu/bolt-cowork/internal/config"
	"github.com/halukerenozlu/bolt-cowork/internal/mcp"
	"github.com/halukerenozlu/bolt-cowork/internal/skill"
	"github.com/halukerenozlu/bolt-cowork/internal/tool"
	"github.com/halukerenozlu/bolt-cowork/pkg/types"
)

// AppState holds all runtime state for a bolt-cowork session.
type AppState struct {
	mu           sync.RWMutex
	Cfg          *config.Config
	Messages     []types.Message
	ForceSkills  []string
	ToolRegistry *tool.Registry
	MCPRegistry  *mcp.Registry
	CmdRegistry  *CommandRegistry
	SkillStore   *skill.Store
	Redactor     *agent.Redactor
	WorkDir      string
	PreviousDir  string
	ApprovalMode string
	Version      string
	LineReader   lineReader
}

// NewAppState creates and initializes an AppState from the given config.
// It sets up the command registry, skill store, and resolves the working
// directory. ToolRegistry and MCPRegistry are initialized empty.
// LineReader must be set separately after readline initialization.
func NewAppState(cfg *config.Config, ver string) *AppState {
	cmdReg := NewCommandRegistry()
	RegisterDefaultCommands(cmdReg)

	store := initSkillStore(cfg)

	// Collect API key secrets for redaction.
	var secrets []string
	for _, pc := range cfg.Providers {
		if pc.APIKey != "" {
			secrets = append(secrets, pc.APIKey)
		}
	}
	redactor := agent.NewRedactor(secrets)

	workDir := resolveWorkDir(cfg)
	absDir, err := filepath.Abs(workDir)
	if err != nil {
		absDir = workDir
	}

	return &AppState{
		Cfg:          cfg,
		ToolRegistry: tool.NewRegistry(),
		MCPRegistry:  mcp.NewRegistry(),
		CmdRegistry:  cmdReg,
		SkillStore:   store,
		Redactor:     redactor,
		WorkDir:      absDir,
		ApprovalMode: cfg.ApprovalMode,
		Version:      ver,
	}
}

// ClearHistory removes all conversation messages.
func (s *AppState) ClearHistory() {
	s.mu.Lock()
	s.Messages = nil
	s.mu.Unlock()
}

// AddMessage appends a message to the conversation history.
func (s *AppState) AddMessage(msg types.Message) {
	s.mu.Lock()
	s.Messages = append(s.Messages, msg)
	s.mu.Unlock()
}

// History returns a copy of the current conversation messages.
func (s *AppState) History() []types.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]types.Message, len(s.Messages))
	copy(out, s.Messages)
	return out
}

// SetWorkDir updates the working directory, saving the current one as
// PreviousDir. It also syncs the package-level workDirOverride so that
// resolveWorkDir() and handleDirCommand() continue to work.
func (s *AppState) SetWorkDir(dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.WorkDir != "" {
		s.PreviousDir = s.WorkDir
	}
	s.WorkDir = dir
	workDirOverride = dir
}

// CommandContext builds a CommandContext from the current state.
// The returned context uses pointers into AppState fields so that
// command handlers can mutate them directly (e.g. clearing history).
func (s *AppState) CommandContext() *CommandContext {
	return &CommandContext{
		Cfg:         s.Cfg,
		History:     &s.Messages,
		Store:       s.SkillStore,
		ForceSkills: &s.ForceSkills,
		PreviousDir: &s.PreviousDir,
		LineReader:  s.LineReader,
		State:       s,
	}
}
