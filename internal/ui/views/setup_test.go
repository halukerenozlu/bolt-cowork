package views

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type setupInput struct {
	messages []tea.Msg
	saveFunc func(provider, apiKey string) error
}

type setupExpected struct {
	step          int
	cursor        int
	width         int
	height        int
	provider      string
	errContains   string
	viewContains  []string
	completed     bool
	saveCalled    bool
	savedProvider string
	savedKey      string
}

func TestSetup_StateTransitions(t *testing.T) {
	tests := []struct {
		name     string
		input    setupInput
		expected setupExpected
	}{
		{
			name:     "initial state",
			input:    setupInput{},
			expected: setupExpected{step: 0, cursor: 0},
		},
		{
			name: "window size sets fields",
			input: setupInput{messages: []tea.Msg{
				tea.WindowSizeMsg{Width: 120, Height: 40},
			}},
			expected: setupExpected{step: 0, cursor: 0, width: 120, height: 40},
		},
		{
			name: "cursor moves with arrow keys",
			input: setupInput{messages: []tea.Msg{
				tea.KeyMsg{Type: tea.KeyDown},
				tea.KeyMsg{Type: tea.KeyDown},
				tea.KeyMsg{Type: tea.KeyDown},
				tea.KeyMsg{Type: tea.KeyUp},
			}},
			expected: setupExpected{step: 0, cursor: 1},
		},
		{
			name: "enter on provider advances to key step",
			input: setupInput{messages: []tea.Msg{
				tea.KeyMsg{Type: tea.KeyEnter},
			}},
			expected: setupExpected{step: 1, cursor: 0, provider: "anthropic"},
		},
		{
			name: "second provider selected",
			input: setupInput{messages: []tea.Msg{
				tea.KeyMsg{Type: tea.KeyDown},
				tea.KeyMsg{Type: tea.KeyEnter},
			}},
			expected: setupExpected{step: 1, cursor: 1, provider: "openai"},
		},
		{
			name: "empty API key shows error",
			input: setupInput{messages: []tea.Msg{
				tea.KeyMsg{Type: tea.KeyEnter},
				tea.KeyMsg{Type: tea.KeyEnter},
			}},
			expected: setupExpected{
				step:        1,
				provider:    "anthropic",
				errContains: "API key cannot be empty",
			},
		},
		{
			name: "save error shows message",
			input: setupInput{
				messages: []tea.Msg{
					tea.KeyMsg{Type: tea.KeyEnter},
					tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("sk-key")},
					tea.KeyMsg{Type: tea.KeyEnter},
				},
				saveFunc: func(_, _ string) error {
					return errors.New("keyring unavailable")
				},
			},
			expected: setupExpected{
				step:        1,
				provider:    "anthropic",
				errContains: "keyring unavailable",
			},
		},
		{
			name: "warning error completes setup",
			input: setupInput{
				messages: []tea.Msg{
					tea.KeyMsg{Type: tea.KeyEnter},
					tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("sk-key")},
					tea.KeyMsg{Type: tea.KeyEnter},
				},
				saveFunc: func(_, _ string) error {
					return SetupWarningError{Message: "keyring missing"}
				},
			},
			expected: setupExpected{
				step:        1,
				provider:    "anthropic",
				errContains: "keyring missing",
				completed:   true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, _ := runSetupMessages(tt.input)
			assertSetupState(t, got, tt.expected)
		})
	}
}

func TestSetup_SaveScenarios(t *testing.T) {
	tests := []struct {
		name     string
		input    setupInput
		expected setupExpected
	}{
		{
			name: "valid API key calls save func",
			input: setupInput{messages: []tea.Msg{
				tea.KeyMsg{Type: tea.KeyEnter},
				tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("sk-my-api-key")},
				tea.KeyMsg{Type: tea.KeyEnter},
			}},
			expected: setupExpected{
				step:          1,
				provider:      "anthropic",
				completed:     true,
				saveCalled:    true,
				savedProvider: "anthropic",
				savedKey:      "sk-my-api-key",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, savedProvider, savedKey := runSetupMessages(tt.input)
			assertSetupState(t, got, tt.expected)
			if tt.expected.saveCalled && savedProvider == "" {
				t.Fatal("saveFunc was not called")
			}
			if savedProvider != tt.expected.savedProvider {
				t.Errorf("savedProvider = %q, want %q", savedProvider, tt.expected.savedProvider)
			}
			if savedKey != tt.expected.savedKey {
				t.Errorf("savedKey = %q, want %q", savedKey, tt.expected.savedKey)
			}
		})
	}
}

