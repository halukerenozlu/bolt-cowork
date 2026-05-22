package keys

import "github.com/charmbracelet/bubbles/key"

// KeyMap holds the key bindings for the TUI.
type KeyMap struct {
	Quit    key.Binding
	Palette key.Binding
	Chord   key.Binding // ctrl+x — prefix for chord commands
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
	// Chord is the ctrl+x prefix key.
	// Second key determines the action:
	//   l → switch session
	//   m → switch model
	//   e → open editor
	//   n → new session
	//   h → hide tips
	//   s → view status
	//   t → switch theme
	Chord: key.NewBinding(
		key.WithKeys("ctrl+x"),
		key.WithHelp("ctrl+x", "chord prefix"),
	),
}
