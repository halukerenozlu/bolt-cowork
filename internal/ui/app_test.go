package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/halukerenozlu/bolt-cowork/internal/config"
	"github.com/halukerenozlu/bolt-cowork/internal/ui/views"
	"github.com/halukerenozlu/bolt-cowork/pkg/types"
)

// withFakeHome points userHomeDir at a temp directory for the duration of
// the test, so session storage never touches the real ~/.bolt-cowork.
func withFakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	original := userHomeDir
	userHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { userHomeDir = original })
	return home
}

func TestApp_ReturnToWelcomeMsgSwitchesCurrentView(t *testing.T) {
	cfg := config.Default()
	app := New(cfg, "v-test", views.AgentRunner{})
	app.current = views.NewSession(cfg, "v-test", "hi", views.AgentRunner{})

	model, cmd := app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	app = model.(*App)
	if cmd != nil {
		t.Fatalf("expected no command for window resize, got %T", cmd)
	}

	model, cmd = app.Update(views.ReturnToWelcomeMsg{})
	got := model.(*App)

	if _, ok := got.current.(views.Welcome); !ok {
		t.Fatalf("current view = %T, want views.Welcome", got.current)
	}
	if cmd == nil {
		t.Fatal("expected welcome init command")
	}
}

func TestApp_NewSessionUsesLatestRuntimeModel(t *testing.T) {
	withFakeHome(t)
	cfg := config.Default()
	cfg.DefaultProvider = "anthropic"
	cfg.FallbackChain = []config.FallbackEntry{{Provider: "anthropic", Model: "claude-opus-4-5"}}
	runner := views.AgentRunner{
		Provider:  "anthropic",
		Model:     "claude-opus-4-5",
		Workspace: t.TempDir(),
		Run: func(context.Context, string, []types.Message, func(string), func(views.UIEvent)) views.AgentResult {
			return views.AgentResult{}
		},
	}
	app := New(cfg, "v-test", runner)
	app.width = 120
	app.height = 30

	model, _ := app.Update(views.RuntimeModelChangedMsg{
		Provider: "anthropic",
		Model:    "claude-haiku-4-5-20251001",
	})
	app = model.(*App)
	model, _ = app.Update(views.StartSessionMsg{Input: "new session"})
	got := model.(*App)

	if view := got.View(); !strings.Contains(view, "claude-haiku-4-5-202") {
		t.Fatalf("new session view does not use latest runtime model:\n%s", view)
	}
}

func TestApp_PersistsAndReopensSessions(t *testing.T) {
	withFakeHome(t)
	workspace := t.TempDir()
	cfg := config.Default()
	runner := views.AgentRunner{
		Provider:  "anthropic",
		Model:     "claude-haiku-4-5-20251001",
		Workspace: workspace,
		Run: func(context.Context, string, []types.Message, func(string), func(views.UIEvent)) views.AgentResult {
			return views.AgentResult{}
		},
	}
	app := New(cfg, "v-test", runner)
	app.width = 120
	app.height = 30

	model, _ := app.Update(views.StartSessionMsg{Input: "first request"})
	app = model.(*App)
	first := app.current.(views.Session).Snapshot()
	first.Messages = append(first.Messages, views.SessionMessage{Role: "assistant", Text: "first answer"})
	model, _ = app.Update(views.SaveSessionMsg{Snapshot: first})
	app = model.(*App)

	model, _ = app.Update(views.StartSessionMsg{Input: "second request"})
	app = model.(*App)
	second := app.current.(views.Session).Snapshot()
	if first.ID == "" || second.ID == "" || first.ID == second.ID {
		t.Fatalf("session IDs = %q and %q, want distinct non-empty IDs", first.ID, second.ID)
	}

	model, _ = app.Update(views.OpenSessionMsg{ID: first.ID})
	got := model.(*App)
	if view := stripANSIForApp(got.View()); !strings.Contains(view, "first answer") {
		t.Fatalf("reopened session does not contain first answer:\n%s", view)
	}
}

