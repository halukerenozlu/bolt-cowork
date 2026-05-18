package registry

import (
	"sync"
	"testing"

	"github.com/halukerenozlu/bolt-cowork/internal/mcp"
)

// tool is a short constructor used to keep table rows concise.
func tool(name, desc string) mcp.Tool {
	return mcp.Tool{Name: name, Description: desc}
}

// toolNames extracts the Name field from a slice of Tools.
func toolNames(tools []mcp.Tool) []string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	return names
}

// --- AddTools + ListAll ---

func TestToolRegistry_AddTools_ListAll(t *testing.T) {
	tests := []struct {
		name string
		adds []struct {
			server string
			tools  []mcp.Tool
		}
		wantGroups map[string]int // serverName → expected tool count
	}{
		{
			name: "single_server_two_tools",
			adds: []struct {
				server string
				tools  []mcp.Tool
			}{
				{"fs", []mcp.Tool{tool("read", ""), tool("write", "")}},
			},
			wantGroups: map[string]int{"fs": 2},
		},
		{
			name: "two_servers",
			adds: []struct {
				server string
				tools  []mcp.Tool
			}{
				{"fs", []mcp.Tool{tool("read", ""), tool("write", "")}},
				{"web", []mcp.Tool{tool("fetch", "")}},
			},
			wantGroups: map[string]int{"fs": 2, "web": 1},
		},
		{
			name: "empty_tool_slice",
			adds: []struct {
				server string
				tools  []mcp.Tool
			}{
				{"srv", []mcp.Tool{}},
			},
			wantGroups: map[string]int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewToolRegistry()
			for _, a := range tt.adds {
				r.AddTools(a.server, a.tools)
			}

			got := r.ListAll()

			if len(got) != len(tt.wantGroups) {
				t.Fatalf("ListAll: got %d server groups, want %d", len(got), len(tt.wantGroups))
			}
			for srv, wantCount := range tt.wantGroups {
				tools, ok := got[srv]
				if !ok {
					t.Errorf("ListAll: missing server %q", srv)
					continue
				}
				if len(tools) != wantCount {
					t.Errorf("ListAll[%q]: got %d tools, want %d", srv, len(tools), wantCount)
				}
			}
		})
	}
}

// --- GetTool ---

func TestToolRegistry_GetTool(t *testing.T) {
	r := NewToolRegistry()
	r.AddTools("fs", []mcp.Tool{
		tool("read", "Read a file"),
		tool("write", "Write a file"),
	})
	r.AddTools("web", []mcp.Tool{
		tool("fetch", "Fetch a URL"),
	})

	tests := []struct {
		name       string
		query      string
		wantFound  bool
		wantServer string
		wantDesc   string
	}{
		{"found in first server", "read", true, "fs", "Read a file"},
		{"found in second server", "fetch", true, "web", "Fetch a URL"},
		{"not found", "delete", false, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, srv, ok := r.GetTool(tt.query)

			if ok != tt.wantFound {
				t.Fatalf("GetTool(%q) found = %v, want %v", tt.query, ok, tt.wantFound)
			}
			if !tt.wantFound {
				return
			}
			if srv != tt.wantServer {
				t.Errorf("server = %q, want %q", srv, tt.wantServer)
			}
			if got.Description != tt.wantDesc {
				t.Errorf("Description = %q, want %q", got.Description, tt.wantDesc)
			}
		})
	}
}

// --- Replace semantics ---

func TestToolRegistry_AddTools_ReplacesExistingServerTool(t *testing.T) {
	r := NewToolRegistry()

	r.AddTools("srvA", []mcp.Tool{tool("read", "original")})
	if got, srv, ok := r.GetTool("read"); !ok || got.Description != "original" || srv != "srvA" {
		t.Fatalf("initial state wrong: got %+v, srv=%q, ok=%v", got, srv, ok)
	}

	// Same tool name, different server and description — must replace.
	r.AddTools("srvA", []mcp.Tool{tool("read", "updated")})

	got, srv, ok := r.GetTool("read")
	if !ok {
		t.Fatal("GetTool returned false after replace")
	}
	if got.Description != "updated" {
		t.Errorf("Description = %q, want %q", got.Description, "updated")
	}
	if srv != "srvA" {
		t.Errorf("server = %q, want %q", srv, "srvA")
	}

	// Only one entry should exist for "read".
	all := r.ListAll()
	total := 0
	for _, tools := range all {
		for _, tt := range tools {
			if tt.Name == "read" {
				total++
			}
		}
	}
	if total != 1 {
		t.Errorf("'read' appears %d times in ListAll, want 1", total)
	}
}

