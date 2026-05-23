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
	Name     string // identifier sent in PaletteSelectMsg
	Label    string // display text
	Shortcut string // keyboard shortcut hint, empty if none
	group    string // group membership (assigned from defaultGroups)
}

// paletteGroup holds a named category of commands.
type paletteGroup struct {
	Title    string
	Commands []PaletteCommand
}

// filteredGroup is a subset of a group after search filtering.
type filteredGroup struct {
	title string
	cmds  []PaletteCommand
}

// defaultGroups defines the grouped command list shown in the palette.
var defaultGroups = []paletteGroup{
	{
		Title: "Suggested",
		Commands: []PaletteCommand{
			{Name: "switch-session", Label: "Switch session", Shortcut: "ctrl+x l"},
			{Name: "switch-model", Label: "Switch model", Shortcut: "ctrl+x m"},
			{Name: "connect-provider", Label: "Connect provider"},
		},
	},
	{
		Title: "Session",
		Commands: []PaletteCommand{
			{Name: "open-editor", Label: "Open editor", Shortcut: "ctrl+x e"},
			{Name: "new-session", Label: "New session", Shortcut: "ctrl+x n"},
		},
	},
	{
		Title: "Prompt",
		Commands: []PaletteCommand{
			{Name: "skills", Label: "Skills"},
		},
	},
	{
		Title: "System",
		Commands: []PaletteCommand{
			{Name: "hide-tips", Label: "Hide tips", Shortcut: "ctrl+x h"},
			{Name: "view-status", Label: "View status", Shortcut: "ctrl+x s"},
			{Name: "switch-theme", Label: "Switch theme", Shortcut: "ctrl+x t"},
			{Name: "/clear", Label: "Clear chat"},
			{Name: "/model", Label: "Show model"},
			{Name: "/dir", Label: "Show directory"},
			{Name: "/approval", Label: "Show approval"},
			{Name: "/help", Label: "Show help"},
			{Name: "/quit", Label: "Quit"},
		},
	},
}

// DefaultCommands is the flat list of all built-in commands (for testing).
var DefaultCommands []PaletteCommand

func init() {
	for i := range defaultGroups {
		for j := range defaultGroups[i].Commands {
			defaultGroups[i].Commands[j].group = defaultGroups[i].Title
		}
		DefaultCommands = append(DefaultCommands, defaultGroups[i].Commands...)
	}
}

// PaletteSelectMsg is emitted when the user selects a command.
type PaletteSelectMsg struct{ Command string }

// PaletteCloseMsg is emitted when the user dismisses the palette.
type PaletteCloseMsg struct{}

// Palette is the command palette overlay widget. Its View() returns only the
// modal box; centering over the background is the caller's responsibility.
type Palette struct {
	input        textinput.Model
	allCmds      []PaletteCommand // all commands (flat, immutable)
	filteredFlat []PaletteCommand // search-filtered commands (flat, for cursor)
	filteredGrps []filteredGroup  // search-filtered commands (grouped, for render)
	cursor       int
	width        int // terminal width, used to constrain modal size
}

// NewPalette returns a focused Palette sized for the given terminal width.
func NewPalette(width int) Palette {
	ti := textinput.New()
	ti.Placeholder = ""
	ti.Prompt = ""
	ti.Focus()
	ti.CharLimit = 64

	cmds := make([]PaletteCommand, len(DefaultCommands))
	copy(cmds, DefaultCommands)

	p := Palette{
		input:   ti,
		allCmds: cmds,
		width:   width,
	}
	p.filteredFlat, p.filteredGrps = buildFiltered(cmds, "")
	return p
}

func (p Palette) Init() tea.Cmd { return textinput.Blink }

func (p Palette) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			return p, func() tea.Msg { return PaletteCloseMsg{} }
		case tea.KeyEnter:
			if len(p.filteredFlat) > 0 {
				sel := p.filteredFlat[p.cursor].Name
				return p, func() tea.Msg { return PaletteSelectMsg{Command: sel} }
			}
			return p, func() tea.Msg { return PaletteCloseMsg{} }
		case tea.KeyUp:
			if p.cursor > 0 {
				p.cursor--
			}
			return p, nil
		case tea.KeyDown:
			if p.cursor < len(p.filteredFlat)-1 {
				p.cursor++
			}
			return p, nil
		}
	}

	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)

	filter := strings.ToLower(strings.TrimSpace(p.input.Value()))
	p.filteredFlat, p.filteredGrps = buildFiltered(p.allCmds, filter)
	if p.cursor >= len(p.filteredFlat) {
		p.cursor = max(0, len(p.filteredFlat)-1)
	}
	return p, cmd
}

