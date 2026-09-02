package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/playback"
)

// TestTrackEnqueuedAutoPlayResetsPlaybackFields verifies that the WasEmpty
// auto-play path (tryAutoPlay) clears stale elapsed/peak/durationCorrected
// from a previously-played track. Without resetting these, the new track's
// progress bar starts at the prior track's peak position until the player
// reports its first tick (and durationCorrected stays true, so the duration
// audit will skip the new track).
func TestTrackEnqueuedAutoPlayResetsPlaybackFields(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = playback.StateIdle
	// Simulate stale fields from a track that finished playing before the
	// queue was emptied.
	m.playback.elapsedSecs = 42
	m.playback.totalSecs = 180
	m.playback.peakElapsedSecs = 100
	m.playback.peakTotalSecs = 200
	m.playback.durationCorrected = true

	track := library.Track{Hash: "new-hash", Artist: "New", Title: "Track", Path: "/music/new.flac"}
	msg := libtui.TrackEnqueuedMsg{Track: track, QueueID: 1, WasEmpty: true, InsertAfter: -1}
	next, _ := m.Update(msg)
	nm := next.(Model)

	if nm.playback.state != playback.StatePlaying {
		t.Fatalf("expected PlaybackPlaying, got %v", nm.playback.state)
	}
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
		t.Error("durationCorrected should be false after auto-play")
	}
}

// TestBatchEnqueuedAutoPlayResetsPlaybackFields verifies the same reset for
// the batch enqueue WasEmpty auto-play path.
func TestBatchEnqueuedAutoPlayResetsPlaybackFields(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = playback.StateIdle
	m.playback.elapsedSecs = 42
	m.playback.totalSecs = 180
	m.playback.peakElapsedSecs = 100
	m.playback.peakTotalSecs = 200
	m.playback.durationCorrected = true

	entries := []QueueEntry{
		{ID: 10, Hash: "batch-a", Path: "/a.flac", Display: "A", Artist: "A", Title: "Song A"},
	}
	msg := BatchEnqueuedMsg{
		Entries:      entries,
		QueueIDs:     []int64{10},
		WasEmpty:     true,
		PlaylistName: "Test",
	}
	next, _ := m.Update(msg)
	nm := next.(Model)

	if nm.playback.state != playback.StatePlaying {
		t.Fatalf("expected PlaybackPlaying, got %v", nm.playback.state)
	}
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
		t.Error("durationCorrected should be false after auto-play")
	}
}
