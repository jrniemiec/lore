package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.vom/jrniemiec/lore/config"
	"github.vom/jrniemiec/lore/core"
	"github.vom/jrniemiec/lore/engine"
	"github.vom/jrniemiec/lore/store"
)

func runHeadless(cfg config.Config, cfgPath string) int {
	loreHome := config.LoreHome()
	st := store.New(cfg.TopicsRoot)
	topicName := config.EffectiveTopic(cfg, flagTopic)

	// --- provider-independent admin commands (no engine/API key needed) ---

	if flagHelpNoun != "" {
		return cmdHelpNoun(flagHelpNoun)
	}
	if flagTopicList {
		return cmdListTopics(st)
	}
	if flagConfig {
		return cmdShowConfig(cfg)
	}
	if flagStatus {
		return cmdStatus(cfg, cfgPath, topicName, flagProfile)
	}
	if flagProfileList {
		return cmdShowProviders(cfg)
	}
	if flagStats {
		return cmdShowStats(loreHome, topicName)
	}
	if flagTopicDefaultSet != "" {
		return cmdSetDefaultTopic(cfgPath, cfg, flagTopicDefaultSet)
	}
	if flagProfileDefaultSet != "" {
		return cmdSetDefaultProfile(cfgPath, cfg, flagProfileDefaultSet)
	}
	if flagTopicNew != "" {
		return cmdCreateTopic(st, flagTopicNew, flagSystemSet, flagSystemFile)
	}
	if flagTopicDelete {
		return cmdDeleteTopic(st, topicName, flagForce)
	}
	if flagTopicClear {
		return cmdClearHistory(st, topicName, flagForce)
	}
	if flagTopicHistory {
		return cmdShowHistory(st, topicName, flagSize)
	}
	if flagTopicSummary {
		return cmdShowSummary(st, topicName, flagSize)
	}
	if flagSystem {
		return cmdShowSystem(st, topicName)
	}
	if flagTopicInfo {
		return cmdShowTopic(st, topicName)
	}
	if flagSystemSet != "" || flagSystemFile != "" {
		return cmdSetSystem(st, topicName, flagSystemSet, flagSystemFile)
	}
	if flagTopicResource != "" {
		return cmdAddResource(st, topicName, flagTopicResource)
	}
	if flagNote != "" {
		return cmdAddNote(cfg, cfgPath, loreHome, topicName, flagNote)
	}
	if flagDeleteLast >= 0 {
		n := flagDeleteLast
		if n == 0 {
			n = 1
		}
		return cmdDeleteLast(cfg, cfgPath, loreHome, topicName, n, flagForce)
	}

	// --- chat (needs provider via engine) ---
	return runChat(cfg, cfgPath, loreHome, topicName)
}

// =============================================================================
// Chat
// =============================================================================

func runChat(cfg config.Config, cfgPath, loreHome, topicName string) int {
	prompt, err := resolvePrompt()
	if err != nil {
		fmt.Fprintf(os.Stderr, "prompt: %v\n", err)
		return 1
	}
	if prompt == "" {
		fmt.Fprintln(os.Stderr, "usage: lore [flags] <prompt>")
		flag.PrintDefaults()
		return 1
	}

	if flagAllProfiles {
		return runAllProfiles(cfg, cfgPath, loreHome, topicName, prompt)
	}

	e, err := engine.New(cfg, cfgPath, loreHome, topicName, flagProfile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "engine: %v\n", err)
		return 1
	}

	return doChat(e, prompt)
}

func runAllProfiles(cfg config.Config, cfgPath, loreHome, topicName, prompt string) int {
	exitCode := 0
	for code := range cfg.Profiles {
		fmt.Fprintf(os.Stderr, "\n=== profile: %s ===\n", code)
		e, err := engine.New(cfg, cfgPath, loreHome, topicName, code)
		if err != nil {
			fmt.Fprintf(os.Stderr, "engine: %v\n", err)
			exitCode = 1
			continue
		}
		if c := doChat(e, prompt); c != 0 {
			exitCode = c
		}
	}
	return exitCode
}

