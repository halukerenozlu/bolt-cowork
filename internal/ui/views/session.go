package views

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/halukerenozlu/bolt-cowork/internal/config"
	"github.com/halukerenozlu/bolt-cowork/internal/ui/theme"
)

// Session is the bubbletea model for the active work area. It shows a split
// layout: chat panel on the left (70%) and status panel on the right (30%).
// Agent calls are not wired yet; this is a visual placeholder for v0.4.0.
type Session struct {
	width    int
	height   int
	firstMsg string
	provider string
}

// NewSession creates a Session model seeded with the user's first message.
func NewSession(cfg *config.Config, _ string, firstMsg string) Session {
	provider := cfg.DefaultProvider
	if len(cfg.FallbackChain) > 0 {
		provider = cfg.FallbackChain[0].Provider
	}
	return Session{firstMsg: firstMsg, provider: provider}
}

func (s Session) Init() tea.Cmd { return nil }

func (s Session) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.width = msg.Width
		s.height = msg.Height
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return s, tea.Quit
		}
	}
	return s, nil
}

func (s Session) View() string {
	if s.width == 0 || s.height == 0 {
		return ""
	}

	// Allocate 70% to left panel (total including borders), 30% to right.
	leftTotal := s.width * 7 / 10
	rightTotal := s.width - leftTotal

	// Content width = total − left border (1) − right border (1).
	leftW := leftTotal - 2
	rightW := rightTotal - 2
	if leftW < 1 {
		leftW = 1
	}
	if rightW < 1 {
		rightW = 1
	}

	// Content height = terminal height − top border (1) − bottom border (1).
	panelH := s.height - 2
	if panelH < 1 {
		panelH = 1
	}

	leftStyle := theme.BorderStyle.Width(leftW).Height(panelH)
	rightStyle := theme.BorderStyle.Width(rightW).Height(panelH)

	leftContent := s.firstMsg
	rightContent := fmt.Sprintf("Status\n\n%s", s.provider)

	left := leftStyle.Render(leftContent)
	right := rightStyle.Render(rightContent)

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}
