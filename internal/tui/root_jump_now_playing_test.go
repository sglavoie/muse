package tui

import (
	"context"
	"testing"
)

// TestJumpToNowPlayingWithNothingPlayingUsesSharedWording guards that the
// jump-to-now-playing command reports an idle player with the same "No track
// playing" notice the playback keys use, not a second wording.
func TestJumpToNowPlayingWithNothingPlayingUsesSharedWording(t *testing.T) {
	m := New(context.Background())
	m, _ = m.handleJumpToNowPlayingMsg()
	if got := m.statusBar.TopNotification().Msg; got != "No track playing" {
		t.Errorf("notice = %q, want %q", got, "No track playing")
	}
}
