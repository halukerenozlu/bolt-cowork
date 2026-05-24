package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/halukerenozlu/bolt-cowork/internal/config"
	"github.com/halukerenozlu/bolt-cowork/internal/ui/views"
)

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
