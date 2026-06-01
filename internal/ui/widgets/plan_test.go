package widgets

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPlanView(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "empty", content: "", want: ""},
		{name: "content", content: "Plain plan", want: "Plain plan"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := (Plan{Content: tt.content}).View()
			if tt.want == "" && got != "" {
				t.Fatalf("View() = %q, want empty string", got)
			}
			if tt.want != "" && !strings.Contains(got, tt.want) {
				t.Fatalf("View() = %q, want to contain %q", got, tt.want)
			}
		})
	}
}

func TestPlanBubbleContract(t *testing.T) {
	plan := Plan{Content: "Do work"}

	if cmd := plan.Init(); cmd != nil {
		t.Fatal("Init() returned a command, want nil")
	}
	next, cmd := plan.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("Update() returned a command, want nil")
	}
	if next.(Plan).Content != plan.Content {
		t.Fatalf("Update() content = %q, want %q", next.(Plan).Content, plan.Content)
	}
}

func TestPlanWidgetView(t *testing.T) {
	tests := []struct {
		name     string
		widget   PlanWidget
		contains []string
	}{
		{
			name:     "empty steps",
			widget:   NewPlanWidget(nil, nil, nil, 40),
			contains: nil,
		},
		{
			name: "active step uses spinner frame",
			widget: func() PlanWidget {
				pw := NewPlanWidget([]string{"first step", "second step"}, []bool{false, false}, nil, 40)
				pw.SetActiveStep(0)
				pw.SetSpinnerFrame("*")
				return pw
			}(),
			contains: []string{"PLAN", "first step", "[*]", "second step"},
		},
		{
			name: "done step renders with following step",
			widget: NewPlanWidget(
				[]string{"completed step", "failed step"},
				[]bool{true, true},
				[]error{nil, errors.New("failed")},
				40,
			),
			contains: []string{"PLAN", "completed step", "failed step"},
		},
		{
			name:     "small width still renders",
			widget:   NewPlanWidget([]string{"a very long step that should be shortened"}, nil, nil, 4),
			contains: []string{"PLAN", "a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.widget.View()
			if len(tt.contains) == 0 {
				if got != "" {
					t.Fatalf("View() = %q, want empty string", got)
				}
				return
			}
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Fatalf("View() = %q, want to contain %q", got, want)
				}
			}
		})
	}
}

func TestTruncatePlain(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		maxWidth int
		want     string
	}{
		{name: "zero width", text: "hello", maxWidth: 0, want: ""},
		{name: "fits", text: "hello", maxWidth: 5, want: "hello"},
		{name: "suffix too wide", text: "hello", maxWidth: 2, want: "he"},
		{name: "truncates", text: "hello world", maxWidth: 8, want: "hello..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncatePlain(tt.text, tt.maxWidth); got != tt.want {
				t.Fatalf("truncatePlain() = %q, want %q", got, tt.want)
			}
		})
	}
}
