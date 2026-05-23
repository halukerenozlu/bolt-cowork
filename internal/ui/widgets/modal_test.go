package widgets

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestModalFiltersItems(t *testing.T) {
	m := NewModal("Models", []ModalItem{
		{Label: "claude-sonnet-4-6", Hint: "current"},
		{Label: "gpt-4o", Hint: "openai"},
	}, 80)

	m, _ = updateModalKey(m, "g")

	if len(m.filtered) != 1 {
		t.Fatalf("filtered items = %d, want 1: %#v", len(m.filtered), m.filtered)
	}
	if m.filtered[0].Label != "gpt-4o" {
		t.Fatalf("filtered label = %q", m.filtered[0].Label)
	}
}

func TestModalEnterEmitsSelectMsg(t *testing.T) {
	m := NewModal("Models", []ModalItem{{Label: "claude-sonnet-4-6"}}, 80)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected select command")
	}
	msg := cmd()
	sel, ok := msg.(ModalSelectMsg)
	if !ok {
		t.Fatalf("message = %T, want ModalSelectMsg", msg)
	}
	if sel.Label != "claude-sonnet-4-6" {
		t.Fatalf("selected label = %q", sel.Label)
	}
}

func TestInputModalDoesNotFilterList(t *testing.T) {
	m := NewInputModal("New session", "Session name...", []ModalItem{{Label: "Create session"}}, 80)

	m, _ = updateModalKey(m, "x")

	if len(m.filtered) != 1 {
		t.Fatalf("input modal filtered items = %d, want 1", len(m.filtered))
	}
}

func updateModalKey(m Modal, r string) (Modal, tea.Cmd) {
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(r)})
	return next.(Modal), cmd
}
