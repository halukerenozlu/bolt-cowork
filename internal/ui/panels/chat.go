package panels

import tea "github.com/charmbracelet/bubbletea"

// Chat is the left panel placeholder for displaying conversation messages.
type Chat struct{}

func (c Chat) Init() tea.Cmd                           { return nil }
func (c Chat) Update(msg tea.Msg) (tea.Model, tea.Cmd) { return c, nil }
func (c Chat) View() string                             { return "" }
