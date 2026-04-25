package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.vom/jrniemiec/lore/config"
	"github.vom/jrniemiec/lore/core"
	"github.vom/jrniemiec/lore/store"
)

// knownCommands is the set of command names (without leading /) for bare-word recognition.
var knownCommands = map[string]bool{
	"exit": true, "help": true,
	"topic": true, "topic-switch": true, "topic-new": true, "topic-list": true,
	"topic-delete": true, "topic-clear": true, "topic-default": true,
	"topic-default-set": true, "topic-summary": true, "topic-history": true,
	"topic-resource": true,
	"profile": true, "profile-switch": true, "profile-list": true,
	"profile-default": true, "profile-default-set": true,
	"system": true, "system-set": true, "system-clear": true,
	"config": true, "status": true, "stats": true,
}

// looksLikeCommand returns true if the input (no leading /) has ≤ 2 words and
// the first word matches a known command name.
func looksLikeCommand(val string) bool {
	fields := strings.Fields(val)
	if len(fields) == 0 || len(fields) > 2 {
		return false
	}
	return knownCommands[strings.ToLower(fields[0])]
}

// handleCommand parses and executes a slash command, returning the result for
// display in the bottom pane. The input string includes the leading '/'.
func handleCommand(m *Model, input string) cmdResult {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return errResult(input, "empty command")
	}
	cmd := strings.ToLower(fields[0])
	args := fields[1:]

	switch cmd {
	// --- topic ---
	case "/topic":
		return cmdTopicInfo(m, args)
	case "/topic-switch":
		return cmdTopicSwitch(m, args)
	case "/topic-new":
		return cmdTopicNew(m, args)
	case "/topic-list":
		return cmdTopicList(m)
	case "/topic-delete":
		return cmdTopicDelete(m, args)
	case "/topic-clear":
		return cmdTopicClear(m, args)
	case "/topic-default":
		return cmdTopicDefault(m)
	case "/topic-default-set":
		return cmdTopicDefaultSet(m, args)
	case "/topic-summary":
		return cmdTopicSummary(m)
	case "/topic-history":
		return cmdTopicHistory(m, args)
	case "/topic-resource":
		return cmdTopicResource(m, args)

	// --- profile ---
	case "/profile":
		return cmdProfileInfo(m, args)
	case "/profile-switch":
		return cmdProfileSwitch(m, args)
	case "/profile-list":
		return cmdProfileList(m, args)
	case "/profile-default":
		return cmdProfileDefault(m)
	case "/profile-default-set":
		return cmdProfileDefaultSet(m, args)

	// --- system ---
	case "/system":
		return cmdSystem(m, args)
	case "/system-set":
		return cmdSystemSet(m, args)
	case "/system-clear":
		return cmdSystemClear(m)

	// --- info ---
	case "/config":
		return cmdConfig(m)
	case "/status":
		return cmdStatus(m)
	case "/stats":
		return cmdStats(m)

	// --- help ---
	case "/help":
		return cmdHelp("/help", args)

	// --- exit ---
	case "/exit":
		return cmdResult{input: input, output: nil, isError: false, quit: true}

	default:
		return errResult(input, fmt.Sprintf("unknown command %q — type /help for a list", cmd))
	}
}

// =============================================================================
// topic commands
// =============================================================================

func cmdTopicInfo(m *Model, args []string) cmdResult {
	name := m.eng.TopicName()
	if len(args) > 0 {
		name = args[0]
	}
	h, err := m.eng.Topic().History, error(nil)
	if len(args) > 0 {
		// Load history for a different topic.
		st := store.New(m.cfg.TopicsRoot)
		loaded, e := st.LoadHistory(name)
		if e != nil {
			return errResult("/topic "+name, fmt.Sprintf("load topic: %v", e))
		}
		h = loaded
		err = e
	}
	_ = err
	sys := ""
	if len(args) == 0 {
		sys = m.eng.SystemPrompt()
	}
	lines := []string{
		fmt.Sprintf("topic:   %s", name),
		fmt.Sprintf("system:  %s", yesNoStr(sys != "")),
		fmt.Sprintf("history: %d messages (~%s tokens)", len(h.Msgs),
			core.FormatTokens(totalTokens(h))),
	}
	if len(args) == 0 {
		sumText, through, _ := m.eng.LoadSummary()
		if sumText != "" {
			lines = append(lines, fmt.Sprintf("summary: covers messages 1-%d", through+1))
		} else {
			lines = append(lines, "summary: (none)")
		}
	}
	return okResult("/topic "+name, lines)
}

