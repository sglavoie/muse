package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

// TestPlaylistRenamedMsgAdjustsScrollOffset verifies that after a playlist
// rename causes the entry to sort to a different position, scrollOffset is adjusted.
func TestPlaylistRenamedMsgAdjustsScrollOffset(t *testing.T) {
	var playlists []db.PlaylistRow
	for i := range 10 {
		playlists = append(playlists, db.PlaylistRow{ID: int64(i + 1), Name: string(rune('A' + i))})
	}
	m := New(context.Background(), WithPlaylists(playlists))
	m.activeView = viewPlaylists
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 6})
	m = next.(Model)
	m.playlists.cursor = 0
	m.playlists.scrollOffset = 0

	renamedPlaylists := make([]db.PlaylistRow, 0, 10)
	for i := 1; i < 10; i++ {
		renamedPlaylists = append(renamedPlaylists, db.PlaylistRow{ID: int64(i + 1), Name: string(rune('A' + i))})
	}
	renamedPlaylists = append(renamedPlaylists, db.PlaylistRow{ID: 1, Name: "Z"})

	next, _ = m.Update(playlistRenamedMsg{id: 1, playlists: renamedPlaylists})
	m = next.(Model)

	if m.playlists.cursor != 9 {
		t.Fatalf("expected cursor=9 (renamed entry), got %d", m.playlists.cursor)
	}
	if m.playlists.scrollOffset+3 <= m.playlists.cursor {
		t.Fatalf("cursor %d is not visible: scrollOffset=%d, visibleRows=3", m.playlists.cursor, m.playlists.scrollOffset)
	}
}

// TestPlaylistCreatedMsgAdjustsScrollOffset verifies that after a playlist
// creation causes the entry to sort beyond the visible window, scrollOffset is adjusted.
func TestPlaylistCreatedMsgAdjustsScrollOffset(t *testing.T) {
	var playlists []db.PlaylistRow
	for i := range 5 {
		playlists = append(playlists, db.PlaylistRow{ID: int64(i + 1), Name: string(rune('A' + i))})
	}
	m := New(context.Background(), WithPlaylists(playlists))
	m.activeView = viewPlaylists
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 6})
	m = next.(Model)
	m.playlists.cursor = 0
	m.playlists.scrollOffset = 0

	createdPlaylists := make([]db.PlaylistRow, len(playlists))
	copy(createdPlaylists, playlists)
	createdPlaylists = append(createdPlaylists, db.PlaylistRow{ID: 6, Name: "Z"})

	next, _ = m.Update(playlistCreatedMsg{id: 6, playlists: createdPlaylists})
	m = next.(Model)

	if m.playlists.cursor != 5 {
		t.Fatalf("expected cursor=5 (new entry), got %d", m.playlists.cursor)
	}
	if m.playlists.scrollOffset+3 <= m.playlists.cursor {
		t.Fatalf("cursor %d is not visible: scrollOffset=%d, visibleRows=3", m.playlists.cursor, m.playlists.scrollOffset)
	}
}

// TestPlaylistRenamedMsgCursorUsesEffectiveSort verifies that after a rename,
// the cursor is positioned by searching the effective (sorted) playlist list
// rather than the unsorted one. Before the fix, the cursor would index into
// the sorted list using the unsorted position — wrong when sort is active.
func TestPlaylistRenamedMsgCursorUsesEffectiveSort(t *testing.T) {
	// Unsorted order: Rock(1), Alpha(2), Jazz(3).
	// Sort by name asc: Alpha(2), Jazz(3), Rock(1).
	playlists := []db.PlaylistRow{
		{ID: 1, Name: "Rock"},
		{ID: 2, Name: "Alpha"},
		{ID: 3, Name: "Jazz"},
	}
	m := New(context.Background(), WithPlaylists(playlists))
	m.activeView = viewPlaylists
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(Model)

	// Activate name-ascending sort.
	m.playlists.sortState = SortState{Field: SortName, Dir: SortAsc}
	m.playlists = m.playlists.recomputePlaylistSort()

	// Rename "Rock" (ID=1) to "Zulu". After rename, unsorted order is:
	// Zulu(1), Alpha(2), Jazz(3). Sorted by name asc: Alpha(2), Jazz(3), Zulu(1).
	renamedPlaylists := []db.PlaylistRow{
		{ID: 1, Name: "Zulu"},
		{ID: 2, Name: "Alpha"},
		{ID: 3, Name: "Jazz"},
	}
	next, _ = m.Update(playlistRenamedMsg{id: 1, playlists: renamedPlaylists})
	m = next.(Model)

	// "Zulu" (ID=1) should be at sorted index 2 (Alpha=0, Jazz=1, Zulu=2).
	effective := m.playlists.effectivePlaylists()
	if m.playlists.cursor < 0 || m.playlists.cursor >= len(effective) {
		t.Fatalf("cursor %d out of range [0, %d)", m.playlists.cursor, len(effective))
	}
	if got := effective[m.playlists.cursor]; got.ID != 1 {
		t.Errorf("cursor should point to renamed playlist (ID=1), got ID=%d (%q) at index %d",
			got.ID, got.Name, m.playlists.cursor)
	}
}