func doChat(e *engine.Engine, prompt string) int {
	opts := engine.ChatOptions{
		SkipHistory:      flagSkipHistory,
		NoStream:         flagNoStream || flagJSON,
		StrategyOverride: flagStrategy,
		BudgetOverride:   flagContextLimit,
		Debug:            flagDebug,
	}
	if !flagQuiet {
		opts.Out = os.Stderr
	}

	var (
		result   engine.ChatResult
		err      error
		response strings.Builder
	)

	ctx := interruptContext()

	if flagNoStream || flagJSON {
		// Collect full response then print.
		result, err = e.Chat(ctx, prompt, opts, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "chat: %v\n", err)
			return 1
		}
		// The response was persisted internally; we need it for JSON output.
		// Re-read from history (last assistant message).
		if flagJSON {
			h := e.Topic().History
			if len(h.Msgs) > 0 {
				last := h.Msgs[len(h.Msgs)-1]
				if last.Role == core.RoleAssistant {
					response.WriteString(last.Content)
				}
			}
		}
	} else {
		// Stream tokens directly to stdout.
		onDelta := func(delta string) error {
			fmt.Print(delta)
			return nil
		}
		result, err = e.Chat(ctx, prompt, opts, onDelta)
		fmt.Println() // newline after streamed output
		if err != nil {
			fmt.Fprintf(os.Stderr, "chat: %v\n", err)
			return 1
		}
	}

	if flagJSON {
		return printChatJSON(response.String(), result, e.Profile())
	}

	if flagNoStream && !flagJSON {
		// Print the buffered response.
		h := e.Topic().History
		if len(h.Msgs) > 0 {
			last := h.Msgs[len(h.Msgs)-1]
			if last.Role == core.RoleAssistant {
				fmt.Println(last.Content)
			}
		}
	}

	if !flagQuiet {
		printStats(result, e.Profile())
	}
	return 0
}

func printChatJSON(response string, result engine.ChatResult, profile config.ProviderProfile) int {
	inPer1M, outPer1M, hasPricing := config.ExtractPricing(profile.Info)
	var cost float64
	if hasPricing {
		cost = config.CalcCost(result.Usage.InputTokens, result.Usage.OutputTokens, inPer1M, outPer1M)
	}
	out := map[string]any{
		"response": response,
		"usage": map[string]any{
			"input_tokens":  result.Usage.InputTokens,
			"output_tokens": result.Usage.OutputTokens,
			"estimated":     result.Usage.Estimated,
		},
		"elapsed_ms": result.Elapsed.Milliseconds(),
	}
	if hasPricing {
		out["cost_usd"] = cost
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(b))
	return 0
}

func printStats(result engine.ChatResult, profile config.ProviderProfile) {
	u := result.Usage
	s := fmt.Sprintf("[%d in / %d out tokens", u.InputTokens, u.OutputTokens)
	inPer1M, outPer1M, hasPricing := config.ExtractPricing(profile.Info)
	if hasPricing {
		cost := config.CalcCost(u.InputTokens, u.OutputTokens, inPer1M, outPer1M)
		s += " | " + config.FormatCost(cost)
	}
	s += fmt.Sprintf(" | %dms]", result.Elapsed.Milliseconds())
	fmt.Fprintln(os.Stderr, s)
}

// =============================================================================
// Admin commands
// =============================================================================

func cmdListTopics(st *store.FileStore) int {
	topics, err := st.ListTopics()
	if err != nil {
		fmt.Fprintf(os.Stderr, "list topics: %v\n", err)
		return 1
	}
	if len(topics) == 0 {
		fmt.Fprintln(os.Stderr, "(no topics)")
		return 0
	}
	for _, t := range topics {
		fmt.Println(t)
	}
	return 0
}

func cmdShowConfig(cfg config.Config) int {
	b, _ := json.MarshalIndent(cfg, "", "  ")
	fmt.Println(string(b))
	return 0
}

func cmdStatus(cfg config.Config, cfgPath, topicName, profileCode string) int {
	resolvedCode, profile, err := config.ResolveProfile(cfg, profileCode)
	stratName := "(unknown)"
	if err == nil {
		stratName = resolveStatusStrategy(profile)
	}
	fmt.Printf("config:   %s\n", cfgPath)
	fmt.Printf("topics:   %s\n", cfg.TopicsRoot)
	fmt.Printf("topic:    %s\n", topicName)
	if err == nil {
		fmt.Printf("profile:  %s (%s/%s)\n", resolvedCode, profile.Provider, profile.Model)
		fmt.Printf("strategy: %s\n", stratName)
	} else {
		fmt.Printf("profile:  (none) — %v\n", err)
	}
	return 0
}

func resolveStatusStrategy(p config.ProviderProfile) string {
	if p.Strategy != "" {
		return p.Strategy
	}
	if p.SummarizerProfile != "" {
		return "summarize"
	}
	if p.ContextTokenLimit > 0 || p.MaxContextTokens > 0 {
		return "token-budget"
	}
	return "tail"
}

