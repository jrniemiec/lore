package tui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.vom/jrniemiec/lore/config"
	"github.vom/jrniemiec/lore/engine"
)

// programSend is set by Start() so streaming goroutines can send delta msgs
// back into the Bubbletea event loop.
var programSend func(tea.Msg)

// Start launches the TUI and blocks until the user quits.
func Start(eng *engine.Engine, cfg config.Config, loreHome string) error {
	// Alternate scroll mode: converts mouse wheel events to cursor-key sequences
	// inside alt-screen without enabling mouse reporting. This keeps text
	// selection working normally (no pointer capture).
	// Clear the terminal scrollback buffer so the native scrollbar doesn't
	// expose old shell output when dragged. Then enable alternate scroll mode
	// (wheel → cursor-key) without capturing mouse events.
	fmt.Fprint(os.Stdout, "\033[3J\033[?1007h")
	defer fmt.Fprint(os.Stdout, "\033[?1007l")

	m := New(eng, cfg, loreHome)
	p := tea.NewProgram(
		m,
		tea.WithAltScreen(),
	)
	programSend = func(msg tea.Msg) { p.Send(msg) }
	_, err := p.Run()
	programSend = nil
	return err
}