// buildFiltered produces filtered-flat and filtered-grouped views from allCmds.
func buildFiltered(all []PaletteCommand, filter string) ([]PaletteCommand, []filteredGroup) {
	var flat []PaletteCommand
	var grps []filteredGroup
	var curGrp *filteredGroup

	for _, c := range all {
		if filter != "" &&
			!strings.Contains(strings.ToLower(c.Name), filter) &&
			!strings.Contains(strings.ToLower(c.Label), filter) {
			continue
		}
		flat = append(flat, c)
		if curGrp == nil || curGrp.title != c.group {
			grps = append(grps, filteredGroup{title: c.group})
			curGrp = &grps[len(grps)-1]
		}
		curGrp.cmds = append(curGrp.cmds, c)
	}
	return flat, grps
}

// View renders the palette as a standalone modal box.
// It does NOT position itself; use overlayCenter in session.go to float it
// over the background session view.
func (p Palette) View() string {
	// Modal outer width (including border chars).
	paletteOuterW := 64
	if p.width > 0 && p.width-4 < paletteOuterW {
		paletteOuterW = p.width - 4
	}
	if paletteOuterW < 24 {
		paletteOuterW = 24
	}
	// boxInnerW: Width() arg for lipgloss (= outer - 2 border chars).
	boxInnerW := paletteOuterW - 2
	// textW: usable text width inside 1-char padding on each side.
	textW := max(boxInnerW-2, 8)

	mutedStyle := lipgloss.NewStyle().Foreground(theme.Muted)
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(theme.Primary)
	groupStyle := lipgloss.NewStyle().Bold(true).Foreground(theme.Muted)
	selectedBg := lipgloss.NewStyle().
		Background(theme.Primary).
		Foreground(lipgloss.Color("255"))

	var sb strings.Builder

	// Title bar row: "Commands" left, "esc" right.
	esc := mutedStyle.Render("esc")
	title := titleStyle.Render("Commands")
	gapW := max(textW-lipgloss.Width(title)-lipgloss.Width(esc), 1)
	sb.WriteString(title + strings.Repeat(" ", gapW) + esc + "\n")
	sb.WriteString(strings.Repeat("─", textW) + "\n")

	// Search input row.
	p.input.Width = max(textW-2, 1)
	sb.WriteString("> " + p.input.View() + "\n")
	sb.WriteString(strings.Repeat("─", textW) + "\n")

	// Command list.
	if len(p.filteredFlat) == 0 {
		sb.WriteString(mutedStyle.Render("no matching commands") + "\n")
	} else {
		for gi, grp := range p.filteredGrps {
			if gi > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString(groupStyle.Render(grp.title) + "\n")
			for _, c := range grp.cmds {
				selected := p.cursor < len(p.filteredFlat) &&
					p.filteredFlat[p.cursor].Name == c.Name
				sb.WriteString(p.renderCmdLine(c, textW, selected, selectedBg) + "\n")
			}
		}
	}

	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(theme.Primary).
		Padding(0, 1).
		Width(boxInnerW).
		Render(sb.String())
}

// renderCmdLine formats a single command row: "▶ Label       shortcut".
func (p Palette) renderCmdLine(c PaletteCommand, textW int, selected bool, selStyle lipgloss.Style) string {
	const indW = 2
	const shortW = 12

	ind := "  "
	if selected {
		ind = "▶ "
	}

	var line string
	if c.Shortcut != "" && textW > indW+shortW+4 {
		labelW := textW - indW - shortW
		label := paletteLeft(c.Label, labelW)
		sc := fmt.Sprintf("%-*s", shortW, c.Shortcut)
		line = ind + label + sc
	} else {
		line = ind + c.Label
	}
	if lipgloss.Width(line) > textW {
		line = paletteTruncate(line, textW)
	}

	if selected {
		return selStyle.Width(textW).Render(line)
	}
	return line
}

// paletteLeft returns s padded (or truncated) to exactly w visible columns.
func paletteLeft(s string, w int) string {
	sw := lipgloss.Width(s)
	if sw >= w {
		return paletteTruncate(s, w)
	}
	return s + strings.Repeat(" ", w-sw)
}

// paletteTruncate truncates s to at most w visible columns.
func paletteTruncate(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	const ellipsis = "…"
	limit := w - lipgloss.Width(ellipsis)
	var b strings.Builder
	for _, r := range s {
		next := b.String() + string(r)
		if lipgloss.Width(next) > limit {
			break
		}
		b.WriteRune(r)
	}
	return b.String() + ellipsis
}
