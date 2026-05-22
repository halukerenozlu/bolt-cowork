package panels

import tea "github.com/charmbracelet/bubbletea"

// Status is the right panel placeholder for displaying agent status information.
type Status struct{}

func (s Status) Init() tea.Cmd                           { return nil }
func (s Status) Update(msg tea.Msg) (tea.Model, tea.Cmd) { return s, nil }
func (s Status) View() string                             { return "" }