func TestToolRegistry_DuplicateToolNamesAcrossServers(t *testing.T) {
	r := NewToolRegistry()

	r.AddTools("fs", []mcp.Tool{tool("read", "Read a file")})
	r.AddTools("web", []mcp.Tool{tool("read", "Read a URL")})

	all := r.ListAll()
	if len(all["fs"]) != 1 {
		t.Fatalf("ListAll[fs] has %d tools, want 1", len(all["fs"]))
	}
	if len(all["web"]) != 1 {
		t.Fatalf("ListAll[web] has %d tools, want 1", len(all["web"]))
	}
	if all["fs"][0].Name != "read" || all["fs"][0].Description != "Read a file" {
		t.Errorf("ListAll[fs][0] = %+v, want fs read tool", all["fs"][0])
	}
	if all["web"][0].Name != "read" || all["web"][0].Description != "Read a URL" {
		t.Errorf("ListAll[web][0] = %+v, want web read tool", all["web"][0])
	}

	fsTool, ok := r.GetServerTool("fs", "read")
	if !ok {
		t.Fatal("GetServerTool(fs, read) returned false")
	}
	if fsTool.Description != "Read a file" {
		t.Errorf("fs read Description = %q, want %q", fsTool.Description, "Read a file")
	}

	webTool, ok := r.GetServerTool("web", "read")
	if !ok {
		t.Fatal("GetServerTool(web, read) returned false")
	}
	if webTool.Description != "Read a URL" {
		t.Errorf("web read Description = %q, want %q", webTool.Description, "Read a URL")
	}
}

func TestToolRegistry_ReplaceServerTools(t *testing.T) {
	r := NewToolRegistry()
	r.AddTools("fs", []mcp.Tool{tool("read", ""), tool("write", "")})
	r.AddTools("web", []mcp.Tool{tool("fetch", "")})

	r.ReplaceServerTools("fs", []mcp.Tool{tool("read", "updated")})

	if _, _, ok := r.GetTool("write"); ok {
		t.Error("stale fs tool write remained after ReplaceServerTools")
	}
	got, srv, ok := r.GetTool("read")
	if !ok {
		t.Fatal("read missing after ReplaceServerTools")
	}
	if srv != "fs" {
		t.Errorf("read server = %q, want fs", srv)
	}
	if got.Description != "updated" {
		t.Errorf("read Description = %q, want updated", got.Description)
	}
	if _, srv, ok := r.GetTool("fetch"); !ok || srv != "web" {
		t.Errorf("web fetch tool was affected by ReplaceServerTools: srv=%q ok=%v", srv, ok)
	}
}

// --- RemoveServer ---

func TestToolRegistry_RemoveServer(t *testing.T) {
	r := NewToolRegistry()
	r.AddTools("fs", []mcp.Tool{tool("read", ""), tool("write", "")})
	r.AddTools("web", []mcp.Tool{tool("fetch", "")})

	r.RemoveServer("fs")

	// fs tools must be gone.
	if _, _, ok := r.GetTool("read"); ok {
		t.Error("GetTool(read): expected false after RemoveServer(fs)")
	}
	if _, _, ok := r.GetTool("write"); ok {
		t.Error("GetTool(write): expected false after RemoveServer(fs)")
	}

	// web tools must survive.
	if _, srv, ok := r.GetTool("fetch"); !ok || srv != "web" {
		t.Errorf("GetTool(fetch) = (_, %q, %v), want (_, web, true)", srv, ok)
	}

	// ListAll must not contain fs.
	all := r.ListAll()
	if _, exists := all["fs"]; exists {
		t.Error("ListAll still contains fs after RemoveServer")
	}
	if len(all["web"]) != 1 {
		t.Errorf("ListAll[web] has %d tools, want 1", len(all["web"]))
	}
}

func TestToolRegistry_RemoveServer_Unknown(t *testing.T) {
	r := NewToolRegistry()
	r.AddTools("fs", []mcp.Tool{tool("read", "")})

	// Removing a server that was never registered must not panic or alter
	// existing entries.
	r.RemoveServer("nope")

	if _, _, ok := r.GetTool("read"); !ok {
		t.Error("GetTool(read) returned false after RemoveServer(nope)")
	}
}

// --- Clear ---

func TestToolRegistry_Clear(t *testing.T) {
	r := NewToolRegistry()
	r.AddTools("fs", []mcp.Tool{tool("read", ""), tool("write", "")})
	r.AddTools("web", []mcp.Tool{tool("fetch", "")})

	r.Clear()

	if all := r.ListAll(); len(all) != 0 {
		t.Errorf("ListAll after Clear: got %d groups, want 0", len(all))
	}
	if _, _, ok := r.GetTool("read"); ok {
		t.Error("GetTool after Clear returned true")
	}
}

// --- Deep copy: ListAll and GetTool ---

// toolWithSchema builds a Tool with a populated InputSchema for use in
// deep-copy tests.
func toolWithSchema(name string) mcp.Tool {
	return mcp.Tool{
		Name:        name,
		Description: "original",
		InputSchema: mcp.ToolSchema{
			Type: "object",
			Properties: map[string]mcp.ToolProperty{
				"path": {Type: "string", Description: "file path"},
			},
			Required: []string{"path"},
		},
	}
}

