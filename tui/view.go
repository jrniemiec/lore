package tui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.vom/jrniemiec/lore/config"
)

var multiBlankRE = regexp.MustCompile(`\n{2,}`)

// compactLines collapses multiple consecutive blank lines into a single newline.
func compactLines(s string) string {
	return multiBlankRE.ReplaceAllString(strings.TrimRight(s, "\n"), "\n")
}

// wordWrap wraps plain text (no ANSI) at the given column width, breaking at
// word boundaries. Each existing newline starts a fresh line.
func wordWrap(s string, width int) string {
	if width <= 0 {
		return s
	}
	var out strings.Builder
	for i, line := range strings.Split(s, "\n") {
		if i > 0 {
			out.WriteByte('\n')
		}
		words := strings.Fields(line)
		if len(words) == 0 {
			continue
		}
		col := 0
		for j, w := range words {
			wlen := len([]rune(w))
			if j == 0 {
				out.WriteString(w)
				col = wlen
			} else if col+1+wlen > width {
				out.WriteByte('\n')
				out.WriteString(w)
				col = wlen
			} else {
				out.WriteByte(' ')
				out.WriteString(w)
				col += 1 + wlen
			}
		}
	}
	return out.String()
}

// View renders the full TUI.
func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	return strings.Join([]string{
		renderTopBar(&m),
		m.conv.View(),
		renderInputPane(&m),
		renderStatusBar(&m),
	}, "\n")
}

// =============================================================================
// Raw ANSI helpers — foreground color only, zero background side-effects.
// lipgloss is only used for the box border (structural, not color).
// =============================================================================

// fg wraps text in a truecolor foreground sequence and resets all attributes
// afterward. The reset (\033[0m) clears any stale background from bubbles.
func fg(col lipgloss.Color, text string) string {
	r, g, b, ok := hexToRGB(string(col))
	if !ok {
		return text
	}
	return fmt.Sprintf("\033[38;2;%d;%d;%dm%s\033[0m", r, g, b, text)
}

func fgBold(col lipgloss.Color, text string) string {
	r, g, b, ok := hexToRGB(string(col))
	if !ok {
		return "\033[1m" + text + "\033[0m"
	}
	return fmt.Sprintf("\033[1;38;2;%d;%d;%dm%s\033[0m", r, g, b, text)
}

func fgFaint(col lipgloss.Color, text string) string {
	r, g, b, ok := hexToRGB(string(col))
	if !ok {
		return "\033[2m" + text + "\033[0m"
	}
	return fmt.Sprintf("\033[2;38;2;%d;%d;%dm%s\033[0m", r, g, b, text)
}

func hexToRGB(hex string) (r, g, b int64, ok bool) {
	if len(hex) != 7 || hex[0] != '#' {
		return 0, 0, 0, false
	}
	r, err := strconv.ParseInt(hex[1:3], 16, 64)
	if err != nil {
		return 0, 0, 0, false
	}
	g, err = strconv.ParseInt(hex[3:5], 16, 64)
	if err != nil {
		return 0, 0, 0, false
	}
	b, err = strconv.ParseInt(hex[5:7], 16, 64)
	if err != nil {
		return 0, 0, 0, false
	}
	return r, g, b, true
}

// visibleWidth returns the display width of a string ignoring ANSI escape codes.
func visibleWidth(s string) int {
	return lipgloss.Width(s)
}

// fgLines applies fg() to each \n-separated line individually so every line
// carries its own complete ANSI open/close sequence. This prevents viewport
// line-splitting and lipgloss re-wrapping from breaking multi-line colored text.
func fgLines(col lipgloss.Color, text string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = fg(col, l)
	}
	return strings.Join(lines, "\n")
}

// =============================================================================
// Top bar
// =============================================================================

