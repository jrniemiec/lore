package config

// ProviderProfile defines a named LLM configuration.
type ProviderProfile struct {
	Provider         string         `json:"provider"`
	Model            string         `json:"model"`
	MaxContextTokens int            `json:"max_context_tokens,omitempty"`
	ContextLimit     int            `json:"context_limit,omitempty"`
	MaxUserMessages  int            `json:"max_user_messages,omitempty"`
	MaxOutputTokens  int            `json:"max_output_tokens,omitempty"`
	Strategy         string         `json:"strategy,omitempty"`
	Color            string         `json:"color,omitempty"`
	Info             map[string]any `json:"info,omitempty"`
}

// Config is the top-level lore configuration.
type Config struct {
	TopicsRoot     string                     `json:"topics_root"`
	DefaultTopic   string                     `json:"default_topic,omitempty"`
	DefaultProfile string                     `json:"default_profile,omitempty"`
	WindowMessages int                        `json:"window_messages,omitempty"`
	Profiles       map[string]ProviderProfile `json:"profiles"`
}
