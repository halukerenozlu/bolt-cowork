package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/halukerenozlu/bolt-cowork/internal/mcp"
	"github.com/halukerenozlu/bolt-cowork/internal/prompt"
	"github.com/halukerenozlu/bolt-cowork/internal/provider"
	"github.com/halukerenozlu/bolt-cowork/internal/skill"
	"github.com/halukerenozlu/bolt-cowork/pkg/types"
)

// StepAction represents the type of file operation.
type StepAction string

const (
	ActionRead        StepAction = "read"
	ActionWrite       StepAction = "write"
	ActionDelete      StepAction = "delete"
	ActionMove        StepAction = "move"
	ActionRename      StepAction = "rename"
	ActionList        StepAction = "list"
	ActionCopy        StepAction = "copy"
	ActionMkdir       StepAction = "mkdir"
	ActionCallMCPTool     StepAction = "call_mcp_tool"
	ActionReadMCPResource StepAction = "read_mcp_resource"
)

// Step is a single operation in a plan.
type Step struct {
	Action      StepAction     `json:"action"`
	Description string         `json:"description"`
	Path        string         `json:"path"`
	Destination string         `json:"destination,omitempty"`
	Content     string         `json:"content,omitempty"`
	Recursive   bool           `json:"recursive,omitempty"`
	ServerName  string         `json:"server_name,omitempty"`
	ToolName    string         `json:"tool_name,omitempty"`
	Args        map[string]any `json:"args,omitempty"`
	ResourceURI string         `json:"resource_uri,omitempty"`
}

// Plan is an ordered list of steps created by the LLM.
type Plan struct {
	Description string `json:"description"`
	Steps       []Step `json:"steps"`
}

// MCPToolSchema carries the full schema for a single MCP tool, including the
// parameter descriptions injected into the planner system prompt so the LLM
// can produce correct call_mcp_tool steps with the right argument names.
type MCPToolSchema struct {
	ServerName  string
	ToolName    string
	Description string
	InputSchema map[string]any // parameter name → property definition
	Required    []string       // names of required parameters
}