func renderTopBar(m *Model) string {
	t := ActiveTheme

	const pad = 1

	left := fgBold(t.TopBarText, "lore")
	barSep := fg(t.Dimmed, " │ ")

	// Center: topic · [context capacity ·] model
	pct := m.contextFillPct()
	var ctxPart string
	if pct > 0 {
		var fillStr string
		if pct >= 90 {
			fillStr = fgBold(t.ContextWarning, fmt.Sprintf("%d%%", pct))
		} else {
			fillStr = fg(t.TopBarText, fmt.Sprintf("%d%%", pct))
		}
		ctxPart = fg(t.TopBarText, " · context: ") + fillStr
	}
	center := fg(t.TopBarText, "topic: "+m.eng.TopicName()) +
		ctxPart +
		fg(t.TopBarText, " · model: "+m.eng.Profile().Model)

	// Right: scroll position indicator when not at the bottom.
	var right string
	if m.userScrolled {
		total := m.conv.TotalLineCount()
		if total > 0 {
			scrollPct := (m.conv.YOffset * 100) / total
			var arrow string
			switch {
			case scrollPct < 40:
				arrow = "↓"
			case scrollPct >= 60:
				arrow = "↑"
			default:
				arrow = "↕"
			}
			right = fg(t.InputPrompt, fmt.Sprintf("%s %d%%", arrow, scrollPct))
		}
	}

	leftCenter := strings.Repeat(" ", pad) + left + barSep + center
	gap := m.width - visibleWidth(leftCenter) - visibleWidth(right) - pad
	if gap < 1 {
		gap = 1
	}
	bar := leftCenter + strings.Repeat(" ", gap) + right + strings.Repeat(" ", pad)
	return bar + "\n" + fg(t.Dimmed, strings.Repeat("─", m.width))
}

// =============================================================================
// Conversation pane
// =============================================================================

// renderConversation builds the full conversation string written into the viewport.
func renderConversation(m *Model) string {
	t := ActiveTheme

	// Box style is the one place we use lipgloss — structural border only,
	// no background color set.
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BoxBorder).
		Width(m.width - 4)

	var sb strings.Builder
	for i, ex := range m.exchanges {
		focused := m.focus == paneConv && m.focusedExIdx == i

		// Layout: col 1 = bullet (●), col 2 = space, col 3+ = text.
		// Both user and assistant text start at column 3 (2-char prefix).
		const prefixW = 2
		wrapWidth := m.width - prefixW

		// addPrefix prepends a 2-char prefix to each line.
		// firstPrefix is used on line 0, restPrefix on continuation lines.
		addPrefix := func(s, firstPrefix, restPrefix string) string {
			lines := strings.Split(s, "\n")
			for i, l := range lines {
				if i == 0 {
					lines[i] = firstPrefix + l
				} else {
					lines[i] = restPrefix + l
				}
			}
			return strings.Join(lines, "\n")
		}

		var turnContent string
		if ex.isNote {
			noteRaw := wordWrap(compactLines(ex.userMsg.Content), wrapWidth)
			noteColor := lipgloss.Color("#6B7A8D") // muted blue-grey
			turnContent = addPrefix(fgLines(noteColor, noteRaw),
				fg(noteColor, "📌 "),
				"  ")
		} else {
			userRaw := wordWrap(compactLines(ex.userMsg.Content), wrapWidth)
			userContent := addPrefix(fgLines(t.UserText, userRaw),
				fg(t.AssistantText, "● "),
				"  ")

			var asstRaw string
			if ex.complete {
				asstRaw = wordWrap(compactLines(ex.asstMsg.Content), wrapWidth)
			} else {
				asstRaw = wordWrap(compactLines(m.streamBuf), wrapWidth)
			}
			asstContent := addPrefix(fgLines(t.AssistantText, asstRaw), "  ", "  ")
			turnContent = userContent + "\n" + asstContent
		}

		if focused {
			tsUser := fg(t.Dimmed, ex.userMsg.Time.Format("15:04"))
			tsAsst := ""
			if ex.complete {
				tsAsst = " → " + fg(t.Dimmed, ex.asstMsg.Time.Format("15:04"))
			}
			header := tsUser + tsAsst
			turnContent = boxStyle.Render(header + "\n" + turnContent)
		}

		sb.WriteString(turnContent)
		if i < len(m.exchanges)-1 {
			sb.WriteString("\n\n")
		}
	}

	return sb.String()
}