func cmdShowProviders(cfg config.Config) int {
	for code, p := range cfg.Profiles {
		strat := resolveStatusStrategy(p)
		line := fmt.Sprintf("%-16s  %-12s  %-32s  strategy: %s", code, p.Provider, p.Model, strat)
		if p.MaxContextTokens > 0 {
			line += fmt.Sprintf("  context: %s", core.FormatTokens(p.MaxContextTokens))
		}
		inPer1M, outPer1M, ok := config.ExtractPricing(p.Info)
		if ok {
			line += fmt.Sprintf("  $%.2f/$%.2f per 1M", inPer1M, outPer1M)
		}
		fmt.Println(line)
	}
	return 0
}

func cmdShowStats(loreHome, topicFilter string) int {
	logPath := store.UsageLogPath(loreHome)
	entries, err := store.ReadUsageLog(logPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read usage log: %v\n", err)
		return 1
	}
	if len(entries) == 0 {
		fmt.Fprintln(os.Stderr, "(no usage recorded)")
		return 0
	}
	agg := store.AggregateUsage(entries, topicFilter, 0)
	fmt.Printf("total calls:   %d\n", agg.Total.Calls)
	fmt.Printf("input tokens:  %s\n", core.FormatTokens(agg.Total.InputTokens))
	fmt.Printf("output tokens: %s\n", core.FormatTokens(agg.Total.OutputTokens))
	if agg.Total.CostUSD > 0 {
		fmt.Printf("cost:          %s\n", config.FormatCost(agg.Total.CostUSD))
	}
	if len(agg.ByProfile) > 1 {
		fmt.Println("\nby profile:")
		for code, s := range agg.ByProfile {
			fmt.Printf("  %-16s  %d calls  %s in  %s out", code, s.Calls,
				core.FormatTokens(s.InputTokens), core.FormatTokens(s.OutputTokens))
			if s.CostUSD > 0 {
				fmt.Printf("  %s", config.FormatCost(s.CostUSD))
			}
			fmt.Println()
		}
	}
	return 0
}

