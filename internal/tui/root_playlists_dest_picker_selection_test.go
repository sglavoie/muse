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
	m.genres.selectedHashes = map[string]struct{}{"h1": {}, "h2": {}}

	t.Run("dismissed", func(t *testing.T) {
		got := m.handleDestinationPickerDismissedMsg()
		if got.genres.selectedHashes != nil {
			t.Error("genres selectedHashes should be nil after dismiss")
		}
	})

	t.Run("create", func(t *testing.T) {
		got := m.handleDestinationPickerCreateMsg()
		if got.genres.selectedHashes != nil {
			t.Error("genres selectedHashes should be nil after create")
		}
	})

	t.Run("picked", func(t *testing.T) {
		got, _ := m.handleDestinationPickedMsg(pltui.DestinationPickedMsg{IsQueue: true})
		if got.genres.selectedHashes != nil {
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

// TestDestPickerClearsRecentlyAddedAndHistorySelection verifies that
// dismissing, creating from, or confirming a destination picker clears
// the recentlyAdded and history views' bulk selections.
func TestDestPickerClearsRecentlyAddedAndHistorySelection(t *testing.T) {
	m := New(context.Background())
	m.recentlyAdded.selectedHashes = map[string]struct{}{"h1": {}}
	m.history.selectedHashes = map[string]struct{}{"h2": {}}

	t.Run("dismissed", func(t *testing.T) {
		got := m.handleDestinationPickerDismissedMsg()
		if got.recentlyAdded.selectedHashes != nil {
			t.Error("recentlyAdded selectedHashes should be nil after dismiss")
		}
		if got.history.selectedHashes != nil {
			t.Error("history selectedHashes should be nil after dismiss")
		}
	})

	t.Run("create", func(t *testing.T) {
		got := m.handleDestinationPickerCreateMsg()
		if got.recentlyAdded.selectedHashes != nil {
			t.Error("recentlyAdded selectedHashes should be nil after create")
		}
		if got.history.selectedHashes != nil {
			t.Error("history selectedHashes should be nil after create")
		}
	})

	t.Run("picked", func(t *testing.T) {
		got, _ := m.handleDestinationPickedMsg(pltui.DestinationPickedMsg{IsQueue: true})
		if got.recentlyAdded.selectedHashes != nil {
			t.Error("recentlyAdded selectedHashes should be nil after pick")
		}
		if got.history.selectedHashes != nil {
			t.Error("history selectedHashes should be nil after pick")
		}
	})
}