func renderSpinner(m *Model) string {
	t := ActiveTheme
	if m.spinnerFrame%2 == 0 {
		return fgBold(t.Spinner, "❄")
	}
	return fgFaint(t.Spinner, "❄")
}

// =============================================================================
// Input pane
// =============================================================================

// renderInputPane renders the prompt + text entirely with raw ANSI — never
// calling m.input.View(). This guarantees no background color sequences are
// emitted (termenv always wraps with \033[m...\033[0m which resets the
// background, potentially differing from the terminal's configured background).
//
// The textarea model is kept for editing state and cursor position only.
func renderInputPane(m *Model) string {
	t := ActiveTheme
	sep := fg(t.Dimmed, strings.Repeat("─", m.width))

	prompt := m.inputPrompt()
	promptRunes := []rune(prompt)
	const padW = 1
	line0W := m.width - padW - len(promptRunes)
	contW := m.width - padW
	if line0W < 1 {
		line0W = 1
	}
	if contW < 1 {
		contW = 1
	}

	curLogLine := m.input.Line()
	curLogCol := m.input.LineInfo().ColumnOffset

	logicalLines := strings.Split(m.input.Value(), "\n")
	if len(logicalLines) == 0 {
		logicalLines = []string{""}
	}

	var rendered []string
	firstVisualLine := true

	for li, line := range logicalLines {
		runes := []rune(line)
		wW := contW
		if li == 0 {
			wW = line0W
		}

		// Split logical line into visual chunks.
		type chunk struct {
			runes      []rune
			logStart   int // column offset within logical line
		}
		var chunks []chunk
		if len(runes) == 0 {
			chunks = []chunk{{runes: []rune{}, logStart: 0}}
		} else {
			for start := 0; start < len(runes); start += wW {
				end := start + wW
				if end > len(runes) {
					end = len(runes)
				}
				chunks = append(chunks, chunk{runes: runes[start:end], logStart: start})
			}
		}

		for ci, ch := range chunks {
			// Build prefix.
			var prefix string
			if firstVisualLine {
				prefix = strings.Repeat(" ", padW) + fg(t.InputPrompt, prompt)
				firstVisualLine = false
			} else {
				prefix = strings.Repeat(" ", padW)
			}

			// Is cursor in this chunk?
			if li == curLogLine && m.focus == paneInput {
				chunkEnd := ch.logStart + len(ch.runes)
				isLast := ci == len(chunks)-1
				if curLogCol >= ch.logStart && (curLogCol < chunkEnd || isLast) {
					colInChunk := curLogCol - ch.logStart
					if colInChunk > len(ch.runes) {
						colInChunk = len(ch.runes)
					}
					before := string(ch.runes[:colInChunk])
					var curChar, after string
					if colInChunk < len(ch.runes) {
						curChar = string(ch.runes[colInChunk])
						after = string(ch.runes[colInChunk+1:])
					} else {
						curChar = " "
					}
					var cursorSeq string
					if m.cursorVisible {
						cursorSeq = "\033[4m" + curChar + "\033[24m"
					} else {
						cursorSeq = fg(t.InputText, curChar)
						if curChar == " " {
							cursorSeq = " "
						}
					}
					rendered = append(rendered,
						prefix+fg(t.InputText, before)+cursorSeq+fg(t.InputText, after))
					continue
				}
			}
			rendered = append(rendered, prefix+fg(t.InputText, string(ch.runes)))
		}
	}

	return sep + "\n" + strings.Join(rendered, "\n")
}

// =============================================================================
// Status bar
// =============================================================================

// renderCmdOutput builds the scrollable content for the command pane.
func renderCmdOutput(m *Model) string {
	if m.lastCmd == nil {
		return ""
	}
	t := ActiveTheme
	r := m.lastCmd
	var dot string
	if r.isError {
		dot = fg("#FF6B6B", "●")
	} else {
		dot = fg("#A3BE8C", "●")
	}
	header := dot + " " + fg(t.Dimmed, r.input)
	var sb strings.Builder
	sb.WriteString(header)
	for _, line := range r.output {
		sb.WriteByte('\n')
		if r.isError {
			sb.WriteString("  " + fg("#FF6B6B", line))
		} else {
			sb.WriteString("  " + fg(t.AssistantText, line))
		}
	}
	// Inline cursor after the last output line when a destructive action is pending.
	if m.pendingAction != nil {
		sb.WriteString(" " + fg(t.InputText, m.confirmBuf) + "\033[4m \033[24m")
	}
	// Half-line padding at the bottom.
	sb.WriteString("\n")
	return sb.String()
}

