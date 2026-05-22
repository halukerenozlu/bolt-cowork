package views

import (
	"os/exec"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/halukerenozlu/bolt-cowork/internal/ui/widgets"
	"github.com/halukerenozlu/bolt-cowork/pkg/types"
)

func TestNewSession_InputPromptDoesNotDuplicateOuterPrompt(t *testing.T) {
	s := NewSession(nil, "", "hello", AgentRunner{})

	if s.input.Prompt != "" {
		t.Fatalf("input prompt = %q, want empty because session renders its own prompt", s.input.Prompt)
	}
}

func TestSession_CtrlPOpensPaletteWithInitCmd(t *testing.T) {
	s := Session{width: 80, height: 24}

	model, cmd := s.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	got := model.(Session)

	if !got.paletteOpen {
		t.Fatal("palette should be open after Ctrl+P")
	}
	if cmd == nil {
		t.Fatal("expected palette init command when opening palette")
	}
}

func TestSession_ClearWhileRunningDoesNotResetState(t *testing.T) {
	s := Session{
		running:     true,
		messages:    []chatMsg{{role: "user", text: "make changes"}},
		history:     []types.Message{{Role: types.RoleUser, Content: "previous"}},
		planActive:  true,
		planSteps:   []string{"step 1"},
		stepDone:    []bool{false},
		stepErrors:  []error{nil},
		execLog:     []string{"running"},
		tokenCount:  42,
		paletteOpen: true,
	}

	model, cmd := s.Update(widgets.PaletteSelectMsg{Command: "/clear"})
	got := model.(Session)

	if cmd != nil {
		t.Fatalf("expected no command, got %T", cmd)
	}
	if got.paletteOpen {
		t.Fatal("palette should close after selecting /clear")
	}
	if len(got.history) != 1 {
		t.Fatalf("history should be preserved while running, got %d entries", len(got.history))
	}
	if len(got.planSteps) != 1 || len(got.execLog) != 1 || got.tokenCount != 42 {
		t.Fatalf("run state should be preserved while running: plan=%v log=%v tokens=%d", got.planSteps, got.execLog, got.tokenCount)
	}
	if len(got.messages) < 2 || got.messages[len(got.messages)-1].text != "Cannot clear while agent is running." {
		t.Fatalf("expected warning message after blocked clear, got %#v", got.messages)
	}
}

func TestSession_TypedClearRunsCommand(t *testing.T) {
	s := NewSession(nil, "", "previous", AgentRunner{})
	s.running = false
	s.messages = []chatMsg{
		{role: "user", text: "previous"},
		{role: "assistant", text: "answer"},
	}
	s.history = []types.Message{{Role: types.RoleUser, Content: "previous"}}
	s.tokenCount = 12
	s.input.SetValue("clear")

	model, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := model.(Session)

	if cmd != nil {
		t.Fatalf("expected no command, got %T", cmd)
	}
	if len(got.messages) != 0 || len(got.history) != 0 || got.tokenCount != 0 {
		t.Fatalf("typed clear should reset chat state, got messages=%d history=%d tokens=%d", len(got.messages), len(got.history), got.tokenCount)
	}
}

func TestSession_ChatContentClampsLongLines(t *testing.T) {
	const panelW = 32
	s := Session{
		messages: []chatMsg{
			{role: "user", text: strings.Repeat("u", 120)},
			{role: "assistant", text: "Error: " + strings.Repeat("e", 120)},
		},
	}

	for _, line := range strings.Split(s.chatContent(panelW, 10), "\n") {
		if lipgloss.Width(line) > panelW {
			t.Fatalf("line width = %d, want <= %d: %q", lipgloss.Width(line), panelW, line)
		}
	}
}

func TestDisplayAgentErrorHidesRawProviderEndpoint(t *testing.T) {
	raw := `agent: create plan: agent: planner chat: anthropic: http request: Post "https://api.anthropic.com/v1/messages": net/http: TLS handshake timeout`

	got := displayAgentError(assertErr(raw))

	if strings.Contains(got, "https://api.anthropic.com") {
		t.Fatalf("display error should not expose raw provider endpoint: %q", got)
	}
	if !strings.Contains(got, "network timeout") {
		t.Fatalf("display error = %q, want network timeout message", got)
	}
}

func TestSession_StatusContentIncludesProviderName(t *testing.T) {
	s := Session{runner: AgentRunner{Provider: "anthropic", Model: "claude", Workspace: strings.Repeat("w", 80)}}

	content := s.clippedStatusContent(10, 24)

	if !strings.Contains(content, "Name   : anthropic") {
		t.Fatalf("status content missing provider name: %q", content)
	}
	for _, line := range strings.Split(content, "\n") {
		if lipgloss.Width(line) > 24 {
			t.Fatalf("status line width = %d, want <= 24: %q", lipgloss.Width(line), line)
		}
	}
}

func TestFetchGitBranchUsesWorkspace(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "checkout", "-b", "workspace-branch")

	if got := fetchGitBranch(repo); got != "workspace-branch" {
		t.Fatalf("fetchGitBranch(%q) = %q, want workspace-branch", repo, got)
	}
}

func TestSession_RenderStatusBarClampsToWidth(t *testing.T) {
	tests := []struct {
		name    string
		width   int
		version string
	}{
		{name: "normal width", width: 80, version: "v0.4.2"},
		{name: "narrow width", width: 16, version: "v0.4.2"},
		{name: "version wider than terminal", width: 6, version: "version-is-long"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Session{
				width:     tt.width,
				version:   tt.version,
				gitBranch: "feature/status-bar",
				runner: AgentRunner{
					Workspace: strings.Repeat("workspace-", 20),
				},
			}

			got := s.renderStatusBar()
			if w := lipgloss.Width(got); w > tt.width {
				t.Fatalf("status bar width = %d, want <= %d: %q", w, tt.width, got)
			}
		})
	}
}

func TestSession_ReadMCPResourceEventTracksResourceIdentifier(t *testing.T) {
	s := Session{}

	got := s.handleUIEvent(StepDoneEvent{
		Index:  0,
		Action: "read_mcp_resource",
		Info:   "docs/file://README.md: resource output",
	})

	if got.lastMCPTool != "docs/file://README.md" {
		t.Fatalf("lastMCPTool = %q, want docs/file://README.md", got.lastMCPTool)
	}
	if got.lastMCPStatus != "ok" {
		t.Fatalf("lastMCPStatus = %q, want ok", got.lastMCPStatus)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		if _, lookErr := exec.LookPath("git"); lookErr != nil {
			t.Skip("git is not available")
		}
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
