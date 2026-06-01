package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/halukerenozlu/bolt-cowork/internal/sandbox"
)

func newBuiltinTestSandbox(t *testing.T) *sandbox.Sandbox {
	t.Helper()

	sb, err := sandbox.New(t.TempDir())
	if err != nil {
		t.Fatalf("sandbox.New: %v", err)
	}
	return sb
}

func TestInputBool(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]any
		want  bool
	}{
		{name: "missing", input: map[string]any{}, want: false},
		{name: "bool true", input: map[string]any{"recursive": true}, want: true},
		{name: "non bool", input: map[string]any{"recursive": "true"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := inputBool(tt.input, "recursive"); got != tt.want {
				t.Fatalf("inputBool() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuiltinToolMetadataAndSchemas(t *testing.T) {
	reg := DefaultRegistry(newBuiltinTestSandbox(t))

	tests := []struct {
		name          string
		readOnly      bool
		destructive   bool
		requiredKeys  []string
		descriptionIs string
	}{
		{name: "read", readOnly: true, requiredKeys: []string{"path"}, descriptionIs: "Read the contents of a file"},
		{name: "write", destructive: true, requiredKeys: []string{"path", "content"}, descriptionIs: "Write content to a file"},
		{name: "delete", destructive: true, requiredKeys: []string{"path", "recursive"}, descriptionIs: "Delete a file or directory"},
		{name: "mkdir", requiredKeys: []string{"path"}, descriptionIs: "Create a directory (and parents)"},
		{name: "copy", requiredKeys: []string{"path", "destination"}, descriptionIs: "Copy a file to a new location"},
		{name: "move", destructive: true, requiredKeys: []string{"path", "destination"}, descriptionIs: "Move a file to a new location"},
		{name: "rename", destructive: true, requiredKeys: []string{"path", "destination"}, descriptionIs: "Rename a file or directory"},
		{name: "list", readOnly: true, requiredKeys: []string{"path"}, descriptionIs: "List directory contents"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool, ok := reg.Get(tt.name)
			if !ok {
				t.Fatalf("tool %q not registered", tt.name)
			}
			if tool.Description() != tt.descriptionIs {
				t.Fatalf("Description() = %q, want %q", tool.Description(), tt.descriptionIs)
			}
			if tool.IsReadOnly() != tt.readOnly {
				t.Fatalf("IsReadOnly() = %v, want %v", tool.IsReadOnly(), tt.readOnly)
			}
			if tool.IsDestructive() != tt.destructive {
				t.Fatalf("IsDestructive() = %v, want %v", tool.IsDestructive(), tt.destructive)
			}

			schema := tool.InputSchema()
			for _, key := range tt.requiredKeys {
				if _, ok := schema[key]; !ok {
					t.Fatalf("InputSchema() missing key %q: %#v", key, schema)
				}
			}
		})
	}
}

func TestRegistryReadOnly(t *testing.T) {
	reg := DefaultRegistry(newBuiltinTestSandbox(t))

	got := reg.ReadOnly()
	if len(got) != 2 {
		t.Fatalf("ReadOnly() returned %d tools, want 2", len(got))
	}
	if got[0].Name() != "list" || got[1].Name() != "read" {
		t.Fatalf("ReadOnly() names = [%s %s], want [list read]", got[0].Name(), got[1].Name())
	}
}

func TestBuiltinToolsCallSuccess(t *testing.T) {
	sb := newBuiltinTestSandbox(t)
	root := sb.Root()
	ctx := context.Background()

	source := filepath.Join(root, "source.txt")
	if err := os.WriteFile(source, []byte("hello"), 0644); err != nil {
		t.Fatalf("seed source file: %v", err)
	}

	tests := []struct {
		name       string
		tool       Tool
		input      map[string]any
		wantStatus ResultStatus
		wantOutput string
		assertFS   func(t *testing.T)
	}{
		{
			name:       "read",
			tool:       &ReadTool{sb: sb},
			input:      map[string]any{"path": source},
			wantStatus: StatusSuccess,
			wantOutput: "hello",
		},
		{
			name:       "write",
			tool:       &WriteTool{sb: sb},
			input:      map[string]any{"path": filepath.Join(root, "write.txt"), "content": "created"},
			wantStatus: StatusSuccess,
			wantOutput: "Wrote",
			assertFS: func(t *testing.T) {
				data, err := os.ReadFile(filepath.Join(root, "write.txt"))
				if err != nil {
					t.Fatalf("read written file: %v", err)
				}
				if string(data) != "created" {
					t.Fatalf("written content = %q, want created", string(data))
				}
			},
		},
		{
			name:       "mkdir",
			tool:       &MkdirTool{sb: sb},
			input:      map[string]any{"path": filepath.Join(root, "nested", "dir")},
			wantStatus: StatusSuccess,
			wantOutput: "Created directory",
			assertFS: func(t *testing.T) {
				info, err := os.Stat(filepath.Join(root, "nested", "dir"))
				if err != nil {
					t.Fatalf("stat created dir: %v", err)
				}
				if !info.IsDir() {
					t.Fatal("created path is not a directory")
				}
			},
		},
		{
			name:       "copy",
			tool:       &CopyTool{sb: sb},
			input:      map[string]any{"path": source, "destination": filepath.Join(root, "copy.txt")},
			wantStatus: StatusSuccess,
			wantOutput: "Copied",
			assertFS: func(t *testing.T) {
				data, err := os.ReadFile(filepath.Join(root, "copy.txt"))
				if err != nil {
					t.Fatalf("read copied file: %v", err)
				}
				if string(data) != "hello" {
					t.Fatalf("copied content = %q, want hello", string(data))
				}
			},
		},
		{
			name: "move",
			tool: &MoveTool{sb: sb},
			input: map[string]any{
				"path":        seedFile(t, root, "move-source.txt", "move me"),
				"destination": filepath.Join(root, "moved.txt"),
			},
			wantStatus: StatusSuccess,
			wantOutput: "Moved",
			assertFS: func(t *testing.T) {
				if _, err := os.Stat(filepath.Join(root, "moved.txt")); err != nil {
					t.Fatalf("stat moved destination: %v", err)
				}
				if _, err := os.Stat(filepath.Join(root, "move-source.txt")); !os.IsNotExist(err) {
					t.Fatalf("move source still exists or stat failed unexpectedly: %v", err)
				}
			},
		},
		{
			name: "rename",
			tool: &RenameTool{sb: sb},
			input: map[string]any{
				"path":        seedFile(t, root, "rename-source.txt", "rename me"),
				"destination": filepath.Join(root, "renamed.txt"),
			},
			wantStatus: StatusSuccess,
			wantOutput: "Renamed",
			assertFS: func(t *testing.T) {
				if _, err := os.Stat(filepath.Join(root, "renamed.txt")); err != nil {
					t.Fatalf("stat renamed destination: %v", err)
				}
			},
		},
		{
			name:       "list",
			tool:       &ListTool{sb: sb},
			input:      map[string]any{"path": root},
			wantStatus: StatusSuccess,
			wantOutput: "source.txt",
		},
		{
			name:       "delete",
			tool:       &DeleteTool{sb: sb},
			input:      map[string]any{"path": seedFile(t, root, "delete-me.txt", "bye")},
			wantStatus: StatusSuccess,
			wantOutput: "Deleted",
			assertFS: func(t *testing.T) {
				if _, err := os.Stat(filepath.Join(root, "delete-me.txt")); !os.IsNotExist(err) {
					t.Fatalf("deleted file still exists or stat failed unexpectedly: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := tt.tool.Call(ctx, tt.input)
			if err != nil {
				t.Fatalf("Call() error = %v", err)
			}
			if res.Status != tt.wantStatus {
				t.Fatalf("Status = %v, want %v; error=%q", res.Status, tt.wantStatus, res.Error)
			}
			if !strings.Contains(res.Output, tt.wantOutput) {
				t.Fatalf("Output = %q, want to contain %q", res.Output, tt.wantOutput)
			}
			if tt.assertFS != nil {
				tt.assertFS(t)
			}
		})
	}
}

func TestBuiltinToolsMissingRequiredInput(t *testing.T) {
	sb := newBuiltinTestSandbox(t)

	tests := []struct {
		name      string
		tool      Tool
		input     map[string]any
		wantError string
	}{
		{name: "read path", tool: &ReadTool{sb: sb}, input: map[string]any{}, wantError: "missing required input: path"},
		{name: "write path", tool: &WriteTool{sb: sb}, input: map[string]any{}, wantError: "missing required input: path"},
		{name: "delete path", tool: &DeleteTool{sb: sb}, input: map[string]any{}, wantError: "missing required input: path"},
		{name: "mkdir path", tool: &MkdirTool{sb: sb}, input: map[string]any{}, wantError: "missing required input: path"},
		{name: "copy path", tool: &CopyTool{sb: sb}, input: map[string]any{}, wantError: "missing required input: path"},
		{name: "copy destination", tool: &CopyTool{sb: sb}, input: map[string]any{"path": "a.txt"}, wantError: "missing required input: destination"},
		{name: "move path", tool: &MoveTool{sb: sb}, input: map[string]any{}, wantError: "missing required input: path"},
		{name: "move destination", tool: &MoveTool{sb: sb}, input: map[string]any{"path": "a.txt"}, wantError: "missing required input: destination"},
		{name: "rename path", tool: &RenameTool{sb: sb}, input: map[string]any{}, wantError: "missing required input: path"},
		{name: "rename destination", tool: &RenameTool{sb: sb}, input: map[string]any{"path": "a.txt"}, wantError: "missing required input: destination"},
		{name: "list path", tool: &ListTool{sb: sb}, input: map[string]any{}, wantError: "missing required input: path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := tt.tool.Call(context.Background(), tt.input)
			if err != nil {
				t.Fatalf("Call() error = %v", err)
			}
			if res.Status != StatusFailed {
				t.Fatalf("Status = %v, want StatusFailed", res.Status)
			}
			if res.Error != tt.wantError {
				t.Fatalf("Error = %q, want %q", res.Error, tt.wantError)
			}
		})
	}
}

func seedFile(t *testing.T, root, name, content string) string {
	t.Helper()

	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("seed file %q: %v", name, err)
	}
	return path
}
