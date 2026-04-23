package core

// Topic holds a loaded topic's data.
type Topic struct {
	Name         string
	SystemPrompt string
	History      History
}

// Store is the interface for topic persistence.
type Store interface {
	LoadTopic(name string) (*Topic, error)
	SaveHistory(name string, h History) error
	SaveSystem(name string, text string) error
	LoadSystem(name string) (string, error)
	LoadHistory(name string) (*History, error)
	ListTopics() ([]string, error)
	DeleteTopic(name string) error
	ClearHistory(name string) error
}
