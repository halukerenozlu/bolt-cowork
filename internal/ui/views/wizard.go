package views

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/halukerenozlu/bolt-cowork/internal/config"
	"github.com/halukerenozlu/bolt-cowork/internal/ui/theme"
)

// Wizard step constants.
const (
	wizStepAuth   = 0 // choose auth method
	wizStepKey    = 1 // enter API key
	wizStepVerify = 2 // verifying connection (spinner)
	wizStepModels = 3 // select model
)

// wizardAuthOptions returns the options for the auth-method step.
// If an env var is detected, the first option uses the detected key.
func wizardAuthOptions(provider string, hasExisting bool) []string {
	preset, _ := config.HostedPresets[provider]
	envKey := config.DetectEnvKey(provider)

	var opts []string
	if hasExisting {
		opts = append(opts, "Use existing credential", "Replace API key", "Cancel")
		return opts
	}
	if envKey != "" {
		masked := "****"
		if len(envKey) >= 10 {
			masked = envKey[:4] + "..." + envKey[len(envKey)-4:]
		}
		opts = append(opts, "Use $"+preset.EnvVar+" ("+masked+")")
	}
	if !preset.RequiresAPIKey {
		opts = append(opts, "Continue without key")
	}
	opts = append(opts, "Enter API key")
	opts = append(opts, "Cancel")
	return opts
}

func (s Session) startWizard(provider string) (Session, tea.Cmd) {
	s.wizardOpen = true
	s.wizardProvider = provider
	s.wizardStep = wizStepAuth
	s.wizardCursor = 0
	s.wizardErr = ""
	s.wizardModels = nil
	s.wizardAPIKey = ""
	s.wizardPersist = false
	s.wizardPreviousAPIKey = ""
	if s.cfg != nil {
		if pc, ok := s.cfg.Providers[provider]; ok {
			s.wizardPreviousAPIKey = pc.APIKey
		}
	}
	s.modalOpen = false
	s.modalCommand = ""

	ti := textinput.New()
	ti.Placeholder = "sk-..."
	ti.EchoMode = textinput.EchoPassword
	ti.EchoCharacter = '•'
	ti.Width = 44
	s.wizardInput = ti

	return s, nil
}

func (s Session) updateWizard(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return s.updateWizardKey(msg)
	case WizardVerifyResultMsg:
		return s.handleWizardVerifyResult(msg)
	case WizardModelsResultMsg:
		return s.handleWizardModelsResult(msg)
	}

	if s.wizardStep == wizStepKey {
		var cmd tea.Cmd
		s.wizardInput, cmd = s.wizardInput.Update(msg)
		return s, cmd
	}
	return s, nil
}

func (s Session) updateWizardKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		s = s.restoreWizardCredential()
		s.wizardOpen = false
		return s, nil
	case tea.KeyUp:
		if (s.wizardStep == wizStepAuth || s.wizardStep == wizStepModels) && s.wizardCursor > 0 {
			s.wizardCursor--
		}
		return s, nil
	case tea.KeyDown:
		return s.wizardKeyDown()
	case tea.KeyEnter:
		return s.wizardKeyEnter()
	}

	if s.wizardStep == wizStepKey {
		var cmd tea.Cmd
		s.wizardInput, cmd = s.wizardInput.Update(msg)
		return s, cmd
	}
	return s, nil
}

func (s Session) wizardKeyDown() (tea.Model, tea.Cmd) {
	switch s.wizardStep {
	case wizStepAuth:
		opts := wizardAuthOptions(s.wizardProvider, s.wizardPreviousAPIKey != "")
		if s.wizardCursor < len(opts)-1 {
			s.wizardCursor++
		}
	case wizStepModels:
		if s.wizardCursor < len(s.wizardModels)-1 {
			s.wizardCursor++
		}
	}
	return s, nil
}

func (s Session) wizardKeyEnter() (tea.Model, tea.Cmd) {
	switch s.wizardStep {
	case wizStepAuth:
		return s.wizardAuthConfirm()
	case wizStepKey:
		return s.wizardKeyConfirm()
	case wizStepModels:
		return s.wizardModelConfirm()
	}
	return s, nil
}

