package core

import "time"

const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Message is a single turn in a conversation.
type Message struct {
	Role    string    `json:"role"`
	Content string    `json:"content"`
	Time    time.Time `json:"time,omitempty"`
}

// History is the full append-only message log for a topic.
type History struct {
	Msgs []Message `json:"messages"`
}

func NewHistory() *History {
	return &History{Msgs: []Message{}}
}

func (h *History) Append(role, content string) {
	h.Msgs = append(h.Msgs, Message{
		Role:    role,
		Content: content,
		Time:    time.Now(),
	})
}

// ToMessages returns messages covering the last maxUserMessages user turns.
func (h *History) ToMessages(maxUserMessages int) []Message {
	if h == nil || len(h.Msgs) == 0 {
		return nil
	}
	userCount := 0
	start := 0
	for i := len(h.Msgs) - 1; i >= 0; i-- {
		if h.Msgs[i].Role == RoleUser {
			userCount++
			if userCount >= maxUserMessages {
				start = i
				break
			}
		}
	}
	return h.Msgs[start:]
}
