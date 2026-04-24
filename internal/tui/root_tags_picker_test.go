package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// TestTagPickerCreate_AssignsToTrackAndUpdatesCache exercises the full
// create-from-picker flow: TagPickerCreateMsg → tagCreatedMsg → caches.
func TestTagPickerCreate_AssignsToTrackAndUpdatesCache(t *testing.T) {
	const trackHash = "abc123"
	const tagName = "newtag"

	// The reader returns the new tag after creation (simulating the DB read
	// that happens in refreshTagsViewAfterCreate and handleTagCreated).
	newTag := db.TagRow{ID: 1, Name: tagName}
	tr := &stubTagReader{
		tags:      []db.TagRow{newTag},
		tagCounts: map[int64]int{1: 1},
	}
	tw := &stubTagWriter{}

	m := New(context.Background(),
		WithTagReader(tr),
		WithTagWriter(tw),
	)
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{{ID: 1, Hash: trackHash, Display: "Test Track"}}
	m.queue.cursor = 0

	// Open tag picker for the focused queue track.
	m, _ = m.openTagPicker()
	if m.overlay.tagPicker == nil {
		t.Fatal("tag picker did not open")
	}
	if got := m.overlay.tagPicker.Hashes(); len(got) != 1 || got[0] != trackHash {
		t.Fatalf("picker hashes = %v, want [%s]", got, trackHash)
	}

	// Simulate TagPickerCreateMsg (user typed a new name and pressed enter).
	createMsg := TagPickerCreateMsg{Name: tagName, Hashes: []string{trackHash}}
	_, cmd := m.handleTagPickerCreate(createMsg)
	if cmd == nil {
		t.Fatal("handleTagPickerCreate returned nil cmd")
	}

	// Execute the cmd to get tagCreatedMsg.
	raw := cmd()
	msg, ok := raw.(tagCreatedMsg)
	if !ok {
		t.Fatalf("cmd() returned %T, want tagCreatedMsg", raw)
	}
	if msg.err != nil {
		t.Fatalf("tagCreatedMsg.err = %v", msg.err)
	}
	if msg.id == 0 {
		t.Fatal("tagCreatedMsg.id = 0, want >0")
	}
	if len(msg.hashes) != 1 || msg.hashes[0] != trackHash {
		t.Fatalf("tagCreatedMsg.hashes = %v, want [%s]", msg.hashes, trackHash)
	}

	// Verify the writer was called correctly.
	if len(tw.createCalls) != 1 || tw.createCalls[0] != tagName {
		t.Fatalf("createCalls = %v, want [%s]", tw.createCalls, tagName)
	}
	if len(tw.addTrackCalls) != 1 {
		t.Fatalf("addTrackCalls = %v, want 1 call", tw.addTrackCalls)
	}
	if tw.addTrackCalls[0].hash != trackHash || tw.addTrackCalls[0].tagID != msg.id {
		t.Fatalf("addTrackCalls[0] = %+v, want hash=%s tagID=%d", tw.addTrackCalls[0], trackHash, msg.id)
	}

	// Process tagCreatedMsg through the full message routing.
	m = m.refreshTagsViewAfterCreate(msg)
	m, _ = m.handleTagCreated(msg)

	// Assert: m.tags contains the new tag.
	found := false
	for _, tag := range m.tags {
		if tag.ID == msg.id && tag.Name == tagName {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("m.tags does not contain new tag (id=%d, name=%s); tags=%v", msg.id, tagName, m.tags)
	}

	// Assert: m.trackTags has the association.
	if m.trackTags == nil {
		t.Fatal("m.trackTags is nil after handleTagCreated")
	}
	if _, has := m.trackTags[trackHash][msg.id]; !has {
		t.Errorf("m.trackTags[%s] does not contain tag ID %d; trackTags=%v", trackHash, msg.id, m.trackTags)
	}

	// Assert: tagsView.tagCounts has the new tag with count > 0.
	if m.tagsView.tagCounts == nil {
		t.Fatal("m.tagsView.tagCounts is nil")
	}
	if got := m.tagsView.tagCounts[msg.id]; got == 0 {
		t.Errorf("m.tagsView.tagCounts[%d] = 0, want >0", msg.id)
	}

	// Assert: EffectiveTags includes the new tag.
	effective := tagtui.View{Tags: m.tagsView.tags, TagCounts: m.tagsView.tagCounts}.EffectiveTags()
	foundEffective := false
	for _, tag := range effective {
		if tag.ID == msg.id {
			foundEffective = true
			break
		}
	}
	if !foundEffective {
		t.Errorf("EffectiveTags() does not include new tag; effective=%v", effective)
	}

	// Simulate navigating to Tags view (syncTagsViewTags overwrite scenario).
	m = m.switchToView(viewTags)
	effective2 := tagtui.View{Tags: m.tagsView.tags, TagCounts: m.tagsView.tagCounts}.EffectiveTags()
	foundAfterSwitch := false
	for _, tag := range effective2 {
		if tag.ID == msg.id {
			foundAfterSwitch = true
			break
		}
	}
	if !foundAfterSwitch {
		t.Errorf("after switchView(viewTags), EffectiveTags() does not include new tag; tags=%v, counts=%v, effective=%v",
			m.tagsView.tags, m.tagsView.tagCounts, effective2)
	}
}

// TestTagPickerCreate_StaleDBRead verifies that the defensive fallback
// works when the DB read returns stale data (simulating WAL snapshot
// isolation where the reader doesn't see the just-committed write).
func TestTagPickerCreate_StaleDBRead(t *testing.T) {
	const trackHash = "abc123"
	const tagName = "newtag"

	// Simulate WAL stale read: the reader returns EMPTY tags and counts
	// (as if the committed write hasn't been seen yet).
	tr := &stubTagReader{
		tags:      nil,
		tagCounts: nil,
	}
	tw := &stubTagWriter{}

	m := New(context.Background(),
		WithTagReader(tr),
		WithTagWriter(tw),
	)
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{{ID: 1, Hash: trackHash, Display: "Test Track"}}
	m.queue.cursor = 0

	m, _ = m.openTagPicker()

	// Directly construct the tagCreatedMsg as if the cmd executed successfully.
	msg := tagCreatedMsg{id: 1, name: tagName, hashes: []string{trackHash}}

	// Process through the full routing.
	m = m.refreshTagsViewAfterCreate(msg)
	m, _ = m.handleTagCreated(msg)

	// Even with stale DB reads, m.tags must include the new tag.
	if !m.hasTag(msg.id) {
		t.Errorf("m.tags does not contain new tag after stale DB read; tags=%v", m.tags)
	}

	// trackTags must be updated.
	if _, has := m.trackTags[trackHash][msg.id]; !has {
		t.Errorf("m.trackTags[%s] missing tag ID %d", trackHash, msg.id)
	}

	// After switching to Tags view, the tag must still be visible.
	m = m.switchToView(viewTags)
	effective := tagtui.View{Tags: m.tagsView.tags, TagCounts: m.tagsView.tagCounts}.EffectiveTags()
	found := false
	for _, tag := range effective {
		if tag.ID == msg.id {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("after stale DB + switchToView(tags): EffectiveTags() missing new tag; tags=%v, counts=%v, effective=%v",
			m.tagsView.tags, m.tagsView.tagCounts, effective)
	}
}

// TestTagPickerToggleRemove_UpdatesTagFilterHashes verifies that removing a
// tag via the picker updates tagFilterHashes on all views when the removed
// tag matches the active tag filter, and clamps cursors.
func TestTagPickerToggleRemove_UpdatesTagFilterHashes(t *testing.T) {
	tw := &stubTagWriter{}
	m := New(context.Background(), WithTagWriter(tw))

	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "A"})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "B"})
	m = m.syncRecentlyAddedFromLibrary()

	tagID := int64(42)
	m.activeTagFilter = &tagID
	m.trackTags = map[string]map[int64]struct{}{
		"h1": {tagID: {}},
		"h2": {tagID: {}},
	}
	m.tags = []db.TagRow{{ID: tagID, Name: "mytag"}}

	// Both tracks pass the tag filter.
	hashes := map[string]struct{}{"h1": {}, "h2": {}}
	m.library.tagFilterHashes = copyTagHash(hashes)
	m.recentlyAdded.tagFilterHashes = copyTagHash(hashes)
	m.history.tagFilterHashes = copyTagHash(hashes)
	m.queue.tagFilterHashes = copyTagHash(hashes)
	m.queue.entries = []QueueEntry{{Hash: "h1"}, {Hash: "h2"}}

	// Cursor on last item.
	m.library.cursor = 1
	m.recentlyAdded.cursor = 1
	m.queue.cursor = 1

	// Remove tag 42 from h2 via the picker.
	m, _ = m.handleTagPickerToggle(TagPickerToggleMsg{
		TagID:  tagID,
		Hashes: []string{"h2"},
		Add:    false,
	})

	// h2 should be removed from tagFilterHashes on all views.
	if _, ok := m.library.tagFilterHashes["h2"]; ok {
		t.Error("h2 should be removed from library.tagFilterHashes")
	}
	if _, ok := m.recentlyAdded.tagFilterHashes["h2"]; ok {
		t.Error("h2 should be removed from recentlyAdded.tagFilterHashes")
	}
	if _, ok := m.queue.tagFilterHashes["h2"]; ok {
		t.Error("h2 should be removed from queue.tagFilterHashes")
	}

	// Cursors must be clamped (only 1 item visible).
	if m.library.cursor >= m.library.trackCount() {
		t.Errorf("library cursor=%d out of bounds (count=%d)", m.library.cursor, m.library.trackCount())
	}
	qFiltered := m.queue.filteredEntries()
	if m.queue.cursor >= len(qFiltered) {
		t.Errorf("queue cursor=%d out of bounds (filtered=%d)", m.queue.cursor, len(qFiltered))
	}
}

