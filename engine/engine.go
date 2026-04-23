package engine

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.vom/jrniemiec/lore/config"
	"github.vom/jrniemiec/lore/core"
	"github.vom/jrniemiec/lore/provider"
	"github.vom/jrniemiec/lore/store"
	"github.vom/jrniemiec/lore/strategy"
)

// Engine is the shared session object used by both headless and TUI modes.
// It owns the active topic, profile, provider, and store. All chat and admin
// operations go through the engine — neither the headless runner nor the TUI
// import provider or store directly.
type Engine struct {
	cfg      config.Config
	cfgPath  string
	loreHome string
	st       core.Store

	// active session state
	topicName   string
	topic       *core.Topic
	profileCode string
	profile     config.ProviderProfile
	prov        core.Provider

	// streaming guard
	mu        sync.Mutex
	streaming bool
}

// ChatOptions controls per-call behaviour for Chat().
type ChatOptions struct {
	SkipHistory      bool      // do not persist this exchange to history
	NoStream         bool      // use blocking Chat() instead of ChatStream()
	StrategyOverride string    // override context strategy for this call
	BudgetOverride   int       // override token budget for this call
	Out              io.Writer // output for summarize strategy notifications; nil = quiet
}

// ChatResult holds the outcome of a Chat() call.
type ChatResult struct {
	Usage   core.Usage
	Elapsed time.Duration
}

// New creates an Engine, initialising the provider for profileCode and loading
// the topic. Pass empty strings to use the config defaults.
func New(cfg config.Config, cfgPath, loreHome, topicName, profileCode string) (*Engine, error) {
	e := &Engine{
		cfg:      cfg,
		cfgPath:  cfgPath,
		loreHome: loreHome,
		st:       store.New(cfg.TopicsRoot),
	}
	if err := e.SwitchProfile(profileCode); err != nil {
		return nil, err
	}
	if topicName == "" {
		topicName = config.EffectiveTopic(cfg, "")
	}
	if err := e.SwitchTopic(topicName); err != nil {
		return nil, err
	}
	return e, nil
}

// --- Chat -----------------------------------------------------------------

// Chat sends prompt to the active provider, applies the context strategy,
// and (unless SkipHistory) persists the exchange to history.
// onDelta is called for each streaming token; pass nil to suppress streaming.
func (e *Engine) Chat(ctx context.Context, prompt string, opts ChatOptions, onDelta func(string) error) (ChatResult, error) {
	e.mu.Lock()
	if e.streaming {
		e.mu.Unlock()
		return ChatResult{}, fmt.Errorf("a chat is already in progress")
	}
	e.streaming = true
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		e.streaming = false
		e.mu.Unlock()
	}()

	strat := e.buildStrategy(ctx, opts)
	contextMsgs := strat.Apply(e.topic.History, prompt)
	allMsgs := append(contextMsgs, core.Message{Role: core.RoleUser, Content: prompt})

	start := time.Now()
	var (
		response string
		usage    core.Usage
		err      error
	)
	if opts.NoStream || onDelta == nil {
		response, usage, err = e.prov.Chat(ctx, e.topic.SystemPrompt, allMsgs)
	} else {
		response, usage, err = e.prov.ChatStream(ctx, e.topic.SystemPrompt, allMsgs, onDelta)
	}
	elapsed := time.Since(start)

	if err != nil {
		return ChatResult{Usage: usage, Elapsed: elapsed}, err
	}

	if !opts.SkipHistory {
		e.topic.History.Append(core.RoleUser, prompt)
		e.topic.History.Append(core.RoleAssistant, response)
		if saveErr := e.st.SaveHistory(e.topicName, e.topic.History); saveErr != nil {
			// non-fatal: best-effort
			fmt.Fprintf(io.Discard, "save history: %v", saveErr)
		}
		e.appendUsageLog(usage)
	}

	return ChatResult{Usage: usage, Elapsed: elapsed}, nil
}

// buildStrategy constructs the context strategy for a chat call.
// A fresh strategy is built per call so context (for summarize) is always current.
func (e *Engine) buildStrategy(ctx context.Context, opts ChatOptions) strategy.ContextStrategy {
	name := strategy.ResolveStrategyName(opts.StrategyOverride, e.profile)

	if name == strategy.StrategySummarize {
		summCode := e.profile.SummarizerProfile
		summProfile, ok := e.cfg.Profiles[summCode]
		if !ok {
			return strategy.New(strategy.StrategyTokenBudget, e.profile, e.cfg, e.topic.SystemPrompt, opts.BudgetOverride)
		}
		summProv, err := provider.New(summProfile)
		if err != nil {
			return strategy.New(strategy.StrategyTokenBudget, e.profile, e.cfg, e.topic.SystemPrompt, opts.BudgetOverride)
		}
		budget := opts.BudgetOverride
		if budget <= 0 {
			budget = strategy.EffectiveBudget(e.profile)
		}
		return &strategy.SummarizeStrategy{
			SummarizerProvider: summProv,
			SummarizerBudget:   strategy.EffectiveBudget(summProfile),
			TopicName:          e.topicName,
			Store:              e.st,
			Ctx:                ctx,
			Out:                opts.Out,
			Budget:             budget,
			VerbatimRatio:      strategy.EffectiveVerbatimRatio(e.profile),
		}
	}

	return strategy.New(name, e.profile, e.cfg, e.topic.SystemPrompt, opts.BudgetOverride)
}

