package player

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

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

	if idx != -1 {
		t.Errorf("currentIndex = %d after SetQueue(nil), want -1", idx)
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

	t.Run("empty queue sets -1", func(t *testing.T) {
		m := makeManager(0, 0)
		defer m.Close()
		m.SetCurrentIndex(5)
		m.mu.RLock()
		got := m.currentIndex
		m.mu.RUnlock()
		if got != -1 {
			t.Errorf("currentIndex = %d, want -1", got)
		}
	})
}

// TestManager_SetCurrentIndex_RefreshesPrefetch verifies that moving the
// current index away from a prefetched neighbour invalidates the previously
// prefetched track so mpv's internal playlist does not retain a stale
// gapless "next" entry after shuffle/dedup/move/undo operations.
func TestManager_SetCurrentIndex_RefreshesPrefetch(t *testing.T) {
	t.Run("changing index clears and refreshes prefetch", func(t *testing.T) {
		var mu sync.Mutex
		var received [][]any
		m := newConnectedManager(t, "sci-refresh-pf", &mu, &received)

		// Seed the queue with real on-disk files so prefetchNext is willing to
		// stat them and append the new candidate.
		pa := tempFile(t, "a*.flac")
		pb := tempFile(t, "b*.flac")
		pc := tempFile(t, "c*.flac")
		m.mu.Lock()
		m.queue = []QueueTrack{
			{Path: pa, Hash: "h0", QueueID: 1},
			{Path: pb, Hash: "h1", QueueID: 2},
			{Path: pc, Hash: "h2", QueueID: 3},
		}
		m.currentIndex = 0
		m.gaplessEnabled = true
		// Simulate a stale prefetch sitting at index 1 from before the move.
		m.prefetchedIndex = 1
		m.prefetchedPath = pb
		m.mu.Unlock()

		// Move the cursor to index 2 — the prefetched entry at index 1 is now
		// behind us and must be cleared.
		m.SetCurrentIndex(2)

		// The prefetch must be cleared. doClearPrefetch sends a playlist-remove
		// for the old prefetch slot before prefetchNext appends the new one.
		waitForCommand(t, &mu, &received, isPlaylistRemove, time.Second,
			"playlist-remove after SetCurrentIndex change")

		m.mu.RLock()
		pfIdx := m.prefetchedIndex
		m.mu.RUnlock()
		// After SetCurrentIndex(2), there is no successor on disk → prefetch
		// should remain cleared (no new entry was appended).
		if pfIdx != -1 {
			t.Errorf("prefetchedIndex = %d after moving past it, want -1", pfIdx)
		}
	})

	t.Run("setting same index does not touch prefetch", func(t *testing.T) {
		var mu sync.Mutex
		var received [][]any
		m := newConnectedManager(t, "sci-same-pf", &mu, &received)

		pa := tempFile(t, "a*.flac")
		pb := tempFile(t, "b*.flac")
		m.mu.Lock()
		m.queue = []QueueTrack{
			{Path: pa, Hash: "h0", QueueID: 1},
			{Path: pb, Hash: "h1", QueueID: 2},
		}
		m.currentIndex = 0
		m.gaplessEnabled = true
		m.prefetchedIndex = 1
		m.prefetchedPath = pb
		m.mu.Unlock()

		// Setting the same index must not invalidate the existing prefetch.
		m.SetCurrentIndex(0)

		// Give any spurious goroutine a moment to run.
		time.Sleep(50 * time.Millisecond)

		mu.Lock()
		for _, cmd := range received {
			if isPlaylistRemove(cmd) {
				mu.Unlock()
				t.Fatalf("SetCurrentIndex(same) must not clear prefetch; got %v", received)
			}
		}
		mu.Unlock()

		m.mu.RLock()
		pfIdx := m.prefetchedIndex
		pfPath := m.prefetchedPath
		m.mu.RUnlock()
		if pfIdx != 1 || pfPath != pb {
			t.Errorf("prefetch state changed: idx=%d path=%q, want idx=1 path=%q", pfIdx, pfPath, pb)
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

// TestNewManager_SetQueueKeepsNoActiveTrack verifies that a newly-created
// production manager does not treat queue index 0 as currently playing before
// any playback command succeeds.
func TestNewManager_SetQueueKeepsNoActiveTrack(t *testing.T) {
	m := NewManager(shortSockPath(t, "newmgr-no-active"))
	defer m.Close()

	m.SetQueue([]QueueTrack{
		{Path: "/a.flac", Hash: "h0", QueueID: 1},
		{Path: "/b.flac", Hash: "h1", QueueID: 2},
	})

	m.mu.RLock()
	idx := m.currentIndex
	m.mu.RUnlock()

	if idx != -1 {
		t.Errorf("currentIndex = %d after SetQueue on new manager, want -1", idx)
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

// TestManager_Next_NotStarted verifies that Next returns ErrNotConnected when
// the manager has not been started (no client and no cmdQueue).
func TestManager_Next_NotStarted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel}
	defer m.Close()

	err := m.Next()
	if !errors.Is(err, ErrNotConnected) {
		t.Errorf("Next before Start: want ErrNotConnected, got %v", err)
	}
}

// TestManager_Previous_NotStarted verifies that Previous returns ErrNotConnected
// when the manager has not been started.
func TestManager_Previous_NotStarted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel}
	defer m.Close()

	err := m.Previous()
	if !errors.Is(err, ErrNotConnected) {
		t.Errorf("Previous before Start: want ErrNotConnected, got %v", err)
	}
}

// TestManager_StopAfterCurrent_GetterSetter verifies the StopAfterCurrent
// getter reflects what SetStopAfterCurrent has set.
func TestManager_StopAfterCurrent_GetterSetter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel}
	defer m.Close()

	if m.StopAfterCurrent() {
		t.Error("default should be false")
	}
	m.SetStopAfterCurrent(true)
	if !m.StopAfterCurrent() {
		t.Error("expected true after SetStopAfterCurrent(true)")
	}
	m.SetStopAfterCurrent(false)
	if m.StopAfterCurrent() {
		t.Error("expected false after SetStopAfterCurrent(false)")
	}
}
