package panels

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/halukerenozlu/bolt-cowork/internal/ui/theme"
)

// StatusBar renders the bottom bar showing the working directory, git branch,
// and version string.
type StatusBar struct {
	width     int
	workDir   string
	gitBranch string
	version   string
}

// NewStatusBar creates a StatusBar with the given metadata.
func NewStatusBar(workDir, gitBranch, version string) StatusBar {
	return StatusBar{workDir: workDir, gitBranch: gitBranch, version: version}
}

func (sb StatusBar) Init() tea.Cmd                           { return nil }
func (sb StatusBar) Update(msg tea.Msg) (tea.Model, tea.Cmd) { return sb, nil }

func (sb StatusBar) View() string {
	left := fmt.Sprintf(" %s", sb.workDir)
	if sb.gitBranch != "" {
		left += fmt.Sprintf(" [%s]", sb.gitBranch)
	}
	right := fmt.Sprintf(" %s ", sb.version)
	gap := sb.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}
	return theme.MutedStyle.Render(left + strings.Repeat(" ", gap) + right)
}
