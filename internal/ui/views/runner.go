package views

import (
	"context"

	"github.com/halukerenozlu/bolt-cowork/pkg/types"
)

// AgentResult is returned by AgentRunner.Run after a single command completes.
type AgentResult struct {
	History []types.Message
	Err     error
}

// AgentRunner wires the TUI session to the underlying agent.
// Constructed in main.go and threaded through App → Session.
type AgentRunner struct {
	// Run executes cmd. It calls onChunk with text as it becomes available.
	// For the current non-streaming implementation onChunk is called once
	// with the full response text; future versions may call it with smaller
	// chunks to enable true streaming.
	// Run must be safe to call from a goroutine.
	Run func(ctx context.Context, cmd string, history []types.Message, onChunk func(string)) AgentResult

	Provider  string // e.g. "anthropic"
	Model     string // e.g. "claude-opus-4-5"
	Workspace string // absolute workspace path
}
