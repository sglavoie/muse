package tui

import (
	"context"
	"testing"
	"time"
)

// TestUndoEnqueueClampsScrollOffset verifies that undoing a single-track
// enqueue clamps scrollOffset so it does not point past the end of the
// shortened queue.
func TestUndoEnqueueClampsScrollOffset(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/a.flac", Display: "A"},
		{ID: 20, Path: "/b.flac", Display: "B"},
	}
	m.undo.slot = undoSlot{
		action:    undoEnqueueAction{id: 20, entry: m.queue.entries[1], idx: 1},
		expiresAt: time.Now().Add(undoWindowDuration),
	}
	m.queue.scrollOffset = 1
	m.queue.cursor = 1

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	if len(nm.queue.entries) != 1 {
		t.Fatalf("after undo enqueue: expected 1 entry, got %d", len(nm.queue.entries))
	}
	if nm.queue.scrollOffset > 0 {
		t.Fatalf("scrollOffset must be clamped to 0 after removing last entry, got %d",
			nm.queue.scrollOffset)
	}
}

// TestUndoBatchEnqueueClampsScrollOffset verifies that undoing a batch enqueue
// clamps scrollOffset so it does not point past the end of the shortened queue.
func TestUndoBatchEnqueueClampsScrollOffset(t *testing.T) {
	d := &stubQueueDequeuer{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueDequeuer(d), WithBatchEnqueuer(be))
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/a.flac", Display: "A"},
		{ID: 100, Path: "/b.flac", Display: "B"},
		{ID: 101, Path: "/c.flac", Display: "C"},
	}
	m.undo.slot = undoSlot{
		action: undoBatchEnqueueAction{
			ids:          []int64{100, 101},
			count:        2,
			startIdx:     1,
			playlistName: "test",
		},
		expiresAt: time.Now().Add(undoWindowDuration),
	}
	m.queue.scrollOffset = 2
	m.queue.cursor = 2

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	if len(nm.queue.entries) != 1 {
		t.Fatalf("after undo batch enqueue: expected 1 entry, got %d", len(nm.queue.entries))
	}
	if nm.queue.scrollOffset > 0 {
		t.Fatalf("scrollOffset must be clamped to 0 after batch undo, got %d",
			nm.queue.scrollOffset)
	}
}

// TestUndoEnqueueSchedulesPurge verifies that undoing a single-track enqueue
// schedules a purge that eventually fires.
func TestUndoEnqueueSchedulesPurge(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/a.flac", Display: "A"},
		{ID: 20, Path: "/b.flac", Display: "B"},
	}
	m.undo.slot = undoSlot{
		action:    undoEnqueueAction{id: 20, entry: m.queue.entries[1], idx: 1},
		expiresAt: time.Now().Add(undoWindowDuration),
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	if len(d.dequeueTrackSoftCalls) != 1 || d.dequeueTrackSoftCalls[0] != 20 {
		t.Fatalf("undo enqueue must soft-delete the entry at the stored index (ID=20); got calls=%v",
			d.dequeueTrackSoftCalls)
	}
	if nm.undo.slot.action != nil {
		t.Fatal("undo slot must be cleared after successful undo")
	}
	if nm.undo.slot.cancelPurge == nil {
		t.Fatal("cancelPurge must be preserved after undo so the purge timer can be cancelled by a future action")
	}
	select {
	case <-nm.undo.slot.cancelPurge:
		t.Fatal("cancelPurge must not be closed — the purge timer should still be able to fire")
	default:
	}
}

