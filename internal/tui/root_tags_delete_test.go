package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/queue"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// TestTagDeleteStoresCanonicalIndex verifies that handleTagDelete stores the
// canonical (unfiltered) index in the undo action and tagDeletedMsg, not the
// display cursor position. When a filter is active the cursor indexes the
// filtered list, which is shorter than the canonical list.
func TestTagDeleteStoresCanonicalIndex(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewTags
	m.tagsView.Level = tagtui.TagLevel
	m.tagsView.Tags = []db.TagRow{
		{ID: 1, Name: "Alpha"},
		{ID: 2, Name: "Beta"},
		{ID: 3, Name: "Charlie"},
		{ID: 4, Name: "Delta"},
	}
	m.tagsView.TagCounts = map[int64]int{1: 1, 2: 1, 3: 1, 4: 1}
	m.tagsView.Height = 40
	m = m.rebuildTagCache(m.tagsView.Tags)

	// Activate a filter that shows only "Charlie" (canonical index 2).
	m.tagsView.Filter = filter.State{Text: "charlie"}
	// Cursor 0 in the filtered list points to Charlie.
	m.tagsView.Cursor = 0

	m, cmd := m.handleTagDelete()

	// The soft-delete is now synchronous, so no Cmd is returned.
	if cmd != nil {
		t.Errorf("expected nil cmd from handleTagDelete, got non-nil")
	}

	// The undo action should reference canonical index 2, not cursor 0.
	if m.undo.slot.action == nil {
		t.Fatal("expected undo slot to be set")
	}
	ua, ok := m.undo.slot.action.(undoDeleteTagAction)
	if !ok {
		t.Fatalf("expected undoDeleteTagAction, got %T", m.undo.slot)
	}
	if ua.idx != 2 {
		t.Errorf("undo canonical index: expected 2, got %d", ua.idx)
	}
	if ua.tag.ID != 3 {
		t.Errorf("undo tag ID: expected 3 (Charlie), got %d", ua.tag.ID)
	}

	// Cleanup stays with the retained action.
	if m.undo.slot.cleanup == nil {
		t.Error("expected cleanup to be set")
	}
}

// TestTagDeletePurgeDoesNotCorruptOtherSlot verifies that replacing a tag undo
// releases only the old cleanup and retains the new action.
func TestTagDeletePurgeDoesNotCorruptOtherSlot(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewTags
	m.tagsView.Level = tagtui.TagLevel
	m.tagsView.Tags = []db.TagRow{
		{ID: 1, Name: "Alpha"},
		{ID: 2, Name: "Beta"},
	}
	m.tagsView.TagCounts = map[int64]int{1: 1, 2: 1}
	m.tagsView.Height = 40
	m = m.rebuildTagCache(m.tagsView.Tags)
	m.tagsView.Cursor = 0

	// Delete "Alpha" — this should set up the undo slot with a cleanup.
	m, _ = m.handleTagDelete()

	if m.undo.slot.cleanup == nil {
		t.Fatal("missing retained tag cleanup")
	}
	retired := false
	m.undo.slot.cleanup = func() { retired = true }
	m.setUndoSlot(undoEnqueueAction{id: 999, entry: queue.Entry{Display: "test"}}, func() { t.Fatal("new cleanup ran early") })
	if !retired || m.undo.slot.cleanup == nil {
		t.Fatal("replacement did not transfer cleanup ownership")
	}
}

// TestAutoDeletedTagPurgeDoesNotCorruptUndoSlot protects retained undo from
// a background automatic deletion.
func TestAutoDeletedTagPurgeDoesNotCorruptUndoSlot(t *testing.T) {
	m := New(context.Background())

	// Set up an undo slot with cleanup that must stay dormant.
	m.undo.slot = undoSlot{action: undoEnqueueAction{id: 42}, cleanup: func() { t.Fatal("automatic deletion purged retained undo") }}
	msg := tagToggledMsg{tagID: 1, hashes: []string{"h1"}, wasAdd: false, autoDeleted: true}
	m, _ = m.handleTagToggled(msg)
	if m.undo.slot.cleanup == nil {
		t.Fatal("automatic deletion lost retained cleanup")
	}
}

// TestTagDeleteUndoRestoresCanonicalPosition verifies that undoing a tag
// deletion re-inserts the tag at its original canonical position, even when
// the deletion happened while a filter was active.
func TestTagDeleteUndoRestoresCanonicalPosition(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewTags
	m.tagsView.Level = tagtui.TagLevel
	m.tagsView.Tags = []db.TagRow{
		{ID: 1, Name: "Alpha"},
		{ID: 2, Name: "Beta"},
		{ID: 3, Name: "Charlie"},
		{ID: 4, Name: "Delta"},
	}
	m.tagsView.TagCounts = map[int64]int{1: 1, 2: 1, 3: 1, 4: 1}
	m.tagsView.Height = 40
	m = m.rebuildTagCache(m.tagsView.Tags)

	// Delete Charlie (canonical index 2) while filter is active.
	m.tagsView.Filter = filter.State{Text: "charlie"}
	m.tagsView.Cursor = 0
	m, _ = m.handleTagDelete()

	// Clear filter to see the canonical list.
	m.tagsView.Filter = filter.State{}

	// Charlie should be removed.
	if len(m.tagsView.Tags) != 3 {
		t.Fatalf("expected 3 tags after delete, got %d", len(m.tagsView.Tags))
	}
	for _, tg := range m.tagsView.Tags {
		if tg.ID == 3 {
			t.Fatal("Charlie should have been removed")
		}
	}

	// Undo: perform the stored undo action.
	ua := m.undo.slot.action.(undoDeleteTagAction)
	m, _ = ua.perform(m)

	// Charlie should be restored at canonical position 2.
	if len(m.tagsView.Tags) != 4 {
		t.Fatalf("expected 4 tags after undo, got %d", len(m.tagsView.Tags))
	}
	if m.tagsView.Tags[2].ID != 3 {
		t.Errorf("expected tag at index 2 to be Charlie (ID=3), got ID=%d Name=%q",
			m.tagsView.Tags[2].ID, m.tagsView.Tags[2].Name)
	}
	// Verify the full order is restored.
	expectedIDs := []int64{1, 2, 3, 4}
	for i, want := range expectedIDs {
		if m.tagsView.Tags[i].ID != want {
			t.Errorf("tag[%d]: expected ID %d, got %d", i, want, m.tagsView.Tags[i].ID)
		}
	}
}

// TestTagDeleteClearsActiveTagFilter verifies that deleting the tag currently
// used as the cross-view tag filter clears m.activeTagFilter so the user is
// not stranded looking at an empty list with a "[tag: ]" badge whose tag name
// lookup now returns the empty string.
func TestTagDeleteClearsActiveTagFilter(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewTags
	m.tagsView.Level = tagtui.TagLevel
	m.tagsView.Tags = []db.TagRow{
		{ID: 1, Name: "Alpha"},
		{ID: 2, Name: "Beta"},
	}
	m.tagsView.TagCounts = map[int64]int{1: 1, 2: 1}
	m.tagsView.Height = 40
	m = m.rebuildTagCache(m.tagsView.Tags)

	// Activate the cross-view filter on tag ID 1 (Alpha).
	alphaID := int64(1)
	m.activeTagFilter = &alphaID

	// Delete Alpha (cursor at 0).
	m.tagsView.Cursor = 0
	m, _ = m.handleTagDelete()

	if m.activeTagFilter != nil {
		t.Errorf("activeTagFilter should be cleared after deleting the filtered tag; got %v", *m.activeTagFilter)
	}
}
