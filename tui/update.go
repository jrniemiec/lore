package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.vom/jrniemiec/lore/config"
	"github.vom/jrniemiec/lore/core"
	"github.vom/jrniemiec/lore/engine"
)

// Update handles all incoming messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	// --- window resize ---
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.syncInputPrompt()
		m.syncLayout()
		m.rebuildConvContent()

	// --- spinner tick ---
	case spinnerTickMsg:
		m.spinnerFrame++
		m.cursorVisible = !m.cursorVisible
		if m.streaming {
			m.rebuildConvContent()
		}
		cmds = append(cmds, spinnerTick())

	// --- streaming token ---
	case streamDeltaMsg:
		m.streamBuf += string(msg)
		// Update the last exchange's in-progress reply
		if len(m.exchanges) > 0 {
			last := &m.exchanges[len(m.exchanges)-1]
			if !last.complete {
				last.asstMsg.Content = m.streamBuf
			}
		}
		m.rebuildConvContent()

	// --- streaming done ---
	case streamDoneMsg:
		m.streaming = false
		m.cancelStream = nil
		if msg.err != nil {
			// Show error as a synthetic assistant message
			if len(m.exchanges) > 0 {
				last := &m.exchanges[len(m.exchanges)-1]
				last.asstMsg.Content = fmt.Sprintf("[error: %v]", msg.err)
				last.complete = true
			}
		} else {
			if len(m.exchanges) > 0 {
				last := &m.exchanges[len(m.exchanges)-1]
				last.complete = true
				last.elapsed = msg.result.Elapsed
				last.costUSD = calcExchangeCost(msg.result, m.eng)
			}
			result := msg.result
			m.lastResult = &result
			m.topicStats.Calls++
			m.topicStats.InputTokens += msg.result.Usage.InputTokens
			m.topicStats.OutputTokens += msg.result.Usage.OutputTokens
			m.topicStats.CostUSD += calcExchangeCost(msg.result, m.eng)
			m.sessionStats.Calls++
			m.sessionStats.InputTokens += msg.result.Usage.InputTokens
			m.sessionStats.OutputTokens += msg.result.Usage.OutputTokens
			m.sessionStats.CostUSD += m.topicStats.CostUSD
		}
		m.streamBuf = ""
		m.rebuildConvContent()
		m.input.Focus()

	// --- mouse ---
	case tea.MouseMsg:
		// Scroll wheel only — clicks are ignored so the terminal can handle
		// text selection (hold Shift to select in most terminals).
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.userScrolled = true
			m.conv.ScrollUp(3)
		case tea.MouseButtonWheelDown:
			m.conv.ScrollDown(3)
			if m.conv.AtBottom() {
				m.userScrolled = false
			}
		}

	// --- keyboard ---
	case tea.KeyMsg:
		switch {

		// Ctrl+C: cancel stream or quit
		case key.Matches(msg, keys.Cancel):
			if m.streaming && m.cancelStream != nil {
				m.cancelStream()
				return m, nil
			}
			now := time.Now()
			if now.Sub(m.lastCtrlC) < 500*time.Millisecond {
				return m, tea.Quit
			}
			m.lastCtrlC = now
			return m, nil

		// Esc: return focus to input
		case key.Matches(msg, keys.Dismiss):
			m.focus = paneInput
			m.focusedExIdx = -1
			m.input.Focus()
			m.rebuildConvContent()

		// Enter: send (input pane) or dismiss (conv pane)
		case key.Matches(msg, keys.Send):
			if m.focus == paneConv {
				m.focus = paneInput
				m.focusedExIdx = -1
				m.input.Focus()
				m.rebuildConvContent()
			} else if !m.streaming {
				if strings.TrimSpace(m.input.Value()) == "" {
					// Empty input: just jump to the bottom of the conversation.
					m.scrollToBottom()
				} else {
					cmds = append(cmds, m.sendMessage())
				}
			}

		// Shift+Enter: newline in input
		case key.Matches(msg, keys.Newline):
			if m.focus == paneInput {
				m.input.InsertString("\n")
			}

		// Arrow up/down: navigate exchanges in conv pane; scroll viewport in input pane
		case key.Matches(msg, keys.NavUp):
			if m.focus == paneConv {
				if m.focusedExIdx < 0 {
					m.focusedExIdx = len(m.exchanges) - 1
				} else if m.focusedExIdx > 0 {
					m.focusedExIdx--
				}
				m.rebuildConvContent()
			} else {
				m.userScrolled = true
				m.conv.ScrollUp(3)
			}

		case key.Matches(msg, keys.NavDown):
			if m.focus == paneConv {
				if m.focusedExIdx >= 0 && m.focusedExIdx < len(m.exchanges)-1 {
					m.focusedExIdx++
				}
				m.rebuildConvContent()
			} else {
				m.conv.ScrollDown(3)
				if m.conv.AtBottom() {
					m.userScrolled = false
				}
			}

		// Page Up/Down: scroll the conversation viewport
		case key.Matches(msg, keys.ScrollUp):
			m.userScrolled = true
			m.conv.HalfPageUp()

		case key.Matches(msg, keys.ScrollDown):
			m.conv.HalfPageDown()
			if m.conv.AtBottom() {
				m.userScrolled = false
			}

		// Ctrl+L: clear screen
		case key.Matches(msg, keys.ClearScreen):
			return m, tea.ClearScreen

		// Ctrl+T / Ctrl+P: topic/profile switching (stub — popup in future)
		case key.Matches(msg, keys.SwitchTopic):
			// TODO: topic picker popup
		case key.Matches(msg, keys.SwitchProfile):
			// TODO: profile picker popup

		default:
			if m.focus == paneInput && !m.streaming {
				m.cursorVisible = true // reset blink phase on keystroke
				var cmd tea.Cmd
				m.input, cmd = m.input.Update(msg)
				// Grow/shrink textarea height with visual (wrapped) line count.
				visualH := m.inputVisualHeight()
				if visualH != m.input.Height() {
					m.input.SetHeight(visualH)
					m.syncLayout()
				}
				cmds = append(cmds, cmd)
			}
		}
	}

	return m, tea.Batch(cmds...)
}

