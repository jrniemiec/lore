package strategy

import "github.vom/jrniemiec/lore/core"

// TailStrategy keeps the last N user turns of history.
type TailStrategy struct {
	MaxUserMessages int
}

func (s *TailStrategy) Name() string { return "tail" }

func (s *TailStrategy) Apply(h *core.History, _ string) []core.Message {
	// TODO: implement
	return nil
}
