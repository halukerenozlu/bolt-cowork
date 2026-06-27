package views

import (
	"context"
	"fmt"
	"math"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/paginator"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/halukerenozlu/bolt-cowork/internal/config"
	"github.com/halukerenozlu/bolt-cowork/internal/provider"
	"github.com/halukerenozlu/bolt-cowork/internal/ui/theme"
	"github.com/halukerenozlu/bolt-cowork/internal/ui/widgets"
	"github.com/halukerenozlu/bolt-cowork/pkg/types"
)

// chatMsg holds one entry in the conversation for display.
type chatMsg struct {
	role string // "user" or "assistant"
	text string
}

// agentMsg is the unified message type for the agent stream.
// Exactly one of chunk, event, or done is set per message.
type agentMsg struct {
	chunk  string  // non-empty for text chunks
	event  UIEvent // non-nil for structured live-update events
	done   bool    // true when the run has finished
	result AgentResult
	ch     <-chan agentMsg // back-ref so Update can schedule the next read
}

// gitDirtyMsg is the result of an async git status check.
type gitDirtyMsg struct{ dirty bool }

// cursorBlinkMsg toggles the streaming cursor visibility.
type cursorBlinkMsg struct{}

// ReturnToWelcomeMsg signals the App to close the current session and return
// to the welcome screen without quitting.
type ReturnToWelcomeMsg struct{}

// minWidthForRightPanel is the terminal width below which the right panel
// collapses to save horizontal space.
const minWidthForRightPanel = 80

var approvalResponseLabels sync.Map

func approvalResponseKey(ch any) uintptr { return reflect.ValueOf(ch).Pointer() }

// ApprovalResponseLabel returns the modal label selected for ch.
func ApprovalResponseLabel(ch <-chan ApprovalResponse) string {
	label, ok := approvalResponseLabels.LoadAndDelete(approvalResponseKey(ch))
	if !ok {
		return ""
	}
	s, _ := label.(string)
	return s
}

// Session is the bubbletea model for the active work area after the user
// sends their first message. It shows a 70/30 split: chat on the left (with
// inline text input at the bottom), status info on the right.
type Session struct {
	width  int
	height int

	runner           AgentRunner
	version          string
	sessionID        string
	sessionTitle     string
	sessionSummaries []SessionSummary
	gitBranch        string
	gitDirty         bool

	// Chat state.
	messages []chatMsg
	history  []types.Message
	running  bool

	// Plan and execution state for the current run.
	planActive  bool     // true when the current run has a plan
	planSteps   []string // step descriptions from PlanReadyEvent
	stepDone    []bool   // stepDone[i] is true when step i has completed
	stepErrors  []error  // stepErrors[i] holds the error for step i (nil = success)
	execLog     []string // one line per completed step
	runResponse string   // final response accumulated while a plan is active

	// Live agent action state (updated by StepStartEvent / StepDoneEvent).
	activeAction string // current step action type ("read", "write", etc.)
	activeTarget string // current step description (truncated)
	currentStep  int    // 0-based index of active step (-1 = idle)

	// Provider fallback tracking.
	activeProvider string // provider that actually handled the last request
	activeModel    string // model that actually handled the last request
	fallbackReason string // why fallback occurred (empty = no fallback)

	// MCP tracking — last completed MCP tool call.
	lastMCPTool   string // "server/tool" identifier
	lastMCPStatus string // "ok" or "error"
	lastMCPOutput string // first line of output

	// Permission warning — last auto-approved dangerous action.
	lastPermWarn string

	// Pending approval request from the agent goroutine.
	approvalPending bool
	approvalCh      chan<- ApprovalResponse

	// Loaded skills at session startup.
	loadedSkills []string

	// Estimated token count (cumulative for the session).
	tokenCount     int
	tokenByteCount int

	// Streaming cursor state.
	streaming  bool // true while chunks are arriving
	cursorShow bool // blink toggle for the ▌ cursor

	// Estimated session cost in USD.
	sessionCost float64

	// Chat viewport provides fixed-height scrolling for the message area.
	// Content is rebuilt via rebuildChatVP whenever messages or plan state change.
	chatVP  viewport.Model
	chatVPW int // inner content width used to build viewport body
	// scrollbarDragging tracks a left-button drag on the chat scrollbar.
	scrollbarDragging bool

	// Input widget at the bottom of the chat panel.
	input textinput.Model

	// Spinner shown while the agent is running without a plan.
	spinner spinner.Model

	// Context used to cancel an in-flight agent call.
	ctx    context.Context
	cancel context.CancelFunc

	// Configuration reference for persisting modal selections.
	cfg        *config.Config
	configPath string

	// Command palette overlay.
	palette      widgets.Palette
	paletteOpen  bool
	modal        widgets.Modal
	modalOpen    bool
	modalCommand string
	modalTarget  string

	// tipsHidden controls whether tips are shown in the right panel.
	tipsHidden bool

	// skillContents maps skill names to their loaded SKILL.md content.
	skillContents map[string]string

	// Skills paginator for when there are more than skillsPerPage skills.
	skillPaginator paginator.Model

	// chordActive is true after ctrl+x is pressed; the next key completes
	// the chord (e.g. ctrl+x l → switch session).
	chordActive bool

	// Connection wizard state.
	wizardOpen     bool            // true when the wizard overlay is active
	wizardProvider string          // provider being configured
	wizardStep     int             // 0=auth-method, 1=key-entry, 2=verifying, 3=model-select
	wizardInput    textinput.Model // masked API key input (step 1)
	wizardModels   []string        // discovered models (step 3)
	wizardCursor   int             // cursor for auth-method and model-select steps
	wizardErr      string          // last error message
	wizardAPIKey   string          // credential being verified (memory only)
	wizardPersist  bool            // persist credential after successful verification

	// Local provider detection cache (refreshed when provider modal opens).
	localDetected map[string]LocalProviderInfo
}

// SessionOption configures optional Session dependencies.
type SessionOption func(*Session)

// WithConfigPath sets the config file path used when persisting modal choices.
func WithConfigPath(path string) SessionOption {
	return func(s *Session) { s.configPath = path }
}

// WithSkillContents sets the loaded skill content used by the Skills modal.
func WithSkillContents(contents map[string]string) SessionOption {
	return func(s *Session) { s.skillContents = contents }
}

func WithSessionState(id, title string, summaries []SessionSummary) SessionOption {
	return func(s *Session) {
		s.sessionID = id
		s.sessionTitle = title
		s.sessionSummaries = append([]SessionSummary(nil), summaries...)
	}
}

func WithRestoredSnapshot(snapshot SessionSnapshot) SessionOption {
	return func(s *Session) {
		s.sessionID = snapshot.ID
		s.sessionTitle = snapshot.Title
		s.runner.Provider = snapshot.Provider
		s.runner.Model = snapshot.Model
		s.messages = make([]chatMsg, 0, len(snapshot.Messages))
		for _, message := range snapshot.Messages {
			s.messages = append(s.messages, chatMsg{role: message.Role, text: message.Text})
		}
		s.history = append([]types.Message(nil), snapshot.History...)
		s.tokenCount = snapshot.TokenCount
		s.tokenByteCount = snapshot.TokenBytes
		s.sessionCost = snapshot.SessionCost
		s.running = false
		s.input.Focus()
	}
}

// NewSession creates a Session seeded with the user's first message.
// The agent is started via Init() immediately after creation.
func NewSession(cfg *config.Config, version string, firstMsg string, runner AgentRunner, opts ...SessionOption) Session {
	ctx, cancel := context.WithCancel(context.Background())

	ti := textinput.New()
	ti.Placeholder = "Ask a follow-up..."
	ti.Prompt = ""
	ti.CharLimit = 512

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = theme.TitleStyle

	vp := viewport.New(0, 0)

	pg := paginator.New()
	pg.Type = paginator.Dots
	totalPages := (len(runner.LoadedSkills) + skillsPerPage - 1) / skillsPerPage
	if totalPages < 1 {
		totalPages = 1
	}
	pg.SetTotalPages(totalPages)
	pg.PerPage = skillsPerPage

	session := Session{
		cfg:            cfg,
		runner:         runner,
		version:        version,
		gitBranch:      fetchGitBranch(runner.Workspace),
		gitDirty:       fetchGitDirty(runner.Workspace),
		loadedSkills:   runner.LoadedSkills,
		skillContents:  runner.SkillContents,
		messages:       []chatMsg{{role: "user", text: firstMsg}},
		running:        true,
		currentStep:    -1,
		chatVP:         vp,
		input:          ti,
		spinner:        sp,
		skillPaginator: pg,
		ctx:            ctx,
		cancel:         cancel,
	}
	for _, opt := range opts {
		opt(&session)
	}
	return session
}

// fetchGitBranch returns the current git branch name, or "" if unavailable.
func fetchGitBranch(workspace string) string {
	cmd := exec.Command("git", "symbolic-ref", "--short", "HEAD")
	if workspace != "" {
		cmd.Dir = workspace
	}
	out, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}

	cmd = exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	if workspace != "" {
		cmd.Dir = workspace
	}
	out, err = cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	return ""
}

// fetchGitDirty reports whether the workspace has uncommitted changes.
func fetchGitDirty(workspace string) bool {
	cmd := exec.Command("git", "status", "--porcelain")
	if workspace != "" {
		cmd.Dir = workspace
	}
	out, err := cmd.Output()
	return err == nil && len(strings.TrimSpace(string(out))) > 0
}

