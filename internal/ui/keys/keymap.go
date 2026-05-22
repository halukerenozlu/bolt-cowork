package keys

import "github.com/charmbracelet/bubbles/key"

// KeyMap holds the key bindings for the TUI.
type KeyMap struct {
	Quit    key.Binding
	Palette key.Binding
}

// Default is the application-wide key map.
var Default = KeyMap{
	Quit: key.NewBinding(
		key.WithKeys("ctrl+c"),
		key.WithHelp("ctrl+c", "quit"),
	),
	Palette: key.NewBinding(
		key.WithKeys("ctrl+p"),
		key.WithHelp("ctrl+p", "command palette"),
	),
}
