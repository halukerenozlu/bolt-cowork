package panels

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPlaceholderPanels(t *testing.T) {
	tests := []struct {
		name  string
		model tea.Model
	}{
		{name: "chat", model: Chat{}},
		{name: "status", model: Status{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if cmd := tt.model.Init(); cmd != nil {
				t.Fatal("Init() returned a command, want nil")
			}
			next, cmd := tt.model.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if cmd != nil {
				t.Fatal("Update() returned a command, want nil")
			}
			if next.View() != "" {
				t.Fatalf("View() = %q, want empty string", next.View())
			}
		})
	}
}

func TestInputPanel(t *testing.T) {
	input := NewInput()

	if input.Init() == nil {
		t.Fatal("Init() returned nil command, want textinput blink command")
	}
	if !strings.Contains(input.View(), "Type a command") {
		t.Fatalf("View() = %q, want placeholder", input.View())
	}

	next, _ := input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("run")})
	got := next.(Input).View()
	if !strings.Contains(got, "run") {
		t.Fatalf("View() after typing = %q, want typed value", got)
	}
}

func TestStatusBarView(t *testing.T) {
	tests := []struct {
		name     string
		bar      StatusBar
		contains []string
	}{
		{
			name:     "with branch",
			bar:      StatusBar{width: 40, workDir: "C:/repo", gitBranch: "main", version: "v1"},
			contains: []string{"C:/repo", "[main]", "v1"},
		},
		{
			name:     "without branch",
			bar:      NewStatusBar("C:/repo", "", "v1"),
			contains: []string{"C:/repo", "v1"},
		},
		{
			name:     "narrow width",
			bar:      StatusBar{width: 4, workDir: "C:/repo", gitBranch: "main", version: "v1"},
			contains: []string{"C:/repo", "v1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if cmd := tt.bar.Init(); cmd != nil {
				t.Fatal("Init() returned a command, want nil")
			}
			next, cmd := tt.bar.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if cmd != nil {
				t.Fatal("Update() returned a command, want nil")
			}
			view := next.View()
			for _, want := range tt.contains {
				if !strings.Contains(view, want) {
					t.Fatalf("View() = %q, want to contain %q", view, want)
				}
			}
		})
	}
}