// fetchGitDirtyCmd returns a tea.Cmd that checks git dirty state asynchronously
// and sends a gitDirtyMsg back to the Update loop.
func fetchGitDirtyCmd(workspace string) tea.Cmd {
	return func() tea.Msg {
		return gitDirtyMsg{dirty: fetchGitDirty(workspace)}
	}
}

// Init implements tea.Model.
func (s Session) Init() tea.Cmd {
	if !s.running || len(s.messages) == 0 {
		return tea.Batch(
			func() tea.Msg { return tea.EnableMouseCellMotion() },
			fetchGitDirtyCmd(s.runner.Workspace),
		)
	}
	return tea.Batch(
		s.spinner.Tick,
		cursorBlinkCmd(),
		runAgentCmd(s.ctx, s.runner, s.messages[0].text, s.history),
		func() tea.Msg { return tea.EnableMouseCellMotion() },
	)
}

// cursorBlinkCmd returns a tea.Cmd that sends a cursorBlinkMsg after 500ms.
func cursorBlinkCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
		return cursorBlinkMsg{}
	})
}

// runAgentCmd spawns a goroutine that runs the agent and returns a tea.Cmd
// that reads the first message from the unified agent stream.
func runAgentCmd(ctx context.Context, runner AgentRunner, cmd string, history []types.Message) tea.Cmd {
	msgCh := make(chan agentMsg, 128)

	go func() {
		result := runner.Run(ctx, cmd, history,
			func(chunk string) {
				select {
				case msgCh <- agentMsg{chunk: chunk}:
				case <-ctx.Done():
				}
			},
			func(event UIEvent) {
				select {
				case msgCh <- agentMsg{event: event}:
				case <-ctx.Done():
				}
			},
		)
		select {
		case msgCh <- agentMsg{done: true, result: result}:
		case <-ctx.Done():
		}
	}()

	return waitNext(msgCh)
}

// waitNext returns a tea.Cmd that blocks until the next agentMsg is available.
func waitNext(ch <-chan agentMsg) tea.Cmd {
	return func() tea.Msg {
		m := <-ch
		if !m.done {
			m.ch = ch
		}
		return m
	}
}

func (s Session) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.width = msg.Width
		s.height = msg.Height
		s = s.resizeChatVP()
		return s, nil

	case gitDirtyMsg:
		s.gitDirty = msg.dirty
		return s, nil

	case ProviderVerifyResultMsg:
		if msg.Err != nil {
			s = s.appendCommandOutput("Verification failed for " + msg.Provider + ": " + msg.Err.Error())
			return s, nil
		}
		s = s.commitProviderSwitch(msg.Provider, msg.Model)
		return s, func() tea.Msg {
			return RuntimeModelChangedMsg{Provider: msg.Provider, Model: msg.Model}
		}

	case WizardVerifyResultMsg:
		return s.updateWizard(msg)
	case WizardModelsResultMsg:
		return s.updateWizard(msg)
	case LocalDetectResultMsg:
		s.localDetected = msg.Detected
		if s.modalOpen && s.modalCommand == "connect-provider" {
			s.modal = s.modal.ReplaceItems(providerModalItems(s.cfg, s.runner, s.localDetected))
		}
		return s, nil

	case tea.MouseMsg:
		if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
			var cmd tea.Cmd
			s.chatVP, cmd = s.chatVP.Update(msg)
			return s, cmd
		}
		if s.scrollbarDragging {
			if msg.Action == tea.MouseActionMotion || msg.Action == tea.MouseActionRelease {
				s = s.scrollChatToMouseY(msg.Y)
			}
			if msg.Action == tea.MouseActionRelease {
				s.scrollbarDragging = false
			}
			return s, nil
		}
		if !s.modalOpen && !s.paletteOpen &&
			msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
			scrollX, scrollTop, scrollHeight := s.chatScrollbarGeometry()
			if msg.X == scrollX && msg.Y >= scrollTop && msg.Y < scrollTop+scrollHeight &&
				s.chatVP.TotalLineCount() > s.chatVP.Height {
				s.scrollbarDragging = true
				s = s.scrollChatToMouseY(msg.Y)
				return s, nil
			}
		}
		// Click outside modal/palette closes it.
		if msg.Button == tea.MouseButtonLeft {
			if s.modalOpen {
				if s.approvalPending {
					return s, nil
				}
				s.modalOpen = false
				if !s.running {
					s.input.Focus()
				}
				return s, nil
			}
			if s.paletteOpen {
				s.paletteOpen = false
				if !s.running {
					s.input.Focus()
				}
				return s, nil
			}
		}
		return s, nil

	case tea.KeyMsg:
		// Ctrl+C always quits.
		if msg.Type == tea.KeyCtrlC {
			s.cancel()
			return s, tea.Quit
		}

		if s.wizardOpen {
			return s.updateWizard(msg)
		}

		if s.modalOpen {
			// Handle pagination for skills modal.
			if s.modalCommand == "skills" && len(s.loadedSkills) > skillsPerPage {
				if msg.Type == tea.KeyLeft || msg.Type == tea.KeyRight {
					totalPages := (len(s.loadedSkills) + skillsPerPage - 1) / skillsPerPage
					if msg.Type == tea.KeyLeft && s.skillPaginator.Page > 0 {
						s.skillPaginator.Page--
					} else if msg.Type == tea.KeyRight && s.skillPaginator.Page < totalPages-1 {
						s.skillPaginator.Page++
					}
					s.modal = widgets.NewModal("Skills", skillModalItems(s.loadedSkills, s.skillPaginator.Page), s.width)
					return s, s.modal.Init()
				}
			}
			m, cmd := s.modal.Update(msg)
			s.modal = m.(widgets.Modal)
			return s, cmd
		}

		// Ctrl+P toggles the command palette.
		if msg.Type == tea.KeyCtrlP {
			s.chordActive = false
			if s.paletteOpen {
				s.paletteOpen = false
				if !s.running {
					s.input.Focus()
				}
			} else {
				s.paletteOpen = true
				s.palette = widgets.NewPalette(s.width)
				return s, s.palette.Init()
			}
			return s, nil
		}

		// Route all keys to palette when it's open.
		if s.paletteOpen {
			m, cmd := s.palette.Update(msg)
			s.palette = m.(widgets.Palette)
			return s, cmd
		}

		// Handle ctrl+x chord prefix.
		if msg.Type == tea.KeyCtrlX {
			s.chordActive = true
			return s, nil
		}

		// Complete a pending ctrl+x chord.
		if s.chordActive {
			s.chordActive = false
			switch msg.String() {
			case "l":
				return s.handlePaletteCmd("switch-session")
			case "m":
				return s.handlePaletteCmd("switch-model")
			case "e":
				return s.handlePaletteCmd("open-editor")
			case "n":
				return s.handlePaletteCmd("new-session")
			case "h":
				return s.handlePaletteCmd("hide-tips")
			case "s":
				return s.handlePaletteCmd("view-status")
			case "t":
				return s.handlePaletteCmd("switch-theme")
			}
			return s, nil
		}

		// Forward scroll keys to chat viewport.
		switch msg.Type {
		case tea.KeyPgUp, tea.KeyPgDown:
			var cmd tea.Cmd
			s.chatVP, cmd = s.chatVP.Update(msg)
			return s, cmd
		}

		// Normal input handling.
		switch msg.Type {
		case tea.KeyEnter:
			if s.running {
				return s, nil
			}
			text := strings.TrimSpace(s.input.Value())
			if text == "" {
				return s, nil
			}
			s.input.Reset()
			if command, ok := normalizeTypedCommand(text); ok {
				return s.handlePaletteCmd(command)
			}
			s.messages = append(s.messages, chatMsg{role: "user", text: text})
			// Reset per-run state.
			s.planActive = false
			s.planSteps = nil
			s.stepDone = nil
			s.stepErrors = nil
			s.execLog = nil
			s.activeAction = ""
			s.activeTarget = ""
			s.activeProvider = ""
			s.activeModel = ""
			s.fallbackReason = ""
			s.currentStep = -1
			s.running = true
			s = s.rebuildChatVP()
			s.chatVP.GotoBottom()
			return s, tea.Batch(
				s.spinner.Tick,
				cursorBlinkCmd(),
				runAgentCmd(s.ctx, s.runner, text, s.history),
			)
		}
		if !s.running {
			var cmd tea.Cmd
			s.input, cmd = s.input.Update(msg)
			return s, cmd
		}
		return s, nil

	case widgets.PaletteSelectMsg:
		s.paletteOpen = false
		return s.handlePaletteCmd(msg.Command)

	case widgets.PaletteCloseMsg:
		s.paletteOpen = false
		if !s.running {
			s.input.Focus()
		}
		return s, nil

	case widgets.ModalSelectMsg:
		s.modalOpen = false
		if s.approvalPending && s.approvalCh != nil {
			s.approvalPending = false
			approvalResponseLabels.Store(approvalResponseKey(s.approvalCh), msg.Label)
			approved := msg.Label == "Approve" || msg.Label == "Approve all" || msg.Label == "Revise"
			s.approvalCh <- ApprovalResponse{Approved: approved}
			s.approvalCh = nil
		} else {
			return s.handleModalSelect(msg)
		}
		if !s.running {
			s.input.Focus()
		}
		return s, nil

	case widgets.ModalActionMsg:
		if s.modalCommand != "switch-session" || msg.Key == "" {
			return s, nil
		}
		s.modalTarget = msg.Key
		switch msg.Action {
		case "rename":
			s.modal = widgets.NewInputModal(
				"Rename session", "New title...",
				[]widgets.ModalItem{
					{Label: "Rename", Hint: "enter"},
					{Label: "Cancel", Hint: "esc"},
				},
				s.width,
			)
			s.modalCommand = "rename-session"
			s.modalOpen = true
			return s, s.modal.Init()
		case "delete":
			s.modal = widgets.NewModal(
				"Delete session?",
				[]widgets.ModalItem{
					{Label: "Delete", Hint: msg.Label},
					{Label: "Cancel", Hint: "keep session"},
				},
				s.width,
			)
			s.modalCommand = "delete-session"
			s.modalOpen = true
			return s, s.modal.Init()
		}
		return s, nil

	case widgets.ModalCloseMsg:
		s.modalOpen = false
		if s.approvalPending && s.approvalCh != nil {
			// Closing the modal without selecting = reject.
			s.approvalPending = false
			approvalResponseLabels.Store(approvalResponseKey(s.approvalCh), "Reject")
			s.approvalCh <- ApprovalResponse{Approved: false}
			s.approvalCh = nil
		}
		if !s.running {
			s.input.Focus()
		}
		return s, nil

	case cursorBlinkMsg:
		if !s.running && !s.streaming {
			return s, nil
		}
		s.cursorShow = !s.cursorShow
		if s.streaming {
			s = s.rebuildChatVP()
		}
		return s, cursorBlinkCmd()

	case spinner.TickMsg:
		if !s.running {
			return s, nil
		}
		var cmd tea.Cmd
		s.spinner, cmd = s.spinner.Update(msg)
		return s, cmd

	case agentMsg:
		if msg.done {
			s.running = false
			s.streaming = false
			s.activeAction = ""
			s.activeTarget = ""
			s.currentStep = -1
			if msg.result.Err != nil {
				s = s.appendToAssistant("Error: " + displayAgentError(msg.result.Err))
				s.planActive = false
			} else {
				s.history = msg.result.History
				s = s.finishActiveRun()
			}
			s = s.rebuildChatVP()
			s.chatVP.GotoBottom()
			s.input.Focus()
			// Re-check git dirty state after agent may have modified files.
			return s, tea.Batch(
				fetchGitDirtyCmd(s.runner.Workspace),
				func() tea.Msg { return SaveSessionMsg{Snapshot: s.Snapshot()} },
			)
		}
		if msg.chunk != "" {
			prevTokens := estimateTokensFromBytes(s.tokenByteCount)
			s.tokenByteCount += len(msg.chunk)
			chunkTokens := estimateTokensFromBytes(s.tokenByteCount) - prevTokens
			s.tokenCount += chunkTokens
			s.sessionCost += estimateChunkCost(s.runner.Provider, s.runner.Model, chunkTokens)
			s.streaming = true
			if s.planActive {
				s.runResponse += msg.chunk
			} else {
				s = s.appendToAssistant(msg.chunk)
			}
		}
		if msg.event != nil {
			s = s.handleUIEvent(msg.event)
		}
		if msg.chunk != "" || msg.event != nil {
			s = s.rebuildChatVP()
			s.chatVP.GotoBottom()
		}
		return s, waitNext(msg.ch)
	}

	return s, nil
}

