package mcp_test

import (
	"testing"

	"github.com/halukerenozlu/bolt-cowork/internal/mcp"
)

func TestResourceRegistry_ReplaceAndList(t *testing.T) {
	r := mcp.NewResourceRegistry()
	resources := []mcp.Resource{
		{URI: "file://a.txt", Name: "A"},
		{URI: "file://b.txt", Name: "B"},
	}
	r.ReplaceServerResources("srv", resources)

	all := r.ListAll()
	if len(all["srv"]) != 2 {
		t.Fatalf("ListAll[srv] len = %d, want 2", len(all["srv"]))
	}
	if all["srv"][0].URI != "file://a.txt" {
		t.Errorf("ListAll[srv][0].URI = %q, want %q", all["srv"][0].URI, "file://a.txt")
	}
}

func TestResourceRegistry_RemoveServer(t *testing.T) {
	r := mcp.NewResourceRegistry()
	r.ReplaceServerResources("srv", []mcp.Resource{{URI: "file://x.txt", Name: "X"}})
	r.RemoveServer("srv")

	all := r.ListAll()
	if _, ok := all["srv"]; ok {
		t.Error("ListAll: server still present after RemoveServer")
	}
}

func TestResourceRegistry_GetResource_Found(t *testing.T) {
	r := mcp.NewResourceRegistry()
	r.ReplaceServerResources("srv", []mcp.Resource{
		{URI: "file://a.txt", Name: "A"},
		{URI: "file://b.txt", Name: "B", MimeType: "text/plain"},
	})

	res, ok := r.GetResource("srv", "file://b.txt")
	if !ok {
		t.Fatal("GetResource: expected to find file://b.txt")
	}
	if res.Name != "B" {
		t.Errorf("GetResource.Name = %q, want %q", res.Name, "B")
	}
	if res.MimeType != "text/plain" {
		t.Errorf("GetResource.MimeType = %q, want %q", res.MimeType, "text/plain")
	}
}

func TestResourceRegistry_GetResource_NotFound(t *testing.T) {
	r := mcp.NewResourceRegistry()
	r.ReplaceServerResources("srv", []mcp.Resource{{URI: "file://a.txt", Name: "A"}})

	_, ok := r.GetResource("srv", "file://does-not-exist.txt")
	if ok {
		t.Error("GetResource: expected not found, got found")
	}
}

func TestResourceRegistry_ReplaceOverwrites(t *testing.T) {
	r := mcp.NewResourceRegistry()
	r.ReplaceServerResources("srv", []mcp.Resource{{URI: "file://old.txt", Name: "Old"}})
	r.ReplaceServerResources("srv", []mcp.Resource{{URI: "file://new.txt", Name: "New"}})

	all := r.ListAll()
	if len(all["srv"]) != 1 {
		t.Fatalf("ListAll[srv] len = %d, want 1", len(all["srv"]))
	}
	if all["srv"][0].URI != "file://new.txt" {
		t.Errorf("ListAll[srv][0].URI = %q, want file://new.txt", all["srv"][0].URI)
	}
}

func TestResourceRegistry_ListAllDeepCopy(t *testing.T) {
	r := mcp.NewResourceRegistry()
	r.ReplaceServerResources("srv", []mcp.Resource{{URI: "file://a.txt", Name: "A"}})

	all := r.ListAll()
	// Mutate the returned copy.
	all["srv"][0].Name = "MUTATED"
	all["extra"] = []mcp.Resource{{URI: "file://extra.txt", Name: "Extra"}}

	// Original registry must be unaffected.
	all2 := r.ListAll()
	if all2["srv"][0].Name != "A" {
		t.Errorf("deep copy: original Name = %q, want A", all2["srv"][0].Name)
	}
	if _, ok := all2["extra"]; ok {
		t.Error("deep copy: extra key leaked into registry")
	}
}

func TestResourceRegistry_ReplaceEmpty_Removes(t *testing.T) {
	r := mcp.NewResourceRegistry()
	r.ReplaceServerResources("srv", []mcp.Resource{{URI: "file://a.txt", Name: "A"}})
	r.ReplaceServerResources("srv", nil) // replace with empty → removes

	all := r.ListAll()
	if _, ok := all["srv"]; ok {
		t.Error("ReplaceServerResources(nil): server still present")
	}
}
