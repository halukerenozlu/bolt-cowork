package tool

import (
	"sort"
	"testing"

	"github.com/halukerenozlu/bolt-cowork/internal/sandbox"
)

func newTestRegistry(t *testing.T) *Registry {
	t.Helper()
	dir := t.TempDir()
	sb, err := sandbox.New(dir)
	if err != nil {
		t.Fatalf("sandbox.New: %v", err)
	}
	return DefaultRegistry(sb)
}

func TestBuiltinToolNames(t *testing.T) {
	reg := newTestRegistry(t)
	all := reg.All()
	if len(all) != 8 {
		t.Fatalf("expected 8 builtin tools, got %d", len(all))
	}

	got := make([]string, len(all))
	for i, tool := range all {
		got[i] = tool.Name()
	}
	sort.Strings(got)

	want := []string{"copy", "delete", "list", "mkdir", "move", "read", "rename", "write"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("tool[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestReadToolIsReadOnly(t *testing.T) {
	reg := newTestRegistry(t)
	tool, ok := reg.Get("read")
	if !ok {
		t.Fatal("read tool not found")
	}
	if !tool.IsReadOnly() {
		t.Error("read tool should be read-only")
	}
	if tool.IsDestructive() {
		t.Error("read tool should not be destructive")
	}
}

func TestDeleteToolIsDestructive(t *testing.T) {
	reg := newTestRegistry(t)
	tool, ok := reg.Get("delete")
	if !ok {
		t.Fatal("delete tool not found")
	}
	if tool.IsReadOnly() {
		t.Error("delete tool should not be read-only")
	}
	if !tool.IsDestructive() {
		t.Error("delete tool should be destructive")
	}
}

func TestRegistryGet(t *testing.T) {
	reg := newTestRegistry(t)
	tool, ok := reg.Get("read")
	if !ok {
		t.Fatal("expected Get(\"read\") to return ok=true")
	}
	if tool.Name() != "read" {
		t.Errorf("expected tool name \"read\", got %q", tool.Name())
	}
}

func TestRegistryGetMissing(t *testing.T) {
	reg := newTestRegistry(t)
	_, ok := reg.Get("xyz")
	if ok {
		t.Error("expected Get(\"xyz\") to return ok=false")
	}
}

func TestWriteToolEmptyContent(t *testing.T) {
	reg := newTestRegistry(t)
	tool, ok := reg.Get("write")
	if !ok {
		t.Fatal("write tool not found")
	}
	res, err := tool.Call(nil, map[string]any{"path": "test.txt", "content": ""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusFailed {
		t.Errorf("expected StatusFailed, got %d", res.Status)
	}
	if res.Error != "write requires non-empty content" {
		t.Errorf("unexpected error message: %q", res.Error)
	}
}

func TestWriteToolIsDestructive(t *testing.T) {
	reg := newTestRegistry(t)
	tool, ok := reg.Get("write")
	if !ok {
		t.Fatal("write tool not found")
	}
	if !tool.IsDestructive() {
		t.Error("write tool should be destructive")
	}
}

func TestResultStatus(t *testing.T) {
	success := Result{Status: StatusSuccess}
	if !success.Ok() {
		t.Error("StatusSuccess.Ok() should be true")
	}

	denied := Result{Status: StatusDenied}
	if denied.Ok() {
		t.Error("StatusDenied.Ok() should be false")
	}

	cancelled := Result{Status: StatusCancelled}
	if cancelled.Ok() {
		t.Error("StatusCancelled.Ok() should be false")
	}

	failed := Result{Status: StatusFailed}
	if failed.Ok() {
		t.Error("StatusFailed.Ok() should be false")
	}
}
