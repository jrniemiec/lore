package tui

import "github.com/charmbracelet/bubbles/key"

// keyMap defines all lore key bindings.
type keyMap struct {
	Send          key.Binding
	Newline       key.Binding
	Cancel        key.Binding
	SwitchTopic   key.Binding
	SwitchProfile key.Binding
	ClearScreen   key.Binding
	NavUp         key.Binding
	NavDown       key.Binding
	ScrollUp      key.Binding
	ScrollDown    key.Binding
	Dismiss       key.Binding
	FocusConv     key.Binding
	FillCompletion key.Binding
	CloseOverlay   key.Binding
	CorrectInput   key.Binding // Ctrl+G: send input for spell/grammar correction
	OpenView       key.Binding // Ctrl+O: open /view prompt
}

var keys = keyMap{
	Send: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "send"),
	),
	Newline: key.NewBinding(
		key.WithKeys("ctrl+j"),
		key.WithHelp("ctrl+j", "newline"),
	),
	Cancel: key.NewBinding(
		key.WithKeys("ctrl+c"),
		key.WithHelp("ctrl+c", "cancel/quit"),
	),
	SwitchTopic: key.NewBinding(
		key.WithKeys("ctrl+t"),
		key.WithHelp("ctrl+t", "switch topic"),
	),
	SwitchProfile: key.NewBinding(
		key.WithKeys("ctrl+p"),
		key.WithHelp("ctrl+p", "switch profile"),
	),
	ClearScreen: key.NewBinding(
		key.WithKeys("ctrl+l"),
		key.WithHelp("ctrl+l", "clear screen"),
	),
	NavUp: key.NewBinding(
		key.WithKeys("up"),
		key.WithHelp("↑", "prev exchange"),
	),
	NavDown: key.NewBinding(
		key.WithKeys("down"),
		key.WithHelp("↓", "next exchange"),
	),
	ScrollUp: key.NewBinding(
		key.WithKeys("pgup"),
		key.WithHelp("pgup", "scroll up"),
	),
	ScrollDown: key.NewBinding(
		key.WithKeys("pgdown"),
		key.WithHelp("pgdn", "scroll down"),
	),
	Dismiss: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "back to input"),
	),
	FocusConv: key.NewBinding(
		key.WithKeys("ctrl+n"),
		key.WithHelp("ctrl+n", "toggle focus: input ↔ conversation"),
	),
	FillCompletion: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "fill selected completion into input"),
	),
	CloseOverlay: key.NewBinding(
		key.WithKeys("ctrl+x"),
		key.WithHelp("ctrl+x", "close overlay"),
	),
	CorrectInput: key.NewBinding(
		key.WithKeys("ctrl+g"),
		key.WithHelp("ctrl+g", "correct spelling/grammar"),
	),
	OpenView: key.NewBinding(
		key.WithKeys("ctrl+o"),
		key.WithHelp("ctrl+o", "open file in viewer"),
	),
}
