package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

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
	ActionCopy   StepAction = "copy"
	ActionMkdir  StepAction = "mkdir"
)

// Step is a single operation in a plan.
type Step struct {
	Action      StepAction `json:"action"`
	Description string     `json:"description"`
	Path        string     `json:"path"`
	Destination string     `json:"destination,omitempty"`
	Content     string     `json:"content,omitempty"`
	Recursive   bool       `json:"recursive,omitempty"`
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
      "action": "read|write|delete|move|rename|list|copy|mkdir",
      "description": "what this step does",
      "path": "target file path",
      "destination": "for move/rename/copy only",
      "content": "for write only",
      "recursive": false
    }
  ]
}
Actions:
- read: read a file
- write: create or overwrite a file (requires content)
- delete: delete a file or directory. Rules for "recursive" field:
  * To delete a single file: set recursive: false
  * To delete an empty directory: set recursive: false
  * To delete a directory with all its contents: set recursive: true
  * DEFAULT: set recursive: false unless the user explicitly says "with contents", "recursively", "with everything inside", "icindekilerle birlikte", or similar. The system will safely return an error if the directory is non-empty, which is the safe default.
- move: move a file (requires destination)
- rename: rename a file (requires destination)
- list: list directory contents
- copy: copy a file (requires destination; if destination is an existing directory, copy into it)
- mkdir: create a directory (and all parent directories)

IMPORTANT:
- Respond ONLY with valid JSON. Do not add any other text, explanation, or markdown formatting. Your entire response must be a single JSON object and nothing else.
- All paths must be relative to the working directory shown in the listing. Do NOT repeat the working directory name as a prefix. For example, use "file.txt" or "sub/file.txt", NOT "workspace/file.txt" when the working directory is "workspace".
- If the command asks to delete/remove something, include at least one "delete" step (do not replace it with "list").
- If the command asks for directory contents (e.g. "icerigi", "contents"), operate on entries inside that directory, not on the directory itself.
- If the user is asking about previous conversation or history (e.g. "what did I ask?", "what did we do?", "what was the result?", "ne sordum?", "az önce ne yaptık?", "önceki komut ne idi?"), do NOT create a file operation plan. Instead, respond with a JSON where description answers the question based on the conversation history, and steps is an empty array: {"description": "Your answer here based on conversation history.", "steps": []}`
// CreatePlan sends the user command to the LLM and returns a parsed plan.
// history contains previous user/assistant messages for multi-turn context.
func (p *Planner) CreatePlan(ctx context.Context, command string, dirListing string, history []types.Message) (*Plan, error) {
	userMsg := fmt.Sprintf("Command: %s\n\nDirectory contents:\n%s", command, dirListing)

	messages := []types.Message{
		{Role: types.RoleSystem, Content: systemPrompt},
	}
	// Inject conversation history (user/assistant only) for multi-turn context.
	for _, m := range history {
		if m.Role == types.RoleUser || m.Role == types.RoleAssistant {
			messages = append(messages, m)
		}
	}
	messages = append(messages, types.Message{Role: types.RoleUser, Content: userMsg})

	resp, err := p.chain.Chat(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("agent: planner chat: %w", err)
	}

	resp = cleanJSONResponse(resp)

	var plan Plan
	if err := json.Unmarshal([]byte(resp), &plan); err != nil {
		extracted, ok := extractJSON(resp)
		if !ok {
			return nil, fmt.Errorf("agent: parse plan JSON: %w\nraw response: %s", err, sanitizeForLog(resp))
		}
		if err2 := json.Unmarshal([]byte(extracted), &plan); err2 != nil {
			return nil, fmt.Errorf("agent: parse plan JSON after extraction: %w\nraw response: %s", err2, sanitizeForLog(resp))
		}
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

// extractJSON attempts to pull a JSON object from a string that contains
// extra text around it. It first tries to find a ```json fenced block,
// then falls back to extracting between the first { and the last }.
func extractJSON(s string) (string, bool) {
	// Try ```json ... ``` fenced block first.
	if start := strings.Index(s, "```json"); start != -1 {
		inner := s[start+len("```json"):]
		if end := strings.Index(inner, "```"); end != -1 {
			candidate := strings.TrimSpace(inner[:end])
			if len(candidate) > 0 {
				return candidate, true
			}
		}
	}

	// Try ``` ... ``` fenced block (without json tag).
	if start := strings.Index(s, "```"); start != -1 {
		inner := s[start+len("```"):]
		if end := strings.Index(inner, "```"); end != -1 {
			candidate := strings.TrimSpace(inner[:end])
			if len(candidate) > 0 && candidate[0] == '{' {
				return candidate, true
			}
		}
	}

	// Fallback: first { to last }.
	first := strings.Index(s, "{")
	last := strings.LastIndex(s, "}")
	if first != -1 && last > first {
		return s[first : last+1], true
	}

	return "", false
}

const maxLogLen = 200

// sanitizeForLog truncates s to maxLogLen characters and replaces control
// characters with spaces to prevent log injection or data leakage.
func sanitizeForLog(s string) string {
	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' {
			return ' '
		}
		return r
	}, s)
	if len(cleaned) > maxLogLen {
		return cleaned[:maxLogLen] + "...[truncated]"
	}
	return cleaned
}
