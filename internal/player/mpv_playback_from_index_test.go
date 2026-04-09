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
