package views

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/halukerenozlu/bolt-cowork/internal/config"
	"github.com/halukerenozlu/bolt-cowork/internal/tool"
	"github.com/halukerenozlu/bolt-cowork/internal/ui/widgets"
	"github.com/halukerenozlu/bolt-cowork/pkg/types"
)

func TestNewSession_InputPromptDoesNotDuplicateOuterPrompt(t *testing.T) {
	s := NewSession(nil, "", "hello", AgentRunner{})

	if s.input.Prompt != "" {
		t.Fatalf("input prompt = %q, want empty because session renders its own prompt", s.input.Prompt)
	}
}

func TestNewSession_BlankSessionFocusesInput(t *testing.T) {
	s := NewSession(nil, "", "", AgentRunner{}, WithRestoredSnapshot(SessionSnapshot{}))

	if !s.input.Focused() {
		t.Fatal("blank session input should be focused so the user can type immediately")
	}
}

func TestNewSession_ReopenedSessionFocusesInput(t *testing.T) {
	snapshot := SessionSnapshot{
		ID:       "abc",
		Title:    "Previous session",
		Messages: []SessionMessage{{Role: "user", Text: "first request"}},
	}
	s := NewSession(nil, "", "", AgentRunner{}, WithRestoredSnapshot(snapshot))

	if !s.input.Focused() {
		t.Fatal("reopened session input should be focused so the user can type immediately")
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
	s.tokenByteCount = 48
	s.sessionCost = 0.42
	s.input.SetValue("clear")

	model, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := model.(Session)

	if cmd != nil {
		t.Fatalf("expected no command, got %T", cmd)
	}
	if len(got.messages) != 0 || len(got.history) != 0 || got.tokenCount != 0 {
		t.Fatalf("typed clear should reset chat state, got messages=%d history=%d tokens=%d", len(got.messages), len(got.history), got.tokenCount)
	}
	if got.tokenByteCount != 0 || got.sessionCost != 0 {
		t.Fatalf("typed clear should reset token/cost counters, got bytes=%d cost=%f", got.tokenByteCount, got.sessionCost)
	}
}

func TestSession_PaletteCommandsOpenModalWithoutChatOutput(t *testing.T) {
	s := NewSession(nil, "", "previous", AgentRunner{
		Model:        "claude-sonnet-4-6",
		Workspace:    "C:\\workspace",
		ApprovalMode: "none",
	})
	s.running = false
	s.messages = []chatMsg{{role: "assistant", text: "first"}}

	model, _ := s.handlePaletteCmd("/model")
	got := model.(Session)

	if !got.modalOpen {
		t.Fatal("model command should open a modal")
	}
	if len(got.messages) != 1 || got.messages[0].text != "first" {
		t.Fatalf("command should not write to chat: %#v", got.messages)
	}
}

func TestSession_SkillsCommandOpensSearchableModal(t *testing.T) {
	s := NewSession(nil, "", "previous", AgentRunner{
		LoadedSkills: []string{"code-reviewer", "git-helper"},
	})
	s.running = false

	model, _ := s.handlePaletteCmd("skills")
	got := model.(Session)
	view := got.modal.View()

	for _, want := range []string{"Skills", "Search...", "code-reviewer", "git-helper"} {
		if !strings.Contains(view, want) {
			t.Fatalf("skills modal missing %q:\n%s", want, view)
		}
	}
}

func TestSession_AllNonDestructivePaletteCommandsOpenModals(t *testing.T) {
	s := NewSession(nil, "", "previous", AgentRunner{
		Provider:     "anthropic",
		Model:        "claude-sonnet-4-6",
		Workspace:    "C:\\workspace",
		ApprovalMode: "none",
		LoadedSkills: []string{"code-reviewer"},
	})
	s.running = false
	s.messages = []chatMsg{{role: "assistant", text: "keep"}}

	commands := []string{
		"switch-session",
		"switch-model",
		"connect-provider",
		"open-editor",
		"new-session",
		"skills",
		"hide-tips",
		"view-status",
		"switch-theme",
		"/model",
		"/dir",
		"/approval",
		"/help",
	}

	for _, command := range commands {
		t.Run(command, func(t *testing.T) {
			model, _ := s.handlePaletteCmd(command)
			got := model.(Session)
			if !got.modalOpen {
				t.Fatal("command did not open modal")
			}
			if got.paletteOpen {
				t.Fatal("palette should close when modal opens")
			}
			if len(got.messages) != 1 || got.messages[0].text != "keep" {
				t.Fatalf("command wrote to chat: %#v", got.messages)
			}
			view := got.modal.View()
			if !strings.Contains(view, "> ") {
				t.Fatalf("modal missing input row:\n%s", view)
			}
		})
	}
}

func TestSession_BuildChatBodyClampsLongLines(t *testing.T) {
	const panelW = 32
	s := Session{
		messages: []chatMsg{
			{role: "user", text: strings.Repeat("u", 120)},
			{role: "assistant", text: "Error: " + strings.Repeat("e", 120)},
		},
	}

	for _, line := range strings.Split(s.buildChatBody(panelW), "\n") {
		if lipgloss.Width(line) > panelW {
			t.Fatalf("line width = %d, want <= %d: %q", lipgloss.Width(line), panelW, line)
		}
	}
}

func TestSession_RenderChatPanelHasFixedHeight(t *testing.T) {
	s := Session{
		chatVP: viewport.New(46, 0),
		messages: []chatMsg{
			{role: "assistant", text: strings.Repeat("line\n", 40)},
		},
	}
	s.chatVPW = 46
	s = s.rebuildChatVP()

	for _, panelH := range []int{3, 6, 12} {
		s.chatVP.Height = max(panelH-2, 0)
		got := s.renderChatPanel(48, panelH)
		if lines := strings.Count(got, "\n") + 1; lines != panelH {
			t.Fatalf("renderChatPanel lines = %d, want %d:\n%s", lines, panelH, got)
		}
	}
}

func TestSession_ChatContentCannotInjectTerminalControls(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{name: "escape sequence", text: "before\x1b[2Jafter"},
		{name: "carriage return", text: "before\rafter"},
		{name: "backspace", text: "before\bafter"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Session{
				chatVP: viewport.New(40, 6),
				messages: []chatMsg{
					{role: "assistant", text: tt.text},
				},
			}
			s.chatVPW = 40
			s = s.rebuildChatVP()

			got := s.renderChatPanel(42, 8)
			for _, unsafe := range []rune{'\x1b', '\r', '\b'} {
				if strings.ContainsRune(got, unsafe) {
					t.Fatalf("rendered chat contains unsafe control %U: %q", unsafe, got)
				}
			}
			if !strings.Contains(got, "> ") {
				t.Fatalf("rendered chat lost input row: %q", got)
			}
		})
	}
}

func TestSession_AgentEventsCannotInjectTerminalControls(t *testing.T) {
	tests := []struct {
		name  string
		event UIEvent
	}{
		{
			name:  "plan step",
			event: PlanReadyEvent{Steps: []string{"inspect\x1b[2Jfile"}},
		},
		{
			name: "execution result",
			event: StepDoneEvent{
				Index:  0,
				Action: "read",
				Info:   "result\rrewritten\b",
			},
		},
		{
			name:  "permission warning",
			event: PermWarnEvent{Warning: "warning\x1b]0;title\a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Session{
				width:    120,
				height:   20,
				chatVP:   viewport.New(0, 0),
				runner:   AgentRunner{Provider: "test", Model: "m"},
				messages: []chatMsg{{role: "user", text: "test"}},
			}
			s = s.resizeChatVP()
			s = s.handleUIEvent(tt.event)
			s = s.rebuildChatVP()

			got := s.baseView()
			for _, unsafe := range []rune{'\x1b', '\r', '\b', '\a'} {
				if strings.ContainsRune(got, unsafe) {
					t.Fatalf("rendered event contains unsafe control %U: %q", unsafe, got)
				}
			}
		})
	}
}

func TestSession_ViewportScrollPreservesHeight(t *testing.T) {
	s := Session{
		width:  120,
		height: 10,
		chatVP: viewport.New(0, 0),
		runner: AgentRunner{Provider: "test", Model: "m"},
		messages: []chatMsg{
			{role: "assistant", text: strings.Repeat("long content line\n", 100)},
		},
	}
	s = s.resizeChatVP()

	got := s.baseView()
	wantLines := s.height - 1
	if lines := strings.Count(got, "\n") + 1; lines != wantLines {
		t.Fatalf("baseView lines = %d, want %d", lines, wantLines)
	}
}

