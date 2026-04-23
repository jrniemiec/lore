package strategy

import "github.vom/jrniemiec/lore/core"

// SummarizeStrategy compresses older history into a rolling summary via a secondary LLM call.
type SummarizeStrategy struct {
	Summarizer core.Provider
}

func (s *SummarizeStrategy) Name() string { return "summarize" }

func (s *SummarizeStrategy) Apply(h *core.History, prompt string) []core.Message {
	// TODO: implement
	return nil
}
