package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

// TestRemoveAllTagsFromTrack_ClearsAllTagFilterHashes verifies that
// handleRemoveAllTagsFromTrack removes the track hash from tagFilterHashes
// on all four views (library, recentlyAdded, history, queue) when a
// cross-view tag filter is active.
func TestRemoveAllTagsFromTrack_ClearsAllTagFilterHashes(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac"})
	m = m.syncRecentlyAddedFromLibrary()

	tagID := int64(42)
	m.activeTagFilter = &tagID
	m.trackTags = map[string]map[int64]struct{}{
		"h1": {tagID: {}},
	}

	// Simulate an active tag filter on all four views.
	hashes := map[string]struct{}{"h1": {}}
	m.library.tagFilterHashes = copyHashSet(hashes)
	m.recentlyAdded.tagFilterHashes = copyHashSet(hashes)
	m.history.tagFilterHashes = copyHashSet(hashes)
	m.queue.tagFilterHashes = copyHashSet(hashes)

	m, _ = m.handleRemoveAllTagsFromTrack(removeAllTagsFromTrackMsg{hash: "h1"})

	if _, ok := m.library.tagFilterHashes["h1"]; ok {
		t.Error("h1 should be removed from library.tagFilterHashes")
	}
	if _, ok := m.recentlyAdded.tagFilterHashes["h1"]; ok {
		t.Error("h1 should be removed from recentlyAdded.tagFilterHashes")
	}
	if _, ok := m.history.tagFilterHashes["h1"]; ok {
		t.Error("h1 should be removed from history.tagFilterHashes")
	}
	if _, ok := m.queue.tagFilterHashes["h1"]; ok {
		t.Error("h1 should be removed from queue.tagFilterHashes")
	}
}

// TestRemoveAllTagsFromTrack_NoFilterActive verifies that
// handleRemoveAllTagsFromTrack does not panic when no tag filter is active.
func TestRemoveAllTagsFromTrack_NoFilterActive(t *testing.T) {
	m := New(context.Background())
	m.trackTags = map[string]map[int64]struct{}{
		"h1": {1: {}},
	}
	// activeTagFilter is nil — should be a no-op for filter hashes.
	m, _ = m.handleRemoveAllTagsFromTrack(removeAllTagsFromTrackMsg{hash: "h1"})
	// No assertion needed beyond not panicking.
}

// TestRemoveAllTagsFromTrack_ClampsCursorsAfterFilterShrink verifies that
// handleRemoveAllTagsFromTrack clamps the library, recentlyAdded, history,
// and queue cursors when removing a track from tagFilterHashes causes the
// effective list to shrink below the current cursor position.
func TestRemoveAllTagsFromTrack_ClampsCursorsAfterFilterShrink(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "A"})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "B"})
	m = m.syncRecentlyAddedFromLibrary()

	tagID := int64(42)
	m.activeTagFilter = &tagID
	m.trackTags = map[string]map[int64]struct{}{
		"h1": {tagID: {}},
		"h2": {tagID: {}},
	}

	// Both tracks pass the tag filter.
	hashes := map[string]struct{}{"h1": {}, "h2": {}}
	m.library.tagFilterHashes = copyHashSet(hashes)
	m.recentlyAdded.tagFilterHashes = copyHashSet(hashes)
	m.history.tagFilterHashes = copyHashSet(hashes)
	m.queue.tagFilterHashes = copyHashSet(hashes)
	m.queue.entries = []QueueEntry{{Hash: "h1"}, {Hash: "h2"}}

	// Place cursor on the last item (index 1) in all views.
	m.library.cursor = 1
	m.recentlyAdded.cursor = 1
	m.history.cursor = 1
	m.queue.cursor = 1

	// Remove all tags from h2 — it will be removed from tagFilterHashes,
	// shrinking the effective list to 1 item.
	m, _ = m.handleRemoveAllTagsFromTrack(removeAllTagsFromTrackMsg{hash: "h2"})

	// Library: effective baseTracks is now 1 item, cursor must be 0.
	libCount := m.library.trackCount()
	if libCount != 1 {
		t.Fatalf("expected library trackCount=1 after filter shrink, got %d", libCount)
	}
	if m.library.cursor >= libCount {
		t.Errorf("library cursor=%d out of bounds (count=%d)", m.library.cursor, libCount)
	}

	// RecentlyAdded: same check.
	raCount := m.recentlyAdded.trackCount()
	if raCount != 1 {
		t.Fatalf("expected recentlyAdded trackCount=1, got %d", raCount)
	}
	if m.recentlyAdded.cursor >= raCount {
		t.Errorf("recentlyAdded cursor=%d out of bounds (count=%d)", m.recentlyAdded.cursor, raCount)
	}

	// Queue: filteredEntries shrinks because h2 is no longer in tagFilterHashes.
	qFiltered := m.queue.filteredEntries()
	if len(qFiltered) != 1 {
		t.Fatalf("expected 1 filtered queue entry, got %d", len(qFiltered))
	}
	if m.queue.cursor >= len(qFiltered) {
		t.Errorf("queue cursor=%d out of bounds (filtered=%d)", m.queue.cursor, len(qFiltered))
	}
}

func copyHashSet(src map[string]struct{}) map[string]struct{} {
	dst := make(map[string]struct{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
