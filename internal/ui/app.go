package ui

import (
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/halukerenozlu/bolt-cowork/internal/config"
	sessionstore "github.com/halukerenozlu/bolt-cowork/internal/session"
	"github.com/halukerenozlu/bolt-cowork/internal/ui/views"
	"github.com/halukerenozlu/bolt-cowork/pkg/types"
)

// userHomeDir is overridden in tests so session storage never touches the
// real ~/.bolt-cowork directory.
var userHomeDir = os.UserHomeDir

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
	sessions   *sessionstore.Store
}

// New creates an App ready to be started with Run.
func New(cfg *config.Config, version string, runner views.AgentRunner, configPath ...string) *App {
	app := &App{cfg: cfg, version: version, runner: runner}
	if runner.Workspace != "" {
		if home, err := userHomeDir(); err == nil {
			if dir, err := sessionstore.DirForWorkspace(home, runner.Workspace); err == nil {
				migrateLegacySessions(runner.Workspace, dir)
				app.sessions = sessionstore.NewStore(dir, time.Now)
			}
		}
	}
	if len(configPath) > 0 {
		app.configPath = configPath[0]
	}
	return app
}

// Run starts the bubbletea program in alternate-screen mode. It blocks until
// the user quits.
func (a *App) Run() error {
	a.current = a.newWelcome()
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
	if m, ok := msg.(views.RuntimeModelChangedMsg); ok {
		a.runner.Provider = m.Provider
		a.runner.Model = m.Model
		a.cfg.DefaultProvider = m.Provider
		if len(a.cfg.FallbackChain) == 0 {
			a.cfg.FallbackChain = []config.FallbackEntry{{Provider: m.Provider, Model: m.Model}}
		} else {
			a.cfg.FallbackChain[0].Provider = m.Provider
			a.cfg.FallbackChain[0].Model = m.Model
		}
		if a.configPath != "" {
			if err := config.SaveFilePreservingSecrets(a.cfg, a.configPath); err != nil {
				a.recordSessionError(err)
			}
		}
		if welcome, ok := a.current.(views.Welcome); ok {
			a.current = welcome.SetRuntimeModel(m.Provider, m.Model)
		}
		return a, nil
	}
	if m, ok := msg.(views.SaveSessionMsg); ok {
		a.saveSnapshot(m.Snapshot)
		if current, ok := a.current.(views.Session); ok {
			a.current = current.ApplySnapshot(m.Snapshot)
		}
		a.refreshCurrentSummaries()
		return a, nil
	}
	if m, ok := msg.(views.OpenSessionMsg); ok {
		return a.openSession(m.ID)
	}
	if m, ok := msg.(views.CreateSessionMsg); ok {
		a.saveCurrentSession()
		return a.createBlankSession(m.Title)
	}
	if m, ok := msg.(views.RenameSessionMsg); ok {
		if a.sessions != nil {
			if err := a.sessions.Rename(m.ID, m.Title); err != nil {
				a.recordSessionError(err)
			} else if current, ok := a.current.(views.Session); ok {
				snapshot := current.Snapshot()
				if snapshot.ID == m.ID {
					snapshot.Title = m.Title
					a.current = current.ApplySnapshot(snapshot)
				}
			}
		}
		a.refreshCurrentSummaries()
		return a, nil
	}
	if m, ok := msg.(views.DeleteSessionMsg); ok {
		if a.sessions != nil {
			if err := a.sessions.Delete(m.ID); err != nil {
				a.recordSessionError(err)
			}
		}
		if current, ok := a.current.(views.Session); ok && current.Snapshot().ID == m.ID {
			welcome := a.newWelcome()
			seeded, sizeCmd := welcome.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
			a.current = seeded
			return a, tea.Batch(sizeCmd, seeded.Init())
		}
		a.refreshCurrentSummaries()
		return a, nil
	}
	if m, ok := msg.(views.StartSessionMsg); ok {
		a.saveCurrentSession()
		id, title := "", m.Input
		if a.sessions != nil {
			record, err := a.sessions.Create(m.Input, a.runner.Provider, a.runner.Model)
			if err != nil {
				a.recordSessionError(err)
			} else {
				id, title = record.ID, record.Title
			}
		}
		session := views.NewSession(
			a.cfg, a.version, m.Input, a.runner,
			views.WithConfigPath(a.configPath),
			views.WithSessionState(id, title, a.sessionSummaries(id)),
		)
		// Seed the current terminal dimensions so Session.View() renders
		// immediately without requiring a subsequent tea.WindowSizeMsg.
		seeded, sizeCmd := session.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
		a.current = seeded
		// Call Init on the new session to start the spinner and first agent run.
		initCmd := seeded.Init()
		return a, tea.Batch(sizeCmd, initCmd)
	}
	if _, ok := msg.(views.ReturnToWelcomeMsg); ok {
		welcome := a.newWelcome()
		seeded, sizeCmd := welcome.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
		a.current = seeded
		initCmd := seeded.Init()
		return a, tea.Batch(sizeCmd, initCmd)
	}
	var cmd tea.Cmd
	a.current, cmd = a.current.Update(msg)
	return a, cmd
}

func (a *App) newWelcome() views.Welcome {
	return views.NewWelcome(a.cfg, a.version).
		SetRuntimeModel(a.runner.Provider, a.runner.Model).
		SetSessionSummaries(a.sessionSummaries(""))
}

