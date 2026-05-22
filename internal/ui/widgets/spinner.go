package widgets

import (
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/halukerenozlu/bolt-cowork/internal/ui/theme"
)

// Spinner shows an animated "thinking" indicator while the agent is running.
type Spinner struct {
	sp      spinner.Model
	message string
}

// NewSpinner creates a Spinner with the given status message.
func NewSpinner(message string) Spinner {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = theme.TitleStyle
	return Spinner{sp: sp, message: message}
}

func (s Spinner) Init() tea.Cmd { return s.sp.Tick }

func (s Spinner) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	s.sp, cmd = s.sp.Update(msg)
	return s, cmd
}

func (s Spinner) View() string {
	return s.sp.View() + " " + s.message
}
