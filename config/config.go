package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultTopicName  = "default"
	defaultWindowMsgs = 1024
)

// ProviderProfile defines a named LLM configuration.
type ProviderProfile struct {
	Provider          string         `json:"provider"`
	Host              string         `json:"host,omitempty"`
	Model             string         `json:"model"`
	MaxContextTokens  int            `json:"max_context_tokens,omitempty"`
	ContextTokenLimit int            `json:"context_token_limit,omitempty"`
	MaxUserMessages   int            `json:"max_user_messages,omitempty"`
	MaxOutputTokens   int            `json:"max_output_tokens,omitempty"`
	Strategy          string         `json:"strategy,omitempty"`
	SummarizerProfile string         `json:"summarizer_profile,omitempty"`
	VerbatimRatio     float64        `json:"verbatim_ratio,omitempty"`
	Color             string         `json:"color,omitempty"`
	Info              map[string]any `json:"info,omitempty"`
}

// Config is the top-level lore configuration.
type Config struct {
	TopicsRoot     string                     `json:"topics_root"`
	DefaultTopic   string                     `json:"default_topic,omitempty"`
	DefaultProfile string                     `json:"default_profile,omitempty"`
	WindowMessages int                        `json:"window_messages,omitempty"`
	Profiles       map[string]ProviderProfile `json:"profiles"`
}

// LoreHome returns the lore data directory ($LORE_HOME or ~/.lore).
func LoreHome() string {
	if h := os.Getenv("LORE_HOME"); h != "" {
		return h
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".lore"
	}
	return filepath.Join(home, ".lore")
}

// DefaultConfigPath returns the path to lore's config.json.
func DefaultConfigPath() string {
	return filepath.Join(LoreHome(), "config.json")
}

// DefaultTopicsRoot returns the default topics directory.
func DefaultTopicsRoot() string {
	return filepath.Join(LoreHome(), "topics")
}

// Load reads config from path. Missing file returns safe defaults.
func Load(path string) (Config, error) {
	cfg := Config{
		TopicsRoot:     DefaultTopicsRoot(),
		DefaultTopic:   defaultTopicName,
		WindowMessages: defaultWindowMsgs,
		Profiles:       map[string]ProviderProfile{},
	}
	if path == "" {
		return cfg, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return Config{}, err
	}
	if len(b) == 0 {
		return cfg, nil
	}
	var loaded Config
	if err := json.Unmarshal(b, &loaded); err != nil {
		return Config{}, fmt.Errorf("config %s: %w", path, err)
	}
	mergeConfig(&cfg, loaded)
	cfg.TopicsRoot = filepath.Clean(cfg.TopicsRoot)
	return cfg, nil
}

func mergeConfig(dst *Config, src Config) {
	if strings.TrimSpace(src.TopicsRoot) != "" {
		dst.TopicsRoot = src.TopicsRoot
	}
	if strings.TrimSpace(src.DefaultTopic) != "" {
		dst.DefaultTopic = src.DefaultTopic
	}
	if strings.TrimSpace(src.DefaultProfile) != "" {
		dst.DefaultProfile = src.DefaultProfile
	}
	if src.WindowMessages > 0 {
		dst.WindowMessages = src.WindowMessages
	}
	if src.Profiles != nil {
		if dst.Profiles == nil {
			dst.Profiles = map[string]ProviderProfile{}
		}
		for code, prof := range src.Profiles {
			if strings.TrimSpace(code) != "" {
				dst.Profiles[code] = prof
			}
		}
	}
}

// SaveAtomic writes cfg to path atomically (temp file + rename).
func SaveAtomic(path string, cfg Config) error {
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Bootstrap copies ~/.ask/config.json to lorePath, rewriting topics_root to
// lore's own directory. Writes a minimal default if ask config is missing.
// No-op if lorePath already exists.
func Bootstrap(lorePath string) error {
	if _, err := os.Stat(lorePath); err == nil {
		return nil
	}
	home, _ := os.UserHomeDir()
	askPath := filepath.Join(home, ".ask", "config.json")
	cfg, err := loadExternalConfig(askPath)
	if err != nil {
		cfg = Config{
			TopicsRoot:     DefaultTopicsRoot(),
			DefaultTopic:   defaultTopicName,
			WindowMessages: defaultWindowMsgs,
			Profiles:       map[string]ProviderProfile{},
		}
	}
	cfg.TopicsRoot = DefaultTopicsRoot()
	return SaveAtomic(lorePath, cfg)
}

func loadExternalConfig(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, err
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]ProviderProfile{}
	}
	return cfg, nil
}

// ResolveProfile returns the profile for code (falls back to DefaultProfile).
func ResolveProfile(cfg Config, code string) (string, ProviderProfile, error) {
	if code == "" {
		code = cfg.DefaultProfile
	}
	if code == "" {
		return "", ProviderProfile{}, fmt.Errorf("no profile selected: set --profile or config default_profile")
	}
	p, ok := cfg.Profiles[code]
	if !ok {
		return "", ProviderProfile{}, fmt.Errorf("unknown profile %q", code)
	}
	if p.Provider == "" {
		return "", ProviderProfile{}, fmt.Errorf("profile %q missing provider", code)
	}
	if p.Model == "" {
		return "", ProviderProfile{}, fmt.Errorf("profile %q missing model", code)
	}
	return code, p, nil
}

// EffectiveTopic returns the active topic name.
func EffectiveTopic(cfg Config, flagTopic string) string {
	if flagTopic != "" {
		return flagTopic
	}
	if cfg.DefaultTopic != "" {
		return cfg.DefaultTopic
	}
	return defaultTopicName
}
