package tool

import (
	"sort"

	"github.com/halukerenozlu/bolt-cowork/internal/sandbox"
)

// Registry holds a named set of tools.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool to the registry. If a tool with the same name already
// exists it is replaced.
func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
}

// Get returns the tool with the given name and true, or nil and false if not found.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// All returns every registered tool sorted by name.
func (r *Registry) All() []Tool {
	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name() < out[j].Name()
	})
	return out
}

// ReadOnly returns only tools where IsReadOnly() is true, sorted by name.
func (r *Registry) ReadOnly() []Tool {
	var out []Tool
	for _, t := range r.tools {
		if t.IsReadOnly() {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name() < out[j].Name()
	})
	return out
}

// DefaultRegistry creates a Registry pre-populated with the 8 builtin
// file-operation tools, all backed by the provided sandbox.
func DefaultRegistry(sb *sandbox.Sandbox) *Registry {
	r := NewRegistry()
	r.Register(&ReadTool{sb: sb})
	r.Register(&WriteTool{sb: sb})
	r.Register(&DeleteTool{sb: sb})
	r.Register(&MkdirTool{sb: sb})
	r.Register(&CopyTool{sb: sb})
	r.Register(&MoveTool{sb: sb})
	r.Register(&RenameTool{sb: sb})
	r.Register(&ListTool{sb: sb})
	return r
}