func cmdTopicSwitch(m *Model, args []string) cmdResult {
	if len(args) == 0 {
		return errResult("/topic-switch", "usage: /topic-switch <name>")
	}
	name := args[0]
	if err := m.eng.SwitchTopic(name); err != nil {
		return errResult("/topic-switch "+name, err.Error())
	}
	m.exchanges = nil
	m.loadHistory()
	m.rebuildConvContent()
	return okResult("/topic-switch "+name, []string{fmt.Sprintf("switched to topic %q", name)})
}

func cmdTopicNew(m *Model, args []string) cmdResult {
	if len(args) == 0 {
		return errResult("/topic-new", "usage: /topic-new <name>")
	}
	name := args[0]
	if err := m.eng.CreateTopic(name, ""); err != nil {
		return errResult("/topic-new "+name, err.Error())
	}
	return okResult("/topic-new "+name, []string{fmt.Sprintf("topic %q created", name)})
}

func cmdTopicList(m *Model) cmdResult {
	topics, err := m.eng.ListTopics()
	if err != nil {
		return errResult("/topic-list", err.Error())
	}
	if len(topics) == 0 {
		return okResult("/topic-list", []string{"(no topics)"})
	}
	cur := m.eng.TopicName()
	lines := make([]string, len(topics))
	for i, t := range topics {
		if t == cur {
			lines[i] = t + " ←"
		} else {
			lines[i] = t
		}
	}
	return okResult("/topic-list", lines)
}

func cmdTopicDelete(m *Model, args []string) cmdResult {
	name := m.eng.TopicName()
	if len(args) > 0 {
		name = args[0]
	}
	// Verify topic exists before asking for confirmation.
	topics, err := m.eng.ListTopics()
	if err != nil {
		return errResult("/topic-delete "+name, err.Error())
	}
	found := false
	for _, t := range topics {
		if t == name {
			found = true
			break
		}
	}
	if !found {
		return errResult("/topic-delete "+name, fmt.Sprintf("topic %q not found", name))
	}
	// Register pending confirmation — executed on "yes" + Enter, cancelled by any other input.
	label := "/topic-delete " + name
	m.pendingAction = func() cmdResult {
		if err := m.eng.DeleteTopic(name); err != nil {
			return errResult(label, err.Error())
		}
		if name == m.eng.TopicName() {
			defaultName := config.EffectiveTopic(m.cfg, "")
			_ = m.eng.SwitchTopic(defaultName)
			m.exchanges = nil
			m.loadHistory()
			m.rebuildConvContent()
		}
		return okResult(label, []string{fmt.Sprintf("topic %q deleted", name)})
	}
	return okResult(label, []string{
		fmt.Sprintf("Topic %q and all its history will be permanently deleted.", name),
		"[yes] to confirm, other input or Esc to cancel:",
	})
}

func cmdTopicClear(m *Model, args []string) cmdResult {
	name := m.eng.TopicName()
	m.pendingAction = func() cmdResult {
		if err := m.eng.ClearHistory(); err != nil {
			return errResult("/topic-clear", err.Error())
		}
		m.exchanges = nil
		m.rebuildConvContent()
		return okResult("/topic-clear", []string{fmt.Sprintf("history cleared for topic %q", name)})
	}
	return okResult("/topic-clear", []string{
		fmt.Sprintf("All history for topic %q will be permanently deleted.", name),
		"[yes] to confirm, other input or Esc to cancel:",
	})
}

