package tui

import (
	"context"
	"os/exec"
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
	paneCmd
)

// exchange holds one complete user+assistant turn, or a standalone note.
type exchange struct {
	userMsg  core.Message
	asstMsg  core.Message // empty while streaming or for notes
	costUSD  float64
	elapsed  time.Duration
	complete bool // false while assistant reply is still streaming
	isNote   bool // true for standalone note entries
	isPasted bool // true when user message was a clipboard paste
	expanded bool // true when pasted content is shown in full (in-memory only)
}

// Bubbletea message types.
type streamDeltaMsg string
type streamDoneMsg struct {
	result engine.ChatResult
	err    error
}
type spinnerTickMsg struct{}
type ttsDoneMsg struct{ err error }

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

	// bottom pane: command output
	lastCmd     *cmdResult
	cmdPaneOpen bool
	cmdScroll   viewport.Model

	// ctrl+c double-press
	lastCtrlC time.Time

	// pending confirmation (e.g. topic-delete, topic-clear)
	pendingAction func() cmdResult
	pendingPost   func(*Model) // model mutation to run after pendingAction, on the real model
	confirmBuf    string

	// input history (bash-style, in-memory only)
	inputHistory []string // oldest first, max 128
	historyIdx   int      // -1 = not browsing
	historySaved string   // draft saved before browsing started

	// paste mode (active when clipboard content exceeds threshold)
	pastedBlob string // full text to send; empty = not in paste mode

	// TTS playback
	ttsCmd   *exec.Cmd // non-nil while TTS is playing
	ttsExIdx int       // exchange being spoken (-1 = none)
	ttsQueue []int     // pending exchange indices for play-all

	// command completion (active when input starts with /)
	completionItems []completionEntry // filtered list
	completionIdx   int               // highlighted row (-1 = none)
}

// cmdResult holds one slash command invocation and its output.
type cmdResult struct {
	input    string
	output   []string
	warnLine string // if non-empty, rendered in red before output lines
	isError  bool
	quit     bool // if true, the app should exit
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
		historyIdx:    -1,
		ttsExIdx:      -1,
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
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

// loadHistory populates exchanges from the engine's current topic history.
func (m *Model) loadHistory() {
	h := m.eng.Topic().History
	for i := 0; i < len(h.Msgs); i++ {
		msg := h.Msgs[i]
		if msg.Role == core.RoleNote {
			m.exchanges = append(m.exchanges, exchange{
				userMsg: msg,
				isNote:  true,
				complete: true,
			})
		} else if msg.Role == core.RoleUser && i+1 < len(h.Msgs) && h.Msgs[i+1].Role == core.RoleAssistant {
			m.exchanges = append(m.exchanges, exchange{
				userMsg:  msg,
				asstMsg:  h.Msgs[i+1],
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

// cmdPaneHeight returns the height of the bottom pane in lines (excluding separator).
// Normal: 1 line (stats). Expanded: capped at 30% of terminal height.
func (m *Model) cmdPaneHeight() int {
	if len(m.completionItems) > 0 {
		return 1 + len(m.completionItems) // header + one row per match
	}
	if !m.cmdPaneOpen || m.lastCmd == nil {
		return 1
	}
	// 1 (header) + len(output lines), capped at 30% of terminal height.
	h := 1 + len(m.lastCmd.output)
	max := m.height * 30 / 100
	if max < 3 {
		max = 3
	}
	if h > max {
		h = max
	}
	return h
}

// syncLayout recalculates the conversation viewport height based on current
// terminal size and textarea height. Call after resize or textarea height change.
func (m *Model) syncLayout() {
	// Layout (each value = number of terminal lines):
	//   top bar:    2 (text + separator)
	//   conv:       convH
	//   input pane: 1 (separator) + textarea.Height()
	//   bottom pane: 1 (separator) + cmdPaneHeight()
	inputH := m.input.Height() + 1
	bottomH := 1 + m.cmdPaneHeight()
	convH := m.height - 2 - inputH - bottomH
	if convH < 3 {
		convH = 3
	}
	m.conv.Width = m.width
	m.conv.Height = convH
	m.cmdScroll.Width = m.width
	m.cmdScroll.Height = m.cmdPaneHeight()
}

// rebuildConvContent re-renders all exchanges into the viewport.
// When paneConv has focus and an exchange is selected, scrolls to show it.
// Otherwise scrolls to the bottom only when the user hasn't manually scrolled up.
func (m *Model) rebuildConvContent() {
	content, offsets := renderConversation(m)
	m.conv.SetContent(content)
	if m.focus == paneConv && m.focusedExIdx >= 0 && m.focusedExIdx < len(offsets) {
		m.conv.SetYOffset(offsets[m.focusedExIdx])
	} else if !m.userScrolled {
		m.conv.GotoBottom()
	}
}

// pushHistory appends val to inputHistory, deduplicating consecutive identical
// entries and capping at 128. Resets historyIdx to -1.
// Entries longer than 64 runes are truncated to 60 + " ...".
func (m *Model) pushHistory(val string) {
	if val == "" {
		return
	}
	entry := val
	if runes := []rune(val); len(runes) > 64 {
		entry = string(runes[:60]) + " ..."
	}
	if len(m.inputHistory) == 0 || m.inputHistory[len(m.inputHistory)-1] != entry {
		m.inputHistory = append(m.inputHistory, entry)
		if len(m.inputHistory) > 128 {
			m.inputHistory = m.inputHistory[len(m.inputHistory)-128:]
		}
	}
	m.historyIdx = -1
	m.historySaved = ""
}

// scrollToBottom forces the viewport to the bottom and clears the userScrolled flag.
func (m *Model) scrollToBottom() {
	m.userScrolled = false
	m.conv.GotoBottom()
}
