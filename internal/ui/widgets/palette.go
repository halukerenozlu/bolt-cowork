package widgets

import tea "github.com/charmbracelet/bubbletea"

// Palette is a placeholder for the command palette overlay widget.
type Palette struct{}

func (p Palette) Init() tea.Cmd                           { return nil }
func (p Palette) Update(msg tea.Msg) (tea.Model, tea.Cmd) { return p, nil }
func (p Palette) View() string                             { return "" }
