package widgets

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

func TestModalEscEmitsCloseMsg(t *testing.T) {
	m := NewModal("Models", []ModalItem{{Label: "gpt-4o"}}, 80)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected close command")
	}
	if _, ok := cmd().(ModalCloseMsg); !ok {
		t.Fatalf("message = %T, want ModalCloseMsg", cmd())
	}
}

func TestModalEnterWithNoMatchesCloses(t *testing.T) {
	m := NewModal("Models", []ModalItem{{Label: "gpt-4o"}}, 80)
	m, _ = updateModalKey(m, "x")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected close command")
	}
	if _, ok := cmd().(ModalCloseMsg); !ok {
		t.Fatalf("message = %T, want ModalCloseMsg", cmd())
	}
}

func TestModalCursorBounds(t *testing.T) {
	m := NewModal("Models", []ModalItem{
		{Label: "claude-sonnet-4-6"},
		{Label: "gpt-4o"},
	}, 80)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = next.(Modal)
	if m.cursor != 0 {
		t.Fatalf("cursor after key up = %d, want 0", m.cursor)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Modal)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Modal)
	if m.cursor != 1 {
		t.Fatalf("cursor after extra key down = %d, want 1", m.cursor)
	}

	m, _ = updateModalKey(m, "claude")
	if m.cursor != 0 {
		t.Fatalf("cursor after filtering to one item = %d, want 0", m.cursor)
	}
}

func TestModalReplaceItemsPreservesSearchAndSelection(t *testing.T) {
	m := NewModal("Providers", []ModalItem{
		{Label: "Native", Disabled: true},
		{Label: "openai"},
		{Label: "gemini"},
	}, 80)
	m, _ = updateModalKey(m, "g")
	if m.filtered[m.cursor].Label != "gemini" {
		t.Fatalf("selected = %q, want gemini", m.filtered[m.cursor].Label)
	}

	m = m.ReplaceItems([]ModalItem{
		{Label: "Native", Disabled: true},
		{Label: "gemini", Hint: "configured"},
		{Label: "Local", Disabled: true},
		{Label: "groq"},
	})

	if m.input.Value() != "g" {
		t.Fatalf("search = %q, want g", m.input.Value())
	}
	if m.filtered[m.cursor].Label != "gemini" {
		t.Fatalf("selected after refresh = %q, want gemini", m.filtered[m.cursor].Label)
	}
}

func TestModalSelectIncludesInputValue(t *testing.T) {
	m := NewInputModal("New session", "Session name...", []ModalItem{{Label: "Create session"}}, 80)
	m, _ = updateModalKey(m, "  sprint  ")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	msg := cmd()
	sel, ok := msg.(ModalSelectMsg)
	if !ok {
		t.Fatalf("message = %T, want ModalSelectMsg", msg)
	}
	if sel.Value != "sprint" {
		t.Fatalf("selected value = %q, want sprint", sel.Value)
	}
}

func TestModalSessionActionsEmitSelectedItem(t *testing.T) {
	tests := []struct {
		name   string
		key    tea.KeyType
		action string
	}{
		{name: "rename", key: tea.KeyCtrlR, action: "rename"},
		{name: "delete", key: tea.KeyCtrlD, action: "delete"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModal("Sessions", []ModalItem{
				{Label: "Today", Disabled: true},
				{Label: "First session", Key: "abc"},
			}, 80)
			_, cmd := m.Update(tea.KeyMsg{Type: tt.key})
			if cmd == nil {
				t.Fatal("expected action command")
			}
			msg, ok := cmd().(ModalActionMsg)
			if !ok {
				t.Fatalf("message = %T, want ModalActionMsg", cmd())
			}
			if msg.Action != tt.action || msg.Key != "abc" {
				t.Fatalf("action message = %#v", msg)
			}
		})
	}
}

func TestModalViewEmptyAndNarrow(t *testing.T) {
	m := NewModal("Models", []ModalItem{{Label: "gpt-4o"}}, 10)
	m, _ = updateModalKey(m, "x")

	view := m.View()
	for _, want := range []string{"Models", "esc", "no matching items"} {
		if !strings.Contains(view, want) {
			t.Fatalf("View() = %q, want to contain %q", view, want)
		}
	}
}

func TestRenderModalItem(t *testing.T) {
	selectedStyle := lipgloss.NewStyle().Bold(true)

	tests := []struct {
		name     string
		item     ModalItem
		textW    int
		selected bool
		contains string
	}{
		{name: "with hint", item: ModalItem{Label: "gpt-4o", Hint: "openai"}, textW: 32, contains: "openai"},
		{name: "without hint", item: ModalItem{Label: "gpt-4o"}, textW: 12, contains: "gpt-4o"},
		{name: "selected", item: ModalItem{Label: "gpt-4o"}, textW: 12, selected: true, contains: "> "},
		{name: "truncated", item: ModalItem{Label: "very-long-model-name"}, textW: 8, contains: "very-"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderModalItem(tt.item, tt.textW, tt.selected, selectedStyle)
			if !strings.Contains(got, tt.contains) {
				t.Fatalf("renderModalItem() = %q, want to contain %q", got, tt.contains)
			}
		})
	}
}

func updateModalKey(m Modal, r string) (Modal, tea.Cmd) {
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(r)})
	return next.(Modal), cmd
}