func cmdTopicDefault(m *Model) cmdResult {
	def := m.cfg.DefaultTopic
	if def == "" {
		def = "(not set)"
	}
	return okResult("/topic-default", []string{"default topic: " + def})
}

func cmdTopicDefaultSet(m *Model, args []string) cmdResult {
	if len(args) == 0 {
		return errResult("/topic-default-set", "usage: /topic-default-set <name>")
	}
	name := args[0]
	if err := m.eng.SetDefaultTopic(name); err != nil {
		return errResult("/topic-default-set "+name, err.Error())
	}
	m.cfg = m.eng.Config()
	return okResult("/topic-default-set "+name, []string{fmt.Sprintf("default topic set to %q", name)})
}

func cmdTopicSummary(m *Model) cmdResult {
	text, through, err := m.eng.LoadSummary()
	if err != nil {
		return errResult("/topic-summary", err.Error())
	}
	if text == "" {
		return okResult("/topic-summary", []string{"(no summary)"})
	}
	lines := []string{fmt.Sprintf("(covers through message %d)", through+1)}
	lines = append(lines, strings.Split(strings.TrimRight(text, "\n"), "\n")...)
	return okResult("/topic-summary", lines)
}

func cmdTopicHistory(m *Model, args []string) cmdResult {
	n := 10
	if len(args) > 0 {
		if v, err := strconv.Atoi(args[0]); err == nil && v > 0 {
			n = v
		}
	}
	h := m.eng.Topic().History
	type pair struct{ user, asst core.Message }
	var pairs []pair
	for i := 0; i+1 < len(h.Msgs); i++ {
		if h.Msgs[i].Role == core.RoleUser && h.Msgs[i+1].Role == core.RoleAssistant {
			pairs = append(pairs, pair{h.Msgs[i], h.Msgs[i+1]})
			i++
		}
	}
	if len(pairs) == 0 {
		return okResult("/topic-history", []string{"(no history)"})
	}
	if n > 0 && len(pairs) > n {
		pairs = pairs[len(pairs)-n:]
	}
	var lines []string
	for i, p := range pairs {
		if i > 0 {
			lines = append(lines, "---")
		}
		lines = append(lines, fmt.Sprintf("you · %s", p.user.Time.Format("15:04")))
		lines = append(lines, p.user.Content)
		lines = append(lines, fmt.Sprintf("lore · %s", p.asst.Time.Format("15:04")))
		lines = append(lines, p.asst.Content)
	}
	return okResult("/topic-history", lines)
}

func cmdTopicResource(m *Model, args []string) cmdResult {
	if len(args) == 0 {
		return errResult("/topic-resource", "usage: /topic-resource <file>")
	}
	path := args[0]
	if err := m.eng.AddResource(path); err != nil {
		return errResult("/topic-resource "+path, err.Error())
	}
	return okResult("/topic-resource "+path, []string{fmt.Sprintf("resource %q added", path)})
}

// =============================================================================
// profile commands
// =============================================================================

func cmdProfileInfo(m *Model, args []string) cmdResult {
	code := m.eng.ProfileCode()
	if len(args) > 0 {
		code = args[0]
	}
	p, ok := m.cfg.Profiles[code]
	if !ok {
		return errResult("/profile "+code, fmt.Sprintf("profile %q not found", code))
	}

	row := func(label, value string) string {
		return fmt.Sprintf("  %-12s%s", label+":", value)
	}

	var lines []string
	lines = append(lines, row("profile", code))
	lines = append(lines, row("provider", p.Provider))
	if p.Host != "" {
		lines = append(lines, row("host", p.Host))
	}
	lines = append(lines, row("model", p.Model))
	if p.MaxContextTokens > 0 {
		ctx := fmt.Sprintf("%d tokens", p.MaxContextTokens)
		if p.ContextTokenLimit > 0 {
			ctx += fmt.Sprintf("  (limit: %d)", p.ContextTokenLimit)
		}
		lines = append(lines, row("context", ctx))
	}
	if p.MaxUserMessages > 0 {
		lines = append(lines, row("messages", fmt.Sprintf("%d max", p.MaxUserMessages)))
	}
	if p.MaxOutputTokens > 0 {
		lines = append(lines, row("max output", fmt.Sprintf("%d tokens", p.MaxOutputTokens)))
	}
	if p.Strategy != "" {
		strat := p.Strategy
		if p.SummarizerProfile != "" {
			strat += fmt.Sprintf("  (via %s)", p.SummarizerProfile)
		}
		lines = append(lines, row("strategy", strat))
	}
	if inPer1M, outPer1M, ok := config.ExtractPricing(p.Info); ok {
		lines = append(lines, row("pricing", fmt.Sprintf("$%.2f / $%.2f per 1M tokens  (in / out)", inPer1M, outPer1M)))
	}

	return okResult("/profile "+code, lines)
}

