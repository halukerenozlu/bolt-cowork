package views

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/halukerenozlu/bolt-cowork/internal/config"
	"github.com/halukerenozlu/bolt-cowork/internal/ui/theme"
)

// StartSessionMsg is emitted when the user submits their first message on the
// welcome screen. App.Update catches it and switches to the session view.
type StartSessionMsg struct{ Input string }

// Welcome is the bubbletea model for the startup screen shown before any
// message is sent.
type Welcome struct {
	input     textinput.Model
	width     int
	height    int
	workDir   string
	gitBranch string
	provider  string
	version   string
}

// NewWelcome creates an initialised Welcome model.
func NewWelcome(cfg *config.Config, version string) Welcome {
	ti := textinput.New()
	ti.Placeholder = "Ask anything..."
	ti.CharLimit = 500
	ti.Width = 52
	ti.Focus()

	workDir := "."
	if len(cfg.Sandbox.AllowedDirs) > 0 && cfg.Sandbox.AllowedDirs[0] != "" {
		workDir = cfg.Sandbox.AllowedDirs[0]
	}
	abs, err := filepath.Abs(workDir)
	if err != nil {
		abs = workDir
	}

	provider := cfg.DefaultProvider
	if len(cfg.FallbackChain) > 0 {
		provider = cfg.FallbackChain[0].Provider
	}

	return Welcome{
		input:     ti,
		workDir:   abs,
		gitBranch: getGitBranch(abs),
		provider:  provider,
		version:   version,
	}
}

// getGitBranch reads the git branch for the given directory, returning "" if
// git is unavailable or the directory is not inside a repository.
func getGitBranch(workDir string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (w Welcome) Init() tea.Cmd {
	return textinput.Blink
}

func (w Welcome) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		w.width = msg.Width
		w.height = msg.Height
		return w, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return w, tea.Quit
		case tea.KeyEnter:
			val := strings.TrimSpace(w.input.Value())
			if val == "" {
				return w, nil
			}
			return w, func() tea.Msg { return StartSessionMsg{Input: val} }
		}
		// 'q' on empty input → quit (vim-style)
		if msg.String() == "q" && w.input.Value() == "" {
			return w, tea.Quit
		}
	}

	var cmd tea.Cmd
	w.input, cmd = w.input.Update(msg)
	return w, cmd
}

func (w Welcome) View() string {
	if w.width == 0 || w.height == 0 {
		return ""
	}

	title := welcomeLogo(w.width)
	info := theme.MutedStyle.Render(
		fmt.Sprintf("dir: %s  |  provider: %s", w.workDir, w.provider),
	)

	center := lipgloss.JoinVertical(
		lipgloss.Center,
		title,
		"",
		w.input.View(),
		"",
		info,
	)

	mainArea := lipgloss.Place(w.width, w.height-1, lipgloss.Center, lipgloss.Center, center)

	// Bottom status bar: working dir + git branch on left, version on right.
	left := " " + w.workDir
	if w.gitBranch != "" {
		left += " [" + w.gitBranch + "]"
	}
	right := " " + w.version + " "
	gap := w.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}
	statusBar := theme.MutedStyle.Render(left + strings.Repeat(" ", gap) + right)

	return mainArea + "\n" + statusBar
}

func welcomeLogo(width int) string {
	boltStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#bfe7ff"))
	zapStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#ff8a1c"))
	coworkStyle := lipgloss.NewStyle().
		Italic(true).
		Foreground(lipgloss.Color("#f7fbff"))

	if width < 78 {
		return lipgloss.JoinHorizontal(
			lipgloss.Center,
			boltStyle.Render("BOLT "),
			zapStyle.Render("⚡ "),
			coworkStyle.Render("Cowork"),
		)
	}

	lines := []string{
		boltStyle.Render("██████  ██████  ██   ████████  ") + coworkStyle.Render("Cowork"),
		boltStyle.Render("██   ██ ██  ██  ██      ██"),
		boltStyle.Render("██████  ██ ") + zapStyle.Render("⚡") + boltStyle.Render(" ██  ██      ██"),
		boltStyle.Render("██   ██ ██  ██  ██      ██"),
		boltStyle.Render("██████  ██████  ██████  ██"),
	}

	return strings.Join(lines, "\n")
}