func TestSetup_View(t *testing.T) {
	tests := []struct {
		name     string
		input    setupInput
		expected setupExpected
	}{
		{
			name: "step 0 shows provider list and security note",
			input: setupInput{messages: []tea.Msg{
				tea.WindowSizeMsg{Width: 100, Height: 30},
			}},
			expected: setupExpected{viewContains: []string{
				"Anthropic", "OpenAI", "Gemini", ".env", ".key", ".pem", "*credentials*",
			}},
		},
		{
			name: "step 1 shows input hint",
			input: setupInput{messages: []tea.Msg{
				tea.KeyMsg{Type: tea.KeyEnter},
				tea.WindowSizeMsg{Width: 100, Height: 30},
			}},
			expected: setupExpected{viewContains: []string{"anthropic"}},
		},
		{
			name: "completed view shows success",
			input: setupInput{messages: []tea.Msg{
				tea.WindowSizeMsg{Width: 100, Height: 30},
				tea.KeyMsg{Type: tea.KeyEnter},
				tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("sk-complete-key")},
				tea.KeyMsg{Type: tea.KeyEnter},
			}},
			expected: setupExpected{viewContains: []string{"Setup complete"}},
		},
		{
			name: "completed warning view shows warning",
			input: setupInput{
				messages: []tea.Msg{
					tea.WindowSizeMsg{Width: 100, Height: 30},
					tea.KeyMsg{Type: tea.KeyEnter},
					tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("sk-key")},
					tea.KeyMsg{Type: tea.KeyEnter},
				},
				saveFunc: func(_, _ string) error {
					return SetupWarningError{Message: "keyring missing"}
				},
			},
			expected: setupExpected{viewContains: []string{"Setup complete", "keyring missing"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, _ := runSetupMessages(tt.input)
			view := stripANSI(got.View())
			for _, want := range tt.expected.viewContains {
				if !strings.Contains(view, want) {
					t.Errorf("view missing %q; got:\n%s", want, view)
				}
			}
		})
	}
}

func runSetupMessages(input setupInput) (Setup, string, string) {
	var savedProvider, savedKey string
	saveFunc := input.saveFunc
	if saveFunc == nil {
		saveFunc = func(provider, apiKey string) error {
			savedProvider = provider
			savedKey = apiKey
			return nil
		}
	}

	s := NewSetup(saveFunc)
	for _, msg := range input.messages {
		s, _ = applyMsg(s, msg)
	}
	return s, savedProvider, savedKey
}

func assertSetupState(t *testing.T, got Setup, want setupExpected) {
	t.Helper()
	if got.step != want.step {
		t.Errorf("step = %d, want %d", got.step, want.step)
	}
	if got.cursor != want.cursor {
		t.Errorf("cursor = %d, want %d", got.cursor, want.cursor)
	}
	if got.width != want.width {
		t.Errorf("width = %d, want %d", got.width, want.width)
	}
	if got.height != want.height {
		t.Errorf("height = %d, want %d", got.height, want.height)
	}
	if got.provider != want.provider {
		t.Errorf("provider = %q, want %q", got.provider, want.provider)
	}
	if got.completed != want.completed {
		t.Errorf("completed = %v, want %v", got.completed, want.completed)
	}
	if want.errContains != "" && !strings.Contains(got.errMsg, want.errContains) {
		t.Errorf("errMsg = %q, want to contain %q", got.errMsg, want.errContains)
	}
}

// applyMsg applies a message to the Setup model and returns the updated Setup.
func applyMsg(s Setup, msg tea.Msg) (Setup, tea.Cmd) {
	m, cmd := s.Update(msg)
	return m.(Setup), cmd
}

// --- TrustModal tests ---

func applyTrustMsg(t TrustModal, msg tea.Msg) (TrustModal, tea.Cmd) {
	m, cmd := t.Update(msg)
	return m.(TrustModal), cmd
}

