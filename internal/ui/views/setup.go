package views

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SetupCompleteMsg is emitted when the setup wizard finishes successfully.
// The caller (tea.Program) checks the final model via IsComplete.
type SetupCompleteMsg struct{}

// SetupWarningError lets saveFunc surface a warning while still completing
// setup successfully.
type SetupWarningError struct {
	Message string
}

func (e SetupWarningError) Error() string { return e.Message }

// setupProvider is an entry in the provider selection list.
type setupProvider struct {
	name     string // display name shown to the user
	key      string // internal config key (lowercase)
	topModel string // default model for that provider
}

var setupProviders = []setupProvider{
	{"Anthropic", "anthropic", "claude-sonnet-4-6"},
	{"OpenAI", "openai", "gpt-4o"},
	{"Gemini", "gemini", "gemini-2.5-pro"},
}

// Setup is the bubbletea model for the initial configuration wizard.
// Step 0: provider selection (cursor-based list).
// Step 1: API key entry (masked textinput).
type Setup struct {
	step      int
	cursor    int
	input     textinput.Model
	provider  string
	width     int
	height    int
	errMsg    string
	completed bool
	saveFunc  func(provider, apiKey string) error
}

// NewSetup creates an initialised Setup model. saveFunc is called with the
// selected provider key and API key when the user completes the wizard.
func NewSetup(saveFunc func(provider, apiKey string) error) Setup {
	ti := textinput.New()
	ti.Placeholder = "Enter API key..."
	ti.EchoMode = textinput.EchoPassword
	ti.EchoCharacter = '•'
	ti.Width = 44

	return Setup{
		input:    ti,
		saveFunc: saveFunc,
	}
}

// IsComplete reports whether setup finished successfully.
func (s Setup) IsComplete() bool { return s.completed }

func (s Setup) Init() tea.Cmd { return nil }

func (s Setup) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.width = msg.Width
		s.height = msg.Height
		return s, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return s, tea.Quit

		case tea.KeyUp:
			if s.step == 0 && s.cursor > 0 {
				s.cursor--
			}
			return s, nil

		case tea.KeyDown:
			if s.step == 0 && s.cursor < len(setupProviders)-1 {
				s.cursor++
			}
			return s, nil

		case tea.KeyEnter:
			if s.step == 0 {
				s.provider = setupProviders[s.cursor].key
				s.step = 1
				s.errMsg = ""
				s.input.Focus()
				return s, textinput.Blink
			}
			// Step 1: validate and save.
			apiKey := strings.TrimSpace(s.input.Value())
			if apiKey == "" {
				s.errMsg = "API key cannot be empty."
				return s, nil
			}
			if s.saveFunc != nil {
				if err := s.saveFunc(s.provider, apiKey); err != nil {
					var warning SetupWarningError
					if errors.As(err, &warning) {
						s.errMsg = warning.Message
						s.completed = true
						return s, tea.Quit
					}
					s.errMsg = "Save error: " + err.Error()
					return s, nil
				}
			}
			s.completed = true
			return s, tea.Quit
		}
	}

	if s.step == 1 {
		var cmd tea.Cmd
		s.input, cmd = s.input.Update(msg)
		return s, cmd
	}
	return s, nil
}

func (s Setup) View() string {
	if s.width == 0 {
		return ""
	}

	if s.completed {
		text := "✓ Setup complete. Loading..."
		if s.errMsg != "" {
			text += "\n" + s.errMsg
		}
		msg := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#50fa7b")).
			Render(text)
		return lipgloss.Place(s.width, s.height, lipgloss.Center, lipgloss.Center, msg)
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#bfe7ff"))
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#88c0ff"))
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555"))

	title := titleStyle.Render("bolt-cowork — Initial Setup")

	var body string
	if s.step == 0 {
		var rows []string
		rows = append(rows, mutedStyle.Render("STEP 1/2 — Provider Selection"))
		rows = append(rows, "")
		for i, p := range setupProviders {
			label := fmt.Sprintf("  %s  (%s)", p.name, p.topModel)
			if i == s.cursor {
				label = selectedStyle.Render("> " + label[2:])
			}
			rows = append(rows, label)
		}
		rows = append(rows, "")
		rows = append(rows, mutedStyle.Render("↑/↓ select, Enter confirm"))
		body = lipgloss.JoinVertical(lipgloss.Left, append([]string{title, ""}, rows...)...)
	} else {
		stepHint := mutedStyle.Render("STEP 2/2 — API Key Entry")
		provLine := "Provider: " + s.provider
		body = lipgloss.JoinVertical(lipgloss.Left,
			title, "", stepHint, "", provLine, "", s.input.View(),
		)
	}

	if s.errMsg != "" {
		body = lipgloss.JoinVertical(lipgloss.Left, body, "", errStyle.Render(s.errMsg))
	}

	securityNote := mutedStyle.Render(
		"These file types will never be accessed:\n" +
			".env .key .pem .ssh/* *credentials* *secrets* and others",
	)

	mainH := s.height - lipgloss.Height(securityNote) - 1
	if mainH < 1 {
		mainH = 1
	}

	main := lipgloss.Place(s.width, mainH, lipgloss.Center, lipgloss.Center, body)
	footer := lipgloss.Place(s.width, lipgloss.Height(securityNote)+1, lipgloss.Center, lipgloss.Bottom, securityNote)

	return main + "\n" + footer
}
