package player

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// TestManager_PlayFromIndex_NegativeIndex verifies that PlayFromIndex returns
// ErrPlaybackFailed when given a negative index.
func TestManager_PlayFromIndex_NegativeIndex(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		ctx:    ctx,
		cancel: cancel,
		queue: []QueueTrack{
			{Path: "/a.flac", Hash: "h0", QueueID: 1},
		},
	}
	defer m.Close()

	err := m.PlayFromIndex(-1)
	if err == nil {
		t.Fatal("PlayFromIndex(-1): expected error, got nil")
	}
	if !errors.Is(err, ErrPlaybackFailed) {
		t.Errorf("PlayFromIndex(-1): want ErrPlaybackFailed, got %v", err)
	}
}

// TestManager_PlayFromIndex_EmptyQueue verifies that PlayFromIndex returns
// ErrPlaybackFailed when the queue is empty.
func TestManager_PlayFromIndex_EmptyQueue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel}
	defer m.Close()

	err := m.PlayFromIndex(0)
	if err == nil {
		t.Fatal("PlayFromIndex(0) on empty queue: expected error, got nil")
	}
	if !errors.Is(err, ErrPlaybackFailed) {
		t.Errorf("PlayFromIndex(0) on empty queue: want ErrPlaybackFailed, got %v", err)
	}
}

// TestManager_PlayFromIndex_IndexBeyondEnd verifies that PlayFromIndex returns
// ErrPlaybackFailed when the index equals len(queue).
func TestManager_PlayFromIndex_IndexBeyondEnd(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		ctx:    ctx,
		cancel: cancel,
		queue: []QueueTrack{
			{Path: "/a.flac", Hash: "h0", QueueID: 1},
			{Path: "/b.flac", Hash: "h1", QueueID: 2},
		},
	}
	defer m.Close()

	err := m.PlayFromIndex(2)
	if err == nil {
		t.Fatal("PlayFromIndex(2) with 2 entries: expected error, got nil")
	}
	if !errors.Is(err, ErrPlaybackFailed) {
		t.Errorf("PlayFromIndex(2): want ErrPlaybackFailed, got %v", err)
	}
}

// TestManager_PlayFromIndex_AllMissing verifies that PlayFromIndex returns
// ErrPlaybackFailed when all entries from the start index onward are missing files.
func TestManager_PlayFromIndex_AllMissing(t *testing.T) {
	var mu sync.Mutex
	var received [][]any
	m := newConnectedManager(t, "pfi-allmissing", &mu, &received)

	m.mu.Lock()
	m.queue = []QueueTrack{
		{Path: "/nonexistent1.flac", Hash: "h0", QueueID: 1},
		{Path: "/nonexistent2.flac", Hash: "h1", QueueID: 2},
		{Path: "/nonexistent3.flac", Hash: "h2", QueueID: 3},
	}
	m.mu.Unlock()

	err := m.PlayFromIndex(0)
	if err == nil {
		t.Fatal("PlayFromIndex with all missing: expected error, got nil")
	}
	if !errors.Is(err, ErrPlaybackFailed) {
		t.Errorf("PlayFromIndex with all missing: want ErrPlaybackFailed, got %v", err)
	}
}

// shrinkingMarker shrinks m.queue down to a single entry when MarkTrackMissing
// is invoked. This simulates a concurrent SetQueue running while PlayFromIndex
// is iterating its (stale) queue snapshot trying to skip past missing files.
type shrinkingMarker struct {
	m       *Manager
	keep    []QueueTrack
	shrunk  bool
}

func (s *shrinkingMarker) MarkTrackMissing(_ string) error {
	if s.shrunk {
		return nil
	}
	s.shrunk = true
	s.m.mu.Lock()
	s.m.queue = append([]QueueTrack(nil), s.keep...)
	s.m.mu.Unlock()
	return nil
}
func (s *shrinkingMarker) MarkQueueEntryMissing(_ int64) error { return nil }

// TestManager_PlayFromIndex_ClampsCurrentIndexWhenQueueShrinks verifies that
// PlayFromIndex clamps currentIndex to the live queue length when a
// concurrent mutation shrinks m.queue between the queue-snapshot read at the
// top of the function and the currentIndex update at the bottom. Without
// this clamp, currentIndex would point past the end of the live queue,
// silently breaking start-file dispatch (which requires idx < len(q)) and
// Next/Previous bookkeeping.
func TestManager_PlayFromIndex_ClampsCurrentIndexWhenQueueShrinks(t *testing.T) {
	var mu sync.Mutex
	var received [][]any
	m := newConnectedManager(t, "pfi-shrink", &mu, &received)

	// Real on-disk file used for the final, playable entry.
	pPlayable := tempFile(t, "p*.flac")

	// Build a 4-entry queue: 0..2 are missing (so PlayQueueEntry calls
	// MarkTrackMissing → triggers the queue shrink), 3 is the playable
	// entry that succeeds. The shrinker replaces m.queue with a 1-entry
	// slice (the missing entry only) on the first missing call, so by
	// the time PlayFromIndex commits currentIndex = 3, the live queue
	// is length 1.
	m.mu.Lock()
	m.queue = []QueueTrack{
		{Path: "/nonexistent-shrink-1.flac", Hash: "h0", QueueID: 1},
		{Path: "/nonexistent-shrink-2.flac", Hash: "h1", QueueID: 2},
		{Path: "/nonexistent-shrink-3.flac", Hash: "h2", QueueID: 3},
		{Path: pPlayable, Hash: "h3", QueueID: 4},
	}
	m.mu.Unlock()

	marker := &shrinkingMarker{
		m:    m,
		keep: []QueueTrack{{Path: "/nonexistent-shrink-1.flac", Hash: "h0", QueueID: 1}},
	}
	m.SetMissingMarker(marker)

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): unexpected error: %v", err)
	}

	m.mu.RLock()
	got := m.currentIndex
	qLen := len(m.queue)
	m.mu.RUnlock()

	if got >= qLen {
		t.Fatalf("currentIndex = %d but live queue len = %d; must be clamped to last valid index", got, qLen)
	}
}

// TestManager_PlayFromIndex_SkipsMissingToPlayable verifies that PlayFromIndex
// skips missing files and plays the first available entry.
func TestManager_PlayFromIndex_SkipsMissingToPlayable(t *testing.T) {
	var mu sync.Mutex
	var received [][]any
	m := newConnectedManager(t, "pfi-skip", &mu, &received)

	pathC := tempFile(t, "c*.flac")

	m.mu.Lock()
	m.queue = []QueueTrack{
		{Path: "/nonexistent1.flac", Hash: "h0", QueueID: 1},
		{Path: "/nonexistent2.flac", Hash: "h1", QueueID: 2},
		{Path: pathC, Hash: "h2", QueueID: 3},
	}
	m.mu.Unlock()

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): unexpected error: %v", err)
	}

	m.mu.RLock()
	idx := m.currentIndex
	m.mu.RUnlock()

	if idx != 2 {
		t.Errorf("currentIndex = %d, want 2 (skipped missing entries)", idx)
	}
}
