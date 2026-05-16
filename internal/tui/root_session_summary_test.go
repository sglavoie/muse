package tui

import (
	"context"
	"testing"
	"time"
)

// TestSessionTopArtistTracksMostPlayedArtist verifies that TrackChangedMsg
// transitions bump the session artist counter for the just-ended track, and
// SessionTopArtist surfaces the leader. Plays only count when the previous
// track's peak elapsed crosses the count-play threshold.
func TestSessionTopArtistTracksMostPlayedArtist(t *testing.T) {
	m := New(context.Background())
	m.playback.state = PlaybackPlaying
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a.flac", Artist: "Alpha"},
		{ID: 2, Hash: "h2", Path: "/b.flac", Artist: "Beta"},
		{ID: 3, Hash: "h3", Path: "/c.flac", Artist: "Alpha"},
	}

	// Simulate finishing track "h1" by Alpha; peak elapsed past the 30 s
	// threshold so the play counts.
	m.playback.currentHash = "h1"
	m.statusBar.artist = "Alpha"
	m.playback.peakElapsedSecs = 60
	m.playback.peakTotalSecs = 200
	next, _ := m.Update(TrackChangedMsg{Hash: "h2", Index: 1, Artist: "Beta", Title: "Two"})
	m = next.(Model)

	// Simulate finishing track "h2" by Beta and transitioning to "h3" (Alpha).
	m.playback.peakElapsedSecs = 60
	m.playback.peakTotalSecs = 200
	next, _ = m.Update(TrackChangedMsg{Hash: "h3", Index: 2, Artist: "Alpha", Title: "Three"})
	m = next.(Model)

	// Simulate finishing the second Alpha track too; this pushes Alpha to 2 plays.
	m.playback.peakElapsedSecs = 60
	m.playback.peakTotalSecs = 200
	next, _ = m.Update(TrackChangedMsg{Hash: "h1", Index: 0, Artist: "Alpha", Title: "One"})
	m = next.(Model)

	if got := m.SessionTopArtist(); got != "Alpha" {
		t.Fatalf("expected top artist Alpha (2 plays vs Beta 1), got %q", got)
	}
	if got := m.SessionTracksPlayed(); got != 3 {
		t.Fatalf("expected sessionTracksPlayed=3, got %d", got)
	}
}

// TestTrackChangedSetsPulseUntil verifies that receiving a TrackChangedMsg
// updates pulseUntil to a future time so View() inverts the now-playing row
// for the pulse window.
func TestTrackChangedSetsPulseUntil(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{{ID: 1, Hash: "h", Path: "/a.flac", Artist: "A"}}

	before := time.Now()
	next, _ := m.Update(TrackChangedMsg{Hash: "h", Index: 0, Artist: "A", Title: "T"})
	nm := next.(Model)

	if !nm.pulseUntil.After(before) {
		t.Fatalf("expected pulseUntil after the message was processed, got %v (before=%v)", nm.pulseUntil, before)
	}
	if delta := nm.pulseUntil.Sub(before); delta > 2*pulseDuration {
		t.Fatalf("expected pulseUntil close to now+pulseDuration, got delta %v", delta)
	}
}