func (s Session) finishActiveRun() Session {
	if !s.planActive {
		return s
	}

	var b strings.Builder
	if len(s.planSteps) <= 1 {
		// Single-step tasks (e.g. a read or list) don't need a PLAN block —
		// a short activity line plus the actual result is enough.
		if len(s.planSteps) == 1 {
			fmt.Fprintf(&b, "→ %s\n\n", s.planSteps[0])
		}
		for i, line := range s.execLog {
			if i > 0 {
				b.WriteString("\n")
			}
			b.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "v "), "x "))
		}
		if response := strings.TrimSpace(s.runResponse); response != "" {
			if b.Len() > 0 {
				b.WriteString("\n\n")
			}
			b.WriteString(response)
		}
	} else {
		b.WriteString("PLAN\n")
		for i, step := range s.planSteps {
			mark := "[ ]"
			if i < len(s.stepErrors) && s.stepErrors[i] != nil {
				mark = "[x]"
			} else if i < len(s.stepDone) && s.stepDone[i] {
				mark = "[✓]"
			}
			fmt.Fprintf(&b, "%d. %s %s\n", i+1, mark, step)
		}
		if len(s.execLog) > 0 {
			b.WriteString("\n")
			for _, line := range s.execLog {
				b.WriteString(line)
				b.WriteString("\n")
			}
		}
		if response := strings.TrimSpace(s.runResponse); response != "" {
			b.WriteString("\n")
			b.WriteString(response)
		}
	}

	s.messages = append(s.messages, chatMsg{role: "assistant", text: strings.TrimSpace(b.String())})
	s.planActive = false
	s.planSteps = nil
	s.stepDone = nil
	s.stepErrors = nil
	s.execLog = nil
	s.runResponse = ""
	return s
}

func (s Session) Snapshot() SessionSnapshot {
	messages := make([]SessionMessage, 0, len(s.messages))
	for _, message := range s.messages {
		messages = append(messages, SessionMessage{Role: message.role, Text: message.text})
	}
	return SessionSnapshot{
		ID:          s.sessionID,
		Title:       s.sessionTitle,
		Provider:    s.runner.Provider,
		Model:       s.runner.Model,
		Messages:    messages,
		History:     append([]types.Message(nil), s.history...),
		TokenCount:  s.tokenCount,
		TokenBytes:  s.tokenByteCount,
		SessionCost: s.sessionCost,
	}
}

func (s Session) SetSessionSummaries(summaries []SessionSummary) Session {
	s.sessionSummaries = append([]SessionSummary(nil), summaries...)
	return s
}

func (s Session) AddNotice(text string) Session {
	s = s.appendCommandOutput(text)
	return s.rebuildChatVP()
}

func (s Session) ApplySnapshot(snapshot SessionSnapshot) Session {
	if snapshot.ID != "" && s.sessionID != "" && snapshot.ID != s.sessionID {
		return s
	}
	s.sessionID = snapshot.ID
	s.sessionTitle = snapshot.Title
	s.runner.Provider = snapshot.Provider
	s.runner.Model = snapshot.Model
	s.messages = make([]chatMsg, 0, len(snapshot.Messages))
	for _, message := range snapshot.Messages {
		s.messages = append(s.messages, chatMsg{role: message.Role, text: message.Text})
	}
	s.history = append([]types.Message(nil), snapshot.History...)
	s.tokenCount = snapshot.TokenCount
	s.tokenByteCount = snapshot.TokenBytes
	s.sessionCost = snapshot.SessionCost
	return s.rebuildChatVP()
}

// handlePaletteCmd executes the selected palette command and returns the
// updated model and a tea.Cmd.
func (s Session) handlePaletteCmd(name string) (tea.Model, tea.Cmd) {
	if !s.running {
		s.input.Focus()
	}
	switch name {
	case "/clear":
		if s.running {
			s = s.appendCommandOutput("Cannot clear while agent is running.")
			return s, nil
		}
		s.messages = nil
		s.history = nil
		s.planActive = false
		s.planSteps = nil
		s.stepDone = nil
		s.stepErrors = nil
		s.execLog = nil
		s.tokenCount = 0
		s.tokenByteCount = 0
		s.sessionCost = 0
		s.activeAction = ""
		s.activeTarget = ""
		s.currentStep = -1
		s = s.rebuildChatVP()
	case "/help":
		return s.openCommandModal(name)
	case "/model":
		return s.openCommandModal(name)
	case "/dir":
		return s.openCommandModal(name)
	case "/approval":
		return s.openCommandModal(name)
	case "/quit":
		s.cancel()
		return s, tea.Quit

	case "switch-session":
		return s.openCommandModal(name)
	case "switch-model":
		return s.openCommandModal(name)
	case "connect-provider":
		return s.openCommandModal(name)
	case "open-editor":
		return s.openCommandModal(name)
	case "new-session":
		return s.openCommandModal(name)
	case "skills":
		return s.openCommandModal(name)
	case "hide-tips":
		return s.openCommandModal(name)
	case "view-status":
		return s.openCommandModal(name)
	case "switch-theme":
		return s.openCommandModal(name)
	case "open-docs":
		if err := openURL("https://halukerenozlu.github.io/bolt-cowork/docs/"); err != nil {
			s = s.appendCommandOutput("Could not open browser: " + err.Error())
		} else {
			s = s.appendCommandOutput("Opened docs in browser.")
		}
		s.paletteOpen = false
		return s, nil
	}
	return s, nil
}

func (s Session) openCommandModal(name string) (tea.Model, tea.Cmd) {
	modal := s.commandModal(name)
	s.modal = modal
	s.modalOpen = true
	s.modalCommand = name
	s.paletteOpen = false

	cmds := []tea.Cmd{modal.Init()}
	if name == "connect-provider" {
		cmds = append(cmds, detectLocalProvidersCmd())
	}
	return s, tea.Batch(cmds...)
}

func detectLocalProvidersCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		detected := provider.DetectLocal(ctx)
		result := make(map[string]LocalProviderInfo)
		for _, lp := range detected {
			result[lp.Name] = LocalProviderInfo{
				Endpoint: lp.Endpoint,
				Models:   append([]string(nil), lp.Models...),
			}
		}
		return LocalDetectResultMsg{Detected: result}
	}
}

