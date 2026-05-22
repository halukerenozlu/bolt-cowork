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
	"github.com/halukerenozlu/bolt-cowork/pkg/types"
)

// chatMsg holds one entry in the conversation for display.
type chatMsg struct {
	role string // "user" or "assistant"
	text string
}

// agentChunkMsg carries a streaming text chunk while the agent is running.
type agentChunkMsg struct {
	text   string
	chunks <-chan string
	done   <-chan AgentResult
}

// agentDoneMsg signals that the agent finished.
type agentDoneMsg struct {
	result AgentResult
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

	// Input widget at the bottom of the chat panel.
	input textinput.Model

	// Spinner shown while the agent is running.
	spinner spinner.Model

	// Context used to cancel an in-flight agent call.
	ctx    context.Context
	cancel context.CancelFunc
}

// NewSession creates a Session seeded with the user's first message.
// The agent is started via Init() immediately after creation.
func NewSession(_ *config.Config, _ string, firstMsg string, runner AgentRunner) Session {
	ctx, cancel := context.WithCancel(context.Background())

	ti := textinput.New()
	ti.Placeholder = "Ask a follow-up..."
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

// Init implements tea.Model. It starts the spinner tick and kicks off the
// first agent run for the user's initial message.
func (s Session) Init() tea.Cmd {
	return tea.Batch(
		s.spinner.Tick,
		runAgentCmd(s.ctx, s.runner, s.messages[0].text, s.history),
	)
}

// runAgentCmd spawns a goroutine that runs the agent and returns a tea.Cmd
// that reads the first streaming chunk (or done signal).
func runAgentCmd(ctx context.Context, runner AgentRunner, cmd string, history []types.Message) tea.Cmd {
	chunkCh := make(chan string, 64)
	doneCh := make(chan AgentResult, 1)

	go func() {
		result := runner.Run(ctx, cmd, history, func(chunk string) {
			select {
			case chunkCh <- chunk:
			case <-ctx.Done():
			}
		})
		close(chunkCh)
		doneCh <- result
	}()

	return waitChunk(chunkCh, doneCh)
}

// waitChunk returns a tea.Cmd that blocks until the next chunk arrives or the
// chunk channel closes (signalling completion).
func waitChunk(chunks <-chan string, done <-chan AgentResult) tea.Cmd {
	return func() tea.Msg {
		text, ok := <-chunks
		if ok {
			return agentChunkMsg{text: text, chunks: chunks, done: done}
		}
		// Channel closed — wait for the final result.
		return agentDoneMsg{result: <-done}
	}
}

func (s Session) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.width = msg.Width
		s.height = msg.Height
		return s, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			s.cancel()
			return s, tea.Quit
		case tea.KeyEnter:
			if s.running {
				return s, nil
			}
			text := strings.TrimSpace(s.input.Value())
			if text == "" {
				return s, nil
			}
			s.input.Reset()
			s.messages = append(s.messages, chatMsg{role: "user", text: text})
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

	case spinner.TickMsg:
		if !s.running {
			// Drop queued ticks once the agent has finished so the spinner
			// does not keep an idle tick loop alive while hidden.
			return s, nil
		}
		var cmd tea.Cmd
		s.spinner, cmd = s.spinner.Update(msg)
		return s, cmd

	case agentChunkMsg:
		s = s.appendToAssistant(msg.text)
		return s, waitChunk(msg.chunks, msg.done)

	case agentDoneMsg:
		s.running = false
		if msg.result.Err != nil {
			s = s.appendToAssistant("Error: " + msg.result.Err.Error())
			// Do not update s.history after a failed run. The failed
			// command is not added to the LLM context, so the next
			// independent user command starts from the last clean state.
		} else {
			s.history = msg.result.History
		}
		s.input.Focus()
		return s, nil
	}

	return s, nil
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
	panelH := s.height - 2
	if panelH < 1 {
		panelH = 1
	}

	leftStyle := theme.BorderStyle.Width(leftW).Height(panelH)
	// AlignVertical(Top) pins content to the top of the box regardless of
	// its height. The box itself is fixed to panelH rows; any overflow is
	// clipped by truncating statusContent before rendering.
	rightStyle := theme.BorderStyle.Width(rightW).Height(panelH).
		AlignVertical(lipgloss.Top)

	left := leftStyle.Render(s.chatContent(panelH))
	right := rightStyle.Render(s.clippedStatusContent(panelH))

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

// chatContent builds the scrollable chat area with an inline input at the
// bottom. panelH is the available content height inside the border.
func (s Session) chatContent(panelH int) string {
	// Build message lines.
	var msgLines []string
	for _, m := range s.messages {
		if m.role == "user" {
			msgLines = append(msgLines, theme.TitleStyle.Render("you")+" "+m.text)
		} else {
			lines := strings.Split(m.text, "\n")
			for i, l := range lines {
				if i == 0 {
					msgLines = append(msgLines, theme.MutedStyle.Render("bolt")+" "+l)
				} else if l != "" {
					msgLines = append(msgLines, "     "+l)
				}
			}
		}
		msgLines = append(msgLines, "") // blank line between messages
	}
	if s.running {
		msgLines = append(msgLines, s.spinner.View()+" thinking...")
	}

	// Reserve 2 rows at the bottom: blank separator + input line.
	scrollRows := panelH - 2
	if scrollRows < 0 {
		scrollRows = 0
	}

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
	b.WriteByte('\n')
	b.WriteString("> " + s.input.View())

	return b.String()
}

// clippedStatusContent returns statusContent truncated to maxLines so it
// never causes the right panel box to grow beyond its fixed height.
func (s Session) clippedStatusContent(maxLines int) string {
	content := s.statusContent()
	lines := strings.Split(content, "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return strings.Join(lines, "\n")
}

// statusContent builds the info shown in the right panel.
func (s Session) statusContent() string {
	status := "idle"
	if s.running {
		status = "running"
	}
	return fmt.Sprintf(
		"Provider:  %s\nModel:     %s\nStatus:    %s\n\nDir:\n%s",
		s.runner.Provider,
		s.runner.Model,
		status,
		s.runner.Workspace,
	)
}
