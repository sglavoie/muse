package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/tui/playback"
)

// TestPlayFromIndexResultStaleAfterTrackJumpIsIgnored verifies that a late
// failure result from a previous PlayFromIndex call does not clobber the
// user's current playback state. Without this guard, an mpv error arriving
// after the user already jumped to a different track would reset everything
// to idle and show a misleading "Play failed" notification for the wrong
// track.
func TestPlayFromIndexResultStaleAfterTrackJumpIsIgnored(t *testing.T) {
	m := New(context.Background())
	// Pretend we are mid-playback on track 1 (the user just jumped here
	// from track 0).
	m.playback.state = playback.StatePlaying
	m.playback.currentIndex = 1
	m.playback.currentHash = "track-1-hash"
	m.statusBar.artist = "Current Artist"
	m.statusBar.title = "Current Title"

	// A late error for the abandoned track-0 attempt arrives.
	stale := playback.PlayFromIndexResultMsg{
		Index: 0,
		Path:  "/track-0.flac",
		Hash:  "track-0-hash",
		Err:   errStubPlay,
	}
	nm := m.handlePlayFromIndexResultMsg(stale)

	if nm.playback.state != playback.StatePlaying {
		t.Errorf("playback state = %v, want PlaybackPlaying (stale failure must not reset)", nm.playback.state)
	}
	if nm.playback.currentIndex != 1 {
		t.Errorf("currentIndex = %d, want 1 (stale failure must not reset)", nm.playback.currentIndex)
	}
	if nm.playback.currentHash != "track-1-hash" {
		t.Errorf("currentHash = %q, want %q", nm.playback.currentHash, "track-1-hash")
	}
	if nm.statusBar.artist != "Current Artist" {
		t.Errorf("artist = %q, want %q", nm.statusBar.artist, "Current Artist")
	}
	if nm.statusBar.title != "Current Title" {
		t.Errorf("title = %q, want %q", nm.statusBar.title, "Current Title")
	}
	if nm.statusBar.topNotification().msg != "" {
		t.Errorf("stale failure must not surface a notification, got %q", nm.statusBar.topNotification().msg)
	}
}

// TestPlayFromIndexResultStaleAfterQueueClearIsIgnored verifies that a late
// failure result arriving after the queue was cleared is also discarded
// rather than re-triggering an error notification for an entry that no
// longer exists.
func TestPlayFromIndexResultStaleAfterQueueClearIsIgnored(t *testing.T) {
	m := New(context.Background())
	// Queue was cleared between dispatch and arrival — currentIndex is -1
	// and currentHash is empty.
	m.playback.state = playback.StateIdle
	m.playback.currentIndex = -1
	m.playback.currentHash = ""

	stale := playback.PlayFromIndexResultMsg{
		Index: 2,
		Path:  "/gone.flac",
		Hash:  "gone-hash",
		Err:   errStubPlay,
	}
	nm := m.handlePlayFromIndexResultMsg(stale)

	if nm.playback.state != playback.StateIdle {
		t.Errorf("playback state = %v, want PlaybackIdle (unchanged)", nm.playback.state)
	}
	if nm.statusBar.topNotification().msg != "" {
		t.Errorf("stale failure after clear must not notify, got %q", nm.statusBar.topNotification().msg)
	}
}
