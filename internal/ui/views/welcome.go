package views

import (
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/halukerenozlu/bolt-cowork/internal/config"
	"github.com/halukerenozlu/bolt-cowork/internal/ui/theme"
	"github.com/halukerenozlu/bolt-cowork/internal/ui/widgets"
)

// StartSessionMsg is emitted when the user submits their first message on the
// welcome screen. App.Update catches it and switches to the session view.
type StartSessionMsg struct{ Input string }

// Welcome is the bubbletea model for the startup screen shown before any
// message is sent.
type Welcome struct {
	cfg       *config.Config
	input     textinput.Model
	width     int
	height    int
	workDir   string
	gitBranch string
	provider  string
	model     string
	version   string

	palette          widgets.Palette
	paletteOpen      bool
	modal            widgets.Modal
	modalOpen        bool
	modalCommand     string
	modelItems       []widgets.ModalItem
	providers        []widgets.ModalItem
	sessionSummaries []SessionSummary
}

// NewWelcome creates an initialised Welcome model.
func NewWelcome(cfg *config.Config, version string) Welcome {
	ti := textinput.New()
	ti.Placeholder = "Ask anything..."
	ti.CharLimit = 500
	ti.Width = 52
	ti.Focus()

	workDir := "."
	if len(cfg.Sandbox.AllowedDirs) > 0 && cfg.Sandbox.AllowedDirs[0] != "" {
		workDir = cfg.Sandbox.AllowedDirs[0]
	}
	abs, err := filepath.Abs(workDir)
	if err != nil {
		abs = workDir
	}

	provider := cfg.DefaultProvider
	model := ""
	if len(cfg.FallbackChain) > 0 {
		provider = cfg.FallbackChain[0].Provider
		model = cfg.FallbackChain[0].Model
	} else if pc, ok := cfg.Providers[provider]; ok && len(pc.Models) > 0 {
		model = pc.Models[0]
	}
	runner := AgentRunner{Provider: provider, Model: model, Workspace: abs, ApprovalMode: cfg.ApprovalMode}

	return Welcome{
		cfg:        cfg,
		input:      ti,
		workDir:    abs,
		gitBranch:  getGitBranch(abs),
		provider:   provider,
		model:      model,
		version:    version,
		modelItems: modelModalItems(cfg, runner),
		providers:  providerModalItems(cfg, runner),
	}
}

func (w Welcome) SetSessionSummaries(summaries []SessionSummary) Welcome {
	w.sessionSummaries = append([]SessionSummary(nil), summaries...)
	return w
}

func (w Welcome) SetRuntimeModel(provider, model string) Welcome {
	w.provider = provider
	w.model = model
	runner := AgentRunner{Provider: provider, Model: model, Workspace: w.workDir}
	w.modelItems = modelModalItems(w.cfg, runner)
	w.providers = providerModalItems(w.cfg, runner)
	return w
}