func cmdProfileSwitch(m *Model, args []string) cmdResult {
	if len(args) == 0 {
		return errResult("/profile-switch", "usage: /profile-switch <name>")
	}
	name := args[0]
	if err := m.eng.SwitchProfile(name); err != nil {
		return errResult("/profile-switch "+name, err.Error())
	}
	m.cfg = m.eng.Config()
	return okResult("/profile-switch "+name, []string{fmt.Sprintf("switched to profile %q", name)})
}

func cmdProfileList(m *Model, args []string) cmdResult {
	if len(m.cfg.Profiles) == 0 {
		return okResult("/profile-list", []string{"(no profiles configured)"})
	}
	cur := m.eng.ProfileCode()

	// If specific names requested, validate them first.
	if len(args) > 0 {
		for _, name := range args {
			if _, ok := m.cfg.Profiles[name]; !ok {
				return errResult("/profile-list", fmt.Sprintf("profile %q not found", name))
			}
		}
	}

	// Build name list: args order when filtered, alphabetical otherwise.
	var names []string
	if len(args) > 0 {
		names = make([]string, len(args))
		for i, a := range args {
			names[len(args)-1-i] = a
		}
	} else {
		names = make([]string, 0, len(m.cfg.Profiles))
		for code := range m.cfg.Profiles {
			names = append(names, code)
		}
		sort.Strings(names)
	}
	sort.Strings(names)

	ctxStr := func(n int) string {
		if n == 0 {
			return ""
		}
		if n >= 1000000 {
			if n%1000000 == 0 {
				return fmt.Sprintf("%dM", n/1000000)
			}
			return fmt.Sprintf("%.1fM", float64(n)/1000000)
		}
		if n >= 1000 {
			return fmt.Sprintf("%dk", n/1000)
		}
		return fmt.Sprintf("%d", n)
	}

	type profileRow struct {
		name, provider, model, msgs, ctx, limit, strategy, summarizer, pricing string
		current                                                                 bool
	}

	rows := make([]profileRow, len(names))
	for i, code := range names {
		p := m.cfg.Profiles[code]
		r := profileRow{
			name:     code,
			provider: p.Provider,
			model:    p.Model,
			ctx:      ctxStr(p.MaxContextTokens),
			current:  code == cur,
		}
		if p.MaxUserMessages > 0 {
			r.msgs = fmt.Sprintf("%d", p.MaxUserMessages)
		}
		if p.ContextTokenLimit > 0 {
			r.limit = ctxStr(p.ContextTokenLimit)
		}
		if p.Strategy != "" {
			r.strategy = p.Strategy
		}
		if p.SummarizerProfile != "" {
			r.summarizer = p.SummarizerProfile
		}
		if inPer1M, outPer1M, ok := config.ExtractPricing(p.Info); ok {
			r.pricing = fmt.Sprintf("$%.2f/$%.2f", inPer1M, outPer1M)
		}
		rows[i] = r
	}

	// Compute column widths from header + data.
	headers := [9]string{"name", "provider", "model", "msgs", "context", "limit", "strategy", "summarizer", "pricing"}
	widths := [9]int{}
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, r := range rows {
		vals := [9]string{r.name, r.provider, r.model, r.msgs, r.ctx, r.limit, r.strategy, r.summarizer, r.pricing}
		for i, v := range vals {
			if len(v) > widths[i] {
				widths[i] = len(v)
			}
		}
	}

	fmtRow := func(vals [9]string) string {
		var sb strings.Builder
		sb.WriteString("  ")
		for i, v := range vals {
			if i == len(vals)-1 {
				sb.WriteString(v)
			} else {
				sb.WriteString(fmt.Sprintf("%-*s  ", widths[i], v))
			}
		}
		return strings.TrimRight(sb.String(), " ")
	}

	sepW := 0
	for i, w := range widths {
		sepW += w
		if i < len(widths)-1 {
			sepW += 2
		}
	}
	sep := "  " + strings.Repeat("─", sepW)

	lines := []string{fmtRow(headers), sep}
	for _, r := range rows {
		line := fmtRow([9]string{r.name, r.provider, r.model, r.msgs, r.ctx, r.limit, r.strategy, r.summarizer, r.pricing})
		if r.current {
			line += "  ←"
		}
		lines = append(lines, line)
	}
	return okResult("/profile-list", lines)
}

