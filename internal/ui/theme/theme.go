package theme

import "github.com/charmbracelet/lipgloss"

var (
	Primary = lipgloss.AdaptiveColor{Light: "#0066cc", Dark: "#88c0ff"}
	Muted   = lipgloss.AdaptiveColor{Light: "#666666", Dark: "#888888"}
	Border  = lipgloss.AdaptiveColor{Light: "#cccccc", Dark: "#444444"}

	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(Primary)

	MutedStyle = lipgloss.NewStyle().
			Foreground(Muted)

	BorderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Border)
)
