package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// TestClearQueueClearsStatusBarTrackInfo verifies that ClearQueueMsg resets the
// status bar artist, title, playlistSource, and currentTrackHash to empty strings.
func TestClearQueueClearsStatusBarTrackInfo(t *testing.T) {
	sp := &stubCommandPlayer{}
	m := New(context.Background(), WithPlayer(sp))
	m.statusBar.Artist = "Old Artist"
	m.statusBar.Title = "Old Title"
	m.statusBar.PlaylistSource = "Old Playlist"
	m.playback.currentHash = "oldhash"
	m.playback.state = playback.StatePlaying
	m.queue.Entries = []queue.Entry{{ID: 10, Path: "/a.flac", Display: "A"}}

	next, _ := m.Update(queue.ClearQueueMsg{})
	nm := next.(Model)

	if nm.statusBar.Artist != "" {
		t.Errorf("statusBar.artist = %q, want empty after ClearQueueMsg", nm.statusBar.Artist)
	}
	if nm.statusBar.Title != "" {
		t.Errorf("statusBar.title = %q, want empty after ClearQueueMsg", nm.statusBar.Title)
	}
	if nm.statusBar.PlaylistSource != "" {
		t.Errorf("statusBar.playlistSource = %q, want empty after ClearQueueMsg", nm.statusBar.PlaylistSource)
	}
	if nm.playback.currentHash != "" {
		t.Errorf("currentTrackHash = %q, want empty after ClearQueueMsg", nm.playback.currentHash)
	}
}

// TestPlaybackEndedClearsStatusBarTrackInfo verifies that PlaybackEndedMsg
// resets the status bar artist, title, playlistSource, and currentTrackHash.
func TestPlaybackEndedClearsStatusBarTrackInfo(t *testing.T) {
	m := New(context.Background(), WithPlayer(&stubPlayer{}))
	m.statusBar.Artist = "Old Artist"
	m.statusBar.Title = "Old Title"
	m.statusBar.PlaylistSource = "Queue"
	m.playback.currentHash = "oldhash"
	m.playback.state = playback.StatePlaying
	m.playback.repeatMode = playback.RepeatOff

	next, _ := m.Update(playback.PlaybackEndedMsg{})
	nm := next.(Model)

	if nm.playback.state != playback.StateIdle {
		t.Errorf("playbackState = %v, want PlaybackIdle", nm.playback.state)
	}
	if nm.statusBar.Artist != "" {
		t.Errorf("statusBar.artist = %q, want empty after PlaybackEndedMsg", nm.statusBar.Artist)
	}
	if nm.statusBar.Title != "" {
		t.Errorf("statusBar.title = %q, want empty after PlaybackEndedMsg", nm.statusBar.Title)
	}
	if nm.statusBar.PlaylistSource != "" {
		t.Errorf("statusBar.playlistSource = %q, want empty after PlaybackEndedMsg", nm.statusBar.PlaylistSource)
	}
	if nm.playback.currentHash != "" {
		t.Errorf("currentTrackHash = %q, want empty after PlaybackEndedMsg", nm.playback.currentHash)
	}
}

// TestTickDoesNotUpdateElapsedWhenIdle verifies that the tick handler does not
// overwrite elapsedSecs/totalSecs when playback is idle.
func TestTickDoesNotUpdateElapsedWhenIdle(t *testing.T) {
	p := &stubElapsedReader{elapsed: 99.0, total: 300.0}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = playback.StateIdle
	m.playback.elapsedSecs = 0
	m.playback.totalSecs = 0

	next, _ := m.Update(tickMsg{})
	nm := next.(Model)

	if nm.playback.elapsedSecs != 0 {
		t.Errorf("elapsedSecs = %v, want 0 when idle", nm.playback.elapsedSecs)
	}
	if nm.playback.totalSecs != 0 {
		t.Errorf("totalSecs = %v, want 0 when idle", nm.playback.totalSecs)
	}
}

// TestClearQueueForReplaceClearsFilter verifies that clearQueueForReplace
// deactivates the queue filter so the replacement entry is always visible.
func TestClearQueueForReplaceClearsFilter(t *testing.T) {
	m := New(context.Background())
	m.queue.Entries = []queue.Entry{
		{ID: 1, Path: "/a.flac", Hash: "aaa", Display: "A", Artist: "Art", Title: "A"},
		{ID: 2, Path: "/b.flac", Hash: "bbb", Display: "B", Artist: "Brt", Title: "B"},
	}
	m.queue.Filter.Active = true
	m.queue.Filter.Text = "art"

	m = m.clearQueueForReplace()

	if m.queue.Filter.Active {
		t.Error("filter.Active must be false after clearQueueForReplace")
	}
	if m.queue.Filter.Text != "" {
		t.Errorf("filter.Text must be empty after clearQueueForReplace, got %q", m.queue.Filter.Text)
	}
	if len(m.queue.Entries) != 0 {
		t.Errorf("queue entries must be empty after clearQueueForReplace, got %d", len(m.queue.Entries))
	}
}

