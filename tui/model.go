package tui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.vom/jrniemiec/lore/config"
	"github.vom/jrniemiec/lore/core"
	"github.vom/jrniemiec/lore/engine"
	"github.vom/jrniemiec/lore/store"
)

// focusPane identifies which pane has keyboard focus.
type focusPane int

const (
	paneInput focusPane = iota
	paneConv
)

// exchange holds one complete user+assistant turn.
type exchange struct {
	userMsg  core.Message
	asstMsg  core.Message // empty while streaming
	costUSD  float64
	elapsed  time.Duration
	complete bool // false while assistant reply is still streaming
}

// Bubbletea message types.
type streamDeltaMsg string
type streamDoneMsg struct {
	result engine.ChatResult
	err    error
}
type spinnerTickMsg struct{}

// Model is the root Bubbletea application model.
type Model struct {
	eng      *engine.Engine
	cfg      config.Config
	loreHome string

	// layout (set by WindowSizeMsg)
	width  int
	height int

	// panes
	conv  viewport.Model
	input textarea.Model
	focus focusPane

	// conversation
	exchanges    []exchange
	focusedExIdx int // index of focused exchange when paneConv is active; -1 = none
	streaming    bool
	streamBuf    string
	cancelStream context.CancelFunc

	// spinner: pulsating snowflake ❄ bold/dim alternating
	spinnerFrame int
	// cursor blink: toggled on every spinner tick (400 ms → 800 ms period)
	cursorVisible bool

	// userScrolled is true when the user has manually scrolled up, suppressing
	// the automatic GotoBottom() that normally keeps the latest content visible.
	userScrolled bool

	// status
	lastResult   *engine.ChatResult
	topicStats   store.CallStats
	sessionStats store.CallStats

	// ctrl+c double-press
	lastCtrlC time.Time
}

// New creates a ready-to-run Model, loading existing history.
func New(eng *engine.Engine, cfg config.Config, loreHome string) Model {
	ta := textarea.New()
	ta.Placeholder = ""
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.SetHeight(1)
	ta.Focus()
	// SetWidth and Prompt are updated in syncInputPrompt() after layout is known.

	// Set textarea styles. Background is handled by the input pane wrapper
	// (raw ANSI), so all styles here use no background.
	noStyle := lipgloss.NewStyle()
	dimStyle := noStyle.Foreground(ActiveTheme.Dimmed)
	textStyle := noStyle.Foreground(ActiveTheme.TopBarText)
	promptStyle := noStyle.Foreground(ActiveTheme.InputPrompt)
	fullReset := textarea.Style{
		Base:             noStyle,
		CursorLine:       noStyle,
		CursorLineNumber: noStyle,
		EndOfBuffer:      noStyle,
		LineNumber:       dimStyle,
		Placeholder:      dimStyle,
		Prompt:           promptStyle,
		Text:             textStyle,
	}
	ta.FocusedStyle = fullReset
	ta.BlurredStyle = fullReset

	vp := viewport.New(0, 0)
	vp.Style = lipgloss.NewStyle()

	m := Model{
		eng:           eng,
		cfg:           cfg,
		loreHome:      loreHome,
		conv:          vp,
		input:         ta,
		focus:         paneInput,
		focusedExIdx:  -1,
		cursorVisible: true,
	}
	m.loadUsageStats()
	m.loadHistory()
	return m
}

// Init starts the spinner ticker. Cursor blink is driven by spinnerTick.
func (m Model) Init() tea.Cmd {
	return spinnerTick()
}

func spinnerTick() tea.Cmd {
	return tea.Tick(400*time.Millisecond, func(time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

// loadHistory populates exchanges from the engine's current topic history.
func (m *Model) loadHistory() {
	h := m.eng.Topic().History
	for i := 0; i+1 < len(h.Msgs); i++ {
		u := h.Msgs[i]
		a := h.Msgs[i+1]
		if u.Role == core.RoleUser && a.Role == core.RoleAssistant {
			m.exchanges = append(m.exchanges, exchange{
				userMsg:  u,
				asstMsg:  a,
				complete: true,
			})
			i++
		}
	}
}

// loadUsageStats reads the usage log into topicStats and sessionStats.
func (m *Model) loadUsageStats() {
	logPath := store.UsageLogPath(m.loreHome)
	entries, err := store.ReadUsageLog(logPath)
	if err != nil || len(entries) == 0 {
		return
	}
	agg := store.AggregateUsage(entries, m.eng.TopicName(), 0)
	m.topicStats = agg.Total
	aggAll := store.AggregateUsage(entries, "", 0)
	m.sessionStats = aggAll.Total
}

// contextFillPct returns 0-100, or -1 if no limit is configured.
func (m *Model) contextFillPct() int {
	limit := m.eng.Profile().MaxContextTokens
	if limit <= 0 {
		return -1
	}
	used := 0
	for _, ex := range m.exchanges {
		used += core.ApproxTokens(ex.userMsg.Content)
		used += core.ApproxTokens(ex.asstMsg.Content)
	}
	pct := used * 100 / limit
	if pct > 100 {
		pct = 100
	}
	return pct
}

// inputPrompt returns the prefix shown in the input pane.
func (m *Model) inputPrompt() string {
	return m.eng.TopicName() + "/" + m.eng.Profile().Model + "> "
}

// inputVisualHeight returns the number of visual (wrapped) lines the input
// text occupies given the current terminal width, accounting for the prompt.
func (m *Model) inputVisualHeight() int {
	if m.width == 0 {
		return 1
	}
	prompt := m.inputPrompt()
	const padW = 1
	line0W := m.width - padW - len([]rune(prompt))
	contW := m.width - padW
	if line0W < 1 {
		line0W = 1
	}
	if contW < 1 {
		contW = 1
	}
	total := 0
	for i, line := range strings.Split(m.input.Value(), "\n") {
		runes := []rune(line)
		wW := contW
		if i == 0 {
			wW = line0W
		}
		if len(runes) == 0 {
			total++
		} else {
			total += (len(runes) + wW - 1) / wW
		}
	}
	if total < 1 {
		total = 1
	}
	return total
}

// syncInputPrompt updates the textarea's built-in Prompt field so the cursor
// appears on the same line as the prefix. Called on resize and profile/topic switch.
func (m *Model) syncInputPrompt() {
	prompt := m.inputPrompt()
	m.input.Prompt = prompt
	m.input.SetWidth(m.width - len([]rune(prompt)))
}

// syncLayout recalculates the conversation viewport height based on current
// terminal size and textarea height. Call after resize or textarea height change.
func (m *Model) syncLayout() {
	// Layout (each value = number of terminal lines):
	//   top bar:    2 (text + separator)
	//   conv:       convH
	//   input pane: 1 (separator) + textarea.Height()
	//   status bar: 2 (separator + stats line)
	inputH := m.input.Height() + 1
	convH := m.height - 2 - inputH - 2
	if convH < 3 {
		convH = 3
	}
	m.conv.Width = m.width
	m.conv.Height = convH
}

// rebuildConvContent re-renders all exchanges into the viewport.
// Scrolls to the bottom only when the user hasn't manually scrolled up.
func (m *Model) rebuildConvContent() {
	m.conv.SetContent(renderConversation(m))
	if !m.userScrolled {
		m.conv.GotoBottom()
	}
}

// scrollToBottom forces the viewport to the bottom and clears the userScrolled flag.
func (m *Model) scrollToBottom() {
	m.userScrolled = false
	m.conv.GotoBottom()
}