func (s Session) commandModal(name string) widgets.Modal {
	switch name {
	case "switch-session":
		return widgets.NewModal("Switch session", sessionModalItems(s), s.width)
	case "switch-model":
		return widgets.NewModal("Switch model", modelModalItems(s.cfg, s.runner), s.width)
	case "connect-provider":
		return widgets.NewModal("Connect provider", providerModalItems(s.cfg, s.runner, s.localDetected), s.width)
	case "open-editor":
		return widgets.NewModal("Open editor", []widgets.ModalItem{
			{Label: "VS Code", Hint: "code"},
			{Label: "Cursor", Hint: "cursor"},
			{Label: "Notepad", Hint: "notepad"},
			{Label: "Vim", Hint: "vim"},
		}, s.width)
	case "new-session":
		return widgets.NewInputModal("New session", "Session name...", []widgets.ModalItem{
			{Label: "Create session", Hint: "enter"},
			{Label: "Cancel", Hint: "esc"},
		}, s.width)
	case "skills":
		return widgets.NewModal("Skills", skillModalItems(s.loadedSkills, s.skillPaginator.Page), s.width)
	case "hide-tips":
		items := []widgets.ModalItem{
			{Label: "Show tips", Hint: "enable"},
			{Label: "Hide tips", Hint: "disable"},
		}
		if s.tipsHidden {
			items[1].Hint = "current"
		} else {
			items[0].Hint = "current"
		}
		return widgets.NewModal("Tips visibility", items, s.width)
	case "view-status":
		return widgets.NewModal("View status", statusModalItems(s), s.width)
	case "switch-theme":
		return widgets.NewModal("Switch theme", []widgets.ModalItem{
			{Label: "System", Hint: "default"},
			{Label: "Dark", Hint: "terminal"},
			{Label: "Light", Hint: "terminal"},
		}, s.width)
	case "/model":
		return widgets.NewModal("Show model", []widgets.ModalItem{
			{Label: s.runner.Model, Hint: "current model"},
		}, s.width)
	case "/dir":
		return widgets.NewModal("Show directory", []widgets.ModalItem{
			{Label: s.runner.Workspace, Hint: "workspace"},
		}, s.width)
	case "/approval":
		return widgets.NewModal("Show approval", approvalModalItems(s.runner.ApprovalMode), s.width)
	case "/help":
		return widgets.NewModal("Keyboard Shortcuts", helpModalItems(), s.width)
	default:
		return widgets.NewModal("Command", []widgets.ModalItem{{Label: name}}, s.width)
	}
}

func sessionModalItems(s Session) []widgets.ModalItem {
	return sessionModalItemsAt(s, time.Now())
}

func sessionModalItemsAt(s Session, now time.Time) []widgets.ModalItem {
	items := []widgets.ModalItem{{Label: "+ New session", Hint: "new"}}
	if len(s.sessionSummaries) == 0 {
		if s.sessionID == "" && len(s.messages) == 0 {
			return items
		}
		label := "Current session"
		if len(s.messages) > 0 && strings.TrimSpace(s.messages[0].text) != "" {
			label = truncatePlain(s.messages[0].text, 42)
		}
		return append(items, widgets.ModalItem{Label: label, Hint: "active", Key: s.sessionID})
	}
	today := localDate(now)
	yesterday := today.AddDate(0, 0, -1)
	groups := []struct {
		label string
		match func(time.Time) bool
	}{
		{label: "Today", match: func(t time.Time) bool { return localDate(t).Equal(today) }},
		{label: "Yesterday", match: func(t time.Time) bool { return localDate(t).Equal(yesterday) }},
		{label: "Older", match: func(t time.Time) bool { return localDate(t).Before(yesterday) }},
	}
	for _, group := range groups {
		start := len(items)
		items = append(items, widgets.ModalItem{Label: group.label, Disabled: true})
		for _, summary := range s.sessionSummaries {
			if !group.match(summary.UpdatedAt) {
				continue
			}
			hint := summary.UpdatedAt.Local().Format("15:04")
			if summary.Active {
				hint = "current"
			}
			items = append(items, widgets.ModalItem{
				Label: summary.Title,
				Hint:  hint,
				Key:   summary.ID,
			})
		}
		if len(items) == start+1 {
			items = items[:start]
		}
	}
	items = append(items, widgets.ModalItem{
		Label:    "rename ctrl+r   delete ctrl+d",
		Disabled: true,
	})
	return items
}

func localDate(t time.Time) time.Time {
	local := t.Local()
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
}

func modelModalItems(cfg *config.Config, runner AgentRunner) []widgets.ModalItem {
	seen := map[string]bool{}
	var items []widgets.ModalItem
	add := func(model, hint string) {
		model = strings.TrimSpace(model)
		if model == "" || seen[model] {
			return
		}
		seen[model] = true
		items = append(items, widgets.ModalItem{Label: model, Hint: hint})
	}

	add(runner.Model, "current")

	provider := runner.Provider
	if provider == "" && cfg != nil {
		provider = cfg.DefaultProvider
	}
	if cfg != nil {
		for _, m := range cfg.GetModelsForProvider(provider) {
			add(m, provider)
		}
		for _, p := range cfg.GetProviders() {
			if p == provider {
				continue
			}
			for _, m := range cfg.GetModelsForProvider(p) {
				add(m, p)
			}
		}
	} else {
		for _, m := range config.DefaultModels[provider] {
			add(m, provider)
		}
	}

	if len(items) == 0 {
		items = append(items, widgets.ModalItem{Label: "No models configured"})
	}
	return items
}

func providerModalItems(cfg *config.Config, runner AgentRunner, localDetected map[string]LocalProviderInfo) []widgets.ModalItem {
	seen := map[string]bool{}
	var nativeItems, compatItems, localItems []widgets.ModalItem

	resolveHint := func(name string) string {
		if name == runner.Provider {
			return "● current"
		}
		preset, isPreset := config.HostedPresets[name]
		if isPreset && preset.Group == "local" {
			if _, detected := localDetected[name]; detected {
				return "detected"
			}
			return "not detected"
		}
		if cfg == nil {
			return "not configured"
		}
		pc, inCfg := cfg.Providers[name]
		if !inCfg {
			return "not configured"
		}
		if pc.APIKey != "" {
			return "configured"
		}
		if !preset.RequiresAPIKey {
			return "configured"
		}
		return "no API key"
	}

	addTo := func(list *[]widgets.ModalItem, name string) {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		*list = append(*list, widgets.ModalItem{Label: name, Hint: resolveHint(name)})
	}

	for _, p := range cfg.GetProviders() {
		preset, isPreset := config.HostedPresets[p]
		if isPreset && preset.Group == "native" {
			addTo(&nativeItems, p)
		} else if isPreset && preset.Group == "local" {
			addTo(&localItems, p)
		} else if isPreset && preset.Group == "compatible" {
			addTo(&compatItems, p)
		} else {
			addTo(&compatItems, p)
		}
	}

	var items []widgets.ModalItem
	if len(nativeItems) > 0 {
		items = append(items, widgets.ModalItem{Label: "── Native ──", Disabled: true})
		items = append(items, nativeItems...)
	}
	if len(compatItems) > 0 {
		items = append(items, widgets.ModalItem{Label: "── OpenAI Compatible ──", Disabled: true})
		items = append(items, compatItems...)
	}
	if len(localItems) > 0 {
		items = append(items, widgets.ModalItem{Label: "── Local ──", Disabled: true})
		items = append(items, localItems...)
	}
	if len(items) == 0 {
		items = append(items, widgets.ModalItem{Label: "No providers configured"})
	}
	return items
}

const skillsPerPage = 8
const skillPaginationPrefix = "← → page "

func skillModalItems(skills []string, page int) []widgets.ModalItem {
	if len(skills) == 0 {
		return []widgets.ModalItem{{Label: "No skills loaded"}}
	}
	start := page * skillsPerPage
	if start >= len(skills) {
		start = 0
	}
	end := start + skillsPerPage
	if end > len(skills) {
		end = len(skills)
	}
	items := make([]widgets.ModalItem, 0, end-start)
	for _, name := range skills[start:end] {
		items = append(items, widgets.ModalItem{Label: name, Hint: "loaded"})
	}
	totalPages := (len(skills) + skillsPerPage - 1) / skillsPerPage
	if totalPages > 1 {
		items = append(items, widgets.ModalItem{
			Label: fmt.Sprintf("%s%d/%d", skillPaginationPrefix, page+1, totalPages),
			Hint:  "navigate",
		})
	}
	return items
}

func isSkillPaginationItem(label string) bool {
	return strings.HasPrefix(label, skillPaginationPrefix)
}

func statusModalItems(s Session) []widgets.ModalItem {
	approval := s.runner.ApprovalMode
	if approval == "" {
		approval = "full"
	}
	mcp := "No MCP tools used"
	if s.lastMCPTool != "" {
		mcp = s.lastMCPTool + " (" + s.lastMCPStatus + ")"
	}
	return []widgets.ModalItem{
		{Label: "Provider: " + s.runner.Provider, Hint: "runtime"},
		{Label: "Model: " + s.runner.Model, Hint: "runtime"},
		{Label: "Workspace: " + s.runner.Workspace, Hint: "dir"},
		{Label: "Approval: " + approval, Hint: "mode"},
		{Label: "MCP: " + mcp, Hint: "tools"},
	}
}

func approvalModalItems(current string) []widgets.ModalItem {
	if current == "" {
		current = "full"
	}
	modes := []string{"full", "plan-only", "dangerous-only", "none"}
	items := make([]widgets.ModalItem, 0, len(modes))
	for _, mode := range modes {
		hint := "available"
		if mode == current {
			hint = "current"
		}
		items = append(items, widgets.ModalItem{Label: mode, Hint: hint})
	}
	return items
}

