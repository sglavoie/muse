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

	if nm.statusBar.TopNotification().Msg == "" {
		t.Fatal("expected a status bar notification for playlistAddErrorMsg, got empty")
	}
	if !strings.Contains(nm.statusBar.TopNotification().Msg, "disk full") {
		t.Errorf("notification should contain error text, got %q", nm.statusBar.TopNotification().Msg)
	}
	if nm.statusBar.TopNotification().ExpiresAt.Before(time.Now()) {
		t.Error("notification should expire in the future")
	}
}
