package tui

import (
	"context"
	"testing"
)

// TestDequeuePlayingTrackClearsStatusBar verifies that dequeuing the currently
// playing track stops playback and clears the status bar fields.
func TestDequeuePlayingTrackClearsStatusBar(t *testing.T) {
	p := &stubPlayerQueueRemover{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "abc123"
	m.statusBar.artist = "Artist"
	m.statusBar.title = "Title"
	m.statusBar.playlistSource = "Queue"
	m.playback.elapsedSecs = 42.0
	m.playback.totalSecs = 180.0
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A", Hash: "abc123"},
		{ID: 20, Path: "/music/b.flac", Display: "B", Hash: "other"},
	}

	next, _ := m.Update(DequeueTrackMsg{Index: 0})
	nm := next.(Model)

	if nm.playback.state != PlaybackIdle {
		t.Errorf("playbackState = %v, want PlaybackIdle", nm.playback.state)
	}
	if nm.statusBar.artist != "" {
		t.Errorf("statusBar.artist = %q, want empty", nm.statusBar.artist)
	}
	if nm.statusBar.title != "" {
		t.Errorf("statusBar.title = %q, want empty", nm.statusBar.title)
	}
	if nm.statusBar.playlistSource != "" {
		t.Errorf("statusBar.playlistSource = %q, want empty", nm.statusBar.playlistSource)
	}
	if nm.playback.currentHash != "" {
		t.Errorf("currentTrackHash = %q, want empty", nm.playback.currentHash)
	}
	if nm.playback.elapsedSecs != 0 {
		t.Errorf("elapsedSecs = %v, want 0", nm.playback.elapsedSecs)
	}
	if nm.playback.totalSecs != 0 {
		t.Errorf("totalSecs = %v, want 0", nm.playback.totalSecs)
	}
	if p.stopCalls != 1 {
		t.Errorf("Stop must be called once, got %d calls", p.stopCalls)
	}
}

// TestDequeueDuplicateHashKeepsPlayback verifies that removing a queue entry
// whose hash matches the currently-playing track does NOT stop playback when
// another entry with the same hash remains in the queue.
func TestDequeueDuplicateHashKeepsPlayback(t *testing.T) {
	p := &stubPlayerQueueRemover{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "abc123"
	m.statusBar.artist = "Artist"
	m.statusBar.title = "Title"
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A", Hash: "abc123"},
		{ID: 20, Path: "/music/a.flac", Display: "A (dup)", Hash: "abc123"},
		{ID: 30, Path: "/music/b.flac", Display: "B", Hash: "other"},
	}

	next, _ := m.Update(DequeueTrackMsg{Index: 0})
	nm := next.(Model)

	if nm.playback.state != PlaybackPlaying {
		t.Errorf("playbackState = %v, want PlaybackPlaying (another entry with same hash remains)", nm.playback.state)
	}
	if nm.playback.currentHash != "abc123" {
		t.Errorf("currentHash = %q, want %q", nm.playback.currentHash, "abc123")
	}
	if nm.statusBar.artist != "Artist" {
		t.Errorf("statusBar.artist = %q, want %q", nm.statusBar.artist, "Artist")
	}
	if p.stopCalls != 0 {
		t.Errorf("Stop should not be called when another instance of the hash remains, got %d calls", p.stopCalls)
	}
}

// TestDequeueLastInstanceOfHashStopsPlayback verifies that removing the only
// queue entry with the currently-playing hash stops playback.
func TestDequeueLastInstanceOfHashStopsPlayback(t *testing.T) {
	p := &stubPlayerQueueRemover{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "abc123"
	m.statusBar.artist = "Artist"
	m.statusBar.title = "Title"
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A", Hash: "abc123"},
		{ID: 20, Path: "/music/b.flac", Display: "B", Hash: "other"},
	}

	next, _ := m.Update(DequeueTrackMsg{Index: 0})
	nm := next.(Model)

	if nm.playback.state != PlaybackIdle {
		t.Errorf("playbackState = %v, want PlaybackIdle", nm.playback.state)
	}
	if nm.playback.currentHash != "" {
		t.Errorf("currentHash = %q, want empty", nm.playback.currentHash)
	}
}
