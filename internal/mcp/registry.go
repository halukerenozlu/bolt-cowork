package mcp

import "sort"

// Registry holds MCP server configurations and the tools they expose.
type Registry struct {
	servers map[string]*ServerConfig
	tools   map[string]*MCPTool
}

// NewRegistry creates an empty MCP registry.
func NewRegistry() *Registry {
	return &Registry{
		servers: make(map[string]*ServerConfig),
		tools:   make(map[string]*MCPTool),
	}
}

// AddServer registers an MCP server configuration.
func (r *Registry) AddServer(cfg ServerConfig) {
	r.servers[cfg.Name] = &cfg
}

// GetServer returns the server config for the given name, or false if not found.
func (r *Registry) GetServer(name string) (*ServerConfig, bool) {
	s, ok := r.servers[name]
	return s, ok
}

// Servers returns all registered server configs sorted by name.
func (r *Registry) Servers() []*ServerConfig {
	out := make([]*ServerConfig, 0, len(r.servers))
	for _, s := range r.servers {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// toolKey returns the composite key "server:tool" used to store tools.
func toolKey(server, name string) string {
	return server + ":" + name
}

// RegisterTool adds a tool to the registry, keyed by server+name to avoid
// collisions when different servers expose tools with the same name.
func (r *Registry) RegisterTool(t MCPTool) {
	r.tools[toolKey(t.ServerName, t.Name)] = &t
}

// GetTool returns the tool provided by server with the given name.
func (r *Registry) GetTool(server, name string) (*MCPTool, bool) {
	t, ok := r.tools[toolKey(server, name)]
	return t, ok
}

// GetToolByName returns the first tool matching name regardless of server.
// When multiple servers expose the same tool name the returned tool is
// non-deterministic; prefer GetTool with an explicit server when possible.
func (r *Registry) GetToolByName(name string) (*MCPTool, bool) {
	for _, t := range r.tools {
		if t.Name == name {
			return t, true
		}
	}
	return nil, false
}

// Tools returns all registered tools sorted by name.
func (r *Registry) Tools() []*MCPTool {
	out := make([]*MCPTool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ToolsByServer returns all tools provided by the named server, sorted by name.
func (r *Registry) ToolsByServer(name string) []*MCPTool {
	var out []*MCPTool
	for _, t := range r.tools {
		if t.ServerName == name {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
