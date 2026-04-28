package player

import (
	"context"
	"testing"
)

// TestManager_RemoveFromQueue verifies the three index-adjustment cases for
// RemoveFromQueue: removal after, before, and at currentIndex.
func TestManager_RemoveFromQueue(t *testing.T) {
	makeQueue := func() []QueueTrack {
		return []QueueTrack{
			{Path: "/a.flac", Hash: "h0", QueueID: 1},
			{Path: "/b.flac", Hash: "h1", QueueID: 2},
			{Path: "/c.flac", Hash: "h2", QueueID: 3},
			{Path: "/d.flac", Hash: "h3", QueueID: 4},
		}
	}

	t.Run("remove after currentIndex", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		m := &Manager{ctx: ctx, cancel: cancel, queue: makeQueue(), currentIndex: 1}
		defer m.Close()

		if err := m.RemoveFromQueue(2); err != nil {
			t.Fatalf("RemoveFromQueue(2): unexpected error: %v", err)
		}
		if got := len(m.queue); got != 3 {
			t.Errorf("queue len = %d, want 3", got)
		}
		if m.currentIndex != 1 {
			t.Errorf("currentIndex = %d, want 1 (unchanged)", m.currentIndex)
		}
	})

	t.Run("remove before currentIndex", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		m := &Manager{ctx: ctx, cancel: cancel, queue: makeQueue(), currentIndex: 2}
		defer m.Close()

		if err := m.RemoveFromQueue(1); err != nil {
			t.Fatalf("RemoveFromQueue(1): unexpected error: %v", err)
		}
		if got := len(m.queue); got != 3 {
			t.Errorf("queue len = %d, want 3", got)
		}
		if m.currentIndex != 1 {
			t.Errorf("currentIndex = %d, want 1 (decremented)", m.currentIndex)
		}
	})

	t.Run("remove at currentIndex", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		m := &Manager{ctx: ctx, cancel: cancel, queue: makeQueue(), currentIndex: 2}
		defer m.Close()

		if err := m.RemoveFromQueue(2); err != nil {
			t.Fatalf("RemoveFromQueue(2): unexpected error: %v", err)
		}
		if got := len(m.queue); got != 3 {
			t.Errorf("queue len = %d, want 3", got)
		}
		// currentIndex stays at 2 so auto-advance (PlayFromIndex(2)) resolves
		// to what was previously index 3 (now at position 2 after removal).
		if m.currentIndex != 2 {
			t.Errorf("currentIndex = %d, want 2 (unchanged)", m.currentIndex)
		}
	})

	t.Run("remove last entry at currentIndex clamps index", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		q := []QueueTrack{
			{Path: "/a.flac", Hash: "h0", QueueID: 1},
			{Path: "/b.flac", Hash: "h1", QueueID: 2},
			{Path: "/c.flac", Hash: "h2", QueueID: 3},
		}
		m := &Manager{ctx: ctx, cancel: cancel, queue: q, currentIndex: 2}
		defer m.Close()

		if err := m.RemoveFromQueue(2); err != nil {
			t.Fatalf("RemoveFromQueue(2): unexpected error: %v", err)
		}
		if got := len(m.queue); got != 2 {
			t.Errorf("queue len = %d, want 2", got)
		}
		if m.currentIndex != 1 {
			t.Errorf("currentIndex = %d, want 1 (clamped to last entry)", m.currentIndex)
		}
	})

	t.Run("remove only entry resets currentIndex to -1", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		q := []QueueTrack{
			{Path: "/a.flac", Hash: "h0", QueueID: 1},
		}
		m := &Manager{ctx: ctx, cancel: cancel, queue: q, currentIndex: 0}
		defer m.Close()

		if err := m.RemoveFromQueue(0); err != nil {
			t.Fatalf("RemoveFromQueue(0): unexpected error: %v", err)
		}
		if got := len(m.queue); got != 0 {
			t.Errorf("queue len = %d, want 0", got)
		}
		if m.currentIndex != -1 {
			t.Errorf("currentIndex = %d, want -1", m.currentIndex)
		}
	})

	t.Run("out of range returns error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		m := &Manager{ctx: ctx, cancel: cancel, queue: makeQueue(), currentIndex: 1}
		defer m.Close()

		if err := m.RemoveFromQueue(-1); err == nil {
			t.Error("RemoveFromQueue(-1): want error, got nil")
		}
		if err := m.RemoveFromQueue(4); err == nil {
			t.Error("RemoveFromQueue(4): want error, got nil")
		}
		if got := len(m.queue); got != 4 {
			t.Errorf("queue len = %d, want 4 (unmodified)", got)
		}
	})
}

func TestManager_AppendToQueue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel, queue: []QueueTrack{
		{Path: "/a.flac", Hash: "h0", QueueID: 1},
	}}
	defer m.Close()

	m.AppendToQueue(QueueTrack{Path: "/b.flac", Hash: "h1", QueueID: 2})
	m.AppendToQueue(QueueTrack{Path: "/c.flac", Hash: "h2", QueueID: 3})

	m.mu.RLock()
	n := len(m.queue)
	m.mu.RUnlock()

	if n != 3 {
		t.Fatalf("queue len = %d, want 3", n)
	}
	if m.queue[1].Path != "/b.flac" || m.queue[2].Path != "/c.flac" {
		t.Errorf("queue order wrong: %v", m.queue)
	}
}

// TestManager_RemoveFromQueue_NewBackingArray verifies that RemoveFromQueue
// allocates a new backing array so that a concurrent reader holding the old
// slice header is not affected by the mutation. Under -race this test would
// fail if RemoveFromQueue mutated the old backing array in place.
func TestManager_RemoveFromQueue_NewBackingArray(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel}
	defer m.Close()

	m.mu.Lock()
	m.queue = []QueueTrack{
		{Path: "/a.flac", Hash: "h0", QueueID: 1},
		{Path: "/b.flac", Hash: "h1", QueueID: 2},
		{Path: "/c.flac", Hash: "h2", QueueID: 3},
		{Path: "/d.flac", Hash: "h3", QueueID: 4},
	}
	m.currentIndex = 0
	m.mu.Unlock()

	// Capture a snapshot of the queue as PlayFromIndex would.
	m.mu.RLock()
	snapshot := m.queue
	m.mu.RUnlock()

	// Remove an entry; this must NOT mutate the snapshot's backing array.
	if err := m.RemoveFromQueue(1); err != nil {
		t.Fatalf("RemoveFromQueue(1): %v", err)
	}

	// The snapshot should still have all 4 original entries.
	if len(snapshot) != 4 {
		t.Fatalf("snapshot len = %d, want 4", len(snapshot))
	}
	wantPaths := []string{"/a.flac", "/b.flac", "/c.flac", "/d.flac"}
	for i, want := range wantPaths {
		if snapshot[i].Path != want {
			t.Errorf("snapshot[%d].Path = %q, want %q", i, snapshot[i].Path, want)
		}
	}
}
