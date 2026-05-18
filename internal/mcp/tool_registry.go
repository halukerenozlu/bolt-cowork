package mcp

import (
	"strings"
	"sync"
)

// registeredTool pairs a wire-format Tool with the name of the server that
// exposes it. The server name is stored here rather than inside Tool so that
// the wire type stays free of registry bookkeeping fields.
type registeredTool struct {
	Tool       Tool
	ServerName string
}

// ToolRegistry stores the tools discovered from connected MCP servers.
// Tools are indexed by the composite "serverName/toolName" key so that two
// servers can expose a tool with the same name without clobbering each other.
//
// ToolRegistry is safe for concurrent use from multiple goroutines.
type ToolRegistry struct {
	mu      sync.RWMutex
	tools   map[string]registeredTool // key: serverName/toolName
	aliases map[string]string         // legacy key: toolName -> latest composite key
}

// NewToolRegistry creates an empty ToolRegistry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools:   make(map[string]registeredTool),
		aliases: make(map[string]string),
	}
}

func serverToolKey(serverName, toolName string) string {
	return serverName + "/" + toolName
}

func (r *ToolRegistry) refreshAliasLocked(toolName string) {
	delete(r.aliases, toolName)
	for key, rt := range r.tools {
		if rt.Tool.Name == toolName {
			r.aliases[toolName] = key
		}
	}
}

// cloneToolInRegistry returns a deep copy of t. The reference-typed fields
// InputSchema.Properties (map) and InputSchema.Required (slice) are allocated
// fresh so that no caller can alias the registry's internal state.
//
// ToolProperty contains only string fields (value types), so copying map
// values directly is sufficient — no further recursion is needed.
func cloneToolInRegistry(t Tool) Tool {
	c := t // copies all value-type fields: Name, Description, InputSchema.Type

	if t.InputSchema.Properties != nil {
		c.InputSchema.Properties = make(map[string]ToolProperty, len(t.InputSchema.Properties))
		for k, v := range t.InputSchema.Properties {
			c.InputSchema.Properties[k] = v
		}
	}

	if t.InputSchema.Required != nil {
		c.InputSchema.Required = make([]string, len(t.InputSchema.Required))
		copy(c.InputSchema.Required, t.InputSchema.Required)
	}

	return c
}

// AddTools registers every tool in tools under serverName. If the same server
// reports the same tool again it is replaced; tools with the same name from
// other servers are preserved under their own composite keys.
func (r *ToolRegistry) AddTools(serverName string, tools []Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, t := range tools {
		key := serverToolKey(serverName, t.Name)
		r.tools[key] = registeredTool{Tool: cloneToolInRegistry(t), ServerName: serverName}
		r.aliases[t.Name] = key
	}
}

// ReplaceServerTools atomically replaces all tools owned by serverName with
// tools. Existing tools from other servers are left untouched.
func (r *ToolRegistry) ReplaceServerTools(serverName string, tools []Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for name, rt := range r.tools {
		if rt.ServerName == serverName {
			delete(r.tools, name)
			if r.aliases[rt.Tool.Name] == name {
				r.refreshAliasLocked(rt.Tool.Name)
			}
		}
	}
	for _, t := range tools {
		key := serverToolKey(serverName, t.Name)
		r.tools[key] = registeredTool{Tool: cloneToolInRegistry(t), ServerName: serverName}
		r.aliases[t.Name] = key
	}
}

// GetTool looks up a tool by name. It returns the Tool, the name of the
// server that provides it, and true when found. When no tool with that name
// is registered all return values are zero/false.
func (r *ToolRegistry) GetTool(toolName string) (Tool, string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key, ok := r.aliases[toolName]
	if !ok {
		return Tool{}, "", false
	}
	rt, ok := r.tools[key]
	return cloneToolInRegistry(rt.Tool), rt.ServerName, ok
}

// GetServerTool looks up the exact tool exposed by serverName.
func (r *ToolRegistry) GetServerTool(serverName, toolName string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rt, ok := r.tools[serverToolKey(serverName, toolName)]
	return cloneToolInRegistry(rt.Tool), ok
}

// ListAll returns a snapshot of all registered tools grouped by server name.
// The returned map is a deep copy; mutations to it do not affect the registry.
func (r *ToolRegistry) ListAll() map[string][]Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make(map[string][]Tool)
	for key, rt := range r.tools {
		serverName := rt.ServerName
		if serverName == "" {
			serverName = strings.SplitN(key, "/", 2)[0]
		}
		out[serverName] = append(out[serverName], cloneToolInRegistry(rt.Tool))
	}
	return out
}

// RemoveServer deletes all tools that belong to serverName. Tools owned by
// other servers are unaffected. This should be called when a server
// disconnects so that stale tools are no longer discoverable.
func (r *ToolRegistry) RemoveServer(serverName string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for name, rt := range r.tools {
		if rt.ServerName == serverName {
			delete(r.tools, name)
			if r.aliases[rt.Tool.Name] == name {
				r.refreshAliasLocked(rt.Tool.Name)
			}
		}
	}
}

// Clear removes all registered tools, returning the registry to its initial
// empty state.
func (r *ToolRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tools = make(map[string]registeredTool)
	r.aliases = make(map[string]string)
}
