package statusbar

import (
	"slices"
	"time"
)

const messageHistoryLimit = 50

// Message is a past action result, retained independently of banner expiry.
type Message struct {
	Text     string
	At       time.Time
	Severity Severity
}

func (s *StatusBar) rememberMessage(msg string, severity Severity) {
	if msg == "" {
		return
	}
	// Allocate a fresh slice because StatusBar is copied with the Bubble Tea model.
	start := max(0, len(s.history)-messageHistoryLimit+1)
	next := make([]Message, len(s.history)-start+1)
	copy(next, s.history[start:])
	next[len(next)-1] = Message{Text: msg, At: time.Now(), Severity: severity}
	s.history = next
}

// RecentMessages returns oldest first; the caller owns the returned snapshot.
func (s StatusBar) RecentMessages() []Message { return slices.Clone(s.history) }