// getGitBranch reads the git branch for the given directory, returning "" if
// git is unavailable or the directory is not inside a repository.
func getGitBranch(workDir string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (w Welcome) Init() tea.Cmd {
	return textinput.Blink
}

func (w Welcome) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		w.width = msg.Width
		w.height = msg.Height
		return w, nil

	case tea.KeyMsg:
		if w.modalOpen {
			m, cmd := w.modal.Update(msg)
			w.modal = m.(widgets.Modal)
			return w, cmd
		}
		if msg.Type == tea.KeyCtrlP {
			if w.paletteOpen {
				w.paletteOpen = false
				w.input.Focus()
			} else {
				w.paletteOpen = true
				w.palette = widgets.NewPalette(w.width)
				return w, w.palette.Init()
			}
			return w, nil
		}
		if w.paletteOpen {
			m, cmd := w.palette.Update(msg)
			w.palette = m.(widgets.Palette)
			return w, cmd
		}
		switch msg.Type {
		case tea.KeyCtrlC:
			return w, tea.Quit
		case tea.KeyEnter:
			val := strings.TrimSpace(w.input.Value())
			if val == "" {
				return w, nil
			}
			return w, func() tea.Msg { return StartSessionMsg{Input: val} }
		}
		// 'q' on empty input → quit (vim-style)
		if msg.String() == "q" && w.input.Value() == "" {
			return w, tea.Quit
		}

	case widgets.PaletteSelectMsg:
		w.paletteOpen = false
		w.input.Focus()
		if msg.Command == "/quit" {
			return w, tea.Quit
		}
		if msg.Command != "/clear" {
			modal := w.commandModal(msg.Command)
			w.modal = modal
			w.modalOpen = true
			w.modalCommand = msg.Command
			return w, modal.Init()
		}
		return w, nil

	case widgets.PaletteCloseMsg:
		w.paletteOpen = false
		w.input.Focus()
		return w, nil

	case widgets.ModalSelectMsg:
		w.modalOpen = false
		w.input.Focus()
		switch w.modalCommand {
		case "switch-session":
			if msg.Label == "+ New session" {
				modal := widgets.NewInputModal("New session", "Session name...", []widgets.ModalItem{
					{Label: "Create session", Hint: "enter"},
					{Label: "Cancel", Hint: "esc"},
				}, w.width)
				w.modal = modal
				w.modalOpen = true
				w.modalCommand = "new-session"
				return w, modal.Init()
			}
			if msg.Key != "" {
				return w, func() tea.Msg { return OpenSessionMsg{ID: msg.Key} }
			}
		case "new-session":
			if msg.Label != "Cancel" {
				title := strings.TrimSpace(msg.Value)
				if title == "" {
					title = "New session"
				}
				return w, func() tea.Msg { return CreateSessionMsg{Title: title} }
			}
		case "switch-model":
			if msg.Label != "" {
				provider, err := (Session{
					cfg:    w.cfg,
					runner: AgentRunner{Provider: w.provider, Model: w.model},
				}).providerForModel(msg.Label)
				if err == nil {
					w = w.SetRuntimeModel(provider, msg.Label)
					return w, func() tea.Msg {
						return RuntimeModelChangedMsg{Provider: provider, Model: msg.Label}
					}
				}
			}
		}
		w.modalCommand = ""
		return w, nil

	case widgets.ModalCloseMsg:
		w.modalOpen = false
		w.input.Focus()
		return w, nil
	}

	var cmd tea.Cmd
	w.input, cmd = w.input.Update(msg)
	return w, cmd
}

func (w Welcome) View() string {
	if w.width == 0 || w.height == 0 {
		return ""
	}

	title := welcomeLogo(w.width)
	inputBlock := w.inputBlock()

	center := lipgloss.JoinVertical(
		lipgloss.Center,
		title,
		"",
		inputBlock,
	)

	mainArea := lipgloss.Place(w.width, w.height-1, lipgloss.Center, lipgloss.Center, center)

	// Bottom status bar: working dir + git branch on left, version on right.
	left := " " + w.workDir
	if w.gitBranch != "" {
		left += " [" + w.gitBranch + "]"
	}
	right := " " + w.version + " "
	gap := w.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}
	statusBar := theme.MutedStyle.Render(left + strings.Repeat(" ", gap) + right)

	view := mainArea + "\n" + statusBar
	if w.paletteOpen {
		view = overlayCenter(view, w.palette.View(), w.width, w.height)
	}
	if w.modalOpen {
		return overlayCenter(view, w.modal.View(), w.width, w.height)
	}
	return view
}

