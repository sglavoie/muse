package tui

import (
	"context"
	"testing"
)

// TestPlaybackEndedResumesWhenQueueExtended verifies that when the queue was
// concurrently extended (e.g. paste processed before PlaybackEndedMsg), playback
// resumes from the next track instead of going idle.
func TestPlaybackEndedResumesWhenQueueExtended(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackPlaying
	m.playback.currentIndex = 1 // was playing track at index 1
	m.queue.entries = []QueueEntry{
		{ID: 1, Path: "/a.flac", Artist: "A", Title: "One"},
		{ID: 2, Path: "/b.flac", Artist: "B", Title: "Two"},
		{ID: 3, Path: "/c.flac", Artist: "C", Title: "Three"}, // pasted concurrently
	}

	next, _ := m.Update(PlaybackEndedMsg{})
	nm := next.(Model)

	if nm.playback.state != PlaybackPlaying {
		t.Fatalf("expected PlaybackPlaying (queue was extended), got %v", nm.playback.state)
	}
	if len(p.playFromIndexCalls) != 1 || p.playFromIndexCalls[0] != 2 {
		t.Fatalf("expected PlayFromIndex(2), got calls=%v", p.playFromIndexCalls)
	}
	if nm.playback.queueExhausted {
		t.Fatal("queueExhausted should not be set when playback resumed")
	}
}

// TestPlaybackEndedSetsQueueExhausted verifies that when the queue truly runs
// out, the queueExhausted flag is set so a subsequent paste can auto-resume.
func TestPlaybackEndedSetsQueueExhausted(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackPlaying
	m.playback.currentIndex = 1 // last track
	m.queue.entries = []QueueEntry{
		{ID: 1, Path: "/a.flac", Artist: "A", Title: "One"},
		{ID: 2, Path: "/b.flac", Artist: "B", Title: "Two"},
	}

	next, _ := m.Update(PlaybackEndedMsg{})
	nm := next.(Model)

	if nm.playback.state != PlaybackIdle {
		t.Fatalf("expected PlaybackIdle, got %v", nm.playback.state)
	}
	if !nm.playback.queueExhausted {
		t.Fatal("expected queueExhausted=true when queue truly ran out")
	}
}

// TestPlaybackEndedStopAfterDoesNotSetQueueExhausted verifies that
// stop-after-current does not set the queueExhausted flag.
func TestPlaybackEndedStopAfterDoesNotSetQueueExhausted(t *testing.T) {
	p := &stubPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackPlaying
	m.playback.currentIndex = 0
	m.playback.stopAfterCurrent = true
	m.queue.entries = []QueueEntry{
		{ID: 1, Path: "/a.flac", Artist: "A", Title: "One"},
		{ID: 2, Path: "/b.flac", Artist: "B", Title: "Two"},
	}

	next, _ := m.Update(PlaybackEndedMsg{})
	nm := next.(Model)

	if nm.playback.queueExhausted {
		t.Fatal("queueExhausted should not be set for stop-after-current")
	}
}