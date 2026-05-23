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
		"The following file types will never be accessed:\n" +
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

// TrustModal is a bubbletea model that asks the user to trust the working
// directory before starting a session.
type TrustModal struct {
	dir       string
	cursor    int
	width     int
	height    int
	trusted   bool
	declined  bool
	errMsg    string
	trustFunc func(dir string) error
}

// NewTrustModal creates a TrustModal for dir. trustFunc is called with the
// directory path when the user chooses to trust it.
func NewTrustModal(dir string, trustFunc func(dir string) error) TrustModal {
	return TrustModal{
		dir:       dir,
		trustFunc: trustFunc,
	}
}

// IsTrusted reports whether the user chose to trust the directory.
func (t TrustModal) IsTrusted() bool { return t.trusted }

// IsDeclined reports whether the user chose to exit.
func (t TrustModal) IsDeclined() bool { return t.declined }

func (t TrustModal) Init() tea.Cmd { return nil }

func (t TrustModal) acceptTrust() (TrustModal, tea.Cmd) {
	t.errMsg = ""
	if t.trustFunc != nil {
		if err := t.trustFunc(t.dir); err != nil {
			t.errMsg = fmt.Sprintf("Could not save trusted directory: %v", err)
			return t, nil
		}
	}
	t.trusted = true
	return t, tea.Quit
}

func (t TrustModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.width = msg.Width
		t.height = msg.Height
		return t, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			t.declined = true
			return t, tea.Quit

		case tea.KeyUp:
			if t.cursor > 0 {
				t.cursor--
			}
			return t, nil

		case tea.KeyDown:
			if t.cursor < 1 {
				t.cursor++
			}
			return t, nil

		case tea.KeyEnter:
			if t.cursor == 0 {
				return t.acceptTrust()
			}
			t.declined = true
			return t, tea.Quit

		case tea.KeyRunes:
			ch := string(msg.Runes)
			if ch == "1" || ch == "y" || ch == "Y" {
				return t.acceptTrust()
			}
			if ch == "2" || ch == "n" || ch == "N" {
				t.declined = true
				return t, tea.Quit
			}
		}
	}
	return t, nil
}

func (t TrustModal) View() string {
	if t.width == 0 {
		return ""
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#bfe7ff"))
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#88c0ff"))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff6b6b"))

	title := titleStyle.Render("Trust this directory?")

	dirLine := t.dir

	subtitle := mutedStyle.Render("bolt-cowork will be able to\nread, edit, and execute files here.")

	options := [2]string{
		"  1. Yes, I trust this folder",
		"  2. No, exit",
	}
	for i := range options {
		if i == t.cursor {
			options[i] = selectedStyle.Render("> " + options[i][2:])
		}
	}

	body := lipgloss.JoinVertical(lipgloss.Left,
		title, "", dirLine, "", subtitle, "",
		options[0], options[1], "",
		mutedStyle.Render("↑/↓ select, Enter confirm"),
	)

	if t.errMsg != "" {
		body = lipgloss.JoinVertical(lipgloss.Left, body, "", errorStyle.Render(t.errMsg))
	}

	return lipgloss.Place(t.width, t.height, lipgloss.Center, lipgloss.Center, body)
}
