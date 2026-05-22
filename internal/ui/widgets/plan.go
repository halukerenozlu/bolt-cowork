package widgets

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
)

// Plan is a placeholder for the plan step viewer widget. In v0.4.1+ it will
// render agent plan steps as styled markdown using glamour.
type Plan struct{ Content string }

// glam renders markdown for plan step display (initialised once at startup).
// glamErr captures any initialisation failure so View() can fall back to plain
// text rather than silently discarding content.
var (
	glam    *glamour.TermRenderer
	glamErr error
)

func init() {
	glam, glamErr = glamour.NewTermRenderer(glamour.WithAutoStyle())
}

func (p Plan) Init() tea.Cmd                           { return nil }
func (p Plan) Update(msg tea.Msg) (tea.Model, tea.Cmd) { return p, nil }
func (p Plan) View() string {
	if p.Content == "" {
		return ""
	}
	if glamErr != nil || glam == nil {
		return p.Content
	}
	out, err := glam.Render(p.Content)
	if err != nil {
		return p.Content
	}
	return out
}
