package widgets

import tea "github.com/charmbracelet/bubbletea"

// Approval is a placeholder for the interactive approval widget used during
// plan and execution approval gates.
type Approval struct{}

func (a Approval) Init() tea.Cmd                           { return nil }
func (a Approval) Update(msg tea.Msg) (tea.Model, tea.Cmd) { return a, nil }
func (a Approval) View() string                             { return "" }
