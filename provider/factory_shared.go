//go:build sharedllm

package provider

import (
	"context"
	"os"
	"strings"

	"github.com/jrniemiec/llm"
	"github.com/jrniemiec/lore/config"
	"github.com/jrniemiec/lore/core"
)

// New creates a Provider from a ProviderProfile using the shared llm module.
func New(p config.ProviderProfile) (core.Provider, error) {
	inner, err := llm.New(llm.ProviderConfig{
		Provider:        p.Provider,
		Model:           p.Model,
		Host:            p.Host,
		APIKey:          resolveAPIKey(p.Provider),
		MaxOutputTokens: p.MaxOutputTokens,
	})
	if err != nil {
		return nil, err
	}
	return &sharedAdapter{inner: inner}, nil
}

// resolveAPIKey reads the appropriate environment variable for each provider.
func resolveAPIKey(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "anthropic":
		if k := strings.TrimSpace(os.Getenv("LORE_ANTHROPIC_API_KEY")); k != "" {
			return k
		}
		return strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
	case "openai":
		if k := strings.TrimSpace(os.Getenv("LORE_OPENAI_API_KEY")); k != "" {
			return k
		}
		return strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	default:
		return ""
	}
}

// sharedAdapter bridges llm.Provider → core.Provider by converting the
// structurally identical Message and Usage types across package boundaries.
type sharedAdapter struct {
	inner llm.Provider
}

func (a *sharedAdapter) Name() string { return a.inner.Name() }

func (a *sharedAdapter) Chat(ctx context.Context, systemPrompt string, messages []core.Message) (string, core.Usage, error) {
	text, u, err := a.inner.Chat(ctx, systemPrompt, toLLMMessages(messages))
	return text, fromLLMUsage(u), err
}

func (a *sharedAdapter) ChatStream(
	ctx context.Context,
	systemPrompt string,
	messages []core.Message,
	onDelta func(string) error,
) (string, core.Usage, error) {
	text, u, err := a.inner.ChatStream(ctx, systemPrompt, toLLMMessages(messages), onDelta)
	return text, fromLLMUsage(u), err
}

func toLLMMessages(msgs []core.Message) []llm.Message {
	out := make([]llm.Message, len(msgs))
	for i, m := range msgs {
		out[i] = llm.Message{
			Role:    m.Role,
			Content: m.Content,
			Time:    m.Time,
			Profile: m.Profile,
		}
	}
	return out
}

func fromLLMUsage(u llm.Usage) core.Usage {
	return core.Usage{
		InputTokens:  u.InputTokens,
		OutputTokens: u.OutputTokens,
		Estimated:    u.Estimated,
	}
}
