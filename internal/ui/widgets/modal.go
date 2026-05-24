package widgets

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/halukerenozlu/bolt-cowork/internal/ui/theme"
)

// ModalItem is one selectable row inside a command modal.
type ModalItem struct {
	Label string
	Hint  string
}

// ModalCloseMsg is emitted when a modal is dismissed.
type ModalCloseMsg struct{}

// ModalSelectMsg is emitted when a modal row is selected.
type ModalSelectMsg struct {
	Label string
	Value string
}

// Modal renders a palette-style overlay with an input and selectable list.
type Modal struct {
	title       string
	input       textinput.Model
	items       []ModalItem
	filtered    []ModalItem
	cursor      int
	width       int
	filterItems bool
}

// NewModal creates a searchable command modal.
func NewModal(title string, items []ModalItem, width int) Modal {
	ti := textinput.New()
	ti.Placeholder = "Search..."
	ti.Prompt = ""
	ti.Focus()
	ti.CharLimit = 96

	m := Modal{
		title:       title,
		input:       ti,
		items:       append([]ModalItem(nil), items...),
		width:       width,
		filterItems: true,
	}
	m.filtered = filterModalItems(m.items, "")
	return m
}

// NewInputModal creates a command modal whose input captures free text.
func NewInputModal(title, placeholder string, items []ModalItem, width int) Modal {
	m := NewModal(title, items, width)
	m.input.Placeholder = placeholder
	m.filterItems = false
	return m
}

func (m Modal) Init() tea.Cmd { return textinput.Blink }

func (m Modal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			return m, func() tea.Msg { return ModalCloseMsg{} }
		case tea.KeyEnter:
			if len(m.filtered) > 0 {
				item := m.filtered[m.cursor]
				value := strings.TrimSpace(m.input.Value())
				return m, func() tea.Msg { return ModalSelectMsg{Label: item.Label, Value: value} }
			}
			return m, func() tea.Msg { return ModalCloseMsg{} }
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case tea.KeyDown:
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.filterItems {
		m.filtered = filterModalItems(m.items, strings.ToLower(strings.TrimSpace(m.input.Value())))
		if m.cursor >= len(m.filtered) {
			m.cursor = max(0, len(m.filtered)-1)
		}
	}
	return m, cmd
}

func (m Modal) View() string {
	outerW := 64
	if m.width > 0 && m.width-4 < outerW {
		outerW = m.width - 4
	}
	if outerW < 24 {
		outerW = 24
	}
	boxInnerW := outerW - 2
	textW := max(boxInnerW-2, 8)

	mutedStyle := lipgloss.NewStyle().Foreground(theme.Muted)
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(theme.Primary)
	selectedBg := lipgloss.NewStyle().
		Background(theme.Primary).
		Foreground(lipgloss.Color("255"))

	var sb strings.Builder
	esc := mutedStyle.Render("esc")
	title := titleStyle.Render(m.title)
	gapW := max(textW-lipgloss.Width(title)-lipgloss.Width(esc), 1)
	sb.WriteString(title + strings.Repeat(" ", gapW) + esc + "\n")
	sb.WriteString(strings.Repeat("-", textW) + "\n")

	m.input.Width = max(textW-2, 1)
	sb.WriteString("> " + m.input.View() + "\n")
	sb.WriteString(strings.Repeat("-", textW) + "\n")

	if len(m.filtered) == 0 {
		sb.WriteString(mutedStyle.Render("no matching items") + "\n")
	} else {
		for i, item := range m.filtered {
			sb.WriteString(renderModalItem(item, textW, i == m.cursor, selectedBg) + "\n")
		}
	}

	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(theme.Primary).
		Padding(0, 1).
		Width(boxInnerW).
		Render(sb.String())
}

func filterModalItems(items []ModalItem, filter string) []ModalItem {
	if filter == "" {
		return append([]ModalItem(nil), items...)
	}
	var out []ModalItem
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.Label), filter) ||
			strings.Contains(strings.ToLower(item.Hint), filter) {
			out = append(out, item)
		}
	}
	return out
}

func renderModalItem(item ModalItem, textW int, selected bool, selStyle lipgloss.Style) string {
	const indW = 2
	const hintW = 18

	ind := "  "
	if selected {
		ind = "> "
	}

	var line string
	if item.Hint != "" && textW > indW+hintW+4 {
		labelW := textW - indW - hintW
		label := paletteLeft(item.Label, labelW)
		hint := fmt.Sprintf("%-*s", hintW, item.Hint)
		line = ind + label + hint
	} else {
		line = ind + item.Label
	}
	if lipgloss.Width(line) > textW {
		line = paletteTruncate(line, textW)
	}
	if selected {
		return selStyle.Width(textW).Render(line)
	}
	return line
}
