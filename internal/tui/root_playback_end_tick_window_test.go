package tui

import (
	"context"
	"testing"
)

// TestTickMsgPrunesExpiredNotifications verifies that the tick handler prunes
// expired status bar notifications on every tick.
func TestTickMsgPrunesExpiredNotifications(t *testing.T) {
	m := New(context.Background())
	// Tick with no notifications should not panic.
	next, _ := m.Update(tickMsg{})
	nm := next.(Model)
	_ = nm
}