func helpModalItems() []widgets.ModalItem {
	return []widgets.ModalItem{
		{Label: "Ctrl+P", Hint: "command palette"},
		{Label: "Ctrl+X", Hint: "chord prefix"},
		{Label: "Ctrl+C", Hint: "quit"},
		{Label: "/clear", Hint: "clear chat"},
		{Label: "/model", Hint: "show model"},
		{Label: "/dir", Hint: "show directory"},
		{Label: "/approval", Hint: "show approval"},
		{Label: "/help", Hint: "show help"},
		{Label: "/quit", Hint: "quit"},
	}
}

func normalizeTypedCommand(text string) (string, bool) {
	cmd := strings.ToLower(strings.TrimSpace(text))
	if cmd == "" {
		return "", false
	}
	if !strings.HasPrefix(cmd, "/") {
		cmd = "/" + cmd
	}
	switch cmd {
	case "/clear", "/cls":
		return "/clear", true
	case "/help", "/model", "/dir", "/approval", "/quit":
		return cmd, true
	default:
		return "", false
	}
}

// handleUIEvent applies a structured UIEvent to the session state.
func (s Session) handleUIEvent(event UIEvent) Session {
	switch e := event.(type) {
	case PlanReadyEvent:
		s.planActive = true
		s.planSteps = make([]string, len(e.Steps))
		for i, step := range e.Steps {
			s.planSteps[i] = sanitizeDisplayText(step)
		}
		s.stepDone = make([]bool, len(e.Steps))
		s.stepErrors = make([]error, len(e.Steps))
		s.currentStep = 0

	case StepStartEvent:
		s.currentStep = e.Index
		s.activeAction = sanitizeDisplayText(e.Action)
		s.activeTarget = truncatePlain(sanitizeDisplayText(e.Desc), 40)

	case StepDoneEvent:
		if e.Index >= 0 && e.Index < len(s.stepDone) {
			s.stepDone[e.Index] = true
			s.stepErrors[e.Index] = e.Err
		}
		s.execLog = append(s.execLog, formatExecLogLine(e))
		// Track MCP calls.
		if e.Action == "call_mcp_tool" || e.Action == "read_mcp_resource" {
			s.lastMCPTool = parseMCPTool(e.Info)
			if e.Err != nil {
				s.lastMCPStatus = "error"
			} else {
				s.lastMCPStatus = "ok"
			}
			s.lastMCPOutput = firstLine(sanitizeDisplayText(e.Info))
		}

	case ProviderFallbackEvent:
		s.fallbackReason = sanitizeDisplayText(e.Reason)
		s.execLog = append(s.execLog, fmt.Sprintf("⚡ Fallback: %s → %s (%s)",
			sanitizeDisplayText(e.From), sanitizeDisplayText(e.To), sanitizeDisplayText(e.Reason)))

	case ProviderActiveEvent:
		s.activeProvider = sanitizeDisplayText(e.Provider)
		s.activeModel = sanitizeDisplayText(e.Model)

	case PermWarnEvent:
		s.lastPermWarn = sanitizeDisplayText(e.Warning)

	case ApprovalRequestEvent:
		s.approvalPending = true
		s.approvalCh = e.ResponseCh
		// Build modal items from the approval request.
		items := []widgets.ModalItem{
			{Label: "Approve", Hint: "proceed with this action"},
		}
		switch sanitizeDisplayText(e.Stage) {
		case "plan":
			items = append(items, widgets.ModalItem{Label: "Revise", Hint: "edit the plan"})
		case "execute":
			items = append(items, widgets.ModalItem{Label: "Approve all", Hint: "do not ask again this run"})
		}
		items = append(items, widgets.ModalItem{Label: "Reject", Hint: "stop and cancel"})
		stage := sanitizeDisplayText(e.Stage)
		title := fmt.Sprintf("Approval: %s", stage)
		if e.Dangerous {
			title += " ⚠ DANGEROUS"
		}
		s.modal = widgets.NewModal(title, items, s.width)
		s.modalOpen = true
		// Also show the description in the chat panel so the user knows what
		// they are approving.
		desc := fmt.Sprintf("[approval required] %s: %s", stage, sanitizeDisplayText(e.Description))
		for _, item := range e.Items {
			desc += "\n  • " + sanitizeDisplayText(item)
		}
		s = s.appendToAssistant(desc)
	}
	return s
}

// handleModalSelect dispatches the selected modal item based on which command
// opened the modal. It applies the selection and optionally persists to config.
func (s Session) handleModalSelect(msg widgets.ModalSelectMsg) (tea.Model, tea.Cmd) {
	if !s.running {
		s.input.Focus()
	}

	label := strings.TrimSpace(msg.Label)
	switch s.modalCommand {
	case "switch-session":
		if label == "+ New session" {
			s.modal = widgets.NewInputModal(
				"New session", "Session name...",
				[]widgets.ModalItem{
					{Label: "Create session", Hint: "enter"},
					{Label: "Cancel", Hint: "esc"},
				},
				s.width,
			)
			s.modalCommand = "new-session"
			s.modalOpen = true
			return s, s.modal.Init()
		}
		if msg.Key != "" && msg.Key != s.sessionID {
			s.modalCommand = ""
			return s, func() tea.Msg { return OpenSessionMsg{ID: msg.Key} }
		}
		s = s.appendCommandOutput("Switched to session.")

	case "switch-model":
		if label == "" || label == "No models configured" {
			break
		}
		providerName, err := s.providerForModel(label)
		if err != nil {
			s = s.appendCommandOutput(err.Error())
			break
		}
		s.runner.Model = label
		s.runner.Provider = providerName
		s = s.appendCommandOutput("Model set to " + label + ".")
		if err := s.saveConfigFieldWithMode(func(c *config.Config) bool {
			fullSave := ensureProviderModelConfigured(c, providerName, label)
			c.DefaultProvider = providerName
			if len(c.FallbackChain) > 0 {
				c.FallbackChain[0].Provider = providerName
				c.FallbackChain[0].Model = label
			} else {
				c.FallbackChain = []config.FallbackEntry{{Provider: providerName, Model: label}}
			}
			return fullSave
		}); err != nil {
			s = s.appendCommandOutput("Model changed for this session, but config could not be saved: " + err.Error())
		}
		s.modalCommand = ""
		return s, func() tea.Msg {
			return RuntimeModelChangedMsg{Provider: providerName, Model: label}
		}

	case "connect-provider":
		if label == "" || label == "No providers configured" {
			break
		}
		pendingProvider := label
		s.modalCommand = ""

		// If provider already has an API key or doesn't require one, verify and switch directly.
		hasKey := false
		if s.cfg != nil {
			if pc, ok := s.cfg.Providers[pendingProvider]; ok && pc.APIKey != "" {
				hasKey = true
			}
		}
		preset, isPreset := config.HostedPresets[pendingProvider]
		needsWizard := isPreset && (preset.Group == "local" || (!hasKey && preset.RequiresAPIKey))

		if !needsWizard {
			pendingModel := s.defaultModelForProvider(pendingProvider)
			if pendingModel == "" {
				pendingModel = s.runner.Model
			}
			if s.runner.VerifyProvider != nil {
				s = s.appendCommandOutput("Verifying " + pendingProvider + "...")
				return s, func() tea.Msg {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					err := s.runner.VerifyProvider(ctx, pendingProvider)
					return ProviderVerifyResultMsg{Provider: pendingProvider, Model: pendingModel, Err: err}
				}
			}
			s = s.commitProviderSwitch(pendingProvider, pendingModel)
			return s, func() tea.Msg {
				return RuntimeModelChangedMsg{Provider: pendingProvider, Model: pendingModel}
			}
		}

		return s.startWizard(pendingProvider)

	case "open-editor":
		bin := editorBinary(label)
		if _, err := exec.LookPath(bin); err != nil {
			s = s.appendCommandOutput(label + " was not found. Make sure it is installed.")
			break
		}
		dir := s.runner.Workspace
		if dir == "" {
			dir = "."
		}
		cmd := exec.Command(bin, dir)
		if err := cmd.Start(); err != nil {
			s = s.appendCommandOutput("Failed to open " + label + ": " + err.Error())
		} else {
			s = s.appendCommandOutput("Opened " + label + ".")
		}

	case "new-session":
		if label == "Cancel" {
			break
		}
		title := strings.TrimSpace(msg.Value)
		if title == "" {
			title = "New session"
		}
		if title == s.sessionTitle && len(s.messages) == 0 {
			s = s.appendCommandOutput("This session is already empty.")
			break
		}
		return s, func() tea.Msg { return CreateSessionMsg{Title: title} }

	case "rename-session":
		if label == "Cancel" {
			break
		}
		title := strings.TrimSpace(msg.Value)
		if title == "" {
			s = s.appendCommandOutput("Enter a title to rename the session.")
			break
		}
		target := s.modalTarget
		s.modalTarget = ""
		s.modalCommand = ""
		return s, func() tea.Msg { return RenameSessionMsg{ID: target, Title: title} }

	case "delete-session":
		if label != "Delete" {
			break
		}
		target := s.modalTarget
		s.modalTarget = ""
		s.modalCommand = ""
		return s, func() tea.Msg { return DeleteSessionMsg{ID: target} }

	case "skills":
		if label == "" || label == "No skills loaded" {
			break
		}
		if isSkillPaginationItem(label) {
			s.modal = widgets.NewModal("Skills", skillModalItems(s.loadedSkills, s.skillPaginator.Page), s.width)
			s.modalOpen = true
			return s, s.modal.Init()
		}
		content := s.readSkillContent(label)
		items := []widgets.ModalItem{{Label: content, Hint: "content"}}
		s.modal = widgets.NewModal("Skill: "+label, items, s.width)
		s.modalOpen = true
		s.modalCommand = "skill-detail"
		return s, s.modal.Init()

	case "hide-tips":
		if label == "Hide tips" {
			s.tipsHidden = true
			s = s.appendCommandOutput("Tips hidden.")
		} else {
			s.tipsHidden = false
			s = s.appendCommandOutput("Tips visible.")
		}

	case "switch-theme":
		s = s.appendCommandOutput("Theme set to " + label + ".")
		if err := s.saveConfigField(func(c *config.Config) { c.Theme = strings.ToLower(label) }); err != nil {
			s = s.appendCommandOutput("Theme could not be saved: " + err.Error())
		}

	case "/approval":
		s.runner.ApprovalMode = label
		s = s.appendCommandOutput("Approval mode set to " + label + ".")
		if err := s.saveConfigField(func(c *config.Config) { c.ApprovalMode = label }); err != nil {
			s = s.appendCommandOutput("Approval mode could not be saved: " + err.Error())
		}

	case "/model", "/dir", "/help", "view-status", "skill-detail":
		// Info-only modals — just close.
	}

	s.modalCommand = ""
	return s, nil
}

