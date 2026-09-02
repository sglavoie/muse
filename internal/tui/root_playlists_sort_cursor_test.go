package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

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
	m.playlists.sortState = sorting.SortState{Field: sorting.SortName, Dir: sorting.SortAsc}
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

	m.playlists.sortState = sorting.SortState{Field: sorting.SortName, Dir: sorting.SortAsc}
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

	m.playlists.sortState = sorting.SortState{Field: sorting.SortName, Dir: sorting.SortAsc}
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