func TestApp_CreateSessionOpensBlankConversation(t *testing.T) {
	withFakeHome(t)
	runner := views.AgentRunner{
		Provider:  "anthropic",
		Model:     "claude-haiku-4-5-20251001",
		Workspace: t.TempDir(),
	}
	app := New(config.Default(), "v-test", runner)
	app.width = 120
	app.height = 30

	model, _ := app.Update(views.CreateSessionMsg{Title: "Research"})
	got := model.(*App)
	snapshot := got.current.(views.Session).Snapshot()
	if snapshot.Title != "Research" || len(snapshot.Messages) != 0 {
		t.Fatalf("blank session snapshot = %#v", snapshot)
	}
	if view := stripANSIForApp(got.View()); strings.Contains(view, "you Research") {
		t.Fatalf("session title was incorrectly sent as a user prompt:\n%s", view)
	}
}

func TestApp_RenamesAndDeletesSavedSessions(t *testing.T) {
	withFakeHome(t)
	runner := views.AgentRunner{
		Provider:  "anthropic",
		Model:     "claude-haiku-4-5-20251001",
		Workspace: t.TempDir(),
	}
	app := New(config.Default(), "v-test", runner)
	app.width = 120
	app.height = 30

	model, _ := app.Update(views.CreateSessionMsg{Title: "Original"})
	app = model.(*App)
	id := app.current.(views.Session).Snapshot().ID

	model, _ = app.Update(views.RenameSessionMsg{ID: id, Title: "Renamed"})
	app = model.(*App)
	if title := app.current.(views.Session).Snapshot().Title; title != "Renamed" {
		t.Fatalf("active session title = %q, want Renamed", title)
	}

	model, _ = app.Update(views.DeleteSessionMsg{ID: id})
	got := model.(*App)
	if _, ok := got.current.(views.Welcome); !ok {
		t.Fatalf("current view after deleting active session = %T, want Welcome", got.current)
	}
	if _, err := app.sessions.Load(id); err == nil {
		t.Fatal("deleted session still loads from store")
	}
}

func TestApp_SessionsStoredUnderGlobalHomeDir(t *testing.T) {
	home := withFakeHome(t)
	workspace := t.TempDir()
	runner := views.AgentRunner{
		Provider:  "anthropic",
		Model:     "claude-haiku-4-5-20251001",
		Workspace: workspace,
	}
	app := New(config.Default(), "v-test", runner)
	app.width = 120
	app.height = 30

	model, _ := app.Update(views.CreateSessionMsg{Title: "Global storage"})
	got := model.(*App)
	id := got.current.(views.Session).Snapshot().ID

	if _, err := os.Stat(filepath.Join(workspace, ".cowork", "sessions")); err == nil {
		t.Fatal("session data was written under the workspace, want global storage only")
	}
	if _, err := got.sessions.Load(id); err != nil {
		t.Fatalf("session not found in global store: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".bolt-cowork", "sessions")); err != nil {
		t.Fatalf("expected sessions under global home dir: %v", err)
	}
}

func TestApp_MigratesLegacyProjectLocalSessions(t *testing.T) {
	home := withFakeHome(t)
	workspace := t.TempDir()

	legacyDir := filepath.Join(workspace, ".cowork", "sessions")
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(legacyDir) error = %v", err)
	}
	legacyRecord := `{"version":1,"id":"00000000000000000000000000000001","title":"Legacy"}`
	legacyFile := filepath.Join(legacyDir, "00000000000000000000000000000001.json")
	if err := os.WriteFile(legacyFile, []byte(legacyRecord), 0o600); err != nil {
		t.Fatalf("WriteFile(legacyFile) error = %v", err)
	}

	runner := views.AgentRunner{
		Provider:  "anthropic",
		Model:     "claude-haiku-4-5-20251001",
		Workspace: workspace,
	}
	app := New(config.Default(), "v-test", runner)

	if _, err := os.Stat(legacyDir); err == nil {
		t.Fatal("legacy session directory was not removed after migration")
	}
	record, err := app.sessions.Load("00000000000000000000000000000001")
	if err != nil {
		t.Fatalf("migrated session not loadable: %v", err)
	}
	if record.Title != "Legacy" {
		t.Fatalf("migrated record title = %q, want Legacy", record.Title)
	}
	if _, err := os.Stat(filepath.Join(home, ".bolt-cowork", "sessions")); err != nil {
		t.Fatalf("expected migrated sessions under global home dir: %v", err)
	}
}

func stripANSIForApp(s string) string {
	var out strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if r >= '@' && r <= '~' {
				inEscape = false
			}
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}
