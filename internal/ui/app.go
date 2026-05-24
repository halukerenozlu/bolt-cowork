package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/halukerenozlu/bolt-cowork/internal/config"
	"github.com/halukerenozlu/bolt-cowork/internal/ui/views"
)

// App is the root bubbletea model. It owns the current view and handles
// switching from the welcome screen to the session view when the user sends
// their first message.
type App struct {
	cfg        *config.Config
	configPath string
	version    string
	current    tea.Model
	width      int
	height     int
	runner     views.AgentRunner
}

// New creates an App ready to be started with Run.
func New(cfg *config.Config, version string, runner views.AgentRunner, configPath ...string) *App {
	app := &App{cfg: cfg, version: version, runner: runner}
	if len(configPath) > 0 {
		app.configPath = configPath[0]
	}
	return app
}

// Run starts the bubbletea program in alternate-screen mode. It blocks until
// the user quits.
func (a *App) Run() error {
	a.current = views.NewWelcome(a.cfg, a.version)
	p := tea.NewProgram(a, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// Init implements tea.Model.
func (a *App) Init() tea.Cmd {
	if a.current == nil {
		return nil
	}
	return a.current.Init()
}

// Update implements tea.Model. It intercepts StartSessionMsg to swap the
// current view from Welcome to Session; all other messages are delegated.
// tea.WindowSizeMsg is also stored so the dimensions can be seeded into any
// newly created view immediately, without waiting for a subsequent resize.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		a.width = sz.Width
		a.height = sz.Height
	}
	if m, ok := msg.(views.StartSessionMsg); ok {
		session := views.NewSession(a.cfg, a.version, m.Input, a.runner, views.WithConfigPath(a.configPath))
		// Seed the current terminal dimensions so Session.View() renders
		// immediately without requiring a subsequent tea.WindowSizeMsg.
		seeded, sizeCmd := session.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
		a.current = seeded
		// Call Init on the new session to start the spinner and first agent run.
		initCmd := seeded.Init()
		return a, tea.Batch(sizeCmd, initCmd)
	}
	var cmd tea.Cmd
	a.current, cmd = a.current.Update(msg)
	return a, cmd
}

// View implements tea.Model.
func (a *App) View() string {
	if a.current == nil {
		return ""
	}
	return a.current.View()
}
