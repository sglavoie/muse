package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestPlaylistAddErrorMsgShowsNotification verifies that playlistAddErrorMsg
// sets a transient error notification on the status bar.
func TestPlaylistAddErrorMsgShowsNotification(t *testing.T) {
	m := New(context.Background())
	next, _ := m.Update(playlistAddErrorMsg{err: errors.New("disk full")})
	nm := next.(Model)

	if nm.statusBar.topNotification().msg == "" {
		t.Fatal("expected a status bar notification for playlistAddErrorMsg, got empty")
	}
	if !strings.Contains(nm.statusBar.topNotification().msg, "disk full") {
		t.Errorf("notification should contain error text, got %q", nm.statusBar.topNotification().msg)
	}
	if nm.statusBar.topNotification().expiresAt.Before(time.Now()) {
		t.Error("notification should expire in the future")
	}
}