// TestUndoBatchEnqueueSchedulesPurge verifies that undoing a batch enqueue
// schedules a purge that is not immediately cancelled.
func TestUndoBatchEnqueueSchedulesPurge(t *testing.T) {
	d := &stubQueueDequeuer{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueDequeuer(d), WithBatchEnqueuer(be))
	m.queue.entries = []QueueEntry{
		{ID: 100, Path: "/a.flac", Display: "A"},
		{ID: 101, Path: "/b.flac", Display: "B"},
	}
	m.undo.slot = undoSlot{
		action: undoBatchEnqueueAction{
			ids:      []int64{100, 101},
			count:    2,
			startIdx: 0,
		},
		expiresAt: time.Now().Add(undoWindowDuration),
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	if len(be.dequeueTracksSoftIDs) != 1 {
		t.Fatalf("undo batch enqueue must call DequeueTracksSoft once, got %d calls",
			len(be.dequeueTracksSoftIDs))
	}
	if nm.undo.slot.cancelPurge == nil {
		t.Fatal("cancelPurge must be preserved after undo so the purge timer can be cancelled by a future action")
	}
	select {
	case <-nm.undo.slot.cancelPurge:
		t.Fatal("cancelPurge must not be closed — the purge timer should still be able to fire")
	default:
	}
	if nm.undo.slot.action != nil {
		t.Fatal("undo slot must be cleared after successful undo")
	}
}

// TestUndoEnqueueFindsEntryByIDAfterReorder verifies that undoing an enqueue
// removes the correct entry even if the queue has been reordered since enqueue.
func TestUndoEnqueueFindsEntryByIDAfterReorder(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/a.flac", Hash: "aaa", Display: "A"},
		{ID: 20, Path: "/b.flac", Hash: "bbb", Display: "B"},
		{ID: 30, Path: "/c.flac", Hash: "ccc", Display: "C"},
	}

	m.setUndoSlot(undoEnqueueAction{id: 30, entry: m.queue.entries[2], idx: 2}, nil)
	m.queue.entries = []QueueEntry{
		{ID: 30, Path: "/c.flac", Hash: "ccc", Display: "C"},
		{ID: 10, Path: "/a.flac", Hash: "aaa", Display: "A"},
		{ID: 20, Path: "/b.flac", Hash: "bbb", Display: "B"},
	}

	nm := m.handleUndo()

	if len(nm.queue.entries) != 2 {
		t.Fatalf("expected 2 entries after undo, got %d", len(nm.queue.entries))
	}
	for _, e := range nm.queue.entries {
		if e.ID == 30 {
			t.Fatal("ID=30 should have been removed by undo enqueue")
		}
	}
	if len(d.dequeueTrackSoftCalls) != 1 || d.dequeueTrackSoftCalls[0] != 30 {
		t.Fatalf("expected soft-delete of ID=30, got %v", d.dequeueTrackSoftCalls)
	}
}

// TestUndoBatchEnqueueRemovesByIDNotPosition verifies that undoing a batch
// enqueue removes entries by their database IDs, not by positional index range.
func TestUndoBatchEnqueueRemovesByIDNotPosition(t *testing.T) {
	be := &stubBatchEnqueuer{}
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithBatchEnqueuer(be), WithQueueDequeuer(d))
	m.queue.entries = []QueueEntry{
		{ID: 1, Path: "/old.flac", Hash: "old", Display: "Old"},
		{ID: 100, Path: "/x.flac", Hash: "xxx", Display: "X"},
		{ID: 101, Path: "/y.flac", Hash: "yyy", Display: "Y"},
	}

	m.setUndoSlot(undoBatchEnqueueAction{
		ids:          []int64{100, 101},
		count:        2,
		startIdx:     1,
		playlistName: "Test",
	}, nil)
	m.queue.entries = []QueueEntry{
		{ID: 101, Path: "/y.flac", Hash: "yyy", Display: "Y"},
		{ID: 1, Path: "/old.flac", Hash: "old", Display: "Old"},
		{ID: 100, Path: "/x.flac", Hash: "xxx", Display: "X"},
	}

	nm := m.handleUndo()

	if len(nm.queue.entries) != 1 {
		t.Fatalf("expected 1 entry after batch undo, got %d", len(nm.queue.entries))
	}
	if nm.queue.entries[0].ID != 1 {
		t.Fatalf("expected remaining entry to be ID=1, got ID=%d", nm.queue.entries[0].ID)
	}
	if len(be.dequeueTracksSoftIDs) != 1 {
		t.Fatalf("expected 1 batch soft-delete call, got %d", len(be.dequeueTracksSoftIDs))
	}
}

// TestUndoEnqueueResetsPlaybackWhenPlayingTrackRemoved verifies that undoing
// an enqueue stops playback when the removed track was the currently-playing track.
func TestUndoEnqueueResetsPlaybackWhenPlayingTrackRemoved(t *testing.T) {
	d := &stubQueueDequeuer{}
	p := &stubPlayerQueueRemover{}
	m := New(context.Background(), WithQueueDequeuer(d), WithPlayer(p))
	m.queue.entries = []QueueEntry{{ID: 42, Path: "/song.flac", Hash: "abc123", Display: "Artist – Song"}}
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "abc123"
	m.statusBar.artist = "Artist"
	m.statusBar.title = "Song"

	m.setUndoSlot(undoEnqueueAction{id: 42, entry: m.queue.entries[0], idx: 0}, nil)

	nm := m.handleUndo()

	if len(nm.queue.entries) != 0 {
		t.Fatalf("expected empty queue after undo enqueue, got %d entries", len(nm.queue.entries))
	}
	if nm.playback.state != PlaybackIdle {
		t.Errorf("expected PlaybackIdle after undo enqueue removed playing track, got %v", nm.playback.state)
	}
	if nm.playback.currentHash != "" {
		t.Errorf("expected empty currentHash after undo enqueue, got %q", nm.playback.currentHash)
	}
	if nm.statusBar.artist != "" {
		t.Errorf("expected empty status bar artist after undo enqueue, got %q", nm.statusBar.artist)
	}
	if nm.statusBar.title != "" {
		t.Errorf("expected empty status bar title after undo enqueue, got %q", nm.statusBar.title)
	}
}

