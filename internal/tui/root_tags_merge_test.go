package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

func TestHandleTagMerged_RemapsTrackTagsCache(t *testing.T) {
	m := New(context.Background())
	srcTag := db.TagRow{ID: 10, Name: "src-tag"}
	destTag := db.TagRow{ID: 20, Name: "dest-tag"}

	// Simulate three tracks: two tagged with src, one with dest, one with both.
	m.trackTags = map[string]map[int64]struct{}{
		"hash1": {srcTag.ID: {}},                 // only src
		"hash2": {destTag.ID: {}},                // only dest (unchanged)
		"hash3": {srcTag.ID: {}, destTag.ID: {}}, // both
		"hash4": {99: {}},                        // unrelated tag
	}

	msg := tagMergedMsg{src: srcTag, dest: destTag}
	m, _ = m.handleTagMerged(msg)

	// hash1: src should be remapped to dest.
	if _, has := m.trackTags["hash1"][srcTag.ID]; has {
		t.Error("hash1 still has src tag after merge")
	}
	if _, has := m.trackTags["hash1"][destTag.ID]; !has {
		t.Error("hash1 missing dest tag after merge")
	}

	// hash2: should remain unchanged (only dest).
	if _, has := m.trackTags["hash2"][destTag.ID]; !has {
		t.Error("hash2 lost dest tag")
	}
	if len(m.trackTags["hash2"]) != 1 {
		t.Errorf("hash2 expected 1 tag, got %d", len(m.trackTags["hash2"]))
	}

	// hash3: src removed, dest remains; should have exactly 1 entry (dest).
	if _, has := m.trackTags["hash3"][srcTag.ID]; has {
		t.Error("hash3 still has src tag after merge")
	}
	if _, has := m.trackTags["hash3"][destTag.ID]; !has {
		t.Error("hash3 missing dest tag after merge")
	}
	if len(m.trackTags["hash3"]) != 1 {
		t.Errorf("hash3 expected 1 tag (dest only), got %d", len(m.trackTags["hash3"]))
	}

	// hash4: unrelated, should be untouched.
	if _, has := m.trackTags["hash4"][99]; !has {
		t.Error("hash4 lost unrelated tag")
	}
	if len(m.trackTags["hash4"]) != 1 {
		t.Errorf("hash4 expected 1 tag, got %d", len(m.trackTags["hash4"]))
	}
}

// TestHandleTagMerged_RepointsActiveTagFilter verifies that when the cross-view
// tag filter is set to the tag being merged away, the filter is repointed at
// the destination tag so the user keeps seeing the filtered tracks (now under
// the destination tag) instead of an empty list with a "[tag: ]" badge whose
// name lookup returns the empty string.
func TestHandleTagMerged_RepointsActiveTagFilter(t *testing.T) {
	m := New(context.Background())
	srcTag := db.TagRow{ID: 10, Name: "src-tag"}
	destTag := db.TagRow{ID: 20, Name: "dest-tag"}

	// Two tracks: hash1 tagged with src only, hash2 with dest only.
	m.trackTags = map[string]map[int64]struct{}{
		"hash1": {srcTag.ID: {}},
		"hash2": {destTag.ID: {}},
	}
	srcID := srcTag.ID
	m.activeTagFilter = &srcID
	m = m.applyTagFilterToViews()
	if len(m.library.TagFilterHashes) != 1 {
		t.Fatalf("pre-merge: expected 1 hash in library filter, got %d", len(m.library.TagFilterHashes))
	}
	if _, ok := m.library.TagFilterHashes["hash1"]; !ok {
		t.Fatal("pre-merge: hash1 should be in library tagFilterHashes")
	}

	m, _ = m.handleTagMerged(tagMergedMsg{src: srcTag, dest: destTag})

	if m.activeTagFilter == nil {
		t.Fatal("activeTagFilter should not be cleared after merge")
	}
	if *m.activeTagFilter != destTag.ID {
		t.Errorf("activeTagFilter should be repointed to dest (%d), got %d",
			destTag.ID, *m.activeTagFilter)
	}
	// After repoint, both hash1 (remapped) and hash2 (already dest) should
	// match the active filter.
	if len(m.library.TagFilterHashes) != 2 {
		t.Errorf("post-merge: expected 2 hashes in library filter, got %d",
			len(m.library.TagFilterHashes))
	}
}
