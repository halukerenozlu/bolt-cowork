package widgets

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/halukerenozlu/bolt-cowork/internal/ui/theme"
)

// PaletteCommand is a single command entry shown in the palette.
type PaletteCommand struct {
	Name string
	Desc string
}

// DefaultCommands is the built-in command list.
var DefaultCommands = []PaletteCommand{
	{"/clear", "Clear chat history"},
	{"/model", "Show current model"},
	{"/dir", "Show workspace directory"},
	{"/approval", "Show approval mode"},
	{"/help", "Show help"},
	{"/quit", "Quit"},
}

// PaletteSelectMsg is emitted when the user selects a command.
type PaletteSelectMsg struct{ Command string }

// PaletteCloseMsg is emitted when the user dismisses the palette without selecting.
type PaletteCloseMsg struct{}

// Palette is the command palette overlay widget.
type Palette struct {
	input    textinput.Model
	commands []PaletteCommand
	filtered []PaletteCommand
	cursor   int
	width    int // terminal width, used to size the overlay
}

// NewPalette returns a focused Palette sized for the given terminal width.
func NewPalette(width int) Palette {
	ti := textinput.New()
	ti.Placeholder = ""
	ti.Focus()
	ti.CharLimit = 64

	cmds := make([]PaletteCommand, len(DefaultCommands))
	copy(cmds, DefaultCommands)

	return Palette{
		input:    ti,
		commands: cmds,
		filtered: cmds,
		width:    width,
	}
}

func (p Palette) Init() tea.Cmd { return textinput.Blink }

func (p Palette) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			return p, func() tea.Msg { return PaletteCloseMsg{} }
		case tea.KeyEnter:
			if len(p.filtered) > 0 {
				sel := p.filtered[p.cursor].Name
				return p, func() tea.Msg { return PaletteSelectMsg{Command: sel} }
			}
			return p, func() tea.Msg { return PaletteCloseMsg{} }
		case tea.KeyUp:
			if p.cursor > 0 {
				p.cursor--
			}
			return p, nil
		case tea.KeyDown:
			if p.cursor < len(p.filtered)-1 {
				p.cursor++
			}
			return p, nil
		}
	}

	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)

	filter := strings.ToLower(strings.TrimSpace(p.input.Value()))
	var next []PaletteCommand
	for _, c := range p.commands {
		if filter == "" || strings.Contains(strings.ToLower(c.Name), filter) {
			next = append(next, c)
		}
	}
	p.filtered = next
	if p.cursor >= len(p.filtered) {
		p.cursor = max(0, len(p.filtered)-1)
	}
	return p, cmd
}

func (p Palette) View() string {
	const minW = 24
	paletteW := 48
	if p.width > 0 && p.width-4 < paletteW {
		paletteW = p.width - 4
	}
	if paletteW < minW {
		paletteW = minW
	}
	// boxInnerW is the Width() arg for lipgloss (excluding border, including padding).
	boxInnerW := paletteW - 2
	// textW is the usable text width inside the padding chars.
	textW := boxInnerW - 2
	textW = max(textW, 1)

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(theme.Primary)
	selectedStyle := lipgloss.NewStyle().
		Background(theme.Primary).
		Foreground(lipgloss.Color("255")).
		Width(textW)

	var sb strings.Builder
	sb.WriteString("> " + p.input.View() + "\n")
	sb.WriteString(strings.Repeat("─", textW) + "\n")

	if len(p.filtered) == 0 {
		sb.WriteString(theme.MutedStyle.Render("no matching commands") + "\n")
	} else {
		for i, c := range p.filtered {
			line := fmt.Sprintf("%-14s %s", c.Name, c.Desc)
			if len(line) > textW {
				line = line[:textW]
			}
			if i == p.cursor {
				sb.WriteString(selectedStyle.Render(line) + "\n")
			} else {
				sb.WriteString(line + "\n")
			}
		}
	}

	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(theme.Primary).
		Padding(0, 1).
		Width(boxInnerW).
		Render(titleStyle.Render("COMMANDS") + "\n" + sb.String())
}
