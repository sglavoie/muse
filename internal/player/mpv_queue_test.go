package player

import (
	"context"
	"fmt"
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

	t.Run("remove only entry resets currentIndex to 0", func(t *testing.T) {
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
		if m.currentIndex != 0 {
			t.Errorf("currentIndex = %d, want 0", m.currentIndex)
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

// TestManager_SetQueue_ClampsCurrentIndex verifies that SetQueue clamps
// currentIndex to the last valid index when the new queue is shorter than
// the old one, preventing out-of-bounds access from Next/Previous.
func TestManager_SetQueue_ClampsCurrentIndex(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		ctx:    ctx,
		cancel: cancel,
		queue: []QueueTrack{
			{Path: "/a.flac", Hash: "h0", QueueID: 1},
			{Path: "/b.flac", Hash: "h1", QueueID: 2},
			{Path: "/c.flac", Hash: "h2", QueueID: 3},
			{Path: "/d.flac", Hash: "h3", QueueID: 4},
		},
		currentIndex: 3,
	}
	defer m.Close()

	// Replace with a shorter queue — currentIndex must be clamped.
	m.SetQueue([]QueueTrack{
		{Path: "/x.flac", Hash: "hx", QueueID: 10},
		{Path: "/y.flac", Hash: "hy", QueueID: 11},
	})

	m.mu.RLock()
	idx := m.currentIndex
	qLen := len(m.queue)
	m.mu.RUnlock()

	if idx >= qLen {
		t.Fatalf("currentIndex = %d after SetQueue with %d entries; expected clamped to %d", idx, qLen, qLen-1)
	}
	if idx != 1 {
		t.Errorf("currentIndex = %d, want 1 (last valid index)", idx)
	}
}

// TestManager_SetQueue_EmptyQueue verifies that SetQueue resets currentIndex
// to 0 when the new queue is empty.
func TestManager_SetQueue_EmptyQueue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		ctx:    ctx,
		cancel: cancel,
		queue: []QueueTrack{
			{Path: "/a.flac", Hash: "h0", QueueID: 1},
		},
		currentIndex: 0,
	}
	defer m.Close()

	m.SetQueue(nil)

	m.mu.RLock()
	idx := m.currentIndex
	m.mu.RUnlock()

	if idx != 0 {
		t.Errorf("currentIndex = %d after SetQueue(nil), want 0", idx)
	}
}

// TestManager_SetCurrentIndex verifies that SetCurrentIndex sets the index
// correctly and clamps out-of-range values.
func TestManager_SetCurrentIndex(t *testing.T) {
	makeManager := func(queueLen, startIdx int) *Manager {
		ctx, cancel := context.WithCancel(context.Background())
		q := make([]QueueTrack, queueLen)
		for i := range q {
			q[i] = QueueTrack{Path: fmt.Sprintf("/%d.flac", i), Hash: fmt.Sprintf("h%d", i), QueueID: int64(i + 1)}
		}
		return &Manager{ctx: ctx, cancel: cancel, queue: q, currentIndex: startIdx}
	}

	t.Run("valid index", func(t *testing.T) {
		m := makeManager(5, 0)
		defer m.Close()
		m.SetCurrentIndex(3)
		m.mu.RLock()
		got := m.currentIndex
		m.mu.RUnlock()
		if got != 3 {
			t.Errorf("currentIndex = %d, want 3", got)
		}
	})

	t.Run("negative clamped to 0", func(t *testing.T) {
		m := makeManager(5, 2)
		defer m.Close()
		m.SetCurrentIndex(-1)
		m.mu.RLock()
		got := m.currentIndex
		m.mu.RUnlock()
		if got != 0 {
			t.Errorf("currentIndex = %d, want 0", got)
		}
	})

	t.Run("too large clamped to last", func(t *testing.T) {
		m := makeManager(3, 0)
		defer m.Close()
		m.SetCurrentIndex(10)
		m.mu.RLock()
		got := m.currentIndex
		m.mu.RUnlock()
		if got != 2 {
			t.Errorf("currentIndex = %d, want 2", got)
		}
	})

	t.Run("empty queue sets 0", func(t *testing.T) {
		m := makeManager(0, 0)
		defer m.Close()
		m.SetCurrentIndex(5)
		m.mu.RLock()
		got := m.currentIndex
		m.mu.RUnlock()
		if got != 0 {
			t.Errorf("currentIndex = %d, want 0", got)
		}
	})
}

// TestManager_SetQueue_KeepsCurrentIndexWhenValid verifies that SetQueue does
// NOT change currentIndex when the new queue is large enough.
func TestManager_SetQueue_KeepsCurrentIndexWhenValid(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		ctx:    ctx,
		cancel: cancel,
		queue: []QueueTrack{
			{Path: "/a.flac", Hash: "h0", QueueID: 1},
			{Path: "/b.flac", Hash: "h1", QueueID: 2},
		},
		currentIndex: 1,
	}
	defer m.Close()

	// Replace with a queue that still has index 1 valid.
	m.SetQueue([]QueueTrack{
		{Path: "/x.flac", Hash: "hx", QueueID: 10},
		{Path: "/y.flac", Hash: "hy", QueueID: 11},
		{Path: "/z.flac", Hash: "hz", QueueID: 12},
	})

	m.mu.RLock()
	idx := m.currentIndex
	m.mu.RUnlock()

	if idx != 1 {
		t.Errorf("currentIndex = %d, want 1 (unchanged)", idx)
	}
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

func TestManager_DisconnectCh(t *testing.T) {
	ch := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel, disconnectCh: ch}
	defer m.Close()

	got := m.DisconnectCh()
	if got != ch {
		t.Error("DisconnectCh did not return the expected channel")
	}
}

func TestNoopMarkerDoesNotError(t *testing.T) {
	var m noopMarker
	if err := m.MarkTrackMissing("hash"); err != nil {
		t.Errorf("noopMarker.MarkTrackMissing: %v", err)
	}
	if err := m.MarkQueueEntryMissing(42); err != nil {
		t.Errorf("noopMarker.MarkQueueEntryMissing: %v", err)
	}
}

func TestManager_SetExtraFlags(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel}
	defer m.Close()

	flags := []string{"--no-video", "--really-quiet"}
	m.SetExtraFlags(flags)

	m.mu.RLock()
	got := m.extraFlags
	m.mu.RUnlock()

	if len(got) != 2 || got[0] != "--no-video" || got[1] != "--really-quiet" {
		t.Errorf("SetExtraFlags: got %v", got)
	}

	// Verify it's a copy, not a reference.
	flags[0] = "modified"
	m.mu.RLock()
	if m.extraFlags[0] == "modified" {
		t.Error("SetExtraFlags must copy the slice, not store a reference")
	}
	m.mu.RUnlock()
}

func TestManager_SetMissingMarker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel}
	defer m.Close()

	marker := &mockMarker{}
	m.SetMissingMarker(marker)

	m.mu.RLock()
	got := m.marker
	m.mu.RUnlock()
	if got != marker {
		t.Error("SetMissingMarker did not set the marker")
	}
}
