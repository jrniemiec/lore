package main

import (
	"flag"
	"fmt"
	"os"

	"github.vom/jrniemiec/lore/config"
	"github.vom/jrniemiec/lore/engine"
	"github.vom/jrniemiec/lore/tui"
)

// --- flag variables -------------------------------------------------------

var (
	// mode
	flagNoTUI bool

	// core
	flagProfile string
	flagTopic   string

	// chat
	flagStrategy      string
	flagContextLimit  int
	flagHistoryWindow int
	flagInputFile     string
	flagSkipHistory   bool
	flagNoStream      bool
	flagQuiet         bool
	flagDebug       bool
	flagAllProfiles bool
	flagJSON        bool
	flagForce       bool

	// display
	flagSize     int
	flagHelpNoun string

	// admin — read/display
	flagTopicList    bool
	flagTopicInfo    bool
	flagTopicHistory bool
	flagTopicSummary bool
	flagSystem       bool
	flagConfig       bool
	flagProfileList  bool
	flagStats        bool
	flagStatus       bool

	// admin — write/mutate
	flagTopicNew          string
	flagTopicDelete       bool
	flagTopicClear        bool
	flagSystemSet         string
	flagSystemFile        string
	flagProfileDefaultSet string
	flagTopicDefaultSet   string
	flagTopicResource     string
)

func init() {
	// mode
	flag.BoolVar(&flagNoTUI, "no-tui", false, "run in headless mode (no TUI)")
	flag.BoolVar(&flagNoTUI, "nw", false, "headless mode (short for --no-tui)")

	// core
	flag.StringVar(&flagProfile, "profile", "", "provider profile code")
	flag.StringVar(&flagProfile, "p", "", "provider profile code")
	flag.StringVar(&flagTopic, "topic", "", "topic name")
	flag.StringVar(&flagTopic, "t", "", "topic name")

	// chat
	flag.StringVar(&flagStrategy, "strategy", "", "context strategy: tail|token-budget|summarize")
	flag.IntVar(&flagContextLimit, "context-limit", 0, "token budget override for this invocation")
	flag.IntVar(&flagHistoryWindow, "history-window", 0, "tail strategy: number of past user turns")
	flag.StringVar(&flagInputFile, "input-file", "", "read prompt from a text file")
	flag.StringVar(&flagInputFile, "i", "", "read prompt from a text file")
	flag.BoolVar(&flagSkipHistory, "skip-history", false, "do not persist this exchange to history")
	flag.BoolVar(&flagSkipHistory, "X", false, "do not persist this exchange to history")
	flag.BoolVar(&flagNoStream, "no-stream", false, "disable streaming; print full response at once")
	flag.BoolVar(&flagNoStream, "N", false, "disable streaming")
	flag.BoolVar(&flagQuiet, "quiet", false, "suppress warnings and stats on stderr")
	flag.BoolVar(&flagQuiet, "q", false, "suppress warnings and stats on stderr")
	flag.BoolVar(&flagDebug, "debug", false, "print request/response debug info to stderr")
	flag.BoolVar(&flagDebug, "D", false, "print request/response debug info to stderr")
	flag.BoolVar(&flagAllProfiles, "all-profiles", false, "run prompt against all configured profiles")
	flag.BoolVar(&flagAllProfiles, "A", false, "run prompt against all configured profiles")
	flag.BoolVar(&flagJSON, "json", false, "output result as JSON")
	flag.BoolVar(&flagForce, "force", false, "skip confirmation prompts")
	flag.BoolVar(&flagForce, "f", false, "skip confirmation prompts")

	// display / help
	flag.IntVar(&flagSize, "size", 20, "exchanges/lines to show for topic-history/topic-summary")
	flag.StringVar(&flagHelpNoun, "help-for", "", "show help for a command group: topic|profile|system|session|info|all")

	// admin — read/display
	flag.BoolVar(&flagTopicList, "topic-list", false, "list all topics")
	flag.BoolVar(&flagTopicList, "T", false, "list all topics")
	flag.BoolVar(&flagTopicInfo, "topic-info", false, "show topic contents")
	flag.BoolVar(&flagTopicHistory, "topic-history", false, "print last N exchanges from history")
	flag.BoolVar(&flagTopicSummary, "topic-summary", false, "print current summary for topic")
	flag.BoolVar(&flagSystem, "system", false, "print system prompt for topic")
	flag.BoolVar(&flagConfig, "config", false, "print resolved configuration")
	flag.BoolVar(&flagProfileList, "profile-list", false, "print configured profiles")
	flag.BoolVar(&flagStats, "stats", false, "print cumulative usage and cost stats")
	flag.BoolVar(&flagStatus, "status", false, "show effective defaults for next invocation")

	// admin — write/mutate
	flag.StringVar(&flagTopicNew, "topic-new", "", "create a new topic")
	flag.BoolVar(&flagTopicDelete, "topic-delete", false, "delete topic and all its files")
	flag.BoolVar(&flagTopicClear, "topic-clear", false, "erase history for topic")
	flag.StringVar(&flagSystemSet, "system-set", "", "set system prompt for topic")
	flag.StringVar(&flagSystemSet, "s", "", "set system prompt for topic")
	flag.StringVar(&flagSystemFile, "system-file", "", "set system prompt from file")
	flag.StringVar(&flagSystemFile, "S", "", "set system prompt from file")
	flag.StringVar(&flagProfileDefaultSet, "profile-default-set", "", "persist default profile to config")
	flag.StringVar(&flagProfileDefaultSet, "P", "", "persist default profile to config")
	flag.StringVar(&flagTopicDefaultSet, "topic-default-set", "", "persist default topic to config")
	flag.StringVar(&flagTopicResource, "topic-resource", "", "copy file into topic resources/")
	flag.StringVar(&flagTopicResource, "u", "", "copy file into topic resources/")
}

func main() {
	flag.Parse()
	os.Exit(run())
}

func run() int {
	cfgPath := config.DefaultConfigPath()
	if err := config.Bootstrap(cfgPath); err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap: %v\n", err)
		return 1
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		return 1
	}
	if flagHistoryWindow > 0 {
		cfg.WindowMessages = flagHistoryWindow
	}

	if isHeadless() {
		return runHeadless(cfg, cfgPath)
	}

	loreHome := config.LoreHome()
	topicName := config.EffectiveTopic(cfg, flagTopic)
	e, err := engine.New(cfg, cfgPath, loreHome, topicName, flagProfile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "engine: %v\n", err)
		return 1
	}
	if err := tui.Start(e, cfg, loreHome); err != nil {
		fmt.Fprintf(os.Stderr, "tui: %v\n", err)
		return 1
	}
	return 0
}

// isHeadless returns true when lore should skip the TUI.
func isHeadless() bool {
	if flagNoTUI {
		return true
	}
	// Admin commands always bypass the TUI.
	if flagTopicList || flagTopicInfo || flagTopicHistory || flagTopicSummary ||
		flagSystem || flagConfig || flagProfileList || flagStats ||
		flagStatus || flagTopicNew != "" || flagTopicDelete || flagTopicClear ||
		flagSystemSet != "" || flagSystemFile != "" || flagProfileDefaultSet != "" ||
		flagTopicDefaultSet != "" || flagTopicResource != "" || flagAllProfiles ||
		flagHelpNoun != "" {
		return true
	}
	return stdinIsPipe()
}

func stdinIsPipe() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice == 0
}
