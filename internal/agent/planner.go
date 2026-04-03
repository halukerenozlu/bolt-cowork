package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/halukerenozlu/bolt-cowork/internal/provider"
	"github.com/halukerenozlu/bolt-cowork/pkg/types"
)

// StepAction represents the type of file operation.
type StepAction string

const (
	ActionRead   StepAction = "read"
	ActionWrite  StepAction = "write"
	ActionDelete StepAction = "delete"
	ActionMove   StepAction = "move"
	ActionRename StepAction = "rename"
	ActionList   StepAction = "list"
)

// Step is a single operation in a plan.
type Step struct {
	Action      StepAction `json:"action"`
	Description string     `json:"description"`
	Path        string     `json:"path"`
	Destination string     `json:"destination,omitempty"`
	Content     string     `json:"content,omitempty"`
}

// Plan is an ordered list of steps created by the LLM.
type Plan struct {
	Description string `json:"description"`
	Steps       []Step `json:"steps"`
}

// Planner creates execution plans from user commands via the LLM.
type Planner struct {
	chain *provider.FallbackChain
}

// NewPlanner creates a Planner backed by the given fallback chain.
func NewPlanner(chain *provider.FallbackChain) *Planner {
	return &Planner{chain: chain}
}

const systemPrompt = `You are a file operations planner. Given a user command and a directory listing, create a plan as a JSON object with this structure:
{
  "description": "brief plan summary",
  "steps": [
    {
      "action": "read|write|delete|move|rename|list",
      "description": "what this step does",
      "path": "target file path",
      "destination": "for move/rename only",
      "content": "for write only"
    }
  ]
}
Actions: read, write, delete, move, rename, list.
Return ONLY valid JSON, no markdown fences or extra text.`

// CreatePlan sends the user command to the LLM and returns a parsed plan.
func (p *Planner) CreatePlan(ctx context.Context, command string, dirListing string) (*Plan, error) {
	userMsg := fmt.Sprintf("Command: %s\n\nDirectory contents:\n%s", command, dirListing)

	messages := []types.Message{
		{Role: types.RoleSystem, Content: systemPrompt},
		{Role: types.RoleUser, Content: userMsg},
	}

	resp, err := p.chain.Chat(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("agent: planner chat: %w", err)
	}

	resp = cleanJSONResponse(resp)

	var plan Plan
	if err := json.Unmarshal([]byte(resp), &plan); err != nil {
		return nil, fmt.Errorf("agent: parse plan JSON: %w", err)
	}

	return &plan, nil
}

// cleanJSONResponse strips markdown fences and whitespace from LLM output.
func cleanJSONResponse(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
