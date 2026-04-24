package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/filter"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// TestTagDeleteStoresCanonicalIndex verifies that handleTagDelete stores the
// canonical (unfiltered) index in the undo action and tagDeletedMsg, not the
// display cursor position. When a filter is active the cursor indexes the
// filtered list, which is shorter than the canonical list.
func TestTagDeleteStoresCanonicalIndex(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewTags
	m.tagsView.level = tagtui.TagLevel
	m.tagsView.tags = []db.TagRow{
		{ID: 1, Name: "Alpha"},
		{ID: 2, Name: "Beta"},
		{ID: 3, Name: "Charlie"},
		{ID: 4, Name: "Delta"},
	}
	m.tagsView.tagCounts = map[int64]int{1: 1, 2: 1, 3: 1, 4: 1}
	m.tagsView.height = 40
	m = m.rebuildTagCache(m.tagsView.tags)

	// Activate a filter that shows only "Charlie" (canonical index 2).
	m.tagsView.filter = filter.State{Text: "charlie"}
	// Cursor 0 in the filtered list points to Charlie.
	m.tagsView.cursor = 0

	m, cmd := m.handleTagDelete()

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

	// The tagDeletedMsg returned by the cmd should also carry the canonical index.
	if cmd == nil {
		t.Fatal("expected a non-nil cmd")
	}
	msg := cmd()
	dm, ok := msg.(tagDeletedMsg)
	if !ok {
		t.Fatalf("expected tagDeletedMsg, got %T", msg)
	}
	if dm.idx != 2 {
		t.Errorf("tagDeletedMsg canonical index: expected 2, got %d", dm.idx)
	}
}

// TestTagDeleteUndoRestoresCanonicalPosition verifies that undoing a tag
// deletion re-inserts the tag at its original canonical position, even when
// the deletion happened while a filter was active.
func TestTagDeleteUndoRestoresCanonicalPosition(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewTags
	m.tagsView.level = tagtui.TagLevel
	m.tagsView.tags = []db.TagRow{
		{ID: 1, Name: "Alpha"},
		{ID: 2, Name: "Beta"},
		{ID: 3, Name: "Charlie"},
		{ID: 4, Name: "Delta"},
	}
	m.tagsView.tagCounts = map[int64]int{1: 1, 2: 1, 3: 1, 4: 1}
	m.tagsView.height = 40
	m = m.rebuildTagCache(m.tagsView.tags)

	// Delete Charlie (canonical index 2) while filter is active.
	m.tagsView.filter = filter.State{Text: "charlie"}
	m.tagsView.cursor = 0
	m, _ = m.handleTagDelete()

	// Clear filter to see the canonical list.
	m.tagsView.filter = filter.State{}

	// Charlie should be removed.
	if len(m.tagsView.tags) != 3 {
		t.Fatalf("expected 3 tags after delete, got %d", len(m.tagsView.tags))
	}
	for _, tg := range m.tagsView.tags {
		if tg.ID == 3 {
			t.Fatal("Charlie should have been removed")
		}
	}

	// Undo: perform the stored undo action.
	ua := m.undo.slot.action.(undoDeleteTagAction)
	m, _ = ua.perform(m)

	// Charlie should be restored at canonical position 2.
	if len(m.tagsView.tags) != 4 {
		t.Fatalf("expected 4 tags after undo, got %d", len(m.tagsView.tags))
	}
	if m.tagsView.tags[2].ID != 3 {
		t.Errorf("expected tag at index 2 to be Charlie (ID=3), got ID=%d Name=%q",
			m.tagsView.tags[2].ID, m.tagsView.tags[2].Name)
	}
	// Verify the full order is restored.
	expectedIDs := []int64{1, 2, 3, 4}
	for i, want := range expectedIDs {
		if m.tagsView.tags[i].ID != want {
			t.Errorf("tag[%d]: expected ID %d, got %d", i, want, m.tagsView.tags[i].ID)
		}
	}
}
