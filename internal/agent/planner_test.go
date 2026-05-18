package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/halukerenozlu/bolt-cowork/internal/mcp"
	"github.com/halukerenozlu/bolt-cowork/internal/provider"
)

// --- MCPToolSchema system prompt injection tests ---

// TestPlanner_MCPToolSchemas_InSystemPrompt verifies that tool schemas set via
// SetMCPToolSchemas appear in the system prompt sent to the LLM: tool names,
// descriptions, parameter names, and required fields must all be present.
func TestPlanner_MCPToolSchemas_InSystemPrompt(t *testing.T) {
	llm := &mockLLMProvider{name: "mock", available: true, response: makePlanJSON(nil)}
	chain := provider.NewFallbackChain([]provider.LLMProvider{llm})
	planner := NewPlanner(chain)

	planner.SetMCPToolSchemas([]MCPToolSchema{
		{
			ServerName:  "weather",
			ToolName:    "get_forecast",
			Description: "Returns a weather forecast for a city",
			InputSchema: map[string]any{
				"city": mcp.ToolProperty{Type: "string", Description: "City name"},
			},
			Required: []string{"city"},
		},
		{
			ServerName:  "calc",
			ToolName:    "add",
			Description: "Adds two numbers",
			InputSchema: map[string]any{
				"a": mcp.ToolProperty{Type: "number"},
				"b": mcp.ToolProperty{Type: "number"},
			},
			Required: []string{"a", "b"},
		},
	})

	if _, err := planner.CreatePlan(context.Background(), "get weather", ".", nil, nil); err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	if len(llm.messages) == 0 {
		t.Fatal("LLM received no messages")
	}
	system := llm.messages[0].Content

	// Both tools must appear in the MCPTools name list (auto-merged from schemas).
	if !strings.Contains(system, "weather/get_forecast") {
		t.Errorf("system prompt missing weather/get_forecast in tool name list")
	}
	if !strings.Contains(system, "calc/add") {
		t.Errorf("system prompt missing calc/add in tool name list")
	}

	// Schema section must include descriptions.
	if !strings.Contains(system, "Returns a weather forecast for a city") {
		t.Errorf("system prompt missing weather tool description")
	}
	if !strings.Contains(system, "Adds two numbers") {
		t.Errorf("system prompt missing calc tool description")
	}

	// Parameter names must appear.
	if !strings.Contains(system, "city") {
		t.Errorf("system prompt missing parameter 'city'")
	}

	// Required fields must appear in the JSON schema block.
	if !strings.Contains(system, `"required"`) {
		t.Errorf("system prompt missing required field")
	}
}

// TestPlanner_MCPToolSchemas_Empty verifies that when no schemas are set the
// system prompt does not contain the schema section header, keeping the prompt
// lean.
func TestPlanner_MCPToolSchemas_Empty(t *testing.T) {
	llm := &mockLLMProvider{name: "mock", available: true, response: makePlanJSON(nil)}
	chain := provider.NewFallbackChain([]provider.LLMProvider{llm})
	planner := NewPlanner(chain)
	// No SetMCPToolSchemas call.

	if _, err := planner.CreatePlan(context.Background(), "list files", ".", nil, nil); err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	if len(llm.messages) == 0 {
		t.Fatal("LLM received no messages")
	}
	system := llm.messages[0].Content

	if strings.Contains(system, "MCP Tool Schemas") {
		t.Error("system prompt should not contain schema section when no schemas are set")
	}
}

// TestPlanner_MCPToolSchemas_NamesAutoMerged verifies that tool names derived
// from schemas appear in the {{.MCPTools}} list even when SetMCPTools was
// never called explicitly.
func TestPlanner_MCPToolSchemas_NamesAutoMerged(t *testing.T) {
	llm := &mockLLMProvider{name: "mock", available: true, response: makePlanJSON(nil)}
	chain := provider.NewFallbackChain([]provider.LLMProvider{llm})
	planner := NewPlanner(chain)

	// Only set schemas — no explicit MCPTools.
	planner.SetMCPToolSchemas([]MCPToolSchema{
		{ServerName: "srv", ToolName: "tool"},
	})

	if _, err := planner.CreatePlan(context.Background(), "use tool", ".", nil, nil); err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	system := llm.messages[0].Content

	// "srv/tool" must appear in the tool name section of the prompt.
	if !strings.Contains(system, "srv/tool") {
		t.Errorf("system prompt missing srv/tool: %q", system[:min(200, len(system))])
	}
}