// sendMessage takes the current input, sends it to the engine, and returns a Cmd.
func (m *Model) sendMessage() tea.Cmd {
	prompt := strings.TrimSpace(m.input.Value())
	if prompt == "" {
		return nil
	}
	m.input.Reset()
	m.input.SetHeight(1)
	m.input.Blur()

	m.exchanges = append(m.exchanges, exchange{
		userMsg: core.Message{
			Role:    core.RoleUser,
			Content: prompt,
		},
		complete: false,
	})
	m.streaming = true
	m.streamBuf = ""
	m.userScrolled = false
	m.rebuildConvContent()

	ctx, cancel := context.WithCancel(context.Background())
	m.cancelStream = cancel

	eng := m.eng
	// Bubbletea Cmds run in a goroutine and return exactly one tea.Msg.
	// For streaming we use tea.Sequence: each delta token is sent as a
	// streamDeltaMsg, and the final result as streamDoneMsg. We implement
	// this by sending deltas via a channel that is drained by a Cmd chain,
	// but the simplest correct approach with Bubbletea is to use
	// tea.Program.Send from the goroutine. We access it via a closure
	// populated by Start().
	return func() tea.Msg {
		opts := engine.ChatOptions{}
		result, err := eng.Chat(ctx, prompt, opts, func(delta string) error {
			if programSend != nil {
				programSend(streamDeltaMsg(delta))
			}
			return nil
		})
		return streamDoneMsg{result: result, err: err}
	}
}

func calcExchangeCost(r engine.ChatResult, eng *engine.Engine) float64 {
	inPer1M, outPer1M, ok := config.ExtractPricing(eng.Profile().Info)
	if !ok {
		return 0
	}
	return config.CalcCost(r.Usage.InputTokens, r.Usage.OutputTokens, inPer1M, outPer1M)
}
