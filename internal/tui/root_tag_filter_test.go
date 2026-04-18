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

func copyHashSet(src map[string]struct{}) map[string]struct{} {
	dst := make(map[string]struct{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
