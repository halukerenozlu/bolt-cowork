package widgets

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewPalette_defaults(t *testing.T) {
	p := NewPalette(80)
	if len(p.commands) != len(DefaultCommands) {
		t.Fatalf("expected %d commands, got %d", len(DefaultCommands), len(p.commands))
	}
	if len(p.filtered) != len(DefaultCommands) {
		t.Fatalf("expected all commands visible initially, got %d", len(p.filtered))
	}
	if p.cursor != 0 {
		t.Fatalf("expected cursor at 0, got %d", p.cursor)
	}
}

func TestPalette_filterByName(t *testing.T) {
	p := NewPalette(80)

	// Type "cl" → only /clear should match.
	p, _ = updateKey(p, "c")
	p, _ = updateKey(p, "l")

	if len(p.filtered) != 1 {
		t.Fatalf("expected 1 filtered command, got %d: %v", len(p.filtered), p.filtered)
	}
	if p.filtered[0].Name != "/clear" {
		t.Errorf("expected /clear, got %q", p.filtered[0].Name)
	}
}

func TestPalette_filterNoMatch(t *testing.T) {
	p := NewPalette(80)
	p, _ = updateKey(p, "z")
	p, _ = updateKey(p, "z")
	p, _ = updateKey(p, "z")

	if len(p.filtered) != 0 {
		t.Errorf("expected 0 filtered commands, got %d", len(p.filtered))
	}
	if p.cursor != 0 {
		t.Errorf("expected cursor clamped to 0, got %d", p.cursor)
	}
}

func TestPalette_cursorNavigation(t *testing.T) {
	p := NewPalette(80)
	n := len(p.filtered)

	m, _ := p.Update(tea.KeyMsg{Type: tea.KeyDown})
	p = m.(Palette)
	if p.cursor != 1 {
		t.Errorf("expected cursor 1 after Down, got %d", p.cursor)
	}

	// Down past the end should clamp.
	for range n + 5 {
		m, _ = p.Update(tea.KeyMsg{Type: tea.KeyDown})
		p = m.(Palette)
	}
	if p.cursor != n-1 {
		t.Errorf("expected cursor clamped to %d, got %d", n-1, p.cursor)
	}

	// Up past the start should clamp.
	for range n + 5 {
		m, _ = p.Update(tea.KeyMsg{Type: tea.KeyUp})
		p = m.(Palette)
	}
	if p.cursor != 0 {
		t.Errorf("expected cursor clamped to 0, got %d", p.cursor)
	}
}

func TestPalette_enterEmitsSelectMsg(t *testing.T) {
	p := NewPalette(80)
	// Select the first entry (/clear).
	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a cmd from Enter, got nil")
	}
	msg := cmd()
	sel, ok := msg.(PaletteSelectMsg)
	if !ok {
		t.Fatalf("expected PaletteSelectMsg, got %T", msg)
	}
	if sel.Command != "/clear" {
		t.Errorf("expected /clear, got %q", sel.Command)
	}
}

func TestPalette_enterOnEmptyEmitsCloseMsg(t *testing.T) {
	p := NewPalette(80)
	// Filter to nothing.
	p, _ = updateKey(p, "z")
	p, _ = updateKey(p, "z")
	p, _ = updateKey(p, "z")
	if len(p.filtered) != 0 {
		t.Skip("filter didn't clear commands, skipping")
	}
	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a cmd, got nil")
	}
	msg := cmd()
	if _, ok := msg.(PaletteCloseMsg); !ok {
		t.Fatalf("expected PaletteCloseMsg, got %T", msg)
	}
}

func TestPalette_escEmitsCloseMsg(t *testing.T) {
	p := NewPalette(80)
	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected a cmd from Esc, got nil")
	}
	msg := cmd()
	if _, ok := msg.(PaletteCloseMsg); !ok {
		t.Fatalf("expected PaletteCloseMsg, got %T", msg)
	}
}

func TestPalette_viewRendersNonEmpty(t *testing.T) {
	p := NewPalette(80)
	v := p.View()
	if len(v) == 0 {
		t.Fatal("View() returned empty string")
	}
	// Spot-check that command names appear somewhere in the rendered output.
	for _, c := range DefaultCommands {
		if !contains(v, c.Name) {
			t.Errorf("command %q not found in palette view", c.Name)
		}
	}
}

// updateKey simulates typing a single character rune into the palette.
func updateKey(p Palette, r string) (Palette, tea.Cmd) {
	m, cmd := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(r)})
	return m.(Palette), cmd
}

// contains reports whether substr appears in s (ANSI-stripped search not
// needed here since command names are plain ASCII without styling).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := range len(s) - len(substr) + 1 {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