func TestSession_SingleStepRunHidesPlanBlockAndAvoidsDuplicateResult(t *testing.T) {
	tests := []struct {
		name     string
		execLog  []string
		response string
		want     []string
	}{
		{
			name:     "single result",
			execLog:  []string{`v Listed ".": file-a, file-b, file-c`},
			response: "Completed.",
			want:     []string{`Listed ".": file-a, file-b, file-c`, "Completed."},
		},
		{
			name: "fallback before result",
			execLog: []string{
				"⚡ Fallback: anthropic → openai (not available)",
				`v Listed ".": file-a, file-b, file-c`,
			},
			want: []string{
				"⚡ Fallback: anthropic → openai (not available)",
				`Listed ".": file-a, file-b, file-c`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Session{
				chatVP:      viewport.New(80, 20),
				chatVPW:     80,
				messages:    []chatMsg{{role: "user", text: "list files"}},
				planActive:  true,
				planSteps:   []string{"List files in the current directory"},
				stepDone:    []bool{true},
				stepErrors:  []error{nil},
				execLog:     tt.execLog,
				runResponse: tt.response,
			}

			s = s.finishActiveRun()
			body := stripANSI(s.buildChatBody(80))

			if strings.Contains(body, "PLAN") {
				t.Errorf("single-step run should not show a PLAN block:\n%s", body)
			}
			if !strings.Contains(body, "→ List files in the current directory") {
				t.Errorf("expected one-line activity indicator:\n%s", body)
			}
			for _, want := range tt.want {
				if n := strings.Count(body, want); n != 1 {
					t.Errorf("expected %q exactly once, got %d:\n%s", want, n, body)
				}
			}
		})
	}
}

func TestSession_CompletedPlanRunRemainsVisibleWhenNextRunStarts(t *testing.T) {
	s := Session{
		chatVP:      viewport.New(80, 20),
		chatVPW:     80,
		messages:    []chatMsg{{role: "user", text: "first request"}},
		planActive:  true,
		planSteps:   []string{"inspect files"},
		stepDone:    []bool{true},
		stepErrors:  []error{nil},
		execLog:     []string{`v Stat "report.pdf": size=42`},
		runResponse: "Found report.pdf.",
	}

	s = s.finishActiveRun()
	s.messages = append(s.messages, chatMsg{role: "user", text: "second request"})
	s.planActive = true
	s.planSteps = []string{"inspect images"}
	s.stepDone = []bool{true}
	s.stepErrors = []error{nil}
	s.execLog = []string{`v Stat "chart.png": size=21`}

	body := stripANSI(s.buildChatBody(80))
	for _, want := range []string{
		"first request",
		"inspect files",
		`Stat "report.pdf": size=42`,
		"Found report.pdf.",
		"second request",
		"inspect images",
		`Stat "chart.png": size=21`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("chat body missing %q:\n%s", want, body)
		}
	}
}

func TestSession_ScrollbarAppearsWhenContentOverflows(t *testing.T) {
	s := Session{
		width:  120,
		height: 20,
		chatVP: viewport.New(0, 0),
		runner: AgentRunner{Provider: "test", Model: "m"},
		messages: []chatMsg{
			{role: "assistant", text: strings.Repeat("line\n", 100)},
		},
	}
	s = s.resizeChatVP()
	s.chatVP.GotoBottom()

	got := s.baseView()
	if !strings.Contains(got, "┃") && !strings.Contains(got, "│") {
		t.Fatal("expected scrollbar track characters in output")
	}
}

