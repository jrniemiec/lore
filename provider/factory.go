package provider

import (
	"fmt"
	"strings"

	"github.vom/jrniemiec/lore/config"
	"github.vom/jrniemiec/lore/core"
)

// New creates a Provider from a ProviderProfile.
func New(p config.ProviderProfile) (core.Provider, error) {
	switch strings.ToLower(strings.TrimSpace(p.Provider)) {
	case "anthropic":
		return NewAnthropicProvider(p.Model, p.MaxOutputTokens)
	case "openai":
		return NewOpenAIProvider(p.Model)
	case "ollama":
		return NewOllamaProvider(p.Host, p.Model)
	default:
		return nil, fmt.Errorf("unknown provider %q", p.Provider)
	}
}
