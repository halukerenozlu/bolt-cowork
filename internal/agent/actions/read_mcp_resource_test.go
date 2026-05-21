package actions_test

import (
	"context"
	"errors"
	"testing"

	"github.com/halukerenozlu/bolt-cowork/internal/agent/actions"
	"github.com/halukerenozlu/bolt-cowork/internal/mcp"
)

// mockReader implements actions.MCPResourceReader for unit tests.
type mockReader struct {
	contents *mcp.ResourceContents
	err      error
}

func (m *mockReader) ReadResource(_ context.Context, _, _ string) (*mcp.ResourceContents, error) {
	return m.contents, m.err
}

func TestReadMCPResourceAction_Success(t *testing.T) {
	reader := &mockReader{
		contents: &mcp.ResourceContents{
			URI:  "file://test.txt",
			Text: "hello resource",
		},
	}
	action := &actions.ReadMCPResourceAction{ServerName: "srv", URI: "file://test.txt"}

	result, err := action.Execute(context.Background(), reader)
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if result.Output != "hello resource" {
		t.Errorf("Output = %q, want %q", result.Output, "hello resource")
	}
	if result.Error != "" {
		t.Errorf("Error = %q, want empty", result.Error)
	}
}

func TestReadMCPResourceAction_ReaderError(t *testing.T) {
	reader := &mockReader{err: errors.New("resource not found")}
	action := &actions.ReadMCPResourceAction{ServerName: "srv", URI: "file://missing.txt"}

	_, err := action.Execute(context.Background(), reader)
	if err == nil {
		t.Fatal("Execute: expected error, got nil")
	}
	if !errors.Is(err, reader.err) {
		t.Errorf("error = %v; want to wrap %v", err, reader.err)
	}
}

func TestReadMCPResourceAction_ContextCancelled(t *testing.T) {
	reader := &mockReader{err: errors.New("should not be reached")}
	action := &actions.ReadMCPResourceAction{ServerName: "srv", URI: "file://x.txt"}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before calling Execute

	_, err := action.Execute(ctx, reader)
	if err == nil {
		t.Fatal("Execute: expected context error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v; want context.Canceled", err)
	}
}

func TestReadMCPResourceAction_IsDangerous(t *testing.T) {
	action := &actions.ReadMCPResourceAction{}
	if action.IsDangerous() {
		t.Error("IsDangerous() = true, want false for read-only action")
	}
}

func TestReadMCPResourceAction_Summary(t *testing.T) {
	action := &actions.ReadMCPResourceAction{ServerName: "srv", URI: "file://doc.txt"}
	want := "Read MCP resource: srv/file://doc.txt"
	if got := action.Summary(); got != want {
		t.Errorf("Summary() = %q, want %q", got, want)
	}
}

func TestReadMCPResourceAction_Type(t *testing.T) {
	action := &actions.ReadMCPResourceAction{}
	if action.Type() != "read_mcp_resource" {
		t.Errorf("Type() = %q, want %q", action.Type(), "read_mcp_resource")
	}
}
