package mcp

import "sync"

// ResourceRegistry stores the resources discovered from each connected MCP server.
// Keys are server names; values are slices of Resource in the order returned by the server.
// ResourceRegistry is safe for concurrent use from multiple goroutines.
//
// Unlike ToolRegistry, the key for individual resources is a URI which may
// contain forward slashes, so a nested map is used instead of a composite string key.
type ResourceRegistry struct {
	mu   sync.RWMutex
	data map[string][]Resource // serverName → resources
}

// NewResourceRegistry creates an empty ResourceRegistry.
func NewResourceRegistry() *ResourceRegistry {
	return &ResourceRegistry{data: make(map[string][]Resource)}
}

// ReplaceServerResources replaces all resources for serverName with a deep copy of resources.
// If resources is empty the entry for serverName is removed.
func (r *ResourceRegistry) ReplaceServerResources(serverName string, resources []Resource) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(resources) == 0 {
		delete(r.data, serverName)
		return
	}
	cp := make([]Resource, len(resources))
	copy(cp, resources)
	r.data[serverName] = cp
}

// RemoveServer removes all resources associated with serverName.
func (r *ResourceRegistry) RemoveServer(serverName string) {
	r.mu.Lock()
	delete(r.data, serverName)
	r.mu.Unlock()
}

// GetResource returns the Resource with the given URI registered under serverName.
// A linear scan is used because URI count per server is expected to be small.
func (r *ResourceRegistry) GetResource(serverName, uri string) (Resource, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, res := range r.data[serverName] {
		if res.URI == uri {
			return res, true
		}
	}
	return Resource{}, false
}

// ListAll returns a deep copy of the entire registry.
// The returned map is safe to modify; changes do not affect the registry.
func (r *ResourceRegistry) ListAll() map[string][]Resource {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string][]Resource, len(r.data))
	for name, list := range r.data {
		cp := make([]Resource, len(list))
		copy(cp, list)
		out[name] = cp
	}
	return out
}
