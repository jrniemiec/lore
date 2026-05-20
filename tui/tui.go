package tui

import (
	"encoding/json"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jrniemiec/lore/config"
	"github.com/jrniemiec/lore/engine"
	"github.com/jrniemiec/lore/internal/clog"
)

// programSend is set by Start() so streaming goroutines can send delta msgs
// back into the Bubbletea event loop.
var programSend func(tea.Msg)

// Start launches the TUI and blocks until the user quits.
func Start(eng *engine.Engine, cfg config.Config, loreData string, theme string, chatLabels bool, foldLines int, foldOnStart bool) error {
	// Alternate scroll mode: converts mouse wheel events to cursor-key sequences
	// inside alt-screen without enabling mouse reporting. This keeps text
	// selection working normally (no pointer capture).
	// Clear the terminal scrollback buffer so the native scrollbar doesn't
	// expose old shell output when dragged. Then enable alternate scroll mode
	// (wheel → cursor-key) without capturing mouse events.
	DetectTerminal()
	ApplyTheme(theme)
	AdjustThemeForTerminal()

	// Clear scrollback so shell history isn't visible above the TUI.
	fmt.Fprint(os.Stdout, "\033[3J")
	// Alternate scroll mode (wheel → cursor keys) works reliably on iTerm2.
	// On Terminal.app it causes scrolling past the top of the UI into shell history.
	if ActiveTerminal == TermITerm2 {
		fmt.Fprint(os.Stdout, "\033[?1007h")
		defer fmt.Fprint(os.Stdout, "\033[?1007l")
	}

	clog.Infof("tui: start topic=%s profile=%s terminal=%s theme=%s", eng.TopicName(), eng.ProfileCode(), ActiveTerminal, theme)
	if clog.IsNew() {
		if b, err := json.Marshal(cfg); err == nil {
			clog.Raw("startup config", string(b))
		}
	}

	m := New(eng, cfg, loreData)
	m.themeMode = theme
	m.chatLabels = chatLabels
	m.foldLines = foldLines
	m.foldOnStart = foldOnStart

	opts := []tea.ProgramOption{tea.WithAltScreen()}
	// On Terminal.app, enable mouse cell motion to capture scroll events and
	// prevent them from reaching the terminal's native scrollback buffer.
	if ActiveTerminal != TermITerm2 {
		opts = append(opts, tea.WithMouseCellMotion())
	}
	p := tea.NewProgram(m, opts...)
	programSend = func(msg tea.Msg) { p.Send(msg) }
	_, err := p.Run()
	programSend = nil
	return err
}
