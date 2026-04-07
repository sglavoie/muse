package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	libtui "github.com/sglavoie/muse/internal/tui/library"
)

// TestPlayNextInsertsAfterCurrentTrack verifies that handleTrackEnqueuedMsg
// inserts the new track immediately after the currently playing track when
// InsertAfter is set to a valid index.
func TestPlayNextInsertsAfterCurrentTrack(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Title: "First", Artist: "A"},
		{ID: 2, Hash: "h2", Title: "Second", Artist: "B"},
		{ID: 3, Hash: "h3", Title: "Third", Artist: "C"},
	}
	m.playback.currentHash = "h1"

	msg := libtui.TrackEnqueuedMsg{
		Track:       library.Track{Hash: "h_new", Title: "New", Artist: "X", Path: "/new.mp3"},
		QueueID:     99,
		InsertAfter: 1,
	}
	m = m.handleTrackEnqueuedMsg(msg)

	if len(m.queue.entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(m.queue.entries))
	}
	if m.queue.entries[1].Hash != "h_new" {
		t.Errorf("expected new track at index 1, got hash %q", m.queue.entries[1].Hash)
	}
	if m.queue.entries[0].Hash != "h1" {
		t.Errorf("index 0: expected h1, got %q", m.queue.entries[0].Hash)
	}
	if m.queue.entries[2].Hash != "h2" {
		t.Errorf("index 2: expected h2, got %q", m.queue.entries[2].Hash)
	}
	if m.queue.entries[3].Hash != "h3" {
		t.Errorf("index 3: expected h3, got %q", m.queue.entries[3].Hash)
	}
}

// TestPlayNextEmptyQueueTriggersAutoPlay verifies that play-next into an empty
// queue triggers auto-play (WasEmpty + PlaybackIdle).
func TestPlayNextEmptyQueueTriggersAutoPlay(t *testing.T) {
	m := New(context.Background())
	m.playback.state = PlaybackIdle

	msg := libtui.TrackEnqueuedMsg{
		Track:       library.Track{Hash: "h1", Title: "Solo", Artist: "A", Path: "/solo.mp3"},
		QueueID:     1,
		WasEmpty:    true,
		InsertAfter: 0,
	}
	m = m.handleTrackEnqueuedMsg(msg)

	if len(m.queue.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(m.queue.entries))
	}
	if m.queue.entries[0].Hash != "h1" {
		t.Errorf("expected h1 at index 0, got %q", m.queue.entries[0].Hash)
	}
}

// TestPlayNextNothingPlayingInsertsAtFront verifies that playNextIndex returns
// 0 when nothing is playing, causing the track to be inserted at the front.
func TestPlayNextNothingPlayingInsertsAtFront(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Title: "First", Artist: "A"},
		{ID: 2, Hash: "h2", Title: "Second", Artist: "B"},
	}
	m.playback.currentHash = ""

	idx := m.playNextIndex()
	if idx != 0 {
		t.Fatalf("expected playNextIndex=0 when nothing playing, got %d", idx)
	}

	msg := libtui.TrackEnqueuedMsg{
		Track:       library.Track{Hash: "h_new", Title: "New", Artist: "X", Path: "/new.mp3"},
		QueueID:     99,
		InsertAfter: idx,
	}
	m = m.handleTrackEnqueuedMsg(msg)

	if len(m.queue.entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(m.queue.entries))
	}
	if m.queue.entries[0].Hash != "h_new" {
		t.Errorf("expected new track at front, got %q", m.queue.entries[0].Hash)
	}
}

// TestPlayNextAppendFallback verifies that InsertAfter=-1 causes the normal
// append-to-end behavior.
func TestPlayNextAppendFallback(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Title: "First", Artist: "A"},
	}

	msg := libtui.TrackEnqueuedMsg{
		Track:       library.Track{Hash: "h_new", Title: "New", Artist: "X", Path: "/new.mp3"},
		QueueID:     99,
		InsertAfter: -1,
	}
	m = m.handleTrackEnqueuedMsg(msg)

	if len(m.queue.entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(m.queue.entries))
	}
	if m.queue.entries[1].Hash != "h_new" {
		t.Errorf("expected new track at end (index 1), got %q", m.queue.entries[1].Hash)
	}
}

// TestPlayNextUndoRemovesInsertedTrack verifies that undoing a play-next
// removes the inserted track by DB ID regardless of its position.
func TestPlayNextUndoRemovesInsertedTrack(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Title: "First", Artist: "A"},
		{ID: 2, Hash: "h2", Title: "Second", Artist: "B"},
	}
	m.playback.currentHash = "h1"

	msg := libtui.TrackEnqueuedMsg{
		Track:       library.Track{Hash: "h_new", Title: "New", Artist: "X", Path: "/new.mp3"},
		QueueID:     99,
		InsertAfter: 1,
	}
	m = m.handleTrackEnqueuedMsg(msg)

	if len(m.queue.entries) != 3 {
		t.Fatalf("before undo: expected 3 entries, got %d", len(m.queue.entries))
	}

	m = m.handleUndo()

	if len(m.queue.entries) != 2 {
		t.Fatalf("after undo: expected 2 entries, got %d", len(m.queue.entries))
	}
	for _, e := range m.queue.entries {
		if e.Hash == "h_new" {
			t.Error("after undo: inserted track should have been removed")
		}
	}
}

// TestPlayNextIndexReturnsCorrectPosition verifies playNextIndex for various
// queue states.
func TestPlayNextIndexReturnsCorrectPosition(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1"},
		{ID: 2, Hash: "h2"},
		{ID: 3, Hash: "h3"},
	}

	tests := []struct {
		name         string
		currentIndex int
		want         int
	}{
		{"nothing playing", -1, 0},
		{"playing first", 0, 1},
		{"playing middle", 1, 2},
		{"playing last", 2, 3},
		{"index out of range", 99, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m.playback.currentIndex = tt.currentIndex
			got := m.playNextIndex()
			if got != tt.want {
				t.Errorf("playNextIndex()=%d, want %d", got, tt.want)
			}
		})
	}
}