func (w Welcome) commandModal(name string) widgets.Modal {
	switch name {
	case "switch-session":
		return widgets.NewModal("Switch session", sessionModalItemsAt(Session{
			sessionSummaries: w.sessionSummaries,
		}, time.Now()), w.width)
	case "switch-model":
		return widgets.NewModal("Switch model", w.modelItems, w.width)
	case "connect-provider":
		return widgets.NewModal("Connect provider", w.providers, w.width)
	case "open-editor":
		return widgets.NewModal("Open editor", []widgets.ModalItem{
			{Label: "VS Code", Hint: "code"},
			{Label: "Cursor", Hint: "cursor"},
			{Label: "Notepad", Hint: "notepad"},
			{Label: "Vim", Hint: "vim"},
		}, w.width)
	case "new-session":
		return widgets.NewInputModal("New session", "Session name...", []widgets.ModalItem{
			{Label: "Create session", Hint: "enter"},
			{Label: "Cancel", Hint: "esc"},
		}, w.width)
	case "skills":
		return widgets.NewModal("Skills", []widgets.ModalItem{{Label: "Skills load after session starts"}}, w.width)
	case "hide-tips":
		return widgets.NewModal("Hide tips", []widgets.ModalItem{
			{Label: "Tips visible", Hint: "current"},
			{Label: "Tips hidden", Hint: "toggle"},
		}, w.width)
	case "view-status":
		return widgets.NewModal("View status", []widgets.ModalItem{
			{Label: "Provider: " + w.provider, Hint: "runtime"},
			{Label: "Model: " + w.model, Hint: "runtime"},
			{Label: "Workspace: " + w.workDir, Hint: "dir"},
		}, w.width)
	case "switch-theme":
		return widgets.NewModal("Switch theme", []widgets.ModalItem{
			{Label: "System", Hint: "default"},
			{Label: "Dark", Hint: "terminal"},
			{Label: "Light", Hint: "terminal"},
		}, w.width)
	case "/model":
		return widgets.NewModal("Show model", []widgets.ModalItem{{Label: w.model, Hint: "current model"}}, w.width)
	case "/dir":
		return widgets.NewModal("Show directory", []widgets.ModalItem{{Label: w.workDir, Hint: "workspace"}}, w.width)
	case "/approval":
		return widgets.NewModal("Show approval", approvalModalItems(""), w.width)
	case "/help":
		return widgets.NewModal("Show help", helpModalItems(), w.width)
	default:
		return widgets.NewModal("Command", []widgets.ModalItem{{Label: name}}, w.width)
	}
}

func (w Welcome) inputBlock() string {
	const inputFrameWidth = 56

	input := w.input
	input.Width = max(inputFrameWidth-lipgloss.Width(input.Prompt), 1)

	frameStyle := lipgloss.NewStyle().
		Width(inputFrameWidth).
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("#88c0ff"))

	inputLine := xansi.Truncate(input.View(), inputFrameWidth, "")
	frame := frameStyle.Render(inputLine)
	frameWidth := inputFrameWidth + 2
	if firstLine, _, ok := strings.Cut(frame, "\n"); ok {
		frameWidth = lipgloss.Width(firstLine)
	}

	provider := theme.MutedStyle.Render("provider: " + w.provider)
	commands := theme.MutedStyle.Render("ctrl+p Commands")
	gap := frameWidth - lipgloss.Width(provider) - lipgloss.Width(commands)
	if gap < 1 {
		gap = 1
	}
	meta := provider + strings.Repeat(" ", gap) + commands

	return lipgloss.JoinVertical(lipgloss.Left, frame, meta)
}

func welcomeLogo(width int) string {
	boltStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#bfe7ff"))

	if width < 78 {
		return boltStyle.Render("BOLT Cowork")
	}

	lines := []string{
		"  ██████╗  ██████╗ ██╗  ████████╗",
		"  ██╔══██╗██╔═══██╗██║  ╚══██╔══╝",
		"  ██████╔╝██║   ██║██║     ██║",
		"  ██╔══██╗██║   ██║██║     ██║   C o w o r k",
		"  ██████╔╝╚██████╔╝███████╗██║",
		"  ╚═════╝  ╚═════╝ ╚══════╝╚═╝",
	}

	return boltStyle.Render(strings.Join(lines, "\n"))
}