// TestPlayQueueIndexMsgClearsStateOnFailure verifies that when PlayFromIndex
// fails, the TUI clears stale status bar and playback fields via the async
// playFromIndexResultMsg round-trip.
func TestPlayQueueIndexMsgClearsStateOnFailure(t *testing.T) {
	p := &stubPlayer{playFromIndexErr: errStubPlay}
	m := New(context.Background(), WithPlayer(p))
	m.queue.Entries = []queue.Entry{{Path: "/music/a.flac", Hash: "aaa", Display: "A", Artist: "Art", Title: "Ttl"}}
	m.playback.state = playback.StatePlaying
	m.playback.currentHash = "old-hash"
	m.playback.elapsedSecs = 42
	m.playback.totalSecs = 180
	m.statusBar.Artist = "Old Artist"
	m.statusBar.Title = "Old Title"
	m.statusBar.PlaylistSource = "Old Playlist"

	next, cmd := m.Update(playback.PlayQueueIndexMsg{Index: 0})
	if cmd == nil {
		t.Fatal("PlayQueueIndexMsg must return an async Cmd")
	}
	// Drive the async Cmd and route its result back through Update so
	// handlePlayFromIndexResultMsg can reset playback state on failure.
	resultMsg := cmd()
	if _, ok := resultMsg.(playback.PlayFromIndexResultMsg); !ok {
		t.Fatalf("expected playFromIndexResultMsg, got %T", resultMsg)
	}
	next, _ = next.(Model).Update(resultMsg)
	nm := next.(Model)

	if nm.playback.state != playback.StateIdle {
		t.Errorf("expected PlaybackIdle, got %v", nm.playback.state)
	}
	if nm.playback.currentHash != "" {
		t.Errorf("expected empty currentHash, got %q", nm.playback.currentHash)
	}
	if nm.playback.elapsedSecs != 0 {
		t.Errorf("expected elapsedSecs=0, got %f", nm.playback.elapsedSecs)
	}
	if nm.playback.totalSecs != 0 {
		t.Errorf("expected totalSecs=0, got %f", nm.playback.totalSecs)
	}
	if nm.statusBar.Artist != "" {
		t.Errorf("expected empty artist, got %q", nm.statusBar.Artist)
	}
	if nm.statusBar.Title != "" {
		t.Errorf("expected empty title, got %q", nm.statusBar.Title)
	}
	if nm.statusBar.PlaylistSource != "" {
		t.Errorf("expected empty playlistSource, got %q", nm.statusBar.PlaylistSource)
	}
	if nm.statusBar.TopNotification().Msg == "" {
		t.Error("expected error notification")
	}
}

// TestPlayQueueIndexResetsPlaybackFields verifies that handlePlayQueueIndexMsg
// resets elapsed, total, peak, and durationCorrected so stale values from the
// previous track do not leak into the optimistic UI update window.
func TestPlayQueueIndexResetsPlaybackFields(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.queue.Entries = []queue.Entry{
		{ID: 1, Path: "/a.flac", Hash: "aaa", Display: "A", Artist: "Art1", Title: "T1"},
		{ID: 2, Path: "/b.flac", Hash: "bbb", Display: "B", Artist: "Art2", Title: "T2"},
	}
	// Simulate mid-playback state from the first track.
	m.playback.state = playback.StatePlaying
	m.playback.currentHash = "aaa"
	m.playback.currentIndex = 0
	m.playback.elapsedSecs = 120
	m.playback.totalSecs = 300
	m.playback.peakElapsedSecs = 120
	m.playback.peakTotalSecs = 300
	m.playback.durationCorrected = true

	// User jumps to the second track.
	next, _ := m.Update(playback.PlayQueueIndexMsg{Index: 1})
	nm := next.(Model)

	if nm.playback.elapsedSecs != 0 {
		t.Errorf("elapsedSecs = %v, want 0", nm.playback.elapsedSecs)
	}
	if nm.playback.totalSecs != 0 {
		t.Errorf("totalSecs = %v, want 0", nm.playback.totalSecs)
	}
	if nm.playback.peakElapsedSecs != 0 {
		t.Errorf("peakElapsedSecs = %v, want 0", nm.playback.peakElapsedSecs)
	}
	if nm.playback.peakTotalSecs != 0 {
		t.Errorf("peakTotalSecs = %v, want 0", nm.playback.peakTotalSecs)
	}
	if nm.playback.durationCorrected {
		t.Error("durationCorrected should be false")
	}
	if nm.playback.currentHash != "bbb" {
		t.Errorf("currentHash = %q, want %q", nm.playback.currentHash, "bbb")
	}
}