func TestToolRegistry_ListAll_ReturnsCopy(t *testing.T) {
	r := NewToolRegistry()
	r.AddTools("fs", []mcp.Tool{toolWithSchema("read")})

	snapshot := r.ListAll()

	// Shallow mutation — replace the whole Tool element in the slice.
	snapshot["fs"][0] = tool("read", "mutated")
	// Inject a foreign server key.
	snapshot["injected"] = []mcp.Tool{tool("evil", "")}

	// Registry must be unchanged after shallow mutation.
	got, _, ok := r.GetTool("read")
	if !ok {
		t.Fatal("GetTool returned false after snapshot mutation")
	}
	if got.Description != "original" {
		t.Errorf("Description = %q after snapshot mutation, want %q", got.Description, "original")
	}
	if _, _, ok := r.GetTool("evil"); ok {
		t.Error("injected tool appeared in registry")
	}

	// Nested (deep) mutation — modify the Properties map inside a snapshot element.
	snapshot2 := r.ListAll()
	snapshot2["fs"][0].InputSchema.Properties["x"] = mcp.ToolProperty{Type: "mutated"}

	// The registry's copy of the tool must not contain the injected key.
	original, _, _ := r.GetTool("read")
	if _, found := original.InputSchema.Properties["x"]; found {
		t.Error("nested Properties mutation propagated into registry")
	}
	// The original property must still be intact.
	if p, ok := original.InputSchema.Properties["path"]; !ok || p.Type != "string" {
		t.Errorf("original Properties[path] = %+v, want {Type: string, ...}", p)
	}
}

func TestToolRegistry_GetTool_ReturnsCopy(t *testing.T) {
	r := NewToolRegistry()
	r.AddTools("fs", []mcp.Tool{toolWithSchema("read")})

	got, _, _ := r.GetTool("read")

	// Mutate the returned value's Properties and Required.
	got.InputSchema.Properties["injected"] = mcp.ToolProperty{Type: "bad"}
	got.InputSchema.Required = append(got.InputSchema.Required, "extra")

	// A fresh GetTool must return the unmodified registry copy.
	again, _, _ := r.GetTool("read")
	if _, found := again.InputSchema.Properties["injected"]; found {
		t.Error("Properties mutation from GetTool return value propagated into registry")
	}
	for _, req := range again.InputSchema.Required {
		if req == "extra" {
			t.Error("Required mutation from GetTool return value propagated into registry")
		}
	}
}

func TestToolRegistry_AddTools_InputIsolated(t *testing.T) {
	props := map[string]mcp.ToolProperty{
		"path": {Type: "string"},
	}
	req := []string{"path"}
	original := mcp.Tool{
		Name: "read",
		InputSchema: mcp.ToolSchema{
			Type:       "object",
			Properties: props,
			Required:   req,
		},
	}

	r := NewToolRegistry()
	r.AddTools("fs", []mcp.Tool{original})

	// Mutate the caller's original map and slice after AddTools.
	props["injected"] = mcp.ToolProperty{Type: "bad"}
	req[0] = "mutated"

	stored, _, _ := r.GetTool("read")
	if _, found := stored.InputSchema.Properties["injected"]; found {
		t.Error("post-AddTools Properties mutation propagated into registry")
	}
	if stored.InputSchema.Required[0] != "path" {
		t.Errorf("Required[0] = %q, want %q", stored.InputSchema.Required[0], "path")
	}
}

// --- Concurrent access ---

func TestToolRegistry_Concurrent(t *testing.T) {
	r := NewToolRegistry()

	const goroutines = 20
	const opsPerGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	// Half the goroutines write; half read. This exercises the RWMutex path
	// and will be caught by the race detector when run with -race.
	for i := 0; i < goroutines; i++ {
		isWriter := i%2 == 0
		go func(writer bool, id int) {
			defer wg.Done()
			serverName := "srv"
			for j := 0; j < opsPerGoroutine; j++ {
				if writer {
					r.AddTools(serverName, []mcp.Tool{tool("read", "v"), tool("write", "v")})
				} else {
					r.GetTool("read")
					r.ListAll()
				}
			}
		}(isWriter, i)
	}

	wg.Wait()

	// Registry must be in a consistent state (no panic, all keys present or absent).
	r.ListAll()
}

func TestToolRegistry_Concurrent_RemoveAndAdd(t *testing.T) {
	r := NewToolRegistry()

	const goroutines = 10

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				r.AddTools("srvA", []mcp.Tool{tool("t1", ""), tool("t2", "")})
				r.RemoveServer("srvA")
				r.AddTools("srvB", []mcp.Tool{tool("t3", "")})
				r.GetTool("t1")
				r.GetTool("t3")
			}
		}(i)
	}

	wg.Wait()
}