func TestRenderMCPToolSchemas_Sanitization(t *testing.T) {
	rendered := renderMCPToolSchemas([]MCPToolSchema{
		{
			ServerName:  "srv",
			ToolName:    "tool",
			Description: "line one\nline two\rline three",
			InputSchema: map[string]any{
				"bad\nname": mcp.ToolProperty{Type: "string"},
			},
			Required: []string{"bad\nname"},
		},
	})

	const fence = "```json\n"
	start := strings.Index(rendered, fence)
	if start == -1 {
		t.Fatalf("rendered schema missing json fence: %q", rendered)
	}
	start += len(fence)
	end := strings.Index(rendered[start:], "\n```")
	if end == -1 {
		t.Fatalf("rendered schema missing closing fence: %q", rendered)
	}
	payload := rendered[start : start+end]
	if !json.Valid([]byte(payload)) {
		t.Fatalf("schema payload is not valid JSON: %q", payload)
	}
	if strings.Contains(payload, "line one\\nline two") || strings.Contains(payload, "line two\\rline three") {
		t.Errorf("description contains escaped control characters: %q", payload)
	}
	if strings.Contains(payload, "bad\\nname") {
		t.Errorf("parameter name contains escaped newline: %q", payload)
	}
	if !strings.Contains(payload, "line one line two line three") {
		t.Errorf("sanitized description missing from payload: %q", payload)
	}
	if !strings.Contains(payload, "bad name") {
		t.Errorf("sanitized parameter name missing from payload: %q", payload)
	}
}

// --- ToolRegistryToSchemas conversion tests ---

// TestToolRegistryToSchemas verifies that tools stored in a ToolRegistry are
// correctly converted to MCPToolSchema values: server name, tool name,
// description, input schema properties, and required fields are all preserved.
func TestToolRegistryToSchemas(t *testing.T) {
	reg := mcp.NewToolRegistry()
	reg.AddTools("srv", []mcp.Tool{
		{
			Name:        "search",
			Description: "Full-text search",
			InputSchema: mcp.ToolSchema{
				Type: "object",
				Properties: map[string]mcp.ToolProperty{
					"query": {Type: "string", Description: "Search query"},
					"limit": {Type: "integer"},
				},
				Required: []string{"query"},
			},
		},
	})

	schemas := ToolRegistryToSchemas(reg)
	if len(schemas) != 1 {
		t.Fatalf("len(schemas) = %d, want 1", len(schemas))
	}

	s := schemas[0]
	if s.ServerName != "srv" {
		t.Errorf("ServerName = %q, want srv", s.ServerName)
	}
	if s.ToolName != "search" {
		t.Errorf("ToolName = %q, want search", s.ToolName)
	}
	if s.Description != "Full-text search" {
		t.Errorf("Description = %q, want 'Full-text search'", s.Description)
	}
	if len(s.Required) != 1 || s.Required[0] != "query" {
		t.Errorf("Required = %v, want [query]", s.Required)
	}
	if len(s.InputSchema) != 2 {
		t.Errorf("InputSchema len = %d, want 2", len(s.InputSchema))
	}
	if _, ok := s.InputSchema["query"]; !ok {
		t.Error("InputSchema missing 'query' property")
	}
	if _, ok := s.InputSchema["limit"]; !ok {
		t.Error("InputSchema missing 'limit' property")
	}
}

// TestToolRegistryToSchemas_MultiServer verifies that tools from different
// servers are all included and carry the correct server names.
func TestToolRegistryToSchemas_MultiServer(t *testing.T) {
	reg := mcp.NewToolRegistry()
	reg.AddTools("alpha", []mcp.Tool{{Name: "ping"}})
	reg.AddTools("beta", []mcp.Tool{{Name: "pong"}})

	schemas := ToolRegistryToSchemas(reg)
	if len(schemas) != 2 {
		t.Fatalf("len(schemas) = %d, want 2", len(schemas))
	}

	names := map[string]string{} // toolName → serverName
	for _, s := range schemas {
		names[s.ToolName] = s.ServerName
	}
	if names["ping"] != "alpha" {
		t.Errorf("ping server = %q, want alpha", names["ping"])
	}
	if names["pong"] != "beta" {
		t.Errorf("pong server = %q, want beta", names["pong"])
	}
}

// TestToolRegistryToSchemas_Empty verifies that an empty registry produces
// an empty slice (not nil-related panics).
func TestToolRegistryToSchemas_Empty(t *testing.T) {
	reg := mcp.NewToolRegistry()
	schemas := ToolRegistryToSchemas(reg)
	if len(schemas) != 0 {
		t.Errorf("len(schemas) = %d, want 0 for empty registry", len(schemas))
	}
}

// --- collectToolNames unit tests ---

func TestCollectToolNames_Dedup(t *testing.T) {
	explicit := []string{"a/x", "b/y"}
	schemas := []MCPToolSchema{
		{ServerName: "b", ToolName: "y"}, // duplicate of b/y
		{ServerName: "c", ToolName: "z"},
	}
	got := collectToolNames(explicit, schemas)
	// Expected: a/x, b/y, c/z (b/y deduplicated)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3: %v", len(got), got)
	}
	if got[0] != "a/x" || got[1] != "b/y" || got[2] != "c/z" {
		t.Errorf("got %v, want [a/x b/y c/z]", got)
	}
}

func TestCollectToolNames_Empty(t *testing.T) {
	got := collectToolNames(nil, nil)
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

// min returns the smaller of a and b (helper for test output trimming).
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
