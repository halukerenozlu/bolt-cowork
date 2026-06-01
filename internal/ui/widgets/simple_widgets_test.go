package widgets

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestApprovalWidgetContract(t *testing.T) {
	approval := Approval{}

	if cmd := approval.Init(); cmd != nil {
		t.Fatal("Init() returned a command, want nil")
	}
	next, cmd := approval.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("Update() returned a command, want nil")
	}
	if next.View() != "" {
		t.Fatalf("View() = %q, want empty string", next.View())
	}
}

func TestSpinnerViewIncludesMessage(t *testing.T) {
	spinner := NewSpinner("thinking")

	if spinner.Init() == nil {
		t.Fatal("Init() returned nil command, want tick command")
	}
	next, _ := spinner.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(next.View(), "thinking") {
		t.Fatalf("View() = %q, want message", next.View())
	}
}