// TestTagPickerToggleAdd_UpdatesTagFilterHashes verifies that adding a tag
// via the picker updates tagFilterHashes on all views when the added tag
// matches the active tag filter.
func TestTagPickerToggleAdd_UpdatesTagFilterHashes(t *testing.T) {
	tw := &stubTagWriter{}
	m := New(context.Background(), WithTagWriter(tw))

	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "A"})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "B"})
	m = m.syncRecentlyAddedFromLibrary()

	tagID := int64(42)
	m.activeTagFilter = &tagID
	m.trackTags = map[string]map[int64]struct{}{
		"h1": {tagID: {}},
	}
	m.tags = []db.TagRow{{ID: tagID, Name: "mytag"}}

	// Only h1 passes the tag filter initially.
	hashes := map[string]struct{}{"h1": {}}
	m.library.tagFilterHashes = copyTagHash(hashes)
	m.recentlyAdded.tagFilterHashes = copyTagHash(hashes)

	// Add tag 42 to h2 via the picker.
	m, _ = m.handleTagPickerToggle(TagPickerToggleMsg{
		TagID:  tagID,
		Hashes: []string{"h2"},
		Add:    true,
	})

	// h2 should now be in tagFilterHashes on all views.
	if _, ok := m.library.tagFilterHashes["h2"]; !ok {
		t.Error("h2 should be added to library.tagFilterHashes")
	}
	if _, ok := m.recentlyAdded.tagFilterHashes["h2"]; !ok {
		t.Error("h2 should be added to recentlyAdded.tagFilterHashes")
	}
}