// saveConfigField applies mutate to the in-memory config and persists it.
func (s Session) saveConfigField(mutate func(*config.Config)) error {
	return s.saveConfigFieldWithMode(func(c *config.Config) bool {
		mutate(c)
		return false
	})
}

func (s Session) saveConfigFieldWithMode(mutate func(*config.Config) bool) error {
	if s.cfg == nil {
		return nil
	}
	fullSave := mutate(s.cfg)
	if s.configPath != "" {
		if fullSave {
			if err := config.SaveFile(s.cfg, s.configPath); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			return nil
		}
		if err := config.SaveFilePreservingSecrets(s.cfg, s.configPath); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
	}
	return nil
}

func (s Session) providerForModel(model string) (string, error) {
	if s.cfg != nil {
		if providerHasModel(s.cfg, s.runner.Provider, model) {
			return s.runner.Provider, nil
		}
		if len(s.cfg.FallbackChain) > 0 && s.cfg.FallbackChain[0].Model == model &&
			providerHasModel(s.cfg, s.cfg.FallbackChain[0].Provider, model) {
			return s.cfg.FallbackChain[0].Provider, nil
		}
		for _, providerName := range s.cfg.GetProviders() {
			if _, ok := s.cfg.Providers[providerName]; !ok {
				continue
			}
			if providerHasModel(s.cfg, providerName, model) {
				return providerName, nil
			}
		}
	}

	if defaultProviderHasModel(s.runner.Provider, model) {
		return s.runner.Provider, nil
	}
	if s.cfg != nil && len(s.cfg.FallbackChain) > 0 && s.cfg.FallbackChain[0].Model == model &&
		defaultProviderHasModel(s.cfg.FallbackChain[0].Provider, model) {
		return s.cfg.FallbackChain[0].Provider, nil
	}
	for _, providerName := range config.Default().GetProviders() {
		if defaultProviderHasModel(providerName, model) {
			return providerName, nil
		}
	}
	return "", fmt.Errorf("model %q is not configured or known", model)
}

func (s Session) commitProviderSwitch(providerName, model string) Session {
	s.runner.Provider = providerName
	s.runner.Model = model
	s = s.appendCommandOutput("Provider set to " + providerName + ".")
	if err := s.saveConfigFieldWithMode(func(c *config.Config) bool {
		fullSave := ensureProviderModelConfigured(c, providerName, model)
		c.DefaultProvider = providerName
		if len(c.FallbackChain) > 0 {
			c.FallbackChain[0].Provider = providerName
			c.FallbackChain[0].Model = model
		} else if model != "" {
			c.FallbackChain = []config.FallbackEntry{{Provider: providerName, Model: model}}
		}
		return fullSave
	}); err != nil {
		s = s.appendCommandOutput("Provider changed for this session, but config could not be saved: " + err.Error())
	}
	return s
}

func (s Session) defaultModelForProvider(provider string) string {
	if s.cfg == nil {
		return s.runner.Model
	}
	if providerHasModel(s.cfg, provider, s.runner.Model) {
		return s.runner.Model
	}
	return firstModelForProvider(s.cfg, provider)
}

func providerHasModel(cfg *config.Config, provider, model string) bool {
	if cfg == nil || provider == "" || model == "" {
		return false
	}
	pc, ok := cfg.Providers[provider]
	if !ok {
		return false
	}
	for _, candidate := range pc.Models {
		if candidate == model {
			return true
		}
	}
	return false
}

func defaultProviderHasModel(provider, model string) bool {
	if provider == "" || model == "" {
		return false
	}
	for _, candidate := range config.DefaultModels[provider] {
		if candidate == model {
			return true
		}
	}
	return false
}

func firstModelForProvider(cfg *config.Config, provider string) string {
	if cfg == nil {
		return ""
	}
	for _, model := range cfg.GetModelsForProvider(provider) {
		return model
	}
	return ""
}

func ensureDefaultProviderConfigured(cfg *config.Config, provider string) bool {
	return ensureProviderModelConfigured(cfg, provider, "")
}

func ensureProviderModelConfigured(cfg *config.Config, provider, model string) bool {
	if cfg == nil {
		return false
	}
	if cfg.Providers == nil {
		cfg.Providers = make(map[string]config.ProviderConfig)
	}
	pc, exists := cfg.Providers[provider]
	defaults := config.DefaultModels[provider]
	preset, isPreset := config.HostedPresets[provider]
	if !exists && len(defaults) == 0 && !isPreset && model == "" {
		return false
	}
	changed := !exists
	if len(pc.Models) == 0 {
		if len(defaults) > 0 {
			pc.Models = append([]string(nil), defaults...)
			changed = true
		}
	}
	if isPreset && preset.Endpoint != "" && pc.Endpoint == "" {
		pc.Endpoint = preset.Endpoint
		changed = true
	}
	if model != "" && !containsModel(pc.Models, model) {
		pc.Models = append(pc.Models, model)
		changed = true
	}
	if changed {
		cfg.Providers[provider] = pc
	}
	return changed
}

func containsModel(models []string, model string) bool {
	for _, candidate := range models {
		if candidate == model {
			return true
		}
	}
	return false
}

// editorBinary maps a display label to the command-line binary name.
func editorBinary(label string) string {
	switch strings.ToLower(label) {
	case "vs code":
		return "code"
	case "cursor":
		return "cursor"
	case "notepad":
		return "notepad"
	case "vim":
		return "vim"
	default:
		return strings.ToLower(label)
	}
}

// readSkillContent returns the loaded SKILL.md content for the named skill.
func (s Session) readSkillContent(name string) string {
	if s.skillContents != nil {
		if content, ok := s.skillContents[name]; ok && strings.TrimSpace(content) != "" {
			return content
		}
	}
	return "Skill content not available for " + name + "."
}

// parseMCPTool extracts the "server/tool" identifier from the prefixed Info
// string. stepInfo() prefixes MCP calls as "server/tool: <output>".
func parseMCPTool(info string) string {
	if idx := strings.Index(info, ": "); idx > 0 {
		candidate := info[:idx]
		if strings.Contains(candidate, "/") {
			return candidate
		}
	}
	return "mcp"
}