// TestUndoEnqueueKeepsPlaybackWhenOtherTrackPlaying verifies that undoing an
// enqueue does NOT stop playback when a different track is currently playing.
func TestUndoEnqueueKeepsPlaybackWhenOtherTrackPlaying(t *testing.T) {
	d := &stubQueueDequeuer{}
	p := &stubPlayerQueueRemover{}
	m := New(context.Background(), WithQueueDequeuer(d), WithPlayer(p))
	m.queue.entries = []QueueEntry{
		{ID: 1, Path: "/a.flac", Hash: "aaa", Display: "A"},
		{ID: 2, Path: "/b.flac", Hash: "bbb", Display: "B"},
	}
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "aaa"
	m.statusBar.artist = "A"
	m.statusBar.title = "Track A"

	m.setUndoSlot(undoEnqueueAction{id: 2, entry: m.queue.entries[1], idx: 1}, nil)

	nm := m.handleUndo()

	if len(nm.queue.entries) != 1 {
		t.Fatalf("expected 1 entry after undo enqueue, got %d", len(nm.queue.entries))
	}
	if nm.playback.state != PlaybackPlaying {
		t.Errorf("playback should remain Playing when a non-playing track is removed, got %v", nm.playback.state)
	}
	if nm.statusBar.artist != "A" {
		t.Errorf("status bar artist should be unchanged, got %q", nm.statusBar.artist)
	}
}

// TestUndoBatchEnqueueResetsPlaybackWhenQueueEmptied verifies that undoing a
// batch enqueue stops playback when the removal empties the queue entirely.
func TestUndoBatchEnqueueResetsPlaybackWhenQueueEmptied(t *testing.T) {
	be := &stubBatchEnqueuer{}
	d := &stubQueueDequeuer{}
	p := &stubPlayerQueueRemover{}
	m := New(context.Background(), WithBatchEnqueuer(be), WithQueueDequeuer(d), WithPlayer(p))
	m.queue.entries = []QueueEntry{
		{ID: 100, Path: "/x.flac", Hash: "xxx", Display: "X"},
		{ID: 101, Path: "/y.flac", Hash: "yyy", Display: "Y"},
	}
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "xxx"
	m.statusBar.artist = "X-Artist"
	m.statusBar.title = "X-Title"

	m.setUndoSlot(undoBatchEnqueueAction{
		ids:          []int64{100, 101},
		count:        2,
		startIdx:     0,
		playlistName: "TestPL",
	}, nil)

	nm := m.handleUndo()

	if len(nm.queue.entries) != 0 {
		t.Fatalf("expected empty queue after undo batch enqueue, got %d", len(nm.queue.entries))
	}
	if nm.playback.state != PlaybackIdle {
		t.Errorf("expected PlaybackIdle after undo batch enqueue emptied queue, got %v", nm.playback.state)
	}
	if nm.playback.currentHash != "" {
		t.Errorf("expected empty currentHash after undo batch enqueue, got %q", nm.playback.currentHash)
	}
	if nm.statusBar.artist != "" {
		t.Errorf("expected empty status bar artist, got %q", nm.statusBar.artist)
	}
}

// TestUndoBatchEnqueueKeepsPlaybackWhenPlayingTrackSurvives verifies that
// undoing a batch enqueue does NOT stop playback when the currently-playing
// track was not part of the batch and remains in the queue.
func TestUndoBatchEnqueueKeepsPlaybackWhenPlayingTrackSurvives(t *testing.T) {
	be := &stubBatchEnqueuer{}
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithBatchEnqueuer(be), WithQueueDequeuer(d))
	m.queue.entries = []QueueEntry{
		{ID: 1, Path: "/keeper.flac", Hash: "keep", Display: "Keeper"},
		{ID: 100, Path: "/x.flac", Hash: "xxx", Display: "X"},
		{ID: 101, Path: "/y.flac", Hash: "yyy", Display: "Y"},
	}
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "keep"
	m.statusBar.artist = "Keeper-Artist"
	m.statusBar.title = "Keeper-Title"

	m.setUndoSlot(undoBatchEnqueueAction{
		ids:          []int64{100, 101},
		count:        2,
		startIdx:     1,
		playlistName: "TestPL",
	}, nil)

	nm := m.handleUndo()

	if len(nm.queue.entries) != 1 {
		t.Fatalf("expected 1 entry after undo, got %d", len(nm.queue.entries))
	}
	if nm.playback.state != PlaybackPlaying {
		t.Errorf("playback should remain Playing when playing track survives, got %v", nm.playback.state)
	}
	if nm.statusBar.artist != "Keeper-Artist" {
		t.Errorf("status bar artist should be unchanged, got %q", nm.statusBar.artist)
	}
}
