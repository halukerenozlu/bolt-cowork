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

// RegisterTool adds a tool to the registry.
func (r *Registry) RegisterTool(t MCPTool) {
	r.tools[t.Name] = &t
}

// GetTool returns the tool with the given name, or false if not found.
func (r *Registry) GetTool(name string) (*MCPTool, bool) {
	t, ok := r.tools[name]
	return t, ok
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