// firstLine returns the first non-empty line of s.
func firstLine(s string) string {
	for l := range strings.SplitSeq(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
}

// formatExecLogLine builds a human-readable execution log entry.
func formatExecLogLine(e StepDoneEvent) string {
	if e.Err != nil {
		return sanitizeDisplayText("x " + e.Info + " - " + displayAgentError(e.Err))
	}
	return sanitizeDisplayText("v " + e.Info)
}

func displayAgentError(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	if strings.Contains(text, "TLS handshake timeout") {
		return "network timeout while contacting the provider; please retry"
	}
	for _, prefix := range []string{
		"agent: create plan: ",
		"agent: planner chat: ",
		"agent: ",
	} {
		text = strings.TrimPrefix(text, prefix)
	}
	if idx := strings.LastIndex(text, "file not found: "); idx >= 0 {
		text = text[idx:]
	}
	return truncatePlain(sanitizeDisplayText(text), 240)
}

// appendToAssistant appends text to the last assistant message, or creates a
// new one if the last message is not from the assistant.
func (s Session) appendToAssistant(text string) Session {
	if len(s.messages) > 0 && s.messages[len(s.messages)-1].role == "assistant" {
		s.messages[len(s.messages)-1].text += text
	} else {
		s.messages = append(s.messages, chatMsg{role: "assistant", text: text})
	}
	return s
}

func (s Session) appendCommandOutput(text string) Session {
	s.messages = append(s.messages, chatMsg{role: "assistant", text: text})
	return s
}

// View implements tea.Model. When the palette is open it overlays the modal
// on top of the rendered session background instead of replacing the view.
func (s Session) View() string {
	if s.width == 0 || s.height == 0 {
		return ""
	}
	bg := s.baseView()
	if s.paletteOpen {
		bg = overlayCenter(bg, s.palette.View(), s.width, s.height)
	}
	if s.wizardOpen {
		return overlayCenter(bg, s.viewWizard(), s.width, s.height)
	}
	if s.modalOpen {
		return overlayCenter(bg, s.modal.View(), s.width, s.height)
	}
	return bg
}

// overlayCenter places overlay centered over bg (rendered as an ANSI string
// of bgW×bgH characters). It composes at the ANSI string level so the
// background content remains visible behind the modal box.
func overlayCenter(bg, overlay string, bgW, bgH int) string {
	bgLines := strings.Split(bg, "\n")
	ovLines := strings.Split(strings.TrimRight(overlay, "\n"), "\n")

	ovH := len(ovLines)
	ovW := 0
	for _, l := range ovLines {
		if w := lipgloss.Width(l); w > ovW {
			ovW = w
		}
	}

	startRow := (bgH - ovH) / 2
	startCol := (bgW - ovW) / 2
	if startRow < 0 {
		startRow = 0
	}
	if startCol < 0 {
		startCol = 0
	}

	result := make([]string, len(bgLines))
	for i, bgLine := range bgLines {
		ovIdx := i - startRow
		if ovIdx >= 0 && ovIdx < len(ovLines) {
			result[i] = overlayLine(bgLine, ovLines[ovIdx], startCol, ovW)
		} else {
			result[i] = bgLine
		}
	}
	return strings.Join(result, "\n")
}

// overlayLine writes ovLine over bgLine starting at visual column startCol.
// It uses ANSI-aware truncation to preserve background content on both sides.
func overlayLine(bgLine, ovLine string, startCol, ovW int) string {
	left := xansi.Truncate(bgLine, startCol, "")
	right := xansi.TruncateLeft(bgLine, startCol+ovW, "")
	return left + ovLine + right
}

func (s Session) baseView() string {
	const statusBarH = 1
	viewW := max(s.width-1, 1)
	viewH := max(s.height-1, 1)
	panelH := max(viewH-2-statusBarH, 1)

	// Collapse right panel when terminal is too narrow.
	if viewW < minWidthForRightPanel {
		leftW := max(viewW-2, 1)
		leftContent := s.renderChatPanel(leftW, panelH)
		leftStyle := theme.BorderStyle.Width(leftW).Height(panelH)
		left := leftStyle.Render(leftContent)
		return left + "\n" + s.renderStatusBarWidth(viewW)
	}

	// Normal 70/30 horizontal split.
	leftTotal := viewW * 7 / 10
	rightTotal := viewW - leftTotal
	leftW := max(leftTotal-2, 1)
	rightW := max(rightTotal-2, 1)

	leftContent := s.renderChatPanel(leftW, panelH)
	leftStyle := theme.BorderStyle.Width(leftW).Height(panelH)
	rightStyle := theme.BorderStyle.Width(rightW).Height(panelH).
		AlignVertical(lipgloss.Top)

	left := leftStyle.Render(leftContent)
	right := rightStyle.Render(s.clippedStatusContent(panelH, rightW))

	panels := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	return panels + "\n" + s.renderStatusBarWidth(viewW)
}

// renderStatusBar renders the bottom status bar: dir:branch[*] on the left,
// version (and [»] indicator when right panel is hidden) on the right.
func (s Session) renderStatusBar() string {
	return s.renderStatusBarWidth(s.width)
}

func (s Session) renderStatusBarWidth(width int) string {
	barStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("237")).
		Foreground(lipgloss.Color("252"))

	dirBranch := s.runner.Workspace
	if s.gitBranch != "" {
		dirBranch += ":" + s.gitBranch
		if s.gitDirty {
			dirBranch += "*"
		}
	}
	ver := s.version
	if ver == "" {
		ver = "dev"
	}

	// Show collapsed-panel indicator when right panel is hidden.
	rightText := ver + " "
	if width > 0 && width < minWidthForRightPanel {
		rightText = "[»] " + ver + " "
	}

	if width <= 0 {
		return ""
	}

	right := barStyle.Render(rightText)
	rightW := lipgloss.Width(right)
	if rightW >= width {
		// Right side alone overflows: truncate everything to terminal width.
		return barStyle.Render(truncatePlain(rightText, width))
	}

	leftMaxW := max(width-rightW-1, 0)
	leftText := truncatePlain(" "+dirBranch, leftMaxW)
	left := barStyle.Render(leftText)
	leftW := lipgloss.Width(left)

	gapW := max(width-leftW-rightW, 0)
	gap := barStyle.Render(strings.Repeat(" ", gapW))

	return left + gap + right
}

// chatViewportInnerW returns the viewport content width for the given panel
// inner width, reserving 1 column for the scrollbar track.
func chatViewportInnerW(panelW int) int { return max(panelW-1, 1) }

// chatScrollbarGeometry returns the terminal cell occupied by the scrollbar
// and its vertical track. The panel border occupies row/column zero.
func (s Session) chatScrollbarGeometry() (x, top, height int) {
	const statusBarH = 1
	viewW := max(s.width-1, 1)
	viewH := max(s.height-1, 1)
	panelH := max(viewH-2-statusBarH, 1)
	scrollH := max(panelH-2, 0)

	leftTotal := viewW * 7 / 10
	if viewW < minWidthForRightPanel {
		leftTotal = viewW
	}
	leftW := max(leftTotal-2, 1)
	return 1 + chatViewportInnerW(leftW), 1, scrollH
}

func (s Session) scrollChatToMouseY(mouseY int) Session {
	_, top, height := s.chatScrollbarGeometry()
	maxOffset := max(s.chatVP.TotalLineCount()-s.chatVP.Height, 0)
	if height <= 1 || maxOffset == 0 {
		s.chatVP.SetYOffset(0)
		return s
	}
	relative := min(max(mouseY-top, 0), height-1)
	offset := int(math.Round(float64(relative) / float64(height-1) * float64(maxOffset)))
	s.chatVP.SetYOffset(offset)
	return s
}

// resizeChatVP recalculates viewport dimensions from current terminal size
// and rebuilds viewport content.
func (s Session) resizeChatVP() Session {
	const statusBarH = 1
	viewW := max(s.width-1, 1)
	viewH := max(s.height-1, 1)
	panelH := max(viewH-2-statusBarH, 1)
	scrollH := max(panelH-2, 0) // reserve 2 rows for blank + input

	leftTotal := viewW * 7 / 10
	if viewW < minWidthForRightPanel {
		leftTotal = viewW
	}
	leftW := max(leftTotal-2, 1)
	vpW := chatViewportInnerW(leftW)

	s.chatVP.Width = vpW
	s.chatVP.Height = scrollH
	s.chatVPW = vpW

	return s.rebuildChatVP()
}

// rebuildChatVP rebuilds the viewport content from current message/plan state.
func (s Session) rebuildChatVP() Session {
	if s.chatVPW <= 0 {
		return s
	}
	body := s.buildChatBody(s.chatVPW)
	s.chatVP.SetContent(body)
	return s
}

// buildChatBody builds all chat message lines without height capping.
// The viewport handles scroll/clipping; this method just produces the full content.
func (s Session) buildChatBody(panelW int) string {
	var lines []string

	for i, m := range s.messages {
		if m.role == "user" {
			lines = appendPrefixedLines(lines, theme.TitleStyle.Render("you")+" ", "    ", m.text, panelW)
			lines = append(lines, "")
		} else {
			text := sanitizeDisplayText(m.text)
			// Append blinking cursor to the last assistant message while streaming.
			isLast := i == len(s.messages)-1
			if isLast && s.streaming && s.cursorShow {
				text += "▌"
			}
			lines = appendPrefixedLines(lines, theme.MutedStyle.Render("bolt")+" ", "     ", text, panelW)
			lines = append(lines, "")
		}
	}

	if s.planActive {
		pw := widgets.NewPlanWidget(s.planSteps, s.stepDone, s.stepErrors, panelW)
		pw.SetActiveStep(s.currentStep)
		pw.SetSpinnerFrame(s.spinner.View())
		for l := range strings.SplitSeq(pw.View(), "\n") {
			lines = append(lines, l)
		}
		lines = append(lines, "")

		for _, l := range s.execLog {
			lines = append(lines, truncatePlain("  "+l, panelW))
		}
		if s.running {
			lines = append(lines, "  Running...")
		}
	} else if s.running {
		lines = append(lines, s.spinner.View()+" thinking...")
	}

	return strings.Join(lines, "\n")
}

// renderChatPanel composes the viewport output with a scrollbar and the input
// line, clamped to exactly panelH lines so the border never overflows.
func (s Session) renderChatPanel(panelW, panelH int) string {
	scrollH := max(panelH-2, 0)

	vpView := s.chatVP.View()
	needsScroll := s.chatVP.TotalLineCount() > s.chatVP.Height && s.chatVP.Height > 0
	vpW := chatViewportInnerW(panelW)
	withSB := renderScrollbar(vpView, scrollH, vpW, s.chatVP.ScrollPercent(), needsScroll)

	s.input.Width = max(panelW-3, 1)

	lines := strings.Split(withSB, "\n")
	lines = append(lines, "", "> "+s.input.View())

	return fixedHeightLines(lines, panelH)
}

// renderScrollbar appends a 1-char scrollbar track to each line of the
// viewport output. When content fits entirely, the track column is blank.
func renderScrollbar(vpView string, height, contentW int, scrollPercent float64, needsScroll bool) string {
	lines := strings.Split(vpView, "\n")
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}

	if !needsScroll {
		for i := range lines {
			lineW := lipgloss.Width(lines[i])
			if lineW < contentW {
				lines[i] += strings.Repeat(" ", contentW-lineW)
			}
			lines[i] += " "
		}
		return strings.Join(lines, "\n")
	}

	thumbSize := max(height/5, 1)
	thumbStart := int(scrollPercent * float64(height-thumbSize))
	if thumbStart < 0 {
		thumbStart = 0
	}
	if thumbStart+thumbSize > height {
		thumbStart = height - thumbSize
	}

	for i := range lines {
		lineW := lipgloss.Width(lines[i])
		if lineW < contentW {
			lines[i] += strings.Repeat(" ", contentW-lineW)
		}
		if i >= thumbStart && i < thumbStart+thumbSize {
			lines[i] += "┃"
		} else {
			lines[i] += "│"
		}
	}
	return strings.Join(lines, "\n")
}

