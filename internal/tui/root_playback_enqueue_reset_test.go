package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/playback"
)

// TestTrackEnqueuedExhaustedResumeResetsPlaybackFields verifies that when a
// track is enqueued after queue exhaustion and playback resumes, the stale
// elapsed/peak/durationCorrected fields from the previous track are cleared
// so the progress bar and duration correction logic start fresh.
func TestTrackEnqueuedExhaustedResumeResetsPlaybackFields(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = playback.StateIdle
	m.playback.queueExhausted = true
	m.playback.currentIndex = -1
	m.playback.currentHash = ""
	// Simulate stale fields from the previous track that ended.
	m.playback.elapsedSecs = 42
	m.playback.totalSecs = 180
	m.playback.peakElapsedSecs = 100
	m.playback.peakTotalSecs = 200
	m.playback.durationCorrected = true
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "old-hash", Path: "/music/old.flac", Display: "Old"},
	}

	track := library.Track{Hash: "new-hash", Artist: "New", Title: "Track", Path: "/music/new.flac"}
	msg := libtui.TrackEnqueuedMsg{Track: track, QueueID: 2, WasEmpty: false, InsertAfter: -1}
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
		t.Error("durationCorrected should be false after exhaustion resume")
	}
}

// TestBatchEnqueuedExhaustedResumeResetsPlaybackFields verifies the same reset
// for batch enqueue after queue exhaustion.
func TestBatchEnqueuedExhaustedResumeResetsPlaybackFields(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = playback.StateIdle
	m.playback.queueExhausted = true
	m.playback.currentIndex = -1
	m.playback.currentHash = ""
	m.playback.elapsedSecs = 42
	m.playback.totalSecs = 180
	m.playback.peakElapsedSecs = 100
	m.playback.peakTotalSecs = 200
	m.playback.durationCorrected = true
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "old-hash", Path: "/music/old.flac", Display: "Old"},
	}

	entries := []QueueEntry{
		{ID: 10, Hash: "batch-a", Path: "/a.flac", Display: "A", Artist: "A", Title: "Song A"},
	}
	msg := BatchEnqueuedMsg{
		Entries:      entries,
		QueueIDs:     []int64{10},
		WasEmpty:     false,
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
		t.Error("durationCorrected should be false after exhaustion resume")
	}
}