func TestSession_BaseViewFitsTerminalHeight(t *testing.T) {
	s := Session{
		width:  120,
		height: 10,
		runner: AgentRunner{
			Provider:     "anthropic",
			Model:        "claude-sonnet-4-6",
			Workspace:    "C:\\workspace",
			ApprovalMode: "none",
			LoadedSkills: []string{"code-reviewer", "file-organizer", "git-helper", "pdf-converter"},
		},
		messages: []chatMsg{
			{role: "assistant", text: strings.Repeat("content\n", 40)},
		},
	}

	got := s.baseView()
	wantLines := s.height - 1
	if lines := strings.Count(got, "\n") + 1; lines != wantLines {
		t.Fatalf("baseView lines = %d, want %d:\n%s", lines, wantLines, got)
	}
	lines := strings.Split(got, "\n")
	for _, line := range lines {
		if w := lipgloss.Width(line); w > s.width-1 {
			t.Fatalf("baseView line width = %d, want <= %d: %q", w, s.width-1, line)
		}
	}
	if !strings.Contains(lines[len(lines)-1], "dev") {
		t.Fatalf("baseView last line should be status bar, got %q", lines[len(lines)-1])
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
	if !strings.Contains(content, "Model  : claude") {
		t.Fatalf("status content missing model name: %q", content)
	}
}

func TestSession_StatusContentRespectsWidth(t *testing.T) {
	s := Session{
		runner:         AgentRunner{Provider: "anthropic", Model: "claude-opus-4-20250514"},
		activeProvider: "openai",
		activeModel:    "gpt-4o-2024-08-06",
	}

	const panelWidth = 24
	content := s.clippedStatusContent(20, panelWidth)
	for i, line := range strings.Split(content, "\n") {
		w := lipgloss.Width(line)
		if w > panelWidth {
			t.Errorf("line %d visual width %d exceeds panel width %d: %q", i, w, panelWidth, stripAnsi(line))
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

func TestApprovalResponseLabel(t *testing.T) {
	ch := make(chan ApprovalResponse, 1)
	s := NewSession(nil, "", "hi", AgentRunner{})
	s.approvalPending = true
	s.approvalCh = ch

	model, cmd := s.Update(widgets.ModalSelectMsg{Label: "Approve all"})
	got := model.(Session)

	if got.approvalPending {
		t.Fatal("approval should no longer be pending after modal selection")
	}
	if cmd != nil {
		t.Fatalf("expected no command, got %T", cmd)
	}
	resp := <-ch
	if !resp.Approved {
		t.Fatalf("approval response = %#v, want approved response", resp)
	}
	if label := ApprovalResponseLabel(ch); label != "Approve all" {
		t.Fatalf("ApprovalResponseLabel() = %q, want Approve all", label)
	}
	if label := ApprovalResponseLabel(ch); label != "" {
		t.Fatalf("ApprovalResponseLabel second read = %q, want empty", label)
	}
}

func TestResolveTypedCommand(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   string
		wantOK bool
	}{
		{name: "empty", input: "   ", wantOK: false},
		{name: "clear alias", input: "cls", want: "/clear", wantOK: true},
		{name: "leading slash", input: "/model", want: "/model", wantOK: true},
		{name: "case insensitive", input: " HELP ", want: "/help", wantOK: true},
		{name: "palette-only command without slash", input: "switch-model", want: "switch-model", wantOK: true},
		{name: "palette-only command with slash", input: "/connect-provider", want: "connect-provider", wantOK: true},
		{name: "unknown", input: "/totally-unknown", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := resolveTypedCommand(tt.input)
			if got != tt.want || ok != tt.wantOK {
				t.Fatalf("resolveTypedCommand(%q) = %q, %v; want %q, %v", tt.input, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestSlashSuggestions(t *testing.T) {
	if got := slashSuggestions("list files"); got != nil {
		t.Fatalf("expected nil suggestions for non-slash input, got %v", got)
	}
	if got := slashSuggestions("/"); len(got) == 0 {
		t.Fatal("expected all commands to be suggested for a bare slash")
	}
	got := slashSuggestions("/he")
	found := false
	for _, c := range got {
		if c.Name == "/help" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected /he to suggest /help, got %v", got)
	}
}

func TestSession_SlashSuggestionsTabCompletesAndEnterRuns(t *testing.T) {
	s := NewSession(nil, "", "hi", AgentRunner{})
	s.running = false
	s.input.SetValue("/he")

	model, _ := s.Update(tea.KeyMsg{Type: tea.KeyTab})
	got := model.(Session)
	if got.input.Value() != "/help" {
		t.Fatalf("input after Tab = %q, want /help", got.input.Value())
	}

	model, _ = got.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got = model.(Session)
	if !got.modalOpen {
		t.Fatal("expected /help to open a modal after Enter")
	}
}

func TestSession_SlashSuggestionEscPreservesTypedText(t *testing.T) {
	s := NewSession(nil, "", "hi", AgentRunner{})
	s.running = false
	s.input.SetValue("/mo")

	model, _ := s.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got := model.(Session)

	if got.input.Value() != "/mo" {
		t.Fatalf("Esc should not clear typed text, got %q", got.input.Value())
	}
	if got.slashDismissedFor != "/mo" {
		t.Fatalf("expected suggestions dismissed for current text, got dismissedFor=%q", got.slashDismissedFor)
	}
}

func TestSession_UnknownSlashCommandIsNotSentToAgent(t *testing.T) {
	s := NewSession(nil, "", "hi", AgentRunner{})
	s.running = false
	s.input.SetValue("/definitely-not-a-command")

	model, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := model.(Session)

	for _, m := range got.messages {
		if m.role == "user" && strings.Contains(m.text, "definitely-not-a-command") {
			t.Fatalf("unknown slash command should not become a user chat message: %#v", got.messages)
		}
	}
	if !got.hasCommandOutput("Unknown command") {
		t.Fatal("expected an unknown-command notice")
	}
}

func TestNormalizeTypedCommand(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   string
		wantOK bool
	}{
		{name: "empty", input: "   ", wantOK: false},
		{name: "clear alias", input: "cls", want: "/clear", wantOK: true},
		{name: "leading slash", input: "/model", want: "/model", wantOK: true},
		{name: "case insensitive", input: " HELP ", want: "/help", wantOK: true},
		{name: "unknown", input: "status", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := normalizeTypedCommand(tt.input)
			if got != tt.want || ok != tt.wantOK {
				t.Fatalf("normalizeTypedCommand(%q) = %q, %v; want %q, %v", tt.input, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestSessionModelHelpers(t *testing.T) {
	cfg := &config.Config{
		DefaultProvider: "anthropic",
		Providers: map[string]config.ProviderConfig{
			"anthropic": {Models: []string{"claude-sonnet-4-6"}},
			"custom":    {Models: []string{"local-model"}},
		},
		FallbackChain: []config.FallbackEntry{{Provider: "custom", Model: "local-model"}},
	}
	s := NewSession(cfg, "", "hi", AgentRunner{Provider: "anthropic", Model: "claude-sonnet-4-6"})

	if got := providerHasModel(cfg, "anthropic", "claude-sonnet-4-6"); !got {
		t.Fatal("providerHasModel should find configured model")
	}
	if got := providerHasModel(cfg, "", "claude-sonnet-4-6"); got {
		t.Fatal("providerHasModel should reject empty provider")
	}
	if got := defaultProviderHasModel("openai", "gpt-4o"); !got {
		t.Fatal("defaultProviderHasModel should find default OpenAI model")
	}
	if got := defaultProviderHasModel("openai", "not-a-model"); got {
		t.Fatal("defaultProviderHasModel should reject unknown model")
	}
	if got := firstModelForProvider(cfg, "custom"); got != "local-model" {
		t.Fatalf("firstModelForProvider = %q, want local-model", got)
	}
	if got := firstModelForProvider(nil, "custom"); got != "" {
		t.Fatalf("firstModelForProvider nil cfg = %q, want empty", got)
	}
	if got := s.defaultModelForProvider("custom"); got != "local-model" {
		t.Fatalf("defaultModelForProvider custom = %q, want local-model", got)
	}
	if provider, err := s.providerForModel("local-model"); err != nil || provider != "custom" {
		t.Fatalf("providerForModel local-model = %q, %v; want custom, nil", provider, err)
	}
}

func TestEnsureDefaultProviderConfigured(t *testing.T) {
	cfg := &config.Config{}

	if !ensureDefaultProviderConfigured(cfg, "openai") {
		t.Fatal("ensureDefaultProviderConfigured should add missing default provider")
	}
	if cfg.Providers["openai"].Models[0] != config.DefaultModels["openai"][0] {
		t.Fatalf("openai models = %v, want defaults", cfg.Providers["openai"].Models)
	}
	if ensureDefaultProviderConfigured(cfg, "openai") {
		t.Fatal("ensureDefaultProviderConfigured should return false for existing provider")
	}
	if ensureDefaultProviderConfigured(cfg, "unknown") {
		t.Fatal("ensureDefaultProviderConfigured should reject unknown provider")
	}
	if ensureDefaultProviderConfigured(nil, "openai") {
		t.Fatal("ensureDefaultProviderConfigured should reject nil config")
	}
}

func TestSessionFormattingHelpers(t *testing.T) {
	if got := parseMCPTool("filesystem/read_file: ok"); got != "filesystem/read_file" {
		t.Fatalf("parseMCPTool = %q, want filesystem/read_file", got)
	}
	if got := parseMCPTool("plain status"); got != "mcp" {
		t.Fatalf("parseMCPTool fallback = %q, want mcp", got)
	}
	if got := firstLine("\n  first  \nsecond"); got != "first" {
		t.Fatalf("firstLine = %q, want first", got)
	}
	if got := firstLine("\n \t\n"); got != "" {
		t.Fatalf("firstLine empty = %q, want empty", got)
	}
	if got := formatExecLogLine(StepDoneEvent{Info: "read file"}); got != "v read file" {
		t.Fatalf("formatExecLogLine success = %q", got)
	}
	if got := formatExecLogLine(StepDoneEvent{Info: "read file", Err: assertErr("agent: file not found: missing.txt")}); !strings.Contains(got, "x read file - file not found: missing.txt") {
		t.Fatalf("formatExecLogLine error = %q", got)
	}
	if got := displayAgentError(assertErr("agent: create plan: agent: planner chat: provider unavailable")); got != "provider unavailable" {
		t.Fatalf("displayAgentError trimmed = %q, want provider unavailable", got)
	}
}

func TestListEntries(t *testing.T) {
	tests := []struct {
		name string
		info string
		want []string
		ok   bool
	}{
		{name: "preserves comma", info: tool.FormatListOutput(".", []string{"report, final.pdf", "subdir/"}), want: []string{"report, final.pdf", "subdir/"}, ok: true},
		{name: "empty directory", info: tool.FormatListOutput(".", nil), want: []string{"(empty)"}, ok: true},
		{name: "non-list result", info: `Stat "file.txt": size=42`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries, ok := listEntries(tt.info)
			if ok != tt.ok || !slices.Equal(entries, tt.want) {
				t.Fatalf("listEntries = %v, %v, want %v, %v", entries, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestFormatExecLogLine_ListResultIsOnePerLineWithoutPrefix(t *testing.T) {
	got := formatExecLogLine(StepDoneEvent{Info: tool.FormatListOutput(".", []string{"file-a", "subdir/"})})

	if strings.Contains(got, `Listed "."`) {
		t.Fatalf("expected the Listed prefix to be dropped, got %q", got)
	}
	for _, want := range []string{"file-a", "subdir/"} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatExecLogLine missing %q in %q", want, got)
		}
	}
	if strings.HasPrefix(got, "v") {
		t.Fatalf("list result should not expose a status letter: %q", got)
	}
	if lines := strings.Split(got, "\n"); len(lines) != 2 {
		t.Fatalf("expected 2 entry lines, got %d lines: %q", len(lines), got)
	}
}

func TestFixedHeightAndPrefixedLines(t *testing.T) {
	if got := fixedHeightLines([]string{"a", "b", "c"}, 2); got != "b\nc" {
		t.Fatalf("fixedHeightLines trim = %q, want b\\nc", got)
	}
	if got := fixedHeightLines([]string{"a"}, 3); got != "a\n\n" {
		t.Fatalf("fixedHeightLines pad = %q, want a followed by blanks", got)
	}
	if got := fixedHeightLines([]string{"a"}, 0); got != "" {
		t.Fatalf("fixedHeightLines zero = %q, want empty", got)
	}

	lines := appendPrefixedLines(nil, "A: ", "   ", "first\n\nsecond long line", 12)
	want := []string{"A: first", "", "   second..."}
	if strings.Join(lines, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("appendPrefixedLines = %#v, want %#v", lines, want)
	}
}

func TestTokenPricingAndTruncationHelpers(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "-"},
		{42, "42"},
		{1200, "1,200"},
	}
	for _, tt := range tests {
		if got := formatTokenCount(tt.n); got != tt.want {
			t.Fatalf("formatTokenCount(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}

	if got := estimateTokensFromBytes(0); got != 0 {
		t.Fatalf("estimateTokensFromBytes(0) = %d, want 0", got)
	}
	if got := estimateTokensFromBytes(5); got != 2 {
		t.Fatalf("estimateTokensFromBytes(5) = %d, want 2", got)
	}
	if got := pricingForModel("gemini", "unknown-model"); got.output != 2.50 {
		t.Fatalf("pricingForModel gemini fallback = %#v, want output 2.50", got)
	}
	if got := pricingForModel("unknown", "unknown-model"); got.input != 2.00 || got.output != 8.00 {
		t.Fatalf("pricingForModel default fallback = %#v, want 2/8", got)
	}
	if got := truncatePlain("hello", 0); got != "" {
		t.Fatalf("truncatePlain zero = %q, want empty", got)
	}
	if got := truncatePlain("hello", 2); got != "he" {
		t.Fatalf("truncatePlain tiny = %q, want he", got)
	}
	if got := truncatePlain("hello world", 8); got != "hello..." {
		t.Fatalf("truncatePlain = %q, want hello...", got)
	}
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripAnsi(s string) string { return ansiRe.ReplaceAllString(s, "") }

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

func TestSession_ModalSelectSwitchModelUpdatesRunner(t *testing.T) {
	s := NewSession(nil, "", "hi", AgentRunner{Provider: "openai", Model: "old-model"})
	s.running = false
	s.modalCommand = "switch-model"

	model, _ := s.Update(widgets.ModalSelectMsg{Label: "gpt-4o"})
	got := model.(Session)

	if got.runner.Model != "gpt-4o" {
		t.Fatalf("runner.Model = %q, want gpt-4o", got.runner.Model)
	}
	if !got.hasCommandOutput("Model set to gpt-4o.") {
		t.Fatal("expected confirmation message")
	}
	if got.modalOpen {
		t.Fatal("modal should be closed")
	}
}

func TestSession_RemoveCredential(t *testing.T) {
	tests := []struct {
		name   string
		target string
		active bool
	}{
		{name: "inactive provider", target: "openai"},
		{name: "active provider prompts reconnect", target: "anthropic", active: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				DefaultProvider: "anthropic",
				Providers: map[string]config.ProviderConfig{
					"anthropic": {APIKey: "key", Models: []string{"claude-sonnet-4-6"}},
					"openai":    {APIKey: "key", Models: []string{"gpt-4o"}},
				},
			}
			deleted := ""
			s := NewSession(cfg, "", "hi", AgentRunner{
				Provider:             "anthropic",
				HasStoredProviderKey: func(string) bool { return true },
				DeleteProviderKey: func(name string) error {
					deleted = name
					return nil
				},
			})
			s.modalCommand = "remove-credential"
			model, _ := s.Update(widgets.ModalSelectMsg{Label: tt.target})
			s = model.(Session)
			model, _ = s.Update(widgets.ModalSelectMsg{Label: "Remove"})
			s = model.(Session)

			if cfg.Providers[tt.target].APIKey != "" || deleted != tt.target {
				t.Fatalf("credential removal failed: key=%q deleted=%q", cfg.Providers[tt.target].APIKey, deleted)
			}
			if tt.active && (!s.modalOpen || s.modalCommand != "connect-provider") {
				t.Fatalf("active removal did not prompt reconnect: open=%v command=%q", s.modalOpen, s.modalCommand)
			}
			if tt.active {
				if s.runner.Provider != "" || cfg.DefaultProvider != "" {
					t.Fatalf("active provider was not cleared: runner=%q default=%q", s.runner.Provider, cfg.DefaultProvider)
				}
				if view := stripANSI(s.modal.View()); strings.Contains(view, "● current") {
					t.Fatalf("credential-less provider still shown as current:\n%s", view)
				}
			}
		})
	}
}

func TestSession_ModalSelectConnectProviderUpdatesRunner(t *testing.T) {
	cfg := &config.Config{
		DefaultProvider: "anthropic",
		Providers: map[string]config.ProviderConfig{
			"anthropic": {APIKey: "key", Models: []string{"claude-sonnet-4-6"}},
			"openai":    {APIKey: "key", Models: []string{"gpt-4o"}},
		},
	}
	s := NewSession(cfg, "", "hi", AgentRunner{Provider: "anthropic"})
	s.running = false
	s.modalCommand = "connect-provider"

	model, _ := s.Update(widgets.ModalSelectMsg{Label: "openai"})
	got := model.(Session)

	if got.runner.Provider != "openai" {
		t.Fatalf("runner.Provider = %q, want openai", got.runner.Provider)
	}
	if !got.hasCommandOutput("Provider set to openai.") {
		t.Fatal("expected confirmation message")
	}
}

func TestSession_ModalSelectConnectProviderUpdatesFallbackChain(t *testing.T) {
	cfg := &config.Config{
		DefaultProvider: "anthropic",
		ApprovalMode:    "full",
		Providers: map[string]config.ProviderConfig{
			"anthropic": {APIKey: "key", Models: []string{"claude-sonnet-4-6"}},
			"openai":    {APIKey: "key", Models: []string{"gpt-4o", "gpt-4o-mini"}},
		},
		FallbackChain: []config.FallbackEntry{{Provider: "anthropic", Model: "claude-sonnet-4-6"}},
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeConfigFile(t, path)

	s := NewSession(cfg, "", "hi", AgentRunner{
		Provider: "anthropic",
		Model:    "claude-sonnet-4-6",
	}, WithConfigPath(path))
	s.running = false
	s.modalCommand = "connect-provider"

	model, _ := s.Update(widgets.ModalSelectMsg{Label: "openai"})
	got := model.(Session)

	if got.runner.Provider != "openai" || got.runner.Model != "gpt-4o" {
		t.Fatalf("runner = %s/%s, want openai/gpt-4o", got.runner.Provider, got.runner.Model)
	}
	if cfg.DefaultProvider != "openai" {
		t.Fatalf("DefaultProvider = %q, want openai", cfg.DefaultProvider)
	}
	if got := cfg.FallbackChain[0]; got.Provider != "openai" || got.Model != "gpt-4o" {
		t.Fatalf("fallback[0] = %#v, want openai/gpt-4o", got)
	}
	assertConfigFileContains(t, path, "api_key: ${OPENAI_API_KEY}", "provider: openai", "model: gpt-4o")
}

func TestSession_ModalSelectConnectProviderAddsDefaultProvider(t *testing.T) {
	cfg := &config.Config{
		DefaultProvider: "anthropic",
		ApprovalMode:    "full",
		Providers: map[string]config.ProviderConfig{
			"anthropic": {APIKey: "key", Models: []string{"claude-sonnet-4-6"}},
		},
		FallbackChain: []config.FallbackEntry{{Provider: "anthropic", Model: "claude-sonnet-4-6"}},
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := config.SaveFile(cfg, path); err != nil {
		t.Fatalf("save config: %v", err)
	}

	s := NewSession(cfg, "", "hi", AgentRunner{
		Provider: "anthropic",
		Model:    "claude-sonnet-4-6",
	}, WithConfigPath(path))
	s.running = false
	s.modalCommand = "connect-provider"

	// Selecting "openai" without an API key triggers the wizard.
	model, _ := s.Update(widgets.ModalSelectMsg{Label: "openai"})
	got := model.(Session)

	if !got.wizardOpen {
		t.Fatal("expected wizard to open for unconfigured provider")
	}
	if got.wizardProvider != "openai" {
		t.Fatalf("wizardProvider = %q, want openai", got.wizardProvider)
	}
}

func TestSession_CommitProviderSwitchPersistsSelectedModel(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		model    string
	}{
		{name: "discovered hosted model", provider: "openrouter", model: "vendor/new-model"},
		{name: "detected local model", provider: "ollama", model: "qwen3:8b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.Providers = map[string]config.ProviderConfig{
				"anthropic": {APIKey: "key", Models: []string{"claude-sonnet-4-6"}},
			}
			cfg.FallbackChain = []config.FallbackEntry{{
				Provider: "anthropic",
				Model:    "claude-sonnet-4-6",
			}}
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := config.SaveFile(cfg, path); err != nil {
				t.Fatalf("save initial config: %v", err)
			}

			s := NewSession(cfg, "", "hi", AgentRunner{}, WithConfigPath(path))
			s = s.commitProviderSwitch(tt.provider, tt.model)

			pc, ok := cfg.Providers[tt.provider]
			if !ok {
				t.Fatalf("provider %q was not added to config", tt.provider)
			}
			if !slices.Contains(pc.Models, tt.model) {
				t.Fatalf("provider models = %v, want selected model %q", pc.Models, tt.model)
			}
			if got := cfg.FallbackChain[0]; got.Provider != tt.provider || got.Model != tt.model {
				t.Fatalf("fallback = %#v, want %s/%s", got, tt.provider, tt.model)
			}
			if err := cfg.Validate(); err != nil {
				t.Fatalf("resulting config is invalid: %v", err)
			}
		})
	}
}

func TestSession_LocalProviderSelectionStartsWizard(t *testing.T) {
	cfg := config.Default()
	cfg.Providers = map[string]config.ProviderConfig{
		"anthropic": {APIKey: "key", Models: []string{"claude-sonnet-4-6"}},
	}
	s := NewSession(cfg, "", "hi", AgentRunner{Provider: "anthropic"})
	s.modalCommand = "connect-provider"

	model, _ := s.Update(widgets.ModalSelectMsg{Label: "ollama"})
	got := model.(Session)

	if !got.wizardOpen {
		t.Fatal("expected local provider selection to open model-discovery wizard")
	}
	if got.wizardProvider != "ollama" {
		t.Fatalf("wizardProvider = %q, want ollama", got.wizardProvider)
	}
}

func TestSession_ModalSelectSwitchModelUpdatesProviderForSelectedModel(t *testing.T) {
	cfg := &config.Config{
		DefaultProvider: "anthropic",
		ApprovalMode:    "full",
		Providers: map[string]config.ProviderConfig{
			"anthropic": {Models: []string{"claude-sonnet-4-6"}},
			"openai":    {APIKey: "key", Models: []string{"gpt-4o"}},
		},
		FallbackChain: []config.FallbackEntry{{Provider: "anthropic", Model: "claude-sonnet-4-6"}},
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeConfigFile(t, path)

	s := NewSession(cfg, "", "hi", AgentRunner{
		Provider: "anthropic",
		Model:    "claude-sonnet-4-6",
	}, WithConfigPath(path))
	s.running = false
	s.modalCommand = "switch-model"

	model, _ := s.Update(widgets.ModalSelectMsg{Label: "gpt-4o"})
	got := model.(Session)

	if got.runner.Provider != "openai" || got.runner.Model != "gpt-4o" {
		t.Fatalf("runner = %s/%s, want openai/gpt-4o", got.runner.Provider, got.runner.Model)
	}
	if got := cfg.FallbackChain[0]; got.Provider != "openai" || got.Model != "gpt-4o" {
		t.Fatalf("fallback[0] = %#v, want openai/gpt-4o", got)
	}
	assertConfigFileContains(t, path, "api_key: ${OPENAI_API_KEY}", "default_provider: openai", "provider: openai", "model: gpt-4o")
}

func TestSession_SwitchModelModalRefreshesCurrentSelection(t *testing.T) {
	cfg := &config.Config{
		DefaultProvider: "anthropic",
		Providers: map[string]config.ProviderConfig{
			"anthropic": {
				Models: []string{"claude-opus-4-5", "claude-haiku-4-5-20251001"},
			},
		},
		FallbackChain: []config.FallbackEntry{{
			Provider: "anthropic",
			Model:    "claude-opus-4-5",
		}},
	}
	s := NewSession(cfg, "", "hi", AgentRunner{
		Provider: "anthropic",
		Model:    "claude-opus-4-5",
	})
	s.running = false
	s.modalCommand = "switch-model"

	model, _ := s.Update(widgets.ModalSelectMsg{Label: "claude-haiku-4-5-20251001"})
	got := model.(Session)
	model, _ = got.handlePaletteCmd("switch-model")
	got = model.(Session)
	view := stripANSI(got.modal.View())

	var haikuLine, opusLine string
	for line := range strings.SplitSeq(view, "\n") {
		switch {
		case strings.Contains(line, "claude-haiku-4-5-20251001"):
			haikuLine = line
		case strings.Contains(line, "claude-opus-4-5"):
			opusLine = line
		}
	}
	if !strings.Contains(haikuLine, "current") {
		t.Fatalf("Haiku line does not show current model: %q\n%s", haikuLine, view)
	}
	if strings.Contains(opusLine, "current") {
		t.Fatalf("Opus line still shows current model: %q\n%s", opusLine, view)
	}
}

func TestSession_SwitchModelKeyboardSelectionAppliesHighlightedModel(t *testing.T) {
	cfg := &config.Config{
		DefaultProvider: "anthropic",
		Providers: map[string]config.ProviderConfig{
			"anthropic": {
				Models: []string{
					"claude-opus-4-5",
					"claude-sonnet-4-6",
					"claude-haiku-4-5-20251001",
				},
			},
		},
		FallbackChain: []config.FallbackEntry{{Provider: "anthropic", Model: "claude-opus-4-5"}},
	}
	s := NewSession(cfg, "", "hi", AgentRunner{
		Provider: "anthropic",
		Model:    "claude-opus-4-5",
	})
	s.running = false
	model, _ := s.handlePaletteCmd("switch-model")
	s = model.(Session)

	for range 2 {
		model, _ = s.Update(tea.KeyMsg{Type: tea.KeyDown})
		s = model.(Session)
	}
	model, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	s = model.(Session)
	if cmd == nil {
		t.Fatal("Enter on highlighted Haiku item returned no command")
	}
	model, runtimeCmd := s.Update(cmd())
	got := model.(Session)
	if got.runner.Model != "claude-haiku-4-5-20251001" {
		t.Fatalf("runner.Model = %q, want highlighted Haiku model", got.runner.Model)
	}
	if runtimeCmd == nil {
		t.Fatal("model selection did not notify root App")
	}
	change, ok := runtimeCmd().(RuntimeModelChangedMsg)
	if !ok || change.Model != "claude-haiku-4-5-20251001" {
		t.Fatalf("runtime change = %#v, want Haiku", change)
	}
}

func TestSession_ModalSelectSwitchModelRequiresCredentialForDefaultProviderModel(t *testing.T) {
	cfg := &config.Config{
		DefaultProvider: "anthropic",
		ApprovalMode:    "full",
		Providers: map[string]config.ProviderConfig{
			"anthropic": {Models: []string{"claude-sonnet-4-6"}},
		},
		FallbackChain: []config.FallbackEntry{{Provider: "anthropic", Model: "claude-sonnet-4-6"}},
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := config.SaveFile(cfg, path); err != nil {
		t.Fatalf("save config: %v", err)
	}

	s := NewSession(cfg, "", "hi", AgentRunner{
		Provider: "anthropic",
		Model:    "claude-sonnet-4-6",
	}, WithConfigPath(path))
	s.running = false
	s.modalCommand = "switch-model"

	model, _ := s.Update(widgets.ModalSelectMsg{Label: "gpt-4o"})
	got := model.(Session)

	if !got.wizardOpen || got.wizardProvider != "openai" {
		t.Fatalf("wizard = open:%v provider:%q, want openai credential wizard", got.wizardOpen, got.wizardProvider)
	}
	if got.runner.Provider != "anthropic" || got.runner.Model != "claude-sonnet-4-6" {
		t.Fatalf("runner changed before verification: %s/%s", got.runner.Provider, got.runner.Model)
	}
	if got := cfg.FallbackChain[0]; got.Provider != "anthropic" {
		t.Fatalf("fallback changed before verification: %#v", got)
	}
}

func TestSession_ProviderForModelUnknownReturnsError(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"anthropic": {Models: []string{"claude-sonnet-4-6"}},
		},
	}
	s := NewSession(cfg, "", "hi", AgentRunner{Provider: "anthropic"})

	if _, err := s.providerForModel("not-a-real-model"); err == nil {
		t.Fatal("expected error for unknown model")
	}
}

func TestSession_ModalSelectSwitchThemeShowsConfirmation(t *testing.T) {
	s := NewSession(nil, "", "hi", AgentRunner{})
	s.running = false
	s.modalCommand = "switch-theme"

	model, _ := s.Update(widgets.ModalSelectMsg{Label: "Dark"})
	got := model.(Session)

	if !got.hasCommandOutput("Theme set to Dark.") {
		t.Fatal("expected theme confirmation message")
	}
}

func TestSession_ModalSelectApprovalModeUpdatesRunner(t *testing.T) {
	s := NewSession(nil, "", "hi", AgentRunner{ApprovalMode: "full"})
	s.running = false
	s.modalCommand = "/approval"

	model, _ := s.Update(widgets.ModalSelectMsg{Label: "dangerous-only"})
	got := model.(Session)

	if got.runner.ApprovalMode != "dangerous-only" {
		t.Fatalf("runner.ApprovalMode = %q, want dangerous-only", got.runner.ApprovalMode)
	}
	if !got.hasCommandOutput("Approval mode set to dangerous-only.") {
		t.Fatal("expected approval confirmation message")
	}
}

func TestSession_ModalSelectInfoOnlyCloses(t *testing.T) {
	for _, cmd := range []string{"/model", "/dir", "/help", "view-status"} {
		t.Run(cmd, func(t *testing.T) {
			s := NewSession(nil, "", "hi", AgentRunner{Model: "m"})
			s.running = false
			s.messages = []chatMsg{{role: "assistant", text: "keep"}}
			s.modalCommand = cmd

			model, _ := s.Update(widgets.ModalSelectMsg{Label: "anything"})
			got := model.(Session)

			if got.modalOpen {
				t.Fatal("modal should be closed")
			}
			if len(got.messages) != 1 {
				t.Fatalf("info-only modal should not add messages, got %d", len(got.messages))
			}
		})
	}
}

func TestSession_ModalSelectNewSessionEmitsCreateMsg(t *testing.T) {
	s := NewSession(nil, "", "hi", AgentRunner{})
	s.running = false
	s.modalCommand = "new-session"

	_, cmd := s.Update(widgets.ModalSelectMsg{Label: "Create session", Value: "summarize repo"})
	if cmd == nil {
		t.Fatal("expected command for new session")
	}
	msg := cmd()
	start, ok := msg.(CreateSessionMsg)
	if !ok {
		t.Fatalf("message = %T, want CreateSessionMsg", msg)
	}
	if start.Title != "summarize repo" {
		t.Fatalf("CreateSessionMsg.Title = %q, want summarize repo", start.Title)
	}
}

func TestSession_ModalSelectNewSessionUsesDefaultTitle(t *testing.T) {
	s := NewSession(nil, "", "hi", AgentRunner{})
	s.running = false
	s.modalCommand = "new-session"

	_, cmd := s.Update(widgets.ModalSelectMsg{Label: "Create session"})
	if cmd == nil {
		t.Fatal("expected create session command")
	}
	msg, ok := cmd().(CreateSessionMsg)
	if !ok || msg.Title != "New session" {
		t.Fatalf("message = %#v, want default CreateSessionMsg", msg)
	}
}

func TestSession_ModalSelectViewStatusIsInfoOnly(t *testing.T) {
	s := NewSession(nil, "", "hi", AgentRunner{
		Provider: "anthropic", Model: "claude-sonnet-4-6",
		Workspace: "C:\\workspace", ApprovalMode: "full",
	})
	s.running = false
	s.messages = []chatMsg{{role: "assistant", text: "keep"}}
	s.modalCommand = "view-status"

	model, _ := s.Update(widgets.ModalSelectMsg{Label: "Model: claude-sonnet-4-6"})
	got := model.(Session)

	if got.modalOpen {
		t.Fatal("view-status should close without action (info-only)")
	}
	if len(got.messages) != 1 {
		t.Fatalf("view-status should not add messages, got %d", len(got.messages))
	}
}

func TestSession_ModalEscClosesWithoutAction(t *testing.T) {
	s := NewSession(nil, "", "hi", AgentRunner{Model: "old"})
	s.running = false
	s.modalCommand = "switch-model"
	s.modalOpen = true
	s.modal = widgets.NewModal("Switch model", []widgets.ModalItem{{Label: "new"}}, 80)
	s.messages = []chatMsg{{role: "assistant", text: "keep"}}

	model, _ := s.Update(widgets.ModalCloseMsg{})
	got := model.(Session)

	if got.modalOpen {
		t.Fatal("modal should be closed after Esc")
	}
	if got.runner.Model != "old" {
		t.Fatalf("runner.Model changed on Esc: %q", got.runner.Model)
	}
	if len(got.messages) != 1 {
		t.Fatalf("Esc should not add messages, got %d", len(got.messages))
	}
}

// hasCommandOutput checks if any assistant message contains the given text.
func (s Session) hasCommandOutput(text string) bool {
	for _, m := range s.messages {
		if m.role == "assistant" && strings.Contains(m.text, text) {
			return true
		}
	}
	return false
}

func writeConfigFile(t *testing.T, path string) {
	t.Helper()
	content := `default_provider: anthropic
approval_mode: full
providers:
  anthropic:
    models:
      - claude-sonnet-4-6
  openai:
    api_key: ${OPENAI_API_KEY}
    models:
      - gpt-4o
      - gpt-4o-mini
fallback_chain:
  - provider: anthropic
    model: claude-sonnet-4-6
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write config file: %v", err)
	}
}

func assertConfigFileContains(t *testing.T, path string, wants ...string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config file: %v", err)
	}
	text := string(data)
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Fatalf("config file missing %q:\n%s", want, text)
		}
	}
}

func TestSession_HelpModalTitleIsKeyboardShortcuts(t *testing.T) {
	s := NewSession(nil, "", "hi", AgentRunner{})
	s.running = false

	model, _ := s.handlePaletteCmd("/help")
	got := model.(Session)

	view := got.modal.View()
	if !strings.Contains(view, "Keyboard Shortcuts") {
		t.Fatalf("help modal should have title 'Keyboard Shortcuts', got:\n%s", view)
	}
}

func TestSession_HideTipsToggle(t *testing.T) {
	s := NewSession(nil, "", "hi", AgentRunner{})
	s.running = false
	s.modalCommand = "hide-tips"

	if content := s.statusContent(40); !strings.Contains(content, "TIPS") {
		t.Fatalf("tips should be visible before toggle:\n%s", content)
	}

	// Hide tips.
	model, _ := s.Update(widgets.ModalSelectMsg{Label: "Hide tips"})
	got := model.(Session)

	if !got.tipsHidden {
		t.Fatal("tipsHidden should be true after selecting 'Hide tips'")
	}
	if !got.hasCommandOutput("Tips hidden.") {
		t.Fatal("expected 'Tips hidden.' confirmation")
	}
	if content := got.statusContent(40); strings.Contains(content, "TIPS") || strings.Contains(content, "Ctrl+P command palette") {
		t.Fatalf("tips should be hidden after selecting 'Hide tips':\n%s", content)
	}

	// Show tips again.
	got.modalCommand = "hide-tips"
	model, _ = got.Update(widgets.ModalSelectMsg{Label: "Show tips"})
	got = model.(Session)

	if got.tipsHidden {
		t.Fatal("tipsHidden should be false after selecting 'Show tips'")
	}
	if !got.hasCommandOutput("Tips visible.") {
		t.Fatal("expected 'Tips visible.' confirmation")
	}
	if content := got.statusContent(40); !strings.Contains(content, "TIPS") || !strings.Contains(content, "Ctrl+P command palette") {
		t.Fatalf("tips should be visible after selecting 'Show tips':\n%s", content)
	}
}

func TestSession_SkillsSelectShowsContent(t *testing.T) {
	s := NewSession(nil, "", "hi", AgentRunner{
		LoadedSkills:  []string{"test-skill"},
		SkillContents: map[string]string{"test-skill": "# Test Skill\nDo something useful."},
	})
	s.running = false
	s.modalCommand = "skills"

	model, cmd := s.Update(widgets.ModalSelectMsg{Label: "test-skill"})
	got := model.(Session)

	if !got.modalOpen {
		t.Fatal("skill-detail modal should be open")
	}
	if got.modalCommand != "skill-detail" {
		t.Fatalf("modalCommand = %q, want skill-detail", got.modalCommand)
	}
	if cmd == nil {
		t.Fatal("expected modal init command")
	}
	view := got.modal.View()
	if !strings.Contains(view, "Test Skill") {
		t.Fatalf("skill detail modal should show content, got:\n%s", view)
	}
}

func TestSession_SkillsSelectMissingFileShowsFallback(t *testing.T) {
	s := NewSession(nil, "", "hi", AgentRunner{
		LoadedSkills: []string{"unknown-skill"},
	})
	s.running = false
	s.modalCommand = "skills"

	model, _ := s.Update(widgets.ModalSelectMsg{Label: "unknown-skill"})
	got := model.(Session)

	if !got.modalOpen {
		t.Fatal("skill-detail modal should be open")
	}
	view := got.modal.View()
	if !strings.Contains(view, "not available") {
		t.Fatalf("expected fallback message, got:\n%s", view)
	}
}

func TestSession_SwitchSessionNewSessionOpensCreationModal(t *testing.T) {
	s := NewSession(nil, "", "hi", AgentRunner{})
	s.running = false
	s.modalCommand = "switch-session"

	model, cmd := s.Update(widgets.ModalSelectMsg{Label: "+ New session"})
	got := model.(Session)
	if !got.modalOpen || got.modalCommand != "new-session" {
		t.Fatalf("new session modal state = open:%v command:%q", got.modalOpen, got.modalCommand)
	}
	if cmd == nil {
		t.Fatal("expected modal init command")
	}
}

func TestSession_SwitchSessionCurrentStaysInSession(t *testing.T) {
	s := NewSession(nil, "", "hi", AgentRunner{})
	s.running = false
	s.modalCommand = "switch-session"

	model, cmd := s.Update(widgets.ModalSelectMsg{Label: "hi"})
	got := model.(Session)

	if cmd != nil {
		t.Fatalf("expected no command for current session, got %T", cmd)
	}
	if !got.hasCommandOutput("Switched to session.") {
		t.Fatal("expected confirmation message")
	}
}

func TestSessionModalItemsGroupsSavedSessionsByDate(t *testing.T) {
	now := time.Date(2026, 6, 21, 20, 0, 0, 0, time.Local)
	s := Session{
		sessionID: "today",
		sessionSummaries: []SessionSummary{
			{ID: "today", Title: "20 MB'dan büyük dosyaları listeleme", UpdatedAt: now.Add(-12 * time.Minute), Active: true},
			{ID: "yesterday", Title: "Dünkü çalışma", UpdatedAt: now.Add(-25 * time.Hour)},
			{ID: "older", Title: "Eski çalışma", UpdatedAt: now.Add(-72 * time.Hour)},
		},
	}

	items := sessionModalItemsAt(s, now)
	var labels []string
	for _, item := range items {
		labels = append(labels, item.Label)
	}
	got := strings.Join(labels, "|")
	for _, want := range []string{
		"+ New session", "Today", "20 MB'dan büyük dosyaları listeleme",
		"Yesterday", "Dünkü çalışma", "Older", "Eski çalışma",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("session items missing %q: %s", want, got)
		}
	}
	if !items[1].Disabled {
		t.Fatal("date heading should be disabled")
	}
}

func TestSession_OpenEditorNotFoundShowsError(t *testing.T) {
	s := NewSession(nil, "", "hi", AgentRunner{Workspace: t.TempDir()})
	s.running = false
	s.modalCommand = "open-editor"

	// Use a binary that almost certainly doesn't exist.
	model, _ := s.Update(widgets.ModalSelectMsg{Label: "NonExistentEditor9999"})
	got := model.(Session)

	if !got.hasCommandOutput("was not found") {
		t.Fatal("expected English not-found error message for missing editor")
	}
}

func TestModelModalItems_IncludesDefaultModels(t *testing.T) {
	cfg := &config.Config{
		DefaultProvider: "anthropic",
		Providers: map[string]config.ProviderConfig{
			"anthropic": {Models: []string{"claude-sonnet-4-6"}},
		},
	}
	runner := AgentRunner{Provider: "anthropic", Model: "claude-sonnet-4-6"}

	items := modelModalItems(cfg, runner)

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	// Should include default anthropic models.
	if !labels["claude-opus-4-5"] {
		t.Fatal("missing default model claude-opus-4-5")
	}
	// Should include models from other providers too.
	if !labels["gpt-4o"] {
		t.Fatal("missing default openai model gpt-4o")
	}
	// Current model should be first.
	if items[0].Label != "claude-sonnet-4-6" {
		t.Fatalf("first item = %q, want claude-sonnet-4-6", items[0].Label)
	}
}

func TestProviderModalItems_IncludesDefaultProviders(t *testing.T) {
	cfg := &config.Config{
		DefaultProvider: "anthropic",
		Providers: map[string]config.ProviderConfig{
			"anthropic": {Models: []string{"claude-sonnet-4-6"}},
		},
	}
	runner := AgentRunner{Provider: "anthropic"}

	items := providerModalItems(cfg, runner, nil)

	// Collect non-header labels.
	labels := make(map[string]bool)
	for _, item := range items {
		if !item.Disabled {
			labels[item.Label] = true
		}
	}

	if !labels["openai"] {
		t.Fatal("missing default provider openai")
	}
	if !labels["gemini"] {
		t.Fatal("missing default provider gemini")
	}
	if !labels["anthropic"] {
		t.Fatal("missing current provider anthropic")
	}
	if !labels["openrouter"] {
		t.Fatal("missing compatible provider openrouter")
	}

	// First item should be a group header.
	if !items[0].Disabled {
		t.Fatalf("first item should be a group header, got %q (disabled=%v)", items[0].Label, items[0].Disabled)
	}
}

func TestProviderModalItems_GroupedLayout(t *testing.T) {
	cfg := &config.Config{
		DefaultProvider: "anthropic",
		Providers: map[string]config.ProviderConfig{
			"anthropic": {APIKey: "key", Models: []string{"claude-sonnet-4-6"}},
		},
	}
	runner := AgentRunner{Provider: "anthropic"}

	items := providerModalItems(cfg, runner, nil)

	// Should have at least two group headers.
	headerCount := 0
	for _, item := range items {
		if item.Disabled {
			headerCount++
		}
	}
	if headerCount < 2 {
		t.Fatalf("expected at least 2 group headers, got %d", headerCount)
	}

	// Anthropic should be "● current".
	for _, item := range items {
		if item.Label == "anthropic" {
			if item.Hint != "● current" {
				t.Errorf("anthropic hint = %q, want %q", item.Hint, "● current")
			}
		}
	}
}

func TestProviderModalItems_ShowsAPIKeyState(t *testing.T) {
	cfg := &config.Config{
		DefaultProvider: "openai",
		Providers: map[string]config.ProviderConfig{
			"openai": {APIKey: "sk-test", Models: []string{"gpt-4o"}},
			"gemini": {Models: []string{"gemini-2.5-flash"}},
		},
	}
	runner := AgentRunner{Provider: "openai"}

	items := providerModalItems(cfg, runner, nil)

	hints := make(map[string]string)
	for _, item := range items {
		if !item.Disabled {
			hints[item.Label] = item.Hint
		}
	}

	if hints["openai"] != "● current" {
		t.Errorf("openai hint = %q, want %q", hints["openai"], "● current")
	}
	if hints["gemini"] != "no API key" {
		t.Errorf("gemini hint = %q, want %q", hints["gemini"], "no API key")
	}
	if hints["anthropic"] != "not configured" {
		t.Errorf("anthropic hint = %q, want %q", hints["anthropic"], "not configured")
	}
}

func TestModelModalItems_NilConfig(t *testing.T) {
	runner := AgentRunner{Provider: "openai", Model: "gpt-4o"}
	items := modelModalItems(nil, runner)

	if len(items) < 2 {
		t.Fatalf("expected default models, got %d items", len(items))
	}
	if items[0].Label != "gpt-4o" {
		t.Fatalf("first item = %q, want gpt-4o", items[0].Label)
	}
}

func TestSession_SpinnerStatusWhileRunning(t *testing.T) {
	s := NewSession(nil, "", "hi", AgentRunner{Provider: "anthropic", Model: "claude-sonnet-4-6"})
	s.running = true
	s.width = 120
	s.height = 30

	content := s.statusContent(40)
	// When running, should not show "○ Idle".
	if strings.Contains(content, "○ Idle") {
		t.Fatal("status should not show Idle while running")
	}
}

func TestSession_SpinnerStatusWhenIdle(t *testing.T) {
	s := NewSession(nil, "", "hi", AgentRunner{Provider: "anthropic", Model: "claude-sonnet-4-6"})
	s.running = false

	content := s.statusContent(40)
	if !strings.Contains(content, "○ Idle") {
		t.Fatalf("status should show Idle when not running, got:\n%s", content)
	}
}

func TestSession_StreamingCursorAppearsInChatBody(t *testing.T) {
	s := Session{
		messages:   []chatMsg{{role: "assistant", text: "hello"}},
		streaming:  true,
		cursorShow: true,
	}

	body := s.buildChatBody(80)
	if !strings.Contains(body, "▌") {
		t.Fatal("streaming cursor should appear in chat body when streaming and cursorShow")
	}
}

func TestSession_NoCursorWhenNotStreaming(t *testing.T) {
	s := Session{
		messages:   []chatMsg{{role: "assistant", text: "hello"}},
		streaming:  false,
		cursorShow: true,
	}

	body := s.buildChatBody(80)
	if strings.Contains(body, "▌") {
		t.Fatal("cursor should not appear when not streaming")
	}
}

func TestContextWindowForModel(t *testing.T) {
	tests := []struct {
		provider string
		model    string
		want     int
	}{
		{"anthropic", "claude-sonnet-4-6", 200_000},
		{"openai", "gpt-4o", 128_000},
		{"gemini", "gemini-2.0-flash", 1_000_000},
		{"unknown", "unknown-model", 128_000},
	}
	for _, tt := range tests {
		got := contextWindowForModel(tt.provider, tt.model)
		if got != tt.want {
			t.Errorf("contextWindowForModel(%q, %q) = %d, want %d", tt.provider, tt.model, got, tt.want)
		}
	}
}

func TestEstimateChunkCost(t *testing.T) {
	cost := estimateChunkCost("anthropic", "claude-sonnet-4-6", 1000)
	// 1000 tokens * $15/1M = $0.015
	if cost < 0.014 || cost > 0.016 {
		t.Fatalf("estimateChunkCost = %f, want ~0.015", cost)
	}
}

func TestFormatCost(t *testing.T) {
	if got := formatCost(0); got != "$0.0000" {
		t.Fatalf("formatCost(0) = %q", got)
	}
	if got := formatCost(0.0123); got != "$0.0123" {
		t.Fatalf("formatCost(0.0123) = %q", got)
	}
}

func TestSession_TokenProgressBarInStatusContent(t *testing.T) {
	s := Session{
		runner:     AgentRunner{Provider: "anthropic", Model: "claude-sonnet-4-6"},
		tokenCount: 20000,
	}

	content := s.statusContent(40)
	if !strings.Contains(content, "█") || !strings.Contains(content, "░") {
		t.Fatalf("status should contain progress bar characters:\n%s", content)
	}
	if !strings.Contains(content, "%") {
		t.Fatal("status should contain percentage")
	}
}

func TestSession_CostInStatusContent(t *testing.T) {
	s := Session{
		runner:      AgentRunner{Provider: "anthropic", Model: "claude-sonnet-4-6"},
		sessionCost: 0.0012,
	}

	content := s.statusContent(40)
	if !strings.Contains(content, "$0.0012") {
		t.Fatalf("status should contain cost, got:\n%s", content)
	}
}

func TestSkillModalItems_Pagination(t *testing.T) {
	skills := make([]string, 20)
	for i := range skills {
		skills[i] = fmt.Sprintf("skill-%02d", i)
	}

	// Page 0 should have 8 skills + 1 navigation item.
	items := skillModalItems(skills, 0)
	if len(items) != 9 {
		t.Fatalf("page 0 items = %d, want 9", len(items))
	}
	if items[0].Label != "skill-00" {
		t.Fatalf("first item = %q, want skill-00", items[0].Label)
	}

	// Page 1 should have 8 skills + 1 navigation item.
	items = skillModalItems(skills, 1)
	if len(items) != 9 {
		t.Fatalf("page 1 items = %d, want 9", len(items))
	}
	if items[0].Label != "skill-08" {
		t.Fatalf("page 1 first item = %q, want skill-08", items[0].Label)
	}

	// Page 2 should have 4 skills + 1 navigation item.
	items = skillModalItems(skills, 2)
	if len(items) != 5 {
		t.Fatalf("page 2 items = %d, want 5", len(items))
	}
}

func TestSkillModalItems_NoPaginationForFewSkills(t *testing.T) {
	skills := []string{"a", "b", "c"}
	items := skillModalItems(skills, 0)
	// No navigation item needed.
	if len(items) != 3 {
		t.Fatalf("items = %d, want 3", len(items))
	}
	for _, item := range items {
		if item.Hint == "navigate" {
			t.Fatal("should not have navigation item for small lists")
		}
	}
}

func TestSession_SkillPaginationItemDoesNotOpenDetail(t *testing.T) {
	skills := make([]string, 9)
	for i := range skills {
		skills[i] = fmt.Sprintf("skill-%02d", i)
	}
	s := NewSession(nil, "", "hi", AgentRunner{LoadedSkills: skills})
	s.running = false
	s.modalCommand = "skills"

	model, cmd := s.Update(widgets.ModalSelectMsg{Label: "← → page 1/2"})
	got := model.(Session)

	if !got.modalOpen {
		t.Fatal("pagination item should keep skills modal open")
	}
	if got.modalCommand != "skills" {
		t.Fatalf("modalCommand = %q, want skills", got.modalCommand)
	}
	if strings.Contains(got.modal.View(), "Skill content not available") {
		t.Fatalf("pagination item should not open skill detail:\n%s", got.modal.View())
	}
	if cmd == nil {
		t.Fatal("expected modal init command")
	}
}

func TestSession_MouseClickOutsideClosesModal(t *testing.T) {
	s := NewSession(nil, "", "hi", AgentRunner{})
	s.running = false
	s.modalOpen = true
	s.modal = widgets.NewModal("Test", []widgets.ModalItem{{Label: "x"}}, 80)

	model, _ := s.Update(tea.MouseMsg{Button: tea.MouseButtonLeft})
	got := model.(Session)

	if got.modalOpen {
		t.Fatal("left click should close modal")
	}
}

func TestSession_MouseClickDoesNotRejectApprovalModal(t *testing.T) {
	ch := make(chan ApprovalResponse, 1)
	s := NewSession(nil, "", "hi", AgentRunner{})
	s.modalOpen = true
	s.approvalPending = true
	s.approvalCh = ch
	s.modal = widgets.NewModal("Approval", []widgets.ModalItem{{Label: "Approve"}}, 80)

	model, _ := s.Update(tea.MouseMsg{Button: tea.MouseButtonLeft})
	got := model.(Session)

	if !got.modalOpen {
		t.Fatal("approval modal should remain open on mouse click")
	}
	if !got.approvalPending {
		t.Fatal("approval should remain pending on mouse click")
	}
	select {
	case resp := <-ch:
		t.Fatalf("mouse click should not send approval response, got %#v", resp)
	default:
	}
}

func TestSession_MouseClickOutsideClosesPalette(t *testing.T) {
	s := NewSession(nil, "", "hi", AgentRunner{})
	s.paletteOpen = true
	s.running = false

	model, _ := s.Update(tea.MouseMsg{Button: tea.MouseButtonLeft})
	got := model.(Session)

	if got.paletteOpen {
		t.Fatal("left click should close palette")
	}
}

func TestSession_MouseDragScrollbarMovesViewport(t *testing.T) {
	tests := []struct {
		name string
		row  func(top, height int) int
		want func(maxOffset int) int
	}{
		{
			name: "top",
			row:  func(top, _ int) int { return top },
			want: func(_ int) int { return 0 },
		},
		{
			name: "middle",
			row:  func(top, height int) int { return top + (height-1)/2 },
			want: func(maxOffset int) int { return maxOffset / 2 },
		},
		{
			name: "bottom",
			row:  func(top, height int) int { return top + height - 1 },
			want: func(maxOffset int) int { return maxOffset },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Session{
				width:  120,
				height: 20,
				chatVP: viewport.New(0, 0),
				runner: AgentRunner{Provider: "test", Model: "m"},
				messages: []chatMsg{
					{role: "assistant", text: strings.Repeat("line\n", 100)},
				},
			}
			s = s.resizeChatVP()
			scrollX, scrollTop, scrollHeight := s.chatScrollbarGeometry()
			maxOffset := s.chatVP.TotalLineCount() - s.chatVP.Height

			model, _ := s.Update(tea.MouseMsg{
				X:      scrollX,
				Y:      tt.row(scrollTop, scrollHeight),
				Button: tea.MouseButtonLeft,
				Action: tea.MouseActionPress,
			})
			got := model.(Session)

			want := tt.want(maxOffset)
			tolerance := max(1, maxOffset/15)
			if delta := got.chatVP.YOffset - want; delta < -tolerance || delta > tolerance {
				t.Fatalf("YOffset = %d, want approximately %d", got.chatVP.YOffset, want)
			}
			if !got.scrollbarDragging {
				t.Fatal("pressing scrollbar should start dragging")
			}

			model, _ = got.Update(tea.MouseMsg{
				X:      scrollX,
				Y:      scrollTop + scrollHeight - 1,
				Button: tea.MouseButtonLeft,
				Action: tea.MouseActionRelease,
			})
			if model.(Session).scrollbarDragging {
				t.Fatal("releasing mouse should stop dragging")
			}
		})
	}
}

func TestSession_CursorBlinkToggle(t *testing.T) {
	s := Session{streaming: true, cursorShow: false}

	model, cmd := s.Update(cursorBlinkMsg{})
	got := model.(Session)

	if !got.cursorShow {
		t.Fatal("cursorShow should toggle to true")
	}
	if cmd == nil {
		t.Fatal("should return next blink command")
	}
}

func TestSession_CursorBlinkStopsWhenIdle(t *testing.T) {
	s := Session{streaming: false, running: false, cursorShow: false}

	model, cmd := s.Update(cursorBlinkMsg{})
	got := model.(Session)

	if got.cursorShow {
		t.Fatal("cursorShow should not toggle when idle")
	}
	if cmd != nil {
		t.Fatalf("expected no next blink command when idle, got %T", cmd)
	}
}

func TestSession_SmallStreamingChunkCountsToken(t *testing.T) {
	s := Session{runner: AgentRunner{Provider: "anthropic", Model: "claude-sonnet-4-6"}}

	model, _ := s.Update(agentMsg{chunk: "a"})
	got := model.(Session)

	if got.tokenCount != 1 {
		t.Fatalf("tokenCount = %d, want 1 for small first chunk", got.tokenCount)
	}
	if got.sessionCost <= 0 {
		t.Fatalf("sessionCost should increase for small first chunk, got %f", got.sessionCost)
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