func renderCompletionPane(m *Model) string {
	t := ActiveTheme
	var sb strings.Builder
	// Find longest command for alignment.
	maxCmd := 0
	for _, e := range m.completionItems {
		if len(e.cmd) > maxCmd {
			maxCmd = len(e.cmd)
		}
	}
	for i, e := range m.completionItems {
		var line string
		cmdPart := fmt.Sprintf(" %-*s  ", maxCmd, e.cmd)
		if i == m.completionIdx {
			line = "\033[7m" + cmdPart + e.desc + "\033[27m"
		} else {
			line = fg(t.InputText, cmdPart) + fg(t.Dimmed, e.desc)
		}
		sb.WriteByte('\n')
		sb.WriteString(line)
	}
	return sb.String()
}

func renderBottomPane(m *Model) string {
	t := ActiveTheme
	sep := fg(t.Dimmed, strings.Repeat("─", m.width))
	if len(m.completionItems) > 0 {
		return sep + renderCompletionPane(m)
	}
	if m.cmdPaneOpen && m.lastCmd != nil {
		return sep + "\n" + m.cmdScroll.View()
	}
	return renderStatsLine(m, sep)
}

func renderStatusBar(m *Model) string {
	return renderBottomPane(m)
}

func renderStatsLine(m *Model, sep string) string {
	t := ActiveTheme
	const pad = 1

	// Left: spinner + "streaming" while in flight, empty otherwise.
	// Both rendered in purple (StreamingText).
	var left string
	if m.streaming {
		if m.spinnerFrame%2 == 0 {
			left = fgBold(t.StreamingText, "❄") + " " + fgBold(t.StreamingText, "streaming ...")
		} else {
			left = fgFaint(t.StreamingText, "❄") + " " + fgFaint(t.StreamingText, "streaming ...")
		}
	}

	// Center: per-request stats — shown permanently after first response.
	var center string
	if m.lastResult != nil {
		r := m.lastResult
		s := fmt.Sprintf("%d in · %d out · %dms", r.Usage.InputTokens, r.Usage.OutputTokens, r.Elapsed.Milliseconds())
		inPer1M, outPer1M, hasPricing := config.ExtractPricing(m.eng.Profile().Info)
		if hasPricing {
			cost := config.CalcCost(r.Usage.InputTokens, r.Usage.OutputTokens, inPer1M, outPer1M)
			s += " · " + config.FormatCost(cost)
		}
		center = fg(t.Dimmed, s)
	}

	// Right: topic + global cumulative stats.
	var right string
	if m.topicStats.Calls > 0 {
		right = fg(t.Dimmed, fmt.Sprintf("topic: %d · %s",
			m.topicStats.Calls, config.FormatCost(m.topicStats.CostUSD)))
		right += fg(t.Dimmed, fmt.Sprintf("  total: %d · %s",
			m.sessionStats.Calls, config.FormatCost(m.sessionStats.CostUSD)))
	}

	leftW := visibleWidth(left)
	centerW := visibleWidth(center)
	rightW := visibleWidth(right)

	// Position center in the middle of the bar.
	// left-pad so center lands at (width-centerW)/2.
	centerStart := (m.width - centerW) / 2
	leftEnd := pad + leftW
	gapLC := centerStart - leftEnd
	if gapLC < 1 {
		gapLC = 1
	}
	// right-pad fills remaining space.
	centerEnd := centerStart + centerW
	gapCR := m.width - pad - rightW - centerEnd
	if gapCR < 1 {
		gapCR = 1
	}

	statsLine := strings.Repeat(" ", pad) +
		left +
		strings.Repeat(" ", gapLC) +
		center +
		strings.Repeat(" ", gapCR) +
		right +
		strings.Repeat(" ", pad)
	return sep + "\n" + statsLine
}