func (s Session) wizardAuthConfirm() (tea.Model, tea.Cmd) {
	opts := wizardAuthOptions(s.wizardProvider, s.wizardPreviousAPIKey != "")
	selected := opts[s.wizardCursor]

	switch {
	case selected == "Cancel":
		s = s.restoreWizardCredential()
		s.wizardOpen = false
		return s, nil

	case selected == "Use existing credential":
		s.wizardErr = ""
		return s.wizardStartVerify()

	case selected == "Replace API key":
		s.wizardStep = wizStepKey
		s.wizardCursor = 0
		s.wizardErr = ""
		s.wizardInput.Reset()
		s.wizardInput.Focus()
		return s, textinput.Blink

	case strings.HasPrefix(selected, "Use $"):
		envKey := config.DetectEnvKey(s.wizardProvider)
		s.wizardErr = ""
		s = s.configureWizardProvider(envKey, false)
		return s.wizardStartVerify()

	case selected == "Enter API key":
		s.wizardStep = wizStepKey
		s.wizardCursor = 0
		s.wizardErr = ""
		s.wizardInput.Reset()
		s.wizardInput.Focus()
		return s, textinput.Blink

	case selected == "Continue without key":
		s.wizardErr = ""
		s = s.configureWizardProvider("", false)
		return s.wizardStartVerify()
	}
	return s, nil
}

func (s Session) wizardKeyConfirm() (tea.Model, tea.Cmd) {
	apiKey := strings.TrimSpace(s.wizardInput.Value())
	if apiKey == "" {
		s.wizardErr = "API key cannot be empty."
		return s, nil
	}
	s.wizardErr = ""
	s = s.configureWizardProvider(apiKey, true)
	return s.wizardStartVerify()
}

func (s Session) configureWizardProvider(apiKey string, persist bool) Session {
	s.wizardAPIKey = apiKey
	s.wizardPersist = persist
	if s.runner.ConfigureProvider != nil {
		s.runner.ConfigureProvider(s.wizardProvider, apiKey)
		return s
	}
	if s.cfg != nil {
		ensureProviderModelConfigured(s.cfg, s.wizardProvider, "")
		pc := s.cfg.Providers[s.wizardProvider]
		pc.APIKey = apiKey
		s.cfg.Providers[s.wizardProvider] = pc
	}
	return s
}

func (s Session) restoreWizardCredential() Session {
	if !s.wizardPersist {
		return s
	}
	if s.runner.ConfigureProvider != nil {
		s.runner.ConfigureProvider(s.wizardProvider, s.wizardPreviousAPIKey)
	} else if s.cfg != nil {
		pc := s.cfg.Providers[s.wizardProvider]
		pc.APIKey = s.wizardPreviousAPIKey
		s.cfg.Providers[s.wizardProvider] = pc
	}
	s.wizardPersist = false
	s.wizardAPIKey = ""
	return s
}

func (s Session) wizardStartVerify() (tea.Model, tea.Cmd) {
	s.wizardStep = wizStepVerify
	provider := s.wizardProvider

	return s, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var err error
		if s.runner.VerifyProvider != nil {
			err = s.runner.VerifyProvider(ctx, provider)
		}
		return WizardVerifyResultMsg{Provider: provider, Err: err}
	}
}

func (s Session) handleWizardVerifyResult(msg WizardVerifyResultMsg) (tea.Model, tea.Cmd) {
	if !s.wizardOpen || msg.Provider != s.wizardProvider {
		return s, nil
	}
	if msg.Err != nil {
		s = s.restoreWizardCredential()
		s.wizardStep = wizStepAuth
		s.wizardCursor = 0
		s.wizardErr = "Verification failed: " + msg.Err.Error()
		return s, nil
	}

	// Try model discovery.
	models := s.wizardFallbackModels()

	if s.runner.DiscoverModels != nil {
		provider := s.wizardProvider
		s.wizardStep = wizStepVerify // keep spinner while discovering
		return s, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			discovered, err := s.runner.DiscoverModels(ctx, provider)
			return WizardModelsResultMsg{Provider: provider, Models: discovered, Err: err}
		}
	}

	if len(models) > 0 {
		s.wizardStep = wizStepModels
		s.wizardModels = models
		s.wizardCursor = 0
		return s, nil
	}

	s.wizardStep = wizStepAuth
	s.wizardErr = "No models are available for " + s.wizardProvider + "."
	return s, nil
}

func (s Session) handleWizardModelsResult(msg WizardModelsResultMsg) (tea.Model, tea.Cmd) {
	if !s.wizardOpen || msg.Provider != s.wizardProvider {
		return s, nil
	}

	models := msg.Models
	if msg.Err != nil || len(models) == 0 {
		// Fall back to default/configured models.
		models = s.wizardFallbackModels()
	}

	if len(models) == 0 {
		s.wizardStep = wizStepAuth
		s.wizardErr = "No models are available for " + s.wizardProvider + "."
		return s, nil
	}

	s.wizardStep = wizStepModels
	s.wizardModels = models
	s.wizardCursor = 0
	return s, nil
}

