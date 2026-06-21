package views

import (
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/halukerenozlu/bolt-cowork/internal/config"
	"github.com/halukerenozlu/bolt-cowork/internal/ui/widgets"
)

func TestWelcomeLogo(t *testing.T) {
	tests := []struct {
		name      string
		width     int
		want      []string
		notWant   []string
		wantLines int
	}{
		{
			name:      "compact",
			width:     60,
			want:      []string{"BOLT", "Cowork"},
			notWant:   []string{"██████"},
			wantLines: 1,
		},
		{
			name:      "wide",
			width:     90,
			want:      []string{"██████╗", "C o w o r k", "╚═════╝"},
			notWant:   []string{"BOLT ⚡ Cowork", "⚡"},
			wantLines: 6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lipgloss.NewStyle().Render(welcomeLogo(tt.width))
			plain := stripANSI(got)

			for _, want := range tt.want {
				if !strings.Contains(plain, want) {
					t.Fatalf("welcomeLogo(%d) missing %q:\n%s", tt.width, want, plain)
				}
			}
			for _, notWant := range tt.notWant {
				if strings.Contains(plain, notWant) {
					t.Fatalf("welcomeLogo(%d) unexpectedly contains %q:\n%s", tt.width, notWant, plain)
				}
			}
			if lines := strings.Count(plain, "\n") + 1; lines != tt.wantLines {
				t.Fatalf("welcomeLogo(%d) lines = %d, want %d:\n%s", tt.width, lines, tt.wantLines, plain)
			}
		})
	}
}

func TestWelcomeViewInputMetadata(t *testing.T) {
	cfg := config.Default()
	cfg.Sandbox.AllowedDirs = []string{t.TempDir()}
	cfg.FallbackChain = []config.FallbackEntry{{Provider: "anthropic", Model: "claude-sonnet-4-6"}}

	model, _ := NewWelcome(cfg, "dev").Update(tea.WindowSizeMsg{Width: 120, Height: 32})
	view := model.(Welcome).View()
	plain := stripANSI(view)

	if strings.Contains(plain, "dir:") {
		t.Fatalf("welcome view still shows duplicated dir metadata:\n%s", plain)
	}
	for _, want := range []string{"provider: anthropic", "ctrl+p Commands", "Ask anything..."} {
		if !strings.Contains(plain, want) {
			t.Fatalf("welcome view missing %q:\n%s", want, plain)
		}
	}
}

func TestWelcomeCtrlPOpensPalette(t *testing.T) {
	cfg := config.Default()
	cfg.Sandbox.AllowedDirs = []string{t.TempDir()}

	model, _ := NewWelcome(cfg, "dev").Update(tea.WindowSizeMsg{Width: 120, Height: 32})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	plain := stripANSI(model.View())

	for _, want := range []string{"Commands", "Suggested", "Switch session"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("welcome palette missing %q:\n%s", want, plain)
		}
	}
}

func TestWelcomePaletteCommandOpensModal(t *testing.T) {
	cfg := config.Default()
	cfg.Sandbox.AllowedDirs = []string{t.TempDir()}
	cfg.FallbackChain = []config.FallbackEntry{{Provider: "anthropic", Model: "claude-sonnet-4-6"}}

	model, _ := NewWelcome(cfg, "dev").Update(tea.WindowSizeMsg{Width: 120, Height: 32})
	model, _ = model.Update(widgets.PaletteSelectMsg{Command: "/model"})
	got := model.(Welcome)

	if !got.modalOpen {
		t.Fatal("model command should open a modal on welcome")
	}
	plain := stripANSI(got.View())
	for _, want := range []string{"Show model", "Search...", "claude-sonnet-4-6"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("welcome modal missing %q:\n%s", want, plain)
		}
	}
}

func TestWelcomeSwitchSessionListsAndOpensSavedSession(t *testing.T) {
	cfg := config.Default()
	w := NewWelcome(cfg, "dev").
		SetSessionSummaries([]SessionSummary{{
			ID:        "saved-id",
			Title:     "20 MB'dan büyük dosyaları listeleme",
			UpdatedAt: time.Now(),
		}})
	model, _ := w.Update(tea.WindowSizeMsg{Width: 120, Height: 32})
	w = model.(Welcome)
	model, _ = w.Update(widgets.PaletteSelectMsg{Command: "switch-session"})
	w = model.(Welcome)

	if view := stripANSI(w.View()); !strings.Contains(view, "20 MB'dan büyük dosyaları listeleme") {
		t.Fatalf("switch session modal missing saved session:\n%s", view)
	}
	_, cmd := w.Update(widgets.ModalSelectMsg{
		Label: "20 MB'dan büyük dosyaları listeleme",
		Key:   "saved-id",
	})
	if cmd == nil {
		t.Fatal("saved session selection returned no command")
	}
	open, ok := cmd().(OpenSessionMsg)
	if !ok || open.ID != "saved-id" {
		t.Fatalf("message = %#v, want OpenSessionMsg", open)
	}
}

func TestWelcomeInputBlockDoesNotWrapLongText(t *testing.T) {
	cfg := config.Default()
	cfg.Sandbox.AllowedDirs = []string{t.TempDir()}

	model, _ := NewWelcome(cfg, "dev").Update(tea.WindowSizeMsg{Width: 120, Height: 32})
	model, _ = model.Update(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune(strings.Repeat("long input ", 12)),
	})
	block := stripANSI(model.(Welcome).inputBlock())

	if lines := strings.Count(block, "\n") + 1; lines != 4 {
		t.Fatalf("input block lines = %d, want 4:\n%s", lines, block)
	}
}

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