func cmdSetDefaultTopic(cfgPath string, cfg config.Config, name string) int {
	cfg.DefaultTopic = name
	if err := config.SaveAtomic(cfgPath, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "save config: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "default topic set to %q\n", name)
	return 0
}

func cmdSetDefaultProfile(cfgPath string, cfg config.Config, code string) int {
	if _, _, err := config.ResolveProfile(cfg, code); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	cfg.DefaultProfile = code
	if err := config.SaveAtomic(cfgPath, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "save config: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "default profile set to %q\n", code)
	return 0
}

func cmdCreateTopic(st *store.FileStore, name, system, systemFile string) int {
	if _, err := st.LoadTopic(name); err != nil {
		fmt.Fprintf(os.Stderr, "create topic: %v\n", err)
		return 1
	}
	text, ok := resolveSystemText(system, systemFile)
	if !ok {
		return 1
	}
	if text != "" {
		if err := st.SaveSystem(name, text); err != nil {
			fmt.Fprintf(os.Stderr, "set system: %v\n", err)
			return 1
		}
	}
	fmt.Fprintf(os.Stderr, "topic %q created\n", name)
	return 0
}

func cmdDeleteTopic(st *store.FileStore, topicName string, force bool) int {
	if !force && !confirm(fmt.Sprintf("Delete topic %q and all its files?", topicName)) {
		fmt.Fprintln(os.Stderr, "aborted")
		return 0
	}
	if err := st.DeleteTopic(topicName); err != nil {
		fmt.Fprintf(os.Stderr, "delete topic: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "topic %q deleted\n", topicName)
	return 0
}

func cmdClearHistory(st *store.FileStore, topicName string, force bool) int {
	if !force && !confirm(fmt.Sprintf("Clear history for topic %q?", topicName)) {
		fmt.Fprintln(os.Stderr, "aborted")
		return 0
	}
	if err := st.ClearHistory(topicName); err != nil {
		fmt.Fprintf(os.Stderr, "clear history: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "history cleared for topic %q\n", topicName)
	return 0
}

func cmdShowHistory(st *store.FileStore, topicName string, n int) int {
	h, err := st.LoadHistory(topicName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load history: %v\n", err)
		return 1
	}
	if len(h.Msgs) == 0 {
		fmt.Fprintln(os.Stderr, "(no history)")
		return 0
	}
	// Collect exchange pairs (user + assistant).
	type pair struct{ user, assistant core.Message }
	var pairs []pair
	for i := 0; i < len(h.Msgs)-1; i++ {
		if h.Msgs[i].Role == core.RoleUser && h.Msgs[i+1].Role == core.RoleAssistant {
			pairs = append(pairs, pair{h.Msgs[i], h.Msgs[i+1]})
			i++ // skip assistant message
		}
	}
	if n > 0 && len(pairs) > n {
		pairs = pairs[len(pairs)-n:]
	}
	for i, p := range pairs {
		if i > 0 {
			fmt.Println("---")
		}
		fmt.Printf("you · %s\n%s\n\n", p.user.Time.Format("15:04"), p.user.Content)
		fmt.Printf("lore · %s\n%s\n", p.assistant.Time.Format("15:04"), p.assistant.Content)
		fmt.Println()
	}
	return 0
}

func cmdShowSummary(st *store.FileStore, topicName string, n int) int {
	text, coversThrough, err := st.LoadSummary(topicName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load summary: %v\n", err)
		return 1
	}
	if text == "" {
		fmt.Fprintln(os.Stderr, "(no summary)")
		return 0
	}
	fmt.Fprintf(os.Stderr, "(covers through message %d)\n", coversThrough+1)
	lines := strings.Split(text, "\n")
	if n > 0 && len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	fmt.Println(strings.Join(lines, "\n"))
	return 0
}

func cmdShowSystem(st *store.FileStore, topicName string) int {
	text, err := st.LoadSystem(topicName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load system: %v\n", err)
		return 1
	}
	if text == "" {
		fmt.Fprintln(os.Stderr, "(no system prompt)")
		return 0
	}
	fmt.Print(text)
	if !strings.HasSuffix(text, "\n") {
		fmt.Println()
	}
	return 0
}

func cmdShowTopic(st *store.FileStore, topicName string) int {
	h, err := st.LoadHistory(topicName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load history: %v\n", err)
		return 1
	}
	sys, _ := st.LoadSystem(topicName)
	summText, coversThrough, _ := st.LoadSummary(topicName)

	fmt.Printf("topic:   %s\n", topicName)
	fmt.Printf("system:  %s\n", yesNo(sys != ""))
	fmt.Printf("history: %d messages (~%s tokens)\n", len(h.Msgs), core.FormatTokens(totalHistoryTokens(h)))
	if summText != "" {
		fmt.Printf("summary: covers messages 1-%d (~%s tokens)\n",
			coversThrough+1, core.FormatTokens(core.ApproxTokens(summText)))
	} else {
		fmt.Println("summary: (none)")
	}
	return 0
}

func cmdSetSystem(st *store.FileStore, topicName, system, systemFile string) int {
	text, ok := resolveSystemText(system, systemFile)
	if !ok {
		return 1
	}
	if err := st.SaveSystem(topicName, text); err != nil {
		fmt.Fprintf(os.Stderr, "set system: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "system prompt set for topic %q\n", topicName)
	return 0
}

func cmdDeleteLast(cfg config.Config, cfgPath, loreHome, topicName string, n int, force bool) int {
	noun := "exchange"
	if n > 1 {
		noun = fmt.Sprintf("%d exchanges", n)
	}
	if !force && !confirm(fmt.Sprintf("Delete last %s from topic %q?", noun, topicName)) {
		fmt.Fprintln(os.Stderr, "aborted")
		return 0
	}
	eng, err := engine.New(cfg, cfgPath, loreHome, topicName, flagProfile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "engine: %v\n", err)
		return 1
	}
	removed, err := eng.DeleteLast(n)
	if err != nil {
		fmt.Fprintf(os.Stderr, "delete-last: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "deleted %d exchange(s) from topic %q\n", removed, topicName)
	return 0
}

func cmdAddNote(cfg config.Config, cfgPath, loreHome, topicName, text string) int {
	eng, err := engine.New(cfg, cfgPath, loreHome, topicName, flagProfile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "engine: %v\n", err)
		return 1
	}
	if err := eng.AddNote(text); err != nil {
		fmt.Fprintf(os.Stderr, "note: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "note saved to topic %q\n", topicName)
	return 0
}

func cmdAddResource(st *store.FileStore, topicName, sourcePath string) int {
	if err := st.CreateResource(topicName, sourcePath); err != nil {
		fmt.Fprintf(os.Stderr, "add resource: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "resource added to topic %q\n", topicName)
	return 0
}

// =============================================================================
// Helpers
// =============================================================================

// resolvePrompt reads the prompt from flags, stdin, or positional args.
// Positional args become the instruction; piped stdin is appended as payload.
func resolvePrompt() (string, error) {
	if flagInputFile != "" {
		b, err := os.ReadFile(flagInputFile)
		if err != nil {
			return "", fmt.Errorf("read input file: %w", err)
		}
		return strings.TrimSpace(string(b)), nil
	}

	positional := strings.TrimSpace(strings.Join(flag.Args(), " "))

	if stdinIsPipe() {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		piped := strings.TrimSpace(string(b))
		if positional != "" && piped != "" {
			return positional + "\n" + piped, nil
		}
		return positional + piped, nil
	}

	return positional, nil
}

// resolveSystemText returns the system prompt text from inline string or file.
func resolveSystemText(inline, filePath string) (string, bool) {
	if filePath != "" {
		b, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read system file: %v\n", err)
			return "", false
		}
		return string(b), true
	}
	return inline, true
}

// confirm prompts the user for y/n confirmation on stderr.
func confirm(prompt string) bool {
	fmt.Fprintf(os.Stderr, "%s [y/N] ", prompt)
	sc := bufio.NewScanner(os.Stdin)
	if sc.Scan() {
		return strings.ToLower(strings.TrimSpace(sc.Text())) == "y"
	}
	return false
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func totalHistoryTokens(h *core.History) int {
	total := 0
	for _, m := range h.Msgs {
		total += core.ApproxTokens(m.Content)
	}
	return total
}

func cmdHelpNoun(noun string) int {
	groups := map[string][]helpEntry{
		"topic": {
			{"--topic-list", "list all topics with message count and last used"},
			{"--topic-info", "show info for current topic (history, system, summary)"},
			{"--topic-history [--size N]", "show last N exchanges (default 20)"},
			{"--topic-summary", "show current context summary for topic"},
			{"--topic-new <name>", "create a new topic and switch to it"},
			{"--topic-delete [--force]", "delete current topic and all its files"},
			{"--topic-clear [--force]", "erase history for current topic"},
			{"--topic-default-set <name>", "persist default topic to config"},
			{"--topic-resource <file>", "add resource file to current topic"},
			{"--note <text>", "save a personal note to topic history (not sent to LLM)"},
			{"--delete-last [n]", "delete last N exchanges from topic history (default 1)"},
		},
		"profile": {
			{"--profile-list", "list all configured profiles"},
			{"--profile-default-set <name>", "persist default profile to config"},
		},
		"system": {
			{"--system", "show system prompt for current topic"},
			{"--system-set <text> / -s", "set system prompt inline"},
			{"--system-file <path> / -S", "set system prompt from file"},
		},
		"session": {
			{"--strategy <name>", "override context strategy: tail|token-budget|summarize"},
			{"--context-limit <n>", "override token budget for this invocation"},
			{"--history-window <n>", "override tail window (number of past user turns)"},
			{"--skip-history / -X", "do not persist this exchange to history"},
			{"--no-stream / -N", "disable streaming; print full response at once"},
			{"--all-profiles / -A", "run prompt against all configured profiles"},
		},
		"info": {
			{"--config", "print resolved configuration"},
			{"--status", "show effective defaults for next invocation"},
			{"--stats", "print cumulative usage and cost stats"},
			{"--debug / -D", "print request/response debug info to stderr"},
			{"--help-for <noun>", "show help for a command group: topic|profile|system|session|info|all"},
		},
	}
	order := []string{"topic", "profile", "system", "session", "info"}

	noun = strings.ToLower(strings.TrimSpace(noun))
	if noun == "all" || noun == "" {
		for _, n := range order {
			printHelpGroup(n, groups[n])
		}
		return 0
	}
	entries, ok := groups[noun]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown noun %q — available: %s\n", noun, strings.Join(order, "|"))
		return 1
	}
	printHelpGroup(noun, entries)
	return 0
}

type helpEntry struct {
	flag string
	desc string
}

func printHelpGroup(noun string, entries []helpEntry) {
	fmt.Printf("%s:\n", noun)
	for _, e := range entries {
		fmt.Printf("  %-38s  %s\n", e.flag, e.desc)
	}
	fmt.Println()
}

// interruptContext returns a context that is cancelled on SIGINT.
func interruptContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		<-c
		cancel()
	}()
	return ctx
}