// --- Session state accessors ----------------------------------------------

func (e *Engine) TopicName() string              { return e.topicName }
func (e *Engine) Topic() *core.Topic             { return e.topic }
func (e *Engine) ProfileCode() string            { return e.profileCode }
func (e *Engine) Profile() config.ProviderProfile { return e.profile }
func (e *Engine) Config() config.Config          { return e.cfg }
func (e *Engine) LoreHome() string               { return e.loreHome }

func (e *Engine) IsStreaming() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.streaming
}

// --- Topic operations -----------------------------------------------------

// SwitchTopic loads a topic by name, making it the active topic.
func (e *Engine) SwitchTopic(name string) error {
	topic, err := e.st.LoadTopic(name)
	if err != nil {
		return fmt.Errorf("load topic %q: %w", name, err)
	}
	e.topicName = name
	e.topic = topic
	return nil
}

func (e *Engine) ListTopics() ([]string, error) {
	return e.st.ListTopics()
}

// CreateTopic creates a topic directory and optionally sets a system prompt.
func (e *Engine) CreateTopic(name, system string) error {
	if _, err := e.st.LoadTopic(name); err != nil {
		return fmt.Errorf("create topic %q: %w", name, err)
	}
	if system != "" {
		if err := e.st.SaveSystem(name, system); err != nil {
			return err
		}
	}
	return nil
}

// DeleteTopic removes the topic directory. Does not affect the active topic
// loaded in memory — callers should switch topic if deleting the active one.
func (e *Engine) DeleteTopic(name string) error {
	return e.st.DeleteTopic(name)
}

// ClearHistory resets history for the active topic (in memory and on disk).
func (e *Engine) ClearHistory() error {
	if err := e.st.ClearHistory(e.topicName); err != nil {
		return err
	}
	e.topic.History = core.NewHistory()
	return nil
}

// SystemPrompt returns the active topic's system prompt.
func (e *Engine) SystemPrompt() string { return e.topic.SystemPrompt }

// SetSystem sets the system prompt for the active topic.
func (e *Engine) SetSystem(text string) error {
	if err := e.st.SaveSystem(e.topicName, text); err != nil {
		return err
	}
	e.topic.SystemPrompt = text
	return nil
}

// SetSystemForTopic sets the system prompt for any topic by name.
func (e *Engine) SetSystemForTopic(topicName, text string) error {
	return e.st.SaveSystem(topicName, text)
}

// LoadSummary returns the current summary for the active topic.
func (e *Engine) LoadSummary() (string, int, error) {
	return e.st.LoadSummary(e.topicName)
}

// AddResource copies a file into the active topic's resources/ directory.
func (e *Engine) AddResource(sourcePath string) error {
	return e.st.CreateResource(e.topicName, sourcePath)
}

// --- Profile operations ---------------------------------------------------

// SwitchProfile resolves and activates a profile, rebuilding the provider.
func (e *Engine) SwitchProfile(code string) error {
	resolvedCode, prof, err := config.ResolveProfile(e.cfg, code)
	if err != nil {
		return err
	}
	prov, err := provider.New(prof)
	if err != nil {
		return fmt.Errorf("init provider for profile %q: %w", resolvedCode, err)
	}
	e.profileCode = resolvedCode
	e.profile = prof
	e.prov = prov
	return nil
}

// --- Config persistence ---------------------------------------------------

// SetDefaultTopic persists the default topic to config.json.
func (e *Engine) SetDefaultTopic(name string) error {
	e.cfg.DefaultTopic = name
	return config.SaveAtomic(e.cfgPath, e.cfg)
}

// SetDefaultProfile persists the default profile to config.json.
func (e *Engine) SetDefaultProfile(code string) error {
	if _, _, err := config.ResolveProfile(e.cfg, code); err != nil {
		return err
	}
	e.cfg.DefaultProfile = code
	return config.SaveAtomic(e.cfgPath, e.cfg)
}

// --- Usage logging --------------------------------------------------------

func (e *Engine) appendUsageLog(u core.Usage) {
	logPath := store.UsageLogPath(e.loreHome)
	inPer1M, outPer1M, hasPricing := config.ExtractPricing(e.profile.Info)
	var cost float64
	if hasPricing {
		cost = config.CalcCost(u.InputTokens, u.OutputTokens, inPer1M, outPer1M)
	}
	entry := store.UsageEntry{
		Timestamp:    time.Now(),
		Topic:        e.topicName,
		Profile:      e.profileCode,
		Model:        e.profile.Model,
		Provider:     e.profile.Provider,
		InputTokens:  u.InputTokens,
		OutputTokens: u.OutputTokens,
		CostUSD:      cost,
		Estimated:    u.Estimated,
	}
	_ = store.AppendUsageLog(logPath, entry)
}
