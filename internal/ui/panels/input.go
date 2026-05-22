package panels

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// Input wraps a bubbles textinput for the session command input box.
type Input struct {
	ti textinput.Model
}

// NewInput creates a focused Input panel.
func NewInput() Input {
	ti := textinput.New()
	ti.Placeholder = "Type a command..."
	ti.Focus()
	return Input{ti: ti}
}

func (i Input) Init() tea.Cmd { return textinput.Blink }

func (i Input) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	i.ti, cmd = i.ti.Update(msg)
	return i, cmd
}

func (i Input) View() string { return i.ti.View() }
