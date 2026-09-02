package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/tui/playback"
)

// TestTrackChangedClearsQueueExhausted verifies that receiving a
// TrackChangedMsg clears the queueExhausted flag.
func TestTrackChangedClearsQueueExhausted(t *testing.T) {
	m := New(context.Background())
	m.playback.queueExhausted = true
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a.flac", Artist: "A", Title: "One"},
	}

	next, _ := m.Update(playback.TrackChangedMsg{Hash: "h1", Artist: "A", Title: "One", Index: 0})
	nm := next.(Model)

	if nm.playback.queueExhausted {
		t.Fatal("expected queueExhausted=false after TrackChangedMsg")
	}
}

// TestTrackChangedRecordsPreviousListen verifies that handleTrackChangedMsg
// ends the outgoing track's listen using the accumulated listen time rather
// than the live elapsed position, which a gapless transition can clobber to the
// incoming track's near-zero position before the message is processed.
func TestTrackChangedRecordsPreviousListen(t *testing.T) {
	hr := &stubHistoryRecorder{}
	m := New(context.Background(), WithHistoryRecorder(hr))
	m.playback.state = playback.StatePlaying
	m.playback.currentHash = "old-hash"
	m = m.armPendingListen("old-hash", "A")
	// Simulate race: tick clobbered elapsedSecs to ~0, accumulation intact.
	m.playback.elapsedSecs = 0.5
	m.playback.totalSecs = 300.0
	m.playback.listenedSecs = 180.0
	m.playback.peakTotalSecs = 200.0
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "old-hash", Path: "/a.flac"},
		{ID: 2, Hash: "new-hash", Path: "/b.flac"},
	}

	next, cmd := m.Update(playback.TrackChangedMsg{Hash: "new-hash", Index: 1, Artist: "B", Title: "T2"})
	nm := next.(Model)

	if cmd == nil {
		t.Fatal("expected non-nil cmd for listen persistence")
	}
	drainCmd(cmd)
	if len(hr.calls) != 1 || hr.calls[0].Hash != "old-hash" {
		t.Fatalf("expected RecordListen for old-hash, got %+v", hr.calls)
	}
	if !hr.calls[0].Counted {
		t.Error("expected Counted=true for 180s of a 200s track")
	}
	if nm.playback.pending.hash != "new-hash" || !nm.playback.pending.armed {
		t.Errorf("expected the incoming track to be armed, got %+v", nm.playback.pending)
	}
}

// TestTrackChangedResetsPeak verifies that peak elapsed fields are reset when
// the track changes.
func TestTrackChangedResetsPeak(t *testing.T) {
	m := New(context.Background())
	m.playback.peakElapsedSecs = 180.0
	m.playback.peakTotalSecs = 200.0
	m.queue.entries = []QueueEntry{{ID: 1, Hash: "h2", Path: "/b.flac"}}

	next, _ := m.Update(playback.TrackChangedMsg{Hash: "h2", Index: 0, Artist: "B", Title: "T2"})
	nm := next.(Model)

	if nm.playback.peakElapsedSecs != 0 {
		t.Fatalf("expected peakElapsedSecs reset to 0, got %v", nm.playback.peakElapsedSecs)
	}
	if nm.playback.peakTotalSecs != 0 {
		t.Fatalf("expected peakTotalSecs reset to 0, got %v", nm.playback.peakTotalSecs)
	}
}

// TestTrackChangedDiscardedOnEmptyQueue verifies that a stale TrackChangedMsg
// arriving after queue clear is silently discarded rather than resurrecting
// playback state on an empty queue.
func TestTrackChangedDiscardedOnEmptyQueue(t *testing.T) {
	m := New(context.Background())
	m.playback.state = playback.StateIdle
	m.playback.currentIndex = -1
	m.playback.currentHash = ""
	// Queue is empty (was just cleared).
	m.queue.entries = nil

	next, cmd := m.Update(playback.TrackChangedMsg{Hash: "stale", Index: 3, Artist: "A", Title: "T"})
	nm := next.(Model)

	if nm.playback.state != playback.StateIdle {
		t.Fatalf("expected PlaybackIdle after stale TrackChangedMsg on empty queue, got %v", nm.playback.state)
	}
	if nm.playback.currentHash != "" {
		t.Fatalf("expected empty currentHash, got %q", nm.playback.currentHash)
	}
	if nm.playback.currentIndex != -1 {
		t.Fatalf("expected currentIndex=-1, got %d", nm.playback.currentIndex)
	}
	if cmd != nil {
		t.Fatal("expected nil cmd for discarded TrackChangedMsg")
	}
}

// TestTrackChangedHashNotInQueueUsesNegativeIndex verifies that when the mpv
// event carries a hash that does not match any queue entry (e.g. the track was
// dequeued between the mpv event and TUI processing), currentIndex is set to
// -1 rather than incorrectly falling back to 0.
func TestTrackChangedHashNotInQueueUsesNegativeIndex(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a.flac", Artist: "A", Title: "One"},
		{ID: 2, Hash: "h2", Path: "/b.flac", Artist: "B", Title: "Two"},
	}

	// Hash "gone" is not in the queue; index 5 is out of bounds.
	next, _ := m.Update(playback.TrackChangedMsg{Hash: "gone", Index: 5, Artist: "G", Title: "Gone"})
	nm := next.(Model)

	if nm.playback.currentIndex != -1 {
		t.Fatalf("expected currentIndex=-1 when hash not in queue, got %d", nm.playback.currentIndex)
	}
	// Status bar should still be updated with the message metadata.
	if nm.statusBar.Artist != "G" {
		t.Errorf("expected artist %q, got %q", "G", nm.statusBar.Artist)
	}
	if nm.statusBar.Title != "Gone" {
		t.Errorf("expected title %q, got %q", "Gone", nm.statusBar.Title)
	}
}

// TestTrackChangedClampsOutOfBoundsIndex verifies that when the mpv event
// carries an index beyond the current queue length (e.g. dequeue raced with
// gapless transition), the handler falls back to a hash-based lookup.
func TestTrackChangedClampsOutOfBoundsIndex(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a.flac"},
		{ID: 2, Hash: "h2", Path: "/b.flac"},
	}

	// Index 5 is out of bounds; hash "h2" is at position 1.
	next, _ := m.Update(playback.TrackChangedMsg{Hash: "h2", Index: 5, Artist: "B", Title: "T2"})
	nm := next.(Model)

	if nm.playback.currentIndex != 1 {
		t.Fatalf("expected currentIndex=1 (hash lookup fallback), got %d", nm.playback.currentIndex)
	}
	if nm.playback.currentHash != "h2" {
		t.Fatalf("expected currentHash=%q, got %q", "h2", nm.playback.currentHash)
	}
}