func (s Session) wizardFallbackModels() []string {
	if local, ok := s.localDetected[s.wizardProvider]; ok && len(local.Models) > 0 {
		return append([]string(nil), local.Models...)
	}
	if models := s.cfg.GetModelsForProvider(s.wizardProvider); len(models) > 0 {
		return models
	}
	return append([]string(nil), config.DefaultModels[s.wizardProvider]...)
}

func (s Session) wizardModelConfirm() (tea.Model, tea.Cmd) {
	if s.wizardCursor >= len(s.wizardModels) {
		return s, nil
	}
	return s.wizardFinish(s.wizardModels[s.wizardCursor])
}

func (s Session) wizardFinish(model string) (tea.Model, tea.Cmd) {
	prov := s.wizardProvider
	s.wizardOpen = false

	if model == "" {
		model = s.defaultModelForProvider(prov)
	}
	if model == "" {
		s.wizardOpen = true
		s.wizardStep = wizStepAuth
		s.wizardErr = "Select or discover a model before connecting."
		return s, nil
	}

	if s.wizardPersist && s.wizardAPIKey != "" && s.runner.PersistProviderKey != nil {
		if err := s.runner.PersistProviderKey(prov, s.wizardAPIKey); err != nil {
			s = s.appendCommandOutput("Keyring unavailable; using credential for this session only: " + err.Error())
		}
	}
	s.wizardPersist = false
	s = s.commitProviderSwitch(prov, model)
	return s, func() tea.Msg {
		return RuntimeModelChangedMsg{Provider: prov, Model: model}
	}
}

// viewWizard renders the connection wizard overlay.
func (s Session) viewWizard() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(theme.Primary)
	mutedStyle := lipgloss.NewStyle().Foreground(theme.Muted)
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555"))
	selectedBg := lipgloss.NewStyle().
		Background(theme.Primary).
		Foreground(lipgloss.Color("255"))

	boxW := 56
	if s.width > 0 && s.width-4 < boxW {
		boxW = s.width - 4
	}
	innerW := boxW - 2
	textW := max(innerW-2, 8)

	var sb strings.Builder

	title := titleStyle.Render("Connect " + s.wizardProvider)
	sb.WriteString(title + "\n")
	sb.WriteString(strings.Repeat("─", textW) + "\n")

	switch s.wizardStep {
	case wizStepAuth:
		sb.WriteString(mutedStyle.Render("Choose authentication method") + "\n\n")
		opts := wizardAuthOptions(s.wizardProvider, s.wizardPreviousAPIKey != "")
		for i, opt := range opts {
			if i == s.wizardCursor {
				line := selectedBg.Width(textW).Render("▶ " + opt)
				sb.WriteString(line + "\n")
			} else {
				sb.WriteString("  " + opt + "\n")
			}
		}

	case wizStepKey:
		sb.WriteString(mutedStyle.Render("Enter API key for "+s.wizardProvider) + "\n\n")
		s.wizardInput.Width = max(textW-2, 1)
		sb.WriteString("> " + s.wizardInput.View() + "\n\n")
		sb.WriteString(mutedStyle.Render("Enter to confirm, Esc to cancel") + "\n")

	case wizStepVerify:
		sb.WriteString(mutedStyle.Render("Verifying connection...") + "\n\n")
		sb.WriteString("⠋ Connecting to " + s.wizardProvider + "\n")

	case wizStepModels:
		sb.WriteString(mutedStyle.Render("Select a model") + "\n\n")
		maxShow := 12
		start := 0
		if s.wizardCursor >= maxShow {
			start = s.wizardCursor - maxShow + 1
		}
		end := start + maxShow
		if end > len(s.wizardModels) {
			end = len(s.wizardModels)
		}
		for i := start; i < end; i++ {
			m := s.wizardModels[i]
			if i == s.wizardCursor {
				line := selectedBg.Width(textW).Render("▶ " + m)
				sb.WriteString(line + "\n")
			} else {
				sb.WriteString("  " + m + "\n")
			}
		}
		if len(s.wizardModels) > maxShow {
			sb.WriteString(mutedStyle.Render("↑/↓ scroll") + "\n")
		}
	}

	if s.wizardErr != "" {
		sb.WriteString("\n" + errStyle.Render(s.wizardErr) + "\n")
	}

	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(theme.Primary).
		Padding(0, 1).
		Width(innerW).
		Render(sb.String())
}