type trustModalInput struct {
	messages []tea.Msg
	trustErr error
}

type trustModalExpected struct {
	trusted     bool
	declined    bool
	trustCalled bool
	errContains string
}

func TestTrustModal_StateTransitions(t *testing.T) {
	tests := []struct {
		name     string
		input    trustModalInput
		expected trustModalExpected
	}{
		{
			name:     "initial state",
			input:    trustModalInput{},
			expected: trustModalExpected{},
		},
		{
			name: "enter on first option trusts",
			input: trustModalInput{messages: []tea.Msg{
				tea.KeyMsg{Type: tea.KeyEnter},
			}},
			expected: trustModalExpected{trusted: true, trustCalled: true},
		},
		{
			name: "trust save error keeps modal open",
			input: trustModalInput{
				messages: []tea.Msg{
					tea.KeyMsg{Type: tea.KeyEnter},
				},
				trustErr: errors.New("disk full"),
			},
			expected: trustModalExpected{
				trusted:     false,
				declined:    false,
				trustCalled: true,
				errContains: "disk full",
			},
		},
		{
			name: "move down then enter declines",
			input: trustModalInput{messages: []tea.Msg{
				tea.KeyMsg{Type: tea.KeyDown},
				tea.KeyMsg{Type: tea.KeyEnter},
			}},
			expected: trustModalExpected{declined: true},
		},
		{
			name: "press y trusts",
			input: trustModalInput{messages: []tea.Msg{
				tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")},
			}},
			expected: trustModalExpected{trusted: true, trustCalled: true},
		},
		{
			name: "press 1 trusts",
			input: trustModalInput{messages: []tea.Msg{
				tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")},
			}},
			expected: trustModalExpected{trusted: true, trustCalled: true},
		},
		{
			name: "press n declines",
			input: trustModalInput{messages: []tea.Msg{
				tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")},
			}},
			expected: trustModalExpected{declined: true},
		},
		{
			name: "press 2 declines",
			input: trustModalInput{messages: []tea.Msg{
				tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")},
			}},
			expected: trustModalExpected{declined: true},
		},
		{
			name: "esc declines",
			input: trustModalInput{messages: []tea.Msg{
				tea.KeyMsg{Type: tea.KeyEsc},
			}},
			expected: trustModalExpected{declined: true},
		},
		{
			name: "cursor clamps at bounds",
			input: trustModalInput{messages: []tea.Msg{
				tea.KeyMsg{Type: tea.KeyUp},
				tea.KeyMsg{Type: tea.KeyDown},
				tea.KeyMsg{Type: tea.KeyDown},
				tea.KeyMsg{Type: tea.KeyDown},
			}},
			expected: trustModalExpected{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var trustCalled bool
			tm := NewTrustModal("/test/dir", func(dir string) error {
				trustCalled = true
				return tt.input.trustErr
			})
			for _, msg := range tt.input.messages {
				tm, _ = applyTrustMsg(tm, msg)
			}
			if tm.IsTrusted() != tt.expected.trusted {
				t.Errorf("trusted = %v, want %v", tm.IsTrusted(), tt.expected.trusted)
			}
			if tm.IsDeclined() != tt.expected.declined {
				t.Errorf("declined = %v, want %v", tm.IsDeclined(), tt.expected.declined)
			}
			if trustCalled != tt.expected.trustCalled {
				t.Errorf("trustCalled = %v, want %v", trustCalled, tt.expected.trustCalled)
			}
			if tt.expected.errContains != "" && !strings.Contains(tm.errMsg, tt.expected.errContains) {
				t.Errorf("errMsg = %q, want to contain %q", tm.errMsg, tt.expected.errContains)
			}
		})
	}
}

func TestTrustModal_View(t *testing.T) {
	tm := NewTrustModal("testdata/sample-dir/project", nil)
	tm, _ = applyTrustMsg(tm, tea.WindowSizeMsg{Width: 100, Height: 30})

	view := stripANSI(tm.View())
	for _, want := range []string{
		"Trust this directory",
		"testdata/sample-dir/project",
		"Yes, I trust this folder",
		"No, exit",
		"read, edit, and execute",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q; got:\n%s", want, view)
		}
	}
}
