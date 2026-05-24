package views

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/halukerenozlu/bolt-cowork/internal/config"
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

func TestSession_ModalSelectSwitchModelUpdatesRunner(t *testing.T) {
	s := NewSession(nil, "", "hi", AgentRunner{Model: "old-model"})
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

func TestSession_ModalSelectConnectProviderUpdatesRunner(t *testing.T) {
	s := NewSession(nil, "", "hi", AgentRunner{Provider: "anthropic"})
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
			"anthropic": {Models: []string{"claude-sonnet-4-6"}},
			"openai":    {Models: []string{"gpt-4o", "gpt-4o-mini"}},
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
	s.modalCommand = "connect-provider"

	model, _ := s.Update(widgets.ModalSelectMsg{Label: "openai"})
	got := model.(Session)

	if got.runner.Provider != "openai" || got.runner.Model != "gpt-4o" {
		t.Fatalf("runner = %s/%s, want openai/gpt-4o", got.runner.Provider, got.runner.Model)
	}
	openAI, ok := cfg.Providers["openai"]
	if !ok {
		t.Fatal("openai provider was not added to config")
	}
	if strings.Join(openAI.Models, "\x00") != strings.Join(config.DefaultModels["openai"], "\x00") {
		t.Fatalf("openai models = %v, want %v", openAI.Models, config.DefaultModels["openai"])
	}
	if got := cfg.FallbackChain[0]; got.Provider != "openai" || got.Model != "gpt-4o" {
		t.Fatalf("fallback[0] = %#v, want openai/gpt-4o", got)
	}
	assertConfigFileContains(t, path, "openai:", "gpt-4o", "gpt-4o-mini")
}

func TestSession_ModalSelectSwitchModelUpdatesProviderForSelectedModel(t *testing.T) {
	cfg := &config.Config{
		DefaultProvider: "anthropic",
		ApprovalMode:    "full",
		Providers: map[string]config.ProviderConfig{
			"anthropic": {Models: []string{"claude-sonnet-4-6"}},
			"openai":    {Models: []string{"gpt-4o"}},
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

func TestSession_ModalSelectSwitchModelUsesDefaultProviderModel(t *testing.T) {
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

	if got.runner.Provider != "openai" || got.runner.Model != "gpt-4o" {
		t.Fatalf("runner = %s/%s, want openai/gpt-4o", got.runner.Provider, got.runner.Model)
	}
	if _, ok := cfg.Providers["openai"]; !ok {
		t.Fatal("openai provider was not added to config")
	}
	if got := cfg.FallbackChain[0]; got.Provider != "openai" || got.Model != "gpt-4o" {
		t.Fatalf("fallback[0] = %#v, want openai/gpt-4o", got)
	}
	assertConfigFileContains(t, path, "default_provider: openai", "openai:", "model: gpt-4o")
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
	for _, cmd := range []string{"/model", "/dir", "/help"} {
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

func TestSession_ModalSelectNewSessionEmitsStartMsg(t *testing.T) {
	s := NewSession(nil, "", "hi", AgentRunner{})
	s.running = false
	s.modalCommand = "new-session"

	_, cmd := s.Update(widgets.ModalSelectMsg{Label: "Create session", Value: "summarize repo"})
	if cmd == nil {
		t.Fatal("expected command for new session")
	}
	msg := cmd()
	start, ok := msg.(StartSessionMsg)
	if !ok {
		t.Fatalf("message = %T, want StartSessionMsg", msg)
	}
	if start.Input != "summarize repo" {
		t.Fatalf("StartSessionMsg.Input = %q, want summarize repo", start.Input)
	}
}

func TestSession_ModalSelectNewSessionRequiresPrompt(t *testing.T) {
	s := NewSession(nil, "", "hi", AgentRunner{})
	s.running = false
	s.modalCommand = "new-session"

	model, cmd := s.Update(widgets.ModalSelectMsg{Label: "Create session"})
	got := model.(Session)

	if cmd != nil {
		t.Fatalf("expected no command for empty session prompt, got %T", cmd)
	}
	if !got.hasCommandOutput("Enter a prompt to start a new session.") {
		t.Fatal("expected prompt warning")
	}
}

func TestSession_ModalSelectViewStatusOpensSubModal(t *testing.T) {
	s := NewSession(nil, "", "hi", AgentRunner{
		Provider: "anthropic", Model: "claude-sonnet-4-6",
		Workspace: "C:\\workspace", ApprovalMode: "full",
	})
	s.running = false
	s.modalCommand = "view-status"

	model, _ := s.Update(widgets.ModalSelectMsg{Label: "Model: claude-sonnet-4-6"})
	got := model.(Session)

	if !got.modalOpen {
		t.Fatal("view-status should open a sub-modal for the selected item")
	}
	if got.modalCommand != "switch-model" {
		t.Fatalf("sub-modal command = %q, want switch-model", got.modalCommand)
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

	items := providerModalItems(cfg, runner)
	wantOrder := []string{"anthropic", "openai", "gemini"}
	for i, want := range wantOrder {
		if items[i].Label != want {
			t.Fatalf("item[%d] = %q, want %q", i, items[i].Label, want)
		}
	}

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
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

type assertErr string

func (e assertErr) Error() string { return string(e) }
