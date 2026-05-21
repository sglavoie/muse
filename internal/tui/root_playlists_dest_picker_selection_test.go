package tui

import (
	"context"
	"testing"

	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

// TestDestPickerClearsGenresSelection verifies that dismissing, creating from,
// or confirming a destination picker clears the genres view's bulk selection.
func TestDestPickerClearsGenresSelection(t *testing.T) {
	m := New(context.Background())
	m.browse.genres.selectedHashes = map[string]struct{}{"h1": {}, "h2": {}}

	t.Run("dismissed", func(t *testing.T) {
		got := m.handleDestinationPickerDismissedMsg()
		if got.browse.genres.selectedHashes != nil {
			t.Error("genres selectedHashes should be nil after dismiss")
		}
	})

	t.Run("create", func(t *testing.T) {
		got := m.handleDestinationPickerCreateMsg()
		if got.browse.genres.selectedHashes != nil {
			t.Error("genres selectedHashes should be nil after create")
		}
	})

	t.Run("picked", func(t *testing.T) {
		got, _ := m.handleDestinationPickedMsg(pltui.DestinationPickedMsg{IsQueue: true})
		if got.browse.genres.selectedHashes != nil {
			t.Error("genres selectedHashes should be nil after pick")
		}
	})
}

// TestDestPickerClearsTagsSelection verifies that dismissing, creating from,
// or confirming a destination picker clears the tags view's bulk selection.
func TestDestPickerClearsTagsSelection(t *testing.T) {
	m := New(context.Background())
	m.tagsView.selectedHashes = map[string]struct{}{"h1": {}, "h2": {}}

	t.Run("dismissed", func(t *testing.T) {
		got := m.handleDestinationPickerDismissedMsg()
		if got.tagsView.selectedHashes != nil {
			t.Error("tagsView selectedHashes should be nil after dismiss")
		}
	})

	t.Run("create", func(t *testing.T) {
		got := m.handleDestinationPickerCreateMsg()
		if got.tagsView.selectedHashes != nil {
			t.Error("tagsView selectedHashes should be nil after create")
		}
	})

	t.Run("picked", func(t *testing.T) {
		got, _ := m.handleDestinationPickedMsg(pltui.DestinationPickedMsg{IsQueue: true})
		if got.tagsView.selectedHashes != nil {
			t.Error("tagsView selectedHashes should be nil after pick")
		}
	})
}

// TestDestPickerClearsFavoritesSelection verifies that dismissing, creating
// from, or confirming a destination picker clears the favorites view's bulk
// selection (selectedItems).
func TestDestPickerClearsFavoritesSelection(t *testing.T) {
	m := New(context.Background())
	m.favorites.selectedItems = map[int]struct{}{0: {}, 2: {}}

	t.Run("dismissed", func(t *testing.T) {
		got := m.handleDestinationPickerDismissedMsg()
		if got.favorites.selectedItems != nil {
			t.Error("favorites selectedItems should be nil after dismiss")
		}
	})

	t.Run("create", func(t *testing.T) {
		got := m.handleDestinationPickerCreateMsg()
		if got.favorites.selectedItems != nil {
			t.Error("favorites selectedItems should be nil after create")
		}
	})

	t.Run("picked", func(t *testing.T) {
		got, _ := m.handleDestinationPickedMsg(pltui.DestinationPickedMsg{IsQueue: true})
		if got.favorites.selectedItems != nil {
			t.Error("favorites selectedItems should be nil after pick")
		}
	})
}

// TestDestPickerCreateExitsVisualMode verifies that creating a playlist via
// the destination picker exits visual mode. Without this, the stale visual
// snapshot from the source view would re-apply on the next cursor move once
// the user navigated back, silently restoring discarded selections.
func TestDestPickerCreateExitsVisualMode(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewLibrary
	m.visual = visualState{active: true, view: viewLibrary, anchor: 0}

	got := m.handleDestinationPickerCreateMsg()

	if got.visual.active {
		t.Error("expected visual mode to be inactive after destination picker create")
	}
	if got.activeView != viewPlaylists {
		t.Errorf("expected activeView=viewPlaylists, got %v", got.activeView)
	}
	if !got.playlists.creating {
		t.Error("expected playlists.creating=true after destination picker create")
	}
}

// TestClearAllSelectionsClearsQueueAndPlaylists verifies that the helper
// invoked from every destination-picker exit path drops bulk selections in
// the queue and playlists views too, not just library/browse/favorites/tags.
func TestClearAllSelectionsClearsQueueAndPlaylists(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{{Hash: "h1", Selected: true}, {Hash: "h2", Selected: true}}
	m.playlists.selectedTrackIDs = map[int64]struct{}{1: {}, 2: {}}

	got := m.clearAllSelections()

	for i, e := range got.queue.entries {
		if e.Selected {
			t.Errorf("queue entry %d should be deselected", i)
		}
	}
	if got.playlists.selectedTrackIDs != nil {
		t.Error("playlists.selectedTrackIDs should be nil after clearAllSelections")
	}
}

// TestDestPickerClearsRecentlyAddedAndHistorySelection verifies that
// dismissing, creating from, or confirming a destination picker clears
// the library view's bulk selections (used for both Recently Added and History modes).
func TestDestPickerClearsRecentlyAddedAndHistorySelection(t *testing.T) {
	m := New(context.Background())
	m.library.selectedHashes = map[string]struct{}{"h1": {}}

	t.Run("dismissed", func(t *testing.T) {
		got := m.handleDestinationPickerDismissedMsg()
		if got.library.selectedHashes != nil {
			t.Error("library selectedHashes should be nil after dismiss")
		}
	})

	t.Run("create", func(t *testing.T) {
		got := m.handleDestinationPickerCreateMsg()
		if got.library.selectedHashes != nil {
			t.Error("library selectedHashes should be nil after create")
		}
	})

	t.Run("picked", func(t *testing.T) {
		got, _ := m.handleDestinationPickedMsg(pltui.DestinationPickedMsg{IsQueue: true})
		if got.library.selectedHashes != nil {
			t.Error("library selectedHashes should be nil after pick")
		}
	})
}
