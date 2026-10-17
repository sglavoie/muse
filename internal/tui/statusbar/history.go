package statusbar

import (
	"slices"
	"strings"
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

// MarkMessagesRead acknowledges the history currently presented to the user.
// Later errors get their own unread indicator, even while the overlay is open.
func (s *StatusBar) MarkMessagesRead() {
	s.hasUnreadErrors = false
	kept := make([]TransientNotification, 0, len(s.Notifications))
	for _, n := range s.Notifications {
		if n.Severity != SeverityError {
			kept = append(kept, n)
		}
	}
	s.Notifications = kept
}

// DismissErrorNotifications retires failures for an operation that recovered.
// History and unread state remain until the user inspects Recent Messages.
func (s *StatusBar) DismissErrorNotifications(prefix string) {
	kept := make([]TransientNotification, 0, len(s.Notifications))
	for _, n := range s.Notifications {
		if n.Severity != SeverityError || !strings.HasPrefix(n.Msg, prefix) {
			kept = append(kept, n)
		}
	}
	s.Notifications = kept
}