func cmdProfileDefault(m *Model) cmdResult {
	def := m.cfg.DefaultProfile
	if def == "" {
		def = "(not set)"
	}
	return okResult("/profile-default", []string{"default profile: " + def})
}

func cmdProfileDefaultSet(m *Model, args []string) cmdResult {
	if len(args) == 0 {
		return errResult("/profile-default-set", "usage: /profile-default-set <name>")
	}
	name := args[0]
	if err := m.eng.SetDefaultProfile(name); err != nil {
		return errResult("/profile-default-set "+name, err.Error())
	}
	m.cfg = m.eng.Config()
	return okResult("/profile-default-set "+name, []string{fmt.Sprintf("default profile set to %q", name)})
}

// =============================================================================
// system commands
// =============================================================================

func cmdSystem(m *Model, args []string) cmdResult {
	text := m.eng.SystemPrompt()
	if text == "" {
		return okResult("/system", []string{"(no system prompt)"})
	}
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	return okResult("/system", lines)
}

func cmdSystemSet(m *Model, args []string) cmdResult {
	if len(args) == 0 {
		return errResult("/system-set", "usage: /system-set <text>")
	}
	text := strings.Join(args, " ")
	if err := m.eng.SetSystem(text); err != nil {
		return errResult("/system-set", err.Error())
	}
	return okResult("/system-set", []string{"system prompt updated"})
}

func cmdSystemClear(m *Model) cmdResult {
	if err := m.eng.SetSystem(""); err != nil {
		return errResult("/system-clear", err.Error())
	}
	return okResult("/system-clear", []string{"system prompt cleared"})
}

// =============================================================================
// info commands
// =============================================================================

func cmdConfig(m *Model) cmdResult {
	cfg := m.eng.Config()
	lines := []string{
		fmt.Sprintf("topics root:     %s", cfg.TopicsRoot),
		fmt.Sprintf("default topic:   %s", orNone(cfg.DefaultTopic)),
		fmt.Sprintf("default profile: %s", orNone(cfg.DefaultProfile)),
		fmt.Sprintf("window messages: %d", cfg.WindowMessages),
	}
	return okResult("/config", lines)
}

func cmdStatus(m *Model) cmdResult {
	p := m.eng.Profile()
	lines := []string{
		fmt.Sprintf("topic:    %s", m.eng.TopicName()),
		fmt.Sprintf("profile:  %s (%s/%s)", m.eng.ProfileCode(), p.Provider, p.Model),
		fmt.Sprintf("lore home: %s", m.eng.LoreHome()),
	}
	return okResult("/status", lines)
}