// TestPlaylistCreatedMsgCursorUsesEffectiveSort verifies that after a create,
// the cursor is positioned by searching the effective (sorted) playlist list.
func TestPlaylistCreatedMsgCursorUsesEffectiveSort(t *testing.T) {
	// Existing unsorted: Rock(1), Alpha(2). Sort by name asc: Alpha(2), Rock(1).
	playlists := []db.PlaylistRow{
		{ID: 1, Name: "Rock"},
		{ID: 2, Name: "Alpha"},
	}
	m := New(context.Background(), WithPlaylists(playlists))
	m.activeView = viewPlaylists
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(Model)

	m.playlists.sortState = SortState{Field: SortName, Dir: SortAsc}
	m.playlists = m.playlists.recomputePlaylistSort()

	// Create "Beta" (ID=3). Unsorted: Rock(1), Alpha(2), Beta(3).
	// Sorted by name asc: Alpha(2), Beta(3), Rock(1).
	createdPlaylists := []db.PlaylistRow{
		{ID: 1, Name: "Rock"},
		{ID: 2, Name: "Alpha"},
		{ID: 3, Name: "Beta"},
	}
	next, _ = m.Update(playlistCreatedMsg{id: 3, playlists: createdPlaylists})
	m = next.(Model)

	// "Beta" (ID=3) should be at sorted index 1 (Alpha=0, Beta=1, Rock=2).
	effective := m.playlists.effectivePlaylists()
	if m.playlists.cursor < 0 || m.playlists.cursor >= len(effective) {
		t.Fatalf("cursor %d out of range [0, %d)", m.playlists.cursor, len(effective))
	}
	if got := effective[m.playlists.cursor]; got.ID != 3 {
		t.Errorf("cursor should point to created playlist (ID=3), got ID=%d (%q) at index %d",
			got.ID, got.Name, m.playlists.cursor)
	}
}

// TestSmartPlaylistCreatedMsgCursorUsesEffectiveSort verifies that after a
// smart playlist is created, the cursor is positioned in the effective
// (sorted) list rather than the unsorted one.
func TestSmartPlaylistCreatedMsgCursorUsesEffectiveSort(t *testing.T) {
	playlists := []db.PlaylistRow{
		{ID: 1, Name: "Rock"},
		{ID: 2, Name: "Alpha"},
	}
	m := New(context.Background(), WithPlaylists(playlists))
	m.activeView = viewPlaylists
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(Model)

	m.playlists.sortState = SortState{Field: SortName, Dir: SortAsc}
	m.playlists = m.playlists.recomputePlaylistSort()

	// Create smart playlist "Beta" (ID=3). Sorted: Alpha, Beta, Rock.
	createdPlaylists := []db.PlaylistRow{
		{ID: 1, Name: "Rock"},
		{ID: 2, Name: "Alpha"},
		{ID: 3, Name: "Beta", IsSmart: true},
	}
	next, _ = m.Update(smartPlaylistCreatedMsg{id: 3, playlists: createdPlaylists})
	m = next.(Model)

	effective := m.playlists.effectivePlaylists()
	if m.playlists.cursor < 0 || m.playlists.cursor >= len(effective) {
		t.Fatalf("cursor %d out of range [0, %d)", m.playlists.cursor, len(effective))
	}
	if got := effective[m.playlists.cursor]; got.ID != 3 {
		t.Errorf("cursor should point to created smart playlist (ID=3), got ID=%d (%q) at index %d",
			got.ID, got.Name, m.playlists.cursor)
	}
}

// TestPlaylistAddErrorMsgShowsNotification verifies that playlistAddErrorMsg
// sets a transient error notification on the status bar.
func TestPlaylistAddErrorMsgShowsNotification(t *testing.T) {
	m := New(context.Background())
	next, _ := m.Update(playlistAddErrorMsg{err: errors.New("disk full")})
	nm := next.(Model)

	if nm.statusBar.notification.msg == "" {
		t.Fatal("expected a status bar notification for playlistAddErrorMsg, got empty")
	}
	if !strings.Contains(nm.statusBar.notification.msg, "disk full") {
		t.Errorf("notification should contain error text, got %q", nm.statusBar.notification.msg)
	}
	if nm.statusBar.notification.expiresAt.Before(time.Now()) {
		t.Error("notification should expire in the future")
	}
}

// TestSmartPlaylistCountsUpdatesSortedCopy verifies that when smart playlist
// track counts are updated, the sorted playlist copy is recomputed so that
// sort-by-TrackCount reflects the new values and order.
func TestSmartPlaylistCountsUpdatesSortedCopy(t *testing.T) {
	playlists := []db.PlaylistRow{
		{ID: 1, Name: "Rock", TrackCount: 10},
		{ID: 2, Name: "Smart", TrackCount: 0, IsSmart: true, Query: `{"field":"artist","op":"eq","value":"X"}`},
	}
	m := New(context.Background(), WithPlaylists(playlists))
	m.activeView = viewPlaylists
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(Model)

	// Sort by track count descending: Rock(10) before Smart(0).
	m.playlists.sortState = SortState{Field: SortTrackCount, Dir: SortDesc}
	m.playlists = m.playlists.recomputePlaylistSort()

	effective := m.playlists.effectivePlaylists()
	if effective[0].ID != 1 {
		t.Fatalf("before update: first should be Rock(ID=1), got ID=%d", effective[0].ID)
	}

	// Simulate smart playlist count update: Smart now has 20 tracks.
	next, _ = m.Update(smartPlaylistCountsUpdatedMsg{counts: map[int64]int{2: 20}})
	m = next.(Model)

	effective = m.playlists.effectivePlaylists()
	if effective[0].ID != 2 {
		t.Errorf("after count update: first should be Smart(ID=2, count=20), got ID=%d (count=%d)",
			effective[0].ID, effective[0].TrackCount)
	}
	if effective[0].TrackCount != 20 {
		t.Errorf("Smart playlist track count should be 20, got %d", effective[0].TrackCount)
	}
}

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
