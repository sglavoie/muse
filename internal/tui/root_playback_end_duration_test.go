package tui

import (
	"context"
	"testing"
)

// TestPlaybackEndedResetsDurationCorrected verifies that handlePlaybackEndedMsg
// resets the durationCorrected flag so that a subsequent exhaustion resume
// does not skip duration correction for the new track.
func TestPlaybackEndedResetsDurationCorrected(t *testing.T) {
	m := New(context.Background())
	m.playback.state = PlaybackPlaying
	m.playback.durationCorrected = true

	next, _ := m.Update(PlaybackEndedMsg{})
	nm := next.(Model)

	if nm.playback.durationCorrected {
		t.Fatal("expected durationCorrected=false after PlaybackEndedMsg")
	}
}

// TestRepeatAllResetsDurationCorrected verifies that when repeat-all wraps
// back, durationCorrected is reset so the restarted track gets its duration
// corrected by the tick handler.
func TestRepeatAllResetsDurationCorrected(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackPlaying
	m.playback.repeatMode = RepeatAll
	m.playback.durationCorrected = true
	m.queue.entries = []QueueEntry{
		{ID: 1, Path: "/a.flac", Artist: "A", Title: "One"},
	}

	next, _ := m.Update(PlaybackEndedMsg{})
	nm := next.(Model)

	if nm.playback.state != PlaybackPlaying {
		t.Fatalf("expected PlaybackPlaying, got %v", nm.playback.state)
	}
	if nm.playback.durationCorrected {
		t.Fatal("expected durationCorrected=false after repeat-all wrap")
	}
}

// TestPlaybackEndedConcurrentExtensionResetsDurationCorrected verifies that
// when the queue was concurrently extended and playback resumes,
// durationCorrected is reset for the new track.
func TestPlaybackEndedConcurrentExtensionResetsDurationCorrected(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackPlaying
	m.playback.currentIndex = 0
	m.playback.durationCorrected = true
	m.queue.entries = []QueueEntry{
		{ID: 1, Path: "/a.flac", Artist: "A", Title: "One"},
		{ID: 2, Path: "/b.flac", Artist: "B", Title: "Two"}, // pasted concurrently
	}

	next, _ := m.Update(PlaybackEndedMsg{})
	nm := next.(Model)

	if nm.playback.state != PlaybackPlaying {
		t.Fatalf("expected PlaybackPlaying (queue was extended), got %v", nm.playback.state)
	}
	if nm.playback.durationCorrected {
		t.Fatal("expected durationCorrected=false after concurrent extension resume")
	}
}