func fixedHeightLines(lines []string, height int) string {
	if height <= 0 {
		return ""
	}
	if len(lines) > height {
		lines = lines[len(lines)-height:]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// clippedStatusContent returns statusContent truncated to maxLines so it
// never causes the right panel box to grow beyond its fixed height.
func (s Session) clippedStatusContent(maxLines, maxWidth int) string {
	content := s.statusContent(maxWidth)
	lines := strings.Split(content, "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	for i, line := range lines {
		lines[i] = truncatePlain(line, maxWidth)
	}
	return strings.Join(lines, "\n")
}

// statusContent builds the info shown in the right panel.
func (s Session) statusContent(w int) string {
	hdr := func(title string) string { return theme.TitleStyle.Render(title) }

	var lines []string

	// Section 1 — PROVIDER
	var statusLabel string
	if s.running {
		statusLabel = s.spinner.View() + " Thinking..."
	} else {
		statusLabel = "○ Idle"
	}
	providerName := s.runner.Provider
	if providerName == "" {
		providerName = "-"
	}
	modelName := s.runner.Model
	if modelName == "" {
		modelName = "-"
	}
	lines = append(lines,
		hdr("PROVIDER"),
		"  Name   : "+providerName,
		"  Model  : "+modelName,
	)
	isFallback := s.activeProvider != "" && s.activeProvider != s.runner.Provider
	if isFallback {
		lines = append(lines,
			"  Active : "+truncatePlain(s.activeProvider+"/"+s.activeModel, max(w-11, 4)),
		)
		if s.fallbackReason != "" {
			lines = append(lines, "  Reason : "+truncatePlain(s.fallbackReason, max(w-11, 4)))
		}
		statusLabel = "⚡ " + statusLabel + " (fallback)"
	}
	lines = append(lines,
		"  Tokens : "+formatTokenCount(s.tokenCount),
		"  Status : "+statusLabel,
		"  Cost   : "+formatCost(s.sessionCost),
	)

	// Token progress bar.
	ctxWindow := contextWindowForModel(s.runner.Provider, s.runner.Model)
	barW := min(w-4, 20)
	if barW < 4 {
		barW = 4
	}
	pct := float64(s.tokenCount) / float64(ctxWindow)
	if pct > 1.0 {
		pct = 1.0
	}
	filled := int(math.Round(pct * float64(barW)))
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barW-filled)
	lines = append(lines, fmt.Sprintf("  [%s] %.1f%%", bar, pct*100))

	// Section 2 — AGENT
	lines = append(lines, "", hdr("AGENT"))
	if s.running && s.planActive && s.currentStep >= 0 && s.currentStep < len(s.planSteps) {
		actionLabel := s.activeAction
		if actionLabel == "" {
			actionLabel = "—"
		}
		target := s.activeTarget
		if target == "" && s.currentStep < len(s.planSteps) {
			target = truncatePlain(s.planSteps[s.currentStep], 36)
		}
		lines = append(lines,
			"  Action : "+actionLabel,
			"  Step   : "+target,
			fmt.Sprintf("  (%d / %d)", s.currentStep+1, len(s.planSteps)),
		)
	} else if s.running {
		lines = append(lines, "  —")
	} else if len(s.planSteps) > 0 {
		lines = append(lines, fmt.Sprintf("  %d steps done", len(s.planSteps)))
	} else {
		lines = append(lines, "  —")
	}

	// Section 3 — MCP
	lines = append(lines, "", hdr("MCP"))
	if s.lastMCPTool != "" {
		statusIcon := "✓"
		if s.lastMCPStatus == "error" {
			statusIcon = "✗"
		}
		lines = append(lines,
			"  Tool   : "+s.lastMCPTool,
			"  Status : "+statusIcon+" "+s.lastMCPStatus,
		)
		if out := s.lastMCPOutput; out != "" {
			lines = append(lines, "  Output : "+truncatePlain(out, max(w-12, 4)))
		}
	} else {
		lines = append(lines, "  No MCP tools used")
	}

	// Section 4 — PERMISSIONS (only when a warning has occurred)
	if s.lastPermWarn != "" {
		mode := s.runner.ApprovalMode
		if mode == "" {
			mode = "full"
		}
		lines = append(lines,
			"", hdr("PERMISSIONS"),
			"  ⚠ "+truncatePlain(s.lastPermWarn, max(w-4, 4)),
			"  Mode: "+mode,
		)
	}

	// Section 5 — SKILLS
	lines = append(lines, "", hdr("SKILLS"))
	if len(s.loadedSkills) == 0 {
		lines = append(lines, "  — (none loaded)")
	} else {
		const maxSkillsShown = 5
		for i, sk := range s.loadedSkills {
			if i >= maxSkillsShown {
				lines = append(lines, fmt.Sprintf("  ... +%d more", len(s.loadedSkills)-maxSkillsShown))
				break
			}
			lines = append(lines, "  ✓ "+sk)
		}
	}

	if !s.tipsHidden {
		lines = append(lines,
			"", hdr("TIPS"),
			"  Ctrl+P command palette",
			"  Ctrl+X shortcuts",
			"  /help keyboard shortcuts",
		)
	}

	return strings.Join(lines, "\n")
}

// formatTokenCount formats a token count for display.
func formatTokenCount(n int) string {
	if n == 0 {
		return "-"
	}
	if n >= 1000 {
		return fmt.Sprintf("%d,%03d", n/1000, n%1000)
	}
	return fmt.Sprintf("%d", n)
}

func appendPrefixedLines(lines []string, firstPrefix, nextPrefix, text string, width int) []string {
	text = sanitizeDisplayText(text)
	parts := strings.Split(text, "\n")
	for i, part := range parts {
		prefix := nextPrefix
		if i == 0 {
			prefix = firstPrefix
		}
		if part == "" && i > 0 {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, prefix+truncatePlain(part, max(width-lipgloss.Width(prefix), 0)))
	}
	return lines
}

func sanitizeDisplayText(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	for _, r := range text {
		switch {
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, "\\x%02x", r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// contextWindowForModel returns the context window size for the given model.
func contextWindowForModel(provider, model string) int {
	switch {
	case strings.Contains(model, "claude"):
		return 200_000
	case strings.Contains(model, "gemini-2.0-flash-lite"):
		return 1_000_000
	case strings.Contains(model, "gemini"):
		return 1_000_000
	case strings.Contains(model, "gpt-4o"):
		return 128_000
	case strings.Contains(model, "gpt-4.1"):
		return 128_000
	case strings.Contains(model, "o3"), strings.Contains(model, "o4"):
		return 200_000
	default:
		return 128_000
	}
}

// modelPricing holds input/output cost per 1M tokens in USD.
type modelPricing struct {
	input  float64
	output float64
}

var pricingTable = map[string]modelPricing{
	"claude-opus-4-5":       {15.00, 75.00},
	"claude-sonnet-4-6":     {3.00, 15.00},
	"claude-haiku-4-5":      {0.80, 4.00},
	"gpt-4o":                {2.50, 10.00},
	"gpt-4o-mini":           {0.15, 0.60},
	"gpt-4.1":               {2.00, 8.00},
	"gpt-4.1-mini":          {0.40, 1.60},
	"gpt-4.1-nano":          {0.10, 0.40},
	"o3":                    {10.00, 40.00},
	"o3-mini":               {1.10, 4.40},
	"o4-mini":               {1.10, 4.40},
	"gemini-2.5-pro":        {1.25, 10.00},
	"gemini-2.5-flash":      {0.30, 2.50},
	"gemini-2.0-flash":      {0.10, 0.40},
	"gemini-2.0-flash-lite": {0.075, 0.30},
}

// estimateChunkCost estimates the cost for a chunk of output tokens.
func estimateChunkCost(provider, model string, tokens int) float64 {
	p := pricingForModel(provider, model)
	return float64(tokens) * p.output / 1_000_000
}

func pricingForModel(provider, model string) modelPricing {
	if p, ok := pricingTable[model]; ok {
		return p
	}
	switch provider {
	case "anthropic":
		return modelPricing{3.00, 15.00}
	case "openai":
		return modelPricing{2.00, 8.00}
	case "gemini":
		return modelPricing{0.30, 2.50}
	default:
		return modelPricing{2.00, 8.00}
	}
}

func estimateTokensFromBytes(n int) int {
	if n <= 0 {
		return 0
	}
	return int(math.Ceil(float64(n) / 4.0))
}

// formatCost formats a cost in USD for display.
func formatCost(cost float64) string {
	if cost < 0.0001 {
		return "$0.0000"
	}
	return fmt.Sprintf("$%.4f", cost)
}

func truncatePlain(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if lipgloss.Width(text) <= maxWidth {
		return text
	}
	const suffix = "..."
	if maxWidth <= len(suffix) {
		runes := []rune(text)
		if len(runes) > maxWidth {
			runes = runes[:maxWidth]
		}
		return string(runes)
	}

	limit := maxWidth - len(suffix)
	var b strings.Builder
	for _, r := range text {
		next := b.String() + string(r)
		if lipgloss.Width(next) > limit {
			break
		}
		b.WriteRune(r)
	}
	return b.String() + suffix
}
