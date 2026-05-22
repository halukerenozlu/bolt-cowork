package widgets

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/halukerenozlu/bolt-cowork/internal/ui/theme"
)

// Plan is the glamour-based markdown plan widget used for raw markdown content.
type Plan struct{ Content string }

// glam renders markdown for plan display (initialised once at startup).
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

// PlanWidget renders plan steps with live checkbox state using lipgloss borders.
// Steps are shown as numbered lines with [ ], [✓], or [✗] indicators.
type PlanWidget struct {
	steps      []string
	done       []bool
	stepErrors []error
	width      int // inner content width of the enclosing panel
}

// NewPlanWidget creates a PlanWidget for the given steps and panel content width.
func NewPlanWidget(steps []string, done []bool, errs []error, panelW int) PlanWidget {
	return PlanWidget{steps: steps, done: done, stepErrors: errs, width: panelW}
}

var (
	planTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.Primary)
	planBoxStyle   = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(theme.Border).
			Padding(0, 1)
	checkOKStyle   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#00aa00", Dark: "#44ff88"})
	checkFailStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#cc0000", Dark: "#ff6666"})
	checkWaitStyle = lipgloss.NewStyle().Foreground(theme.Muted)
)

// View renders the plan widget as a titled lipgloss box.
func (pw PlanWidget) View() string {
	if len(pw.steps) == 0 {
		return ""
	}

	// Inner box width: panel content width minus border chars and outer indent.
	boxW := pw.width - 4
	if boxW < 10 {
		boxW = 10
	}

	var lines []string
	for i, step := range pw.steps {
		var check string
		if i < len(pw.done) && pw.done[i] {
			if i < len(pw.stepErrors) && pw.stepErrors[i] != nil {
				check = checkFailStyle.Render("[✗]")
			} else {
				check = checkOKStyle.Render("[✓]")
			}
		} else {
			check = checkWaitStyle.Render("[ ]")
		}
		lines = append(lines, fmt.Sprintf("  %d. %s %s", i+1, check, step))
	}

	body := strings.Join(lines, "\n")
	box := planBoxStyle.Width(boxW).Render(body)
	title := planTitleStyle.Render("PLAN")
	return title + "\n" + box
}