func cmdStats(m *Model) cmdResult {
	logPath := store.UsageLogPath(m.eng.LoreHome())
	entries, err := store.ReadUsageLog(logPath)
	if err != nil || len(entries) == 0 {
		return okResult("/stats", []string{"(no usage recorded)"})
	}
	agg := store.AggregateUsage(entries, m.eng.TopicName(), 0)
	aggAll := store.AggregateUsage(entries, "", 0)
	lines := []string{
		fmt.Sprintf("topic %q:", m.eng.TopicName()),
		fmt.Sprintf("  calls:  %d", agg.Total.Calls),
		fmt.Sprintf("  tokens: %s in / %s out",
			core.FormatTokens(agg.Total.InputTokens),
			core.FormatTokens(agg.Total.OutputTokens)),
		fmt.Sprintf("  cost:   %s", config.FormatCost(agg.Total.CostUSD)),
		"all topics:",
		fmt.Sprintf("  calls:  %d", aggAll.Total.Calls),
		fmt.Sprintf("  tokens: %s in / %s out",
			core.FormatTokens(aggAll.Total.InputTokens),
			core.FormatTokens(aggAll.Total.OutputTokens)),
		fmt.Sprintf("  cost:   %s", config.FormatCost(aggAll.Total.CostUSD)),
	}
	return okResult("/stats", lines)
}

// =============================================================================
// help
// =============================================================================

func cmdHelp(cmd string, args []string) cmdResult {
	type entry struct {
		cmd  string
		desc string
	}
	groups := map[string][]entry{
		"topic": {
			{"/topic [name]", "show topic info"},
			{"/topic-switch <name>", "switch to existing topic"},
			{"/topic-new <name>", "create and switch to new topic"},
			{"/topic-list", "list all topics"},
			{"/topic-delete", "delete current topic"},
			{"/topic-clear", "clear history for current topic"},
			{"/topic-default", "show default topic"},
			{"/topic-default-set <name>", "set default topic"},
			{"/topic-summary", "show current context summary"},
			{"/topic-history [n]", "show last N exchanges"},
			{"/topic-resource <file>", "add resource file to topic"},
		},
		"profile": {
			{"/profile", "show current profile"},
			{"/profile-switch <name>", "switch to named profile"},
			{"/profile-list", "list all profiles"},
			{"/profile-default", "show default profile"},
			{"/profile-default-set <name>", "set default profile"},
		},
		"system": {
			{"/system", "show system prompt"},
			{"/system-set <text>", "set system prompt"},
			{"/system-clear", "remove system prompt"},
		},
		"info": {
			{"/config", "show resolved configuration"},
			{"/status", "show effective defaults"},
			{"/stats", "show usage and cost stats"},
			{"/help [group]", "show all commands or commands for a group"},
			{"/exit", "exit lore"},
		},
	}
	order := []string{"topic", "profile", "system", "info"}

	noun := ""
	if len(args) > 0 {
		noun = strings.ToLower(args[0])
	}

	var lines []string
	if noun == "" || noun == "all" {
		for _, g := range order {
			lines = append(lines, g+":")
			for _, e := range groups[g] {
				lines = append(lines, fmt.Sprintf("  %-32s  %s", e.cmd, e.desc))
			}
		}
	} else if entries, ok := groups[noun]; ok {
		lines = append(lines, noun+":")
		for _, e := range entries {
			lines = append(lines, fmt.Sprintf("  %-32s  %s", e.cmd, e.desc))
		}
	} else {
		return errResult(cmd+" "+noun, fmt.Sprintf("unknown group %q — available: %s", noun, strings.Join(order, "|")))
	}
	return okResult(cmd, lines)
}

// =============================================================================
// helpers
// =============================================================================

func okResult(input string, output []string) cmdResult {
	return cmdResult{input: input, output: output, isError: false}
}

func errResult(input, msg string) cmdResult {
	return cmdResult{input: input, output: []string{msg}, isError: true}
}

func yesNoStr(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func totalTokens(h *core.History) int {
	total := 0
	for _, m := range h.Msgs {
		total += core.ApproxTokens(m.Content)
	}
	return total
}
