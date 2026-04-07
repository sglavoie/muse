package tui

import (
	"context"
	"testing"
)

// TestEnqueueReplaceEmptyLibraryDoesNotClearQueue verifies that
// actionLibraryEnqueueReplace in the Library view does not clear the existing
// queue when the library is empty (no focused track to replace with).
func TestEnqueueReplaceEmptyLibraryDoesNotClearQueue(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewLibrary
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Title: "Existing", Artist: "A"},
	}

	next, _ := m.handleKeyAction(actionLibraryEnqueueReplace)
	nm := next.(Model)
	if len(nm.queue.entries) == 0 {
		t.Fatal("queue must not be cleared when library is empty")
	}
	if len(nm.queue.entries) != 1 || nm.queue.entries[0].Hash != "h1" {
		t.Errorf("queue should be unchanged; got %d entries", len(nm.queue.entries))
	}
}

// TestEnqueueReplaceFavoritesCategoryLevelDoesNotClearQueue verifies that
// actionLibraryEnqueueReplace on the Favorites view at CategoryLevel does
// not clear the queue (there is no focused track at this level).
func TestEnqueueReplaceFavoritesCategoryLevelDoesNotClearQueue(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewFavorites
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Title: "Existing", Artist: "A"},
	}

	next, _ := m.handleKeyAction(actionLibraryEnqueueReplace)
	nm := next.(Model)
	if len(nm.queue.entries) == 0 {
		t.Fatal("queue must not be cleared at favorites CategoryLevel")
	}
	if len(nm.queue.entries) != 1 || nm.queue.entries[0].Hash != "h1" {
		t.Errorf("queue should be unchanged; got %d entries", len(nm.queue.entries))
	}
}
