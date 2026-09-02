package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/tui/playback"
)

// TestClearQueueMsgStopsPlaybackAndTransitionsToIdle verifies that handling
// ClearQueueMsg calls Stop() on the player exactly once and transitions
// playbackState to PlaybackIdle (not PlaybackStopped).
func TestClearQueueMsgStopsPlaybackAndTransitionsToIdle(t *testing.T) {
	p := &stubCommandPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = playback.StatePlaying
	m.queue.entries = []QueueEntry{
		{ID: 1, Path: "/music/a.flac", Display: "A"},
	}

	next, _ := m.Update(ClearQueueMsg{})
	nm := next.(Model)

	if nm.playback.state != playback.StateIdle {
		t.Fatalf("ClearQueueMsg: expected PlaybackIdle, got %v", nm.playback.state)
	}
	if p.stopCalls != 1 {
		t.Fatalf("ClearQueueMsg: expected 1 Stop() call, got %d", p.stopCalls)
	}
}

// TestClearQueueMsgResetsScrollOffset verifies that ClearQueueMsg resets the
// queue scrollOffset to 0 so that entries added after a clear are visible
// from the first row instead of being off-screen at a stale scroll position.
func TestClearQueueMsgResetsScrollOffset(t *testing.T) {
	p := &stubCommandPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = playback.StatePlaying
	m.queue.entries = make([]QueueEntry, 50)
	for i := range m.queue.entries {
		m.queue.entries[i] = QueueEntry{ID: int64(i + 1), Display: "Track"}
	}
	m.queue.scrollOffset = 30

	next, _ := m.Update(ClearQueueMsg{})
	nm := next.(Model)

	if nm.queue.scrollOffset != 0 {
		t.Fatalf("ClearQueueMsg: expected scrollOffset=0, got %d", nm.queue.scrollOffset)
	}
}

// TestClearQueueMsgExitsVisualMode verifies that clearing the queue exits
// visual selection mode. Leaving visual.active=true after the queue is
// emptied corrupts the next selection cycle because the stored anchor and
// snapshot reference indices into a now-empty slice.
func TestClearQueueMsgExitsVisualMode(t *testing.T) {
	p := &stubCommandPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{ID: 1, Path: "/a.flac", Display: "A"},
		{ID: 2, Path: "/b.flac", Display: "B"},
	}
	m = enterVisualMode(m)
	if !m.visual.active {
		t.Fatalf("precondition failed: visual mode must be active before clear")
	}

	next, _ := m.Update(ClearQueueMsg{})
	nm := next.(Model)

	if nm.visual.active {
		t.Errorf("ClearQueueMsg must exit visual mode; visual.active=%v", nm.visual.active)
	}
	if nm.visual.anchor != 0 {
		t.Errorf("ClearQueueMsg must reset visual anchor; got %d", nm.visual.anchor)
	}
}

// TestClearQueueMsgResetsRadioEnqueuePending verifies that clearing the queue
// resets radio.enqueuePending so the next track change after a fresh enqueue
// can fire a new radio batch. Without this reset a clear-while-pending would
// leave the flag stuck=true until the in-flight batch arrived, suppressing
// any radio low-water-mark trigger during that window.
func TestClearQueueMsgResetsRadioEnqueuePending(t *testing.T) {
	p := &stubCommandPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.queue.entries = []QueueEntry{
		{ID: 1, Path: "/a.flac", Display: "A"},
	}
	m.radio.enabled = true
	m.radio.enqueuePending = true

	next, _ := m.Update(ClearQueueMsg{})
	nm := next.(Model)

	if nm.radio.enqueuePending {
		t.Error("ClearQueueMsg must reset radio.enqueuePending to false")
	}
}
