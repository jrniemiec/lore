package core

import "time"

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
