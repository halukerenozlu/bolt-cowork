package views

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/halukerenozlu/bolt-cowork/internal/config"
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

// Session is the bubbletea model for the active work area after the user
// sends their first message. It shows a 70/30 split: chat on the left (with
// inline text input at the bottom), status info on the right.
type Session struct {
	width  int
	height int

	runner AgentRunner

	// Chat state.
	messages []chatMsg
	history  []types.Message
	running  bool

	// Plan and execution state for the current run.
	planActive bool     // true when the current run has a plan
	planSteps  []string // step descriptions from PlanReadyEvent
	stepDone   []bool   // stepDone[i] is true when step i has completed
	stepErrors []error  // stepErrors[i] holds the error for step i (nil = success)
	execLog    []string // one line per completed step

	// Estimated token count (cumulative for the session).
	tokenCount int

	// Input widget at the bottom of the chat panel.
	input textinput.Model

	// Spinner shown while the agent is running without a plan.
	spinner spinner.Model

	// Context used to cancel an in-flight agent call.
	ctx    context.Context
	cancel context.CancelFunc

	// Command palette overlay.
	palette     widgets.Palette
	paletteOpen bool
}

// NewSession creates a Session seeded with the user's first message.
// The agent is started via Init() immediately after creation.
func NewSession(_ *config.Config, _ string, firstMsg string, runner AgentRunner) Session {
	ctx, cancel := context.WithCancel(context.Background())

	ti := textinput.New()
	ti.Placeholder = "Ask a follow-up..."
	ti.Prompt = ""
	ti.CharLimit = 512

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = theme.TitleStyle

	return Session{
		runner:   runner,
		messages: []chatMsg{{role: "user", text: firstMsg}},
		running:  true,
		input:    ti,
		spinner:  sp,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Init implements tea.Model.
func (s Session) Init() tea.Cmd {
	return tea.Batch(
		s.spinner.Tick,
		runAgentCmd(s.ctx, s.runner, s.messages[0].text, s.history),
	)
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
		return s, nil

	case tea.KeyMsg:
		// Ctrl+C always quits.
		if msg.Type == tea.KeyCtrlC {
			s.cancel()
			return s, tea.Quit
		}
		// Ctrl+P toggles the command palette.
		if msg.Type == tea.KeyCtrlP {
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
		// Route all other keys to palette when it's open.
		if s.paletteOpen {
			m, cmd := s.palette.Update(msg)
			s.palette = m.(widgets.Palette)
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
			// Reset plan state for the new run.
			s.planActive = false
			s.planSteps = nil
			s.stepDone = nil
			s.stepErrors = nil
			s.execLog = nil
			s.running = true
			return s, tea.Batch(
				s.spinner.Tick,
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

	case spinner.TickMsg:
		if !s.running {
			// Drop queued ticks once the agent has finished so the spinner
			// does not keep an idle tick loop alive while hidden.
			return s, nil
		}
		var cmd tea.Cmd
		s.spinner, cmd = s.spinner.Update(msg)
		return s, cmd

	case agentMsg:
		if msg.done {
			s.running = false
			if msg.result.Err != nil {
				s = s.appendToAssistant("Error: " + displayAgentError(msg.result.Err))
				s.planActive = false
			} else {
				s.history = msg.result.History
			}
			s.input.Focus()
			return s, nil
		}
		if msg.chunk != "" {
			// Accumulate tokens regardless of whether plan widget is showing.
			s.tokenCount += len(msg.chunk) / 4
			if !s.planActive {
				// Only add text to messages when no plan widget is active;
				// the plan widget + exec log serve as the visual response.
				s = s.appendToAssistant(msg.chunk)
			}
		}
		if msg.event != nil {
			s = s.handleUIEvent(msg.event)
		}
		return s, waitNext(msg.ch)
	}

	return s, nil
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
			s = s.appendToAssistant("Cannot clear while agent is running.")
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
	case "/help":
		s = s.appendToAssistant(helpText())
	case "/model":
		s = s.appendToAssistant("Model: " + s.runner.Model)
	case "/dir":
		s = s.appendToAssistant("Workspace: " + s.runner.Workspace)
	case "/approval":
		mode := s.runner.ApprovalMode
		if mode == "" {
			mode = "full"
		}
		s = s.appendToAssistant("Approval mode: " + mode)
	case "/quit":
		s.cancel()
		return s, tea.Quit
	}
	return s, nil
}

// helpText returns the formatted help string shown by /help.
func helpText() string {
	return strings.Join([]string{
		"Commands (Ctrl+P to open palette):",
		"  /clear    - clear chat history",
		"  /model    - show current model",
		"  /dir      - show workspace directory",
		"  /approval - show approval mode",
		"  /help     - show this help",
		"  /quit     - quit",
		"",
		"Shortcuts: Ctrl+P palette, Ctrl+C quit",
	}, "\n")
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
	case "/clear", "/cls", "/temizle":
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
		s.planSteps = e.Steps
		s.stepDone = make([]bool, len(e.Steps))
		s.stepErrors = make([]error, len(e.Steps))
	case StepDoneEvent:
		if e.Index >= 0 && e.Index < len(s.stepDone) {
			s.stepDone[e.Index] = true
			s.stepErrors[e.Index] = e.Err
		}
		s.execLog = append(s.execLog, formatExecLogLine(e))
	}
	return s
}

// formatExecLogLine builds a human-readable execution log entry.
func formatExecLogLine(e StepDoneEvent) string {
	if e.Err != nil {
		return "x " + e.Info + " - " + displayAgentError(e.Err)
	}
	return "v " + e.Info
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
	return truncatePlain(text, 240)
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

func (s Session) View() string {
	if s.width == 0 || s.height == 0 {
		return ""
	}
	if s.paletteOpen {
		return lipgloss.Place(
			s.width, s.height,
			lipgloss.Center, lipgloss.Center,
			s.palette.View(),
			lipgloss.WithWhitespaceChars(" "),
			lipgloss.WithWhitespaceBackground(lipgloss.Color("235")),
		)
	}
	return s.baseView()
}

func (s Session) baseView() string {
	// 70/30 horizontal split.
	leftTotal := s.width * 7 / 10
	rightTotal := s.width - leftTotal
	leftW := leftTotal - 2
	rightW := rightTotal - 2
	if leftW < 1 {
		leftW = 1
	}
	if rightW < 1 {
		rightW = 1
	}

	// Content height = terminal height minus top + bottom borders (2).
	panelH := max(s.height-2, 1)

	leftStyle := theme.BorderStyle.Width(leftW).Height(panelH)
	// AlignVertical(Top) pins content to the top of the box regardless of
	// its height. The box itself is fixed to panelH rows; any overflow is
	// clipped by truncating statusContent before rendering.
	rightStyle := theme.BorderStyle.Width(rightW).Height(panelH).
		AlignVertical(lipgloss.Top)

	left := leftStyle.Render(s.chatContent(leftW, panelH))
	right := rightStyle.Render(s.clippedStatusContent(panelH, rightW))

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

// chatContent builds the scrollable chat area with an inline input at the
// bottom. panelW is the inner content width, panelH is the content height.
func (s Session) chatContent(panelW, panelH int) string {
	var msgLines []string

	for _, m := range s.messages {
		if m.role == "user" {
			msgLines = appendPrefixedLines(msgLines, theme.TitleStyle.Render("you")+" ", "    ", m.text, panelW)
			msgLines = append(msgLines, "")
		} else if !s.planActive {
			// Show assistant text only when no plan widget is active.
			msgLines = appendPrefixedLines(msgLines, theme.MutedStyle.Render("bolt")+" ", "     ", m.text, panelW)
			msgLines = append(msgLines, "")
		}
	}

	if s.planActive {
		// Plan widget: structured step list with live checkboxes.
		pw := widgets.NewPlanWidget(s.planSteps, s.stepDone, s.stepErrors, panelW)
		for l := range strings.SplitSeq(pw.View(), "\n") {
			msgLines = append(msgLines, l)
		}
		msgLines = append(msgLines, "")

		// Execution log: one line per completed step.
		for _, l := range s.execLog {
			msgLines = append(msgLines, truncatePlain("  "+l, panelW))
		}
		if s.running {
			msgLines = append(msgLines, "  Running...")
		}
	} else if s.running {
		msgLines = append(msgLines, s.spinner.View()+" thinking...")
	}

	// Reserve 2 rows at the bottom: blank separator + input line.
	scrollRows := max(panelH-2, 0)

	// Show only the most recent lines when the conversation overflows.
	if len(msgLines) > scrollRows {
		msgLines = msgLines[len(msgLines)-scrollRows:]
	}

	var b strings.Builder
	for _, l := range msgLines {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	// Pad with blank lines so the input is always anchored to the bottom.
	for i := len(msgLines); i < scrollRows; i++ {
		b.WriteByte('\n')
	}

	// Separator + input.
	s.input.Width = max(panelW-3, 1)
	b.WriteByte('\n')
	b.WriteString("> " + s.input.View())

	return b.String()
}

// clippedStatusContent returns statusContent truncated to maxLines so it
// never causes the right panel box to grow beyond its fixed height.
func (s Session) clippedStatusContent(maxLines, maxWidth int) string {
	content := s.statusContent()
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
func (s Session) statusContent() string {
	statusLabel := "o Idle"
	if s.running {
		statusLabel = "* Active"
	}
	providerName := s.runner.Provider
	if providerName == "" {
		providerName = "-"
	}
	return fmt.Sprintf(
		"PROVIDER\n  Name   : %s\n  Model  : %s\n  Tokens : %s\n  Status : %s\n\nDir:\n%s",
		providerName,
		s.runner.Model,
		formatTokenCount(s.tokenCount),
		statusLabel,
		s.runner.Workspace,
	)
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