func (a *App) createBlankSession(title string) (tea.Model, tea.Cmd) {
	id := ""
	if a.sessions != nil {
		record, err := a.sessions.Create(title, a.runner.Provider, a.runner.Model)
		if err != nil {
			a.recordSessionError(err)
		} else {
			id = record.ID
			title = record.Title
		}
	}
	snapshot := views.SessionSnapshot{
		ID: id, Title: title, Provider: a.runner.Provider, Model: a.runner.Model,
	}
	session := views.NewSession(
		a.cfg, a.version, "", a.runner,
		views.WithConfigPath(a.configPath),
		views.WithRestoredSnapshot(snapshot),
		views.WithSessionState(id, title, a.sessionSummaries(id)),
	)
	seeded, sizeCmd := session.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
	a.current = seeded
	return a, tea.Batch(sizeCmd, seeded.Init())
}

func (a *App) saveCurrentSession() {
	current, ok := a.current.(views.Session)
	if !ok {
		return
	}
	a.saveSnapshot(current.Snapshot())
}

func (a *App) saveSnapshot(snapshot views.SessionSnapshot) {
	if a.sessions == nil || snapshot.ID == "" {
		return
	}
	record := &sessionstore.Record{
		ID:          snapshot.ID,
		Title:       snapshot.Title,
		Provider:    snapshot.Provider,
		Model:       snapshot.Model,
		History:     append([]types.Message(nil), snapshot.History...),
		TokenCount:  snapshot.TokenCount,
		TokenBytes:  snapshot.TokenBytes,
		SessionCost: snapshot.SessionCost,
	}
	for _, message := range snapshot.Messages {
		record.Messages = append(record.Messages, sessionstore.DisplayMessage{
			Role: message.Role,
			Text: message.Text,
		})
	}
	if existing, err := a.sessions.Load(snapshot.ID); err == nil {
		record.CreatedAt = existing.CreatedAt
	}
	if err := a.sessions.Save(record); err != nil {
		a.recordSessionError(err)
	}
}

func (a *App) sessionSummaries(activeID string) []views.SessionSummary {
	if a.sessions == nil {
		return nil
	}
	summaries, err := a.sessions.List()
	if err != nil {
		a.recordSessionError(err)
		return nil
	}
	result := make([]views.SessionSummary, 0, len(summaries))
	for _, summary := range summaries {
		result = append(result, views.SessionSummary{
			ID: summary.ID, Title: summary.Title, UpdatedAt: summary.UpdatedAt,
			Active: summary.ID == activeID,
		})
	}
	return result
}

func (a *App) refreshCurrentSummaries() {
	current, ok := a.current.(views.Session)
	if !ok {
		return
	}
	snapshot := current.Snapshot()
	a.current = current.SetSessionSummaries(a.sessionSummaries(snapshot.ID))
}

func (a *App) openSession(id string) (tea.Model, tea.Cmd) {
	if a.sessions == nil {
		return a, nil
	}
	a.saveCurrentSession()
	record, err := a.sessions.Load(id)
	if err != nil {
		a.recordSessionError(err)
		return a, nil
	}
	a.runner.Provider = record.Provider
	a.runner.Model = record.Model
	snapshot := views.SessionSnapshot{
		ID: record.ID, Title: record.Title, Provider: record.Provider, Model: record.Model,
		History: record.History, TokenCount: record.TokenCount,
		TokenBytes: record.TokenBytes, SessionCost: record.SessionCost,
	}
	for _, message := range record.Messages {
		snapshot.Messages = append(snapshot.Messages, views.SessionMessage{
			Role: message.Role, Text: message.Text,
		})
	}
	session := views.NewSession(
		a.cfg, a.version, "", a.runner,
		views.WithConfigPath(a.configPath),
		views.WithRestoredSnapshot(snapshot),
		views.WithSessionState(record.ID, record.Title, a.sessionSummaries(record.ID)),
	)
	seeded, sizeCmd := session.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
	a.current = seeded
	return a, tea.Batch(sizeCmd, seeded.Init())
}

func (a *App) recordSessionError(err error) {
	if err == nil {
		return
	}
	if current, ok := a.current.(views.Session); ok {
		a.current = current.AddNotice("Session storage error: " + err.Error())
	}
}

// migrateLegacySessions performs a one-time, best-effort move of session
// records from the old per-project location (<workspace>/.cowork/sessions)
// to the new global location (<home>/.bolt-cowork/sessions/<project-key>).
// It is a no-op once the new directory exists or no legacy data is found.
func migrateLegacySessions(workspace, newDir string) {
	oldDir := filepath.Join(workspace, ".cowork", "sessions")
	if _, err := os.Stat(oldDir); err != nil {
		return
	}
	if _, err := os.Stat(newDir); err == nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(newDir), 0o700); err != nil {
		return
	}
	if err := os.Rename(oldDir, newDir); err == nil {
		return
	}
	// Cross-device rename failed; fall back to a copy-then-remove.
	entries, err := os.ReadDir(oldDir)
	if err != nil {
		return
	}
	if err := os.MkdirAll(newDir, 0o700); err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(oldDir, entry.Name()))
		if err != nil {
			continue
		}
		_ = os.WriteFile(filepath.Join(newDir, entry.Name()), data, 0o600)
	}
	os.RemoveAll(oldDir)
}

// View implements tea.Model.
func (a *App) View() string {
	if a.current == nil {
		return ""
	}
	return a.current.View()
}
