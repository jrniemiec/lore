package strategy

import "github.vom/jrniemiec/lore/core"

// TokenBudgetStrategy keeps the most recent messages that fit within a token budget.
type TokenBudgetStrategy struct {
	MaxContextTokens int
	ReserveTokens    int
}

func (s *TokenBudgetStrategy) Name() string { return "token-budget" }

func (s *TokenBudgetStrategy) Apply(h *core.History, prompt string) []core.Message {
	// TODO: implement
	return nil
}