// TestInlineTagCreate_DoesNotOpenPicker verifies that creating a tag via the
// inline Tags-view input (pressing 'c') does NOT open the tag picker overlay.
// Before the fix, handleTagCreated unconditionally called openTagPicker(),
// which would flash "No track focused" or open an unwanted overlay.
func TestInlineTagCreate_DoesNotOpenPicker(t *testing.T) {
	const tagName = "inlinetag"

	tr := &stubTagReader{
		tags:      []db.TagRow{{ID: 1, Name: tagName}},
		tagCounts: map[int64]int{1: 0},
	}
	tw := &stubTagWriter{}

	m := New(context.Background(),
		WithTagReader(tr),
		WithTagWriter(tw),
	)
	m.activeView = viewTags
	m.tagsView.level = tagtui.TagLevel

	// No tag picker is open (inline create does not use the picker).
	if m.overlay.tagPicker != nil {
		t.Fatal("precondition: tag picker should not be open")
	}

	// Simulate inline create result.
	msg := tagCreatedMsg{id: 1, name: tagName}

	m = m.refreshTagsViewAfterCreate(msg)
	m, cmd := m.handleTagCreated(msg)

	// The tag picker must NOT be opened.
	if m.overlay.tagPicker != nil {
		t.Error("tag picker was opened after inline create; expected nil")
	}
	// No command should be returned (no picker to re-open).
	if cmd != nil {
		t.Error("handleTagCreated returned a non-nil cmd for inline create")
	}

	// The tag should still be in the cache.
	if !m.hasTag(1) {
		t.Error("inline-created tag not found in m.tags")
	}
}

func copyTagHash(src map[string]struct{}) map[string]struct{} {
	dst := make(map[string]struct{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
