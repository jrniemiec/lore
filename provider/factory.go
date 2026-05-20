package provider

import (
	"fmt"
	"strings"

	"github.com/jrniemiec/lore/config"
	"github.com/jrniemiec/lore/core"
)

// New creates a Provider from a ProviderProfile.
// When useShared is true it routes through the shared github.com/jrniemiec/llm
// module; otherwise it uses the built-in provider implementations.
func New(p config.ProviderProfile, useShared bool) (core.Provider, error) {
	if useShared {
		return newShared(p)
	}
	return newInternal(p)
}

func newInternal(p config.ProviderProfile) (core.Provider, error) {
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