type renderedMCPToolSchema struct {
	Server      string         `json:"server"`
	Tool        string         `json:"tool"`
	Description string         `json:"description,omitempty"`
	Required    []string       `json:"required,omitempty"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}

// Planner creates execution plans from user commands via the LLM.
type Planner struct {
	chain          *provider.FallbackChain
	MCPTools       []string
	MCPToolSchemas []MCPToolSchema
}

// NewPlanner creates a Planner backed by the given fallback chain.
func NewPlanner(chain *provider.FallbackChain) *Planner {
	return &Planner{chain: chain}
}

// SetMCPToolSchemas configures the detailed tool schemas injected into the
// system prompt. Tool names are also merged into the MCPTools name list
// automatically, so callers do not need to call both setters.
func (p *Planner) SetMCPToolSchemas(tools []MCPToolSchema) {
	p.MCPToolSchemas = append([]MCPToolSchema(nil), tools...)
}

const systemPrompt = `You are a file operations planner. Given a user command and a directory listing, create a plan as a JSON object with this structure:
{
  "description": "brief plan summary",
  "steps": [
    {
      "action": "read|write|delete|move|rename|list|copy|mkdir|call_mcp_tool|read_mcp_resource",
      "description": "what this step does",
      "path": "target file path",
      "destination": "for move/rename/copy only",
      "content": "for write only",
      "recursive": false,
      "server_name": "for call_mcp_tool or read_mcp_resource only",
      "tool_name": "for call_mcp_tool only",
      "args": {"for": "call_mcp_tool only"},
      "resource_uri": "for read_mcp_resource only"
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
- call_mcp_tool: call an MCP tool. Use this action when the user task requires an MCP tool. Set server_name, tool_name, and args. Existing MCP tools: {{.MCPTools}} (server_name/tool_name format).
- read_mcp_resource: read a resource from an MCP server. Set server_name and resource_uri.
{{.MCPToolSchemas}}
IMPORTANT:
- Respond ONLY with valid JSON. Do not add any other text, explanation, or markdown formatting. Your entire response must be a single JSON object and nothing else.
- All paths must be relative to the working directory shown in the listing. Do NOT repeat the working directory name as a prefix. For example, use "file.txt" or "sub/file.txt", NOT "workspace/file.txt" when the working directory is "workspace".
- If the command asks to delete/remove something, include at least one "delete" step (do not replace it with "list").
- If the command asks for directory contents (e.g. "icerigi", "contents"), operate on entries inside that directory, not on the directory itself.
- If the user is asking about previous conversation or history (e.g. "what did I ask?", "what did we do?", "what was the result?", "ne sordum?", "az önce ne yaptık?", "önceki komut ne idi?"), do NOT create a file operation plan. Instead, respond with a JSON where description answers the question based on the conversation history, and steps is an empty array: {"description": "Your answer here based on conversation history.", "steps": []}`

// CreatePlan sends the user command to the LLM and returns a parsed plan.
// history contains previous user/assistant messages for multi-turn context.
// matchedSkills are injected into the system prompt as active skill context.
func (p *Planner) CreatePlan(ctx context.Context, command string, dirListing string, history []types.Message, matchedSkills []skill.Skill) (*Plan, error) {
	userMsg := fmt.Sprintf("Command: %s\n\nDirectory contents:\n%s", command, dirListing)

	toolNames := collectToolNames(p.MCPTools, p.MCPToolSchemas)
	mcpTools := "none"
	if len(toolNames) > 0 {
		mcpTools = strings.Join(toolNames, ", ")
	}
	basePrompt := strings.ReplaceAll(systemPrompt, "{{.MCPTools}}", mcpTools)
	basePrompt = strings.ReplaceAll(basePrompt, "{{.MCPToolSchemas}}", renderMCPToolSchemas(p.MCPToolSchemas))
	builder := prompt.NewPromptBuilder(basePrompt)
	sysPrompt := builder.Build(prompt.BuildOptions{
		Skills: prompt.SkillContextsFromStore(matchedSkills),
	})
	messages := []types.Message{
		{Role: types.RoleSystem, Content: sysPrompt},
	}
	// Inject conversation history (user/assistant only) for multi-turn context.
	for _, m := range history {
		if m.Role == types.RoleUser || m.Role == types.RoleAssistant {
			messages = append(messages, m)
		}
	}
	messages = append(messages, types.Message{Role: types.RoleUser, Content: userMsg})

	resp, err := p.chain.Chat(ctx, messages, nil)
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

// collectToolNames merges the explicit MCPTools name slice with names derived
// from MCPToolSchemas, deduplicating the result. Explicit names come first.
func collectToolNames(explicit []string, schemas []MCPToolSchema) []string {
	seen := make(map[string]bool, len(explicit)+len(schemas))
	out := make([]string, 0, len(explicit)+len(schemas))
	for _, n := range explicit {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	for _, s := range schemas {
		n := s.ServerName + "/" + s.ToolName
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}

// renderMCPToolSchemas returns the MCP schema section to inject into the
// system prompt. Returns an empty string when no schemas are configured so
// that the prompt does not grow unnecessarily.
func renderMCPToolSchemas(schemas []MCPToolSchema) string {
	if len(schemas) == 0 {
		return ""
	}
	rendered := make([]renderedMCPToolSchema, 0, len(schemas))
	for _, s := range schemas {
		item := renderedMCPToolSchema{
			Server:      sanitizePromptMetadata(s.ServerName),
			Tool:        sanitizePromptMetadata(s.ToolName),
			Description: sanitizePromptMetadata(s.Description),
			Required:    sanitizeStringSlice(s.Required),
		}
		if len(s.InputSchema) > 0 {
			item.InputSchema = make(map[string]any, len(s.InputSchema))
			for name, prop := range s.InputSchema {
				item.InputSchema[sanitizePromptMetadata(name)] = prop
			}
		}
		rendered = append(rendered, item)
	}

	raw, err := json.MarshalIndent(rendered, "", "  ")
	if err != nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("### MCP Tool Schemas (untrusted tool metadata)\n")
	sb.WriteString("```json\n")
	sb.Write(raw)
	sb.WriteString("\n```\n")
	return sb.String()
}

func sanitizePromptMetadata(s string) string {
	replacer := strings.NewReplacer("\n", " ", "\r", " ", "\t", " ")
	return strings.TrimSpace(replacer.Replace(s))
}

func sanitizeStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, sanitizePromptMetadata(s))
	}
	return out
}

// ToolRegistryToSchemas converts all tools in reg into MCPToolSchema values
// for injection into the planner system prompt via SetMCPToolSchemas.
func ToolRegistryToSchemas(reg *mcp.ToolRegistry) []MCPToolSchema {
	if reg == nil {
		return nil
	}
	all := reg.ListAll()
	var schemas []MCPToolSchema
	for serverName, tools := range all {
		for _, t := range tools {
			s := MCPToolSchema{
				ServerName:  serverName,
				ToolName:    t.Name,
				Description: t.Description,
				Required:    append([]string(nil), t.InputSchema.Required...),
			}
			if len(t.InputSchema.Properties) > 0 {
				s.InputSchema = make(map[string]any, len(t.InputSchema.Properties))
				for k, v := range t.InputSchema.Properties {
					s.InputSchema[k] = v
				}
			}
			schemas = append(schemas, s)
		}
	}
	return schemas
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
