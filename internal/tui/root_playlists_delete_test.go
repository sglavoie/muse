package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

// TestPlaylistsBulkRemoveAtTrackLevel verifies the 'a' key at TrackLevel with
// a non-empty bulk selection: selected tracks are removed from the in-memory
// slice, the cursor is clamped, and selectedTrackIDs is cleared.
func TestPlaylistsBulkRemoveAtTrackLevel(t *testing.T) {
	m := New(context.Background())

	// Navigate to playlists view.
	next, _ := m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	nm := next.(Model)

	// Seed the playlists panel directly at TrackLevel with three tracks.
	nm.playlists.level = pltui.TrackLevel
	nm.playlists.tracks = []db.PlaylistTrackRow{{ID: 10, Position: 1, Title: "Track One", Artist: "A", DurationMs: 60000}, {ID: 20, Position: 2, Title: "Track Two", Artist: "B", DurationMs: 60000}, {ID: 30, Position: 3, Title: "Track Three", Artist: "C", DurationMs: 60000}}
	nm.playlists.selectedTrackIDs = map[int64]struct{}{10: {}, 20: {}}
	nm.playlists.cursor = 2

	// Press 'd' — should trigger actionPlaylistDelete which bulk-removes when tracks are selected.
	next2, _ := nm.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	nm2 := next2.(Model)

	if len(nm2.playlists.tracks) != 1 {
		t.Fatalf("expected 1 track remaining, got %d", len(nm2.playlists.tracks))
	}
	if nm2.playlists.tracks[0].ID != 30 {
		t.Fatalf("expected remaining track ID=30, got ID=%d", nm2.playlists.tracks[0].ID)
	}
	if nm2.playlists.cursor >= len(nm2.playlists.tracks) {
		t.Fatalf("cursor %d out of range after bulk remove (len=%d)", nm2.playlists.cursor, len(nm2.playlists.tracks))
	}
	if nm2.playlists.selectedTrackIDs != nil {
		t.Fatalf("expected selectedTrackIDs to be nil after bulk remove, got %v", nm2.playlists.selectedTrackIDs)
	}

	if _, ok := nm2.undo.slot.action.(undoBulkRemovePlaylistTracksAction); !ok {
		t.Fatalf("expected undoBulkRemovePlaylistTracksAction, got %T", nm2.undo.slot.action)
	}
	nm3 := nm2.handleUndo()
	if len(nm3.playlists.tracks) != 3 {
		t.Fatalf("expected 3 tracks after undo, got %d", len(nm3.playlists.tracks))
	}
	wantIDs := []int64{10, 20, 30}
	for i, want := range wantIDs {
		if nm3.playlists.tracks[i].ID != want {
			t.Errorf("track[%d]: expected ID=%d, got ID=%d", i, want, nm3.playlists.tracks[i].ID)
		}
	}
}

// TestPlaylistDeleteClampsScrollOffset verifies that deleting a playlist when
// the list is scrolled clamps scrollOffset so it does not point past the end.
func TestPlaylistDeleteClampsScrollOffset(t *testing.T) {
	pd := &stubPlaylistDeleter{}
	m := New(context.Background(), WithPlaylistDeleter(pd), WithPlaylists([]db.PlaylistRow{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}}))
	m.activeView = viewPlaylists
	m.playlists.cursor = 1
	m.playlists.scrollOffset = 1

	next, _ := m.handleKeyAction(actionPlaylistDelete)
	nm := next.(Model)

	if n := len(nm.playlists.playlists); n != 1 {
		t.Fatalf("expected 1 playlist remaining, got %d", n)
	}
	if nm.playlists.scrollOffset > len(nm.playlists.playlists)-1 {
		t.Fatalf("scrollOffset %d exceeds max valid offset %d after playlist delete", nm.playlists.scrollOffset, len(nm.playlists.playlists)-1)
	}
}

// TestPlaylistDeleteAllClampsScrollOffsetToZero verifies that deleting the last
// playlist resets scrollOffset to 0.
func TestPlaylistDeleteAllClampsScrollOffsetToZero(t *testing.T) {
	pd := &stubPlaylistDeleter{}
	m := New(context.Background(), WithPlaylistDeleter(pd), WithPlaylists([]db.PlaylistRow{{ID: 1, Name: "A"}}))
	m.activeView = viewPlaylists
	m.playlists.cursor = 0
	m.playlists.scrollOffset = 0

	next, _ := m.handleKeyAction(actionPlaylistDelete)
	nm := next.(Model)

	if nm.playlists.scrollOffset != 0 {
		t.Fatalf("scrollOffset must be 0 after deleting all playlists, got %d", nm.playlists.scrollOffset)
	}
}

// TestPlaylistTrackDeleteClampsScrollOffset verifies that removing a track at
// TrackLevel clamps scrollOffset so it does not point past the end.
func TestPlaylistTrackDeleteClampsScrollOffset(t *testing.T) {
	pd := &stubPlaylistDeleter{}
	m := New(context.Background(), WithPlaylistDeleter(pd))
	m.activeView = viewPlaylists

	m.playlists.level = pltui.TrackLevel
	m.playlists.tracks = []db.PlaylistTrackRow{{ID: 10, Position: 1, Title: "A", Artist: "X", DurationMs: 1000}, {ID: 20, Position: 2, Title: "B", Artist: "Y", DurationMs: 2000}}
	m.playlists.cursor = 1
	m.playlists.scrollOffset = 1

	next, _ := m.handleKeyAction(actionPlaylistDelete)
	nm := next.(Model)

	if n := len(nm.playlists.tracks); n != 1 {
		t.Fatalf("expected 1 track remaining, got %d", n)
	}
	if nm.playlists.scrollOffset > len(nm.playlists.tracks)-1 {
		t.Fatalf("scrollOffset %d exceeds max valid offset %d after track delete", nm.playlists.scrollOffset, len(nm.playlists.tracks)-1)
	}
}

// TestBulkRemovePlaylistTracksClampsScrollOffset verifies that bulk-removing
// tracks clamps scrollOffset so it does not point past the end.
func TestBulkRemovePlaylistTracksClampsScrollOffset(t *testing.T) {
	pd := &stubPlaylistDeleter{}
	m := New(context.Background(), WithPlaylistDeleter(pd))
	m.activeView = viewPlaylists

	m.playlists.level = pltui.TrackLevel
	m.playlists.tracks = []db.PlaylistTrackRow{{ID: 10, Position: 1, Title: "A", Artist: "X", DurationMs: 1000}, {ID: 20, Position: 2, Title: "B", Artist: "Y", DurationMs: 2000}, {ID: 30, Position: 3, Title: "C", Artist: "Z", DurationMs: 3000}}
	m.playlists.selectedTrackIDs = map[int64]struct{}{10: {}, 20: {}}
	m.playlists.cursor = 2
	m.playlists.scrollOffset = 2

	next, _ := m.handleKeyAction(actionPlaylistDelete)
	nm := next.(Model)

	if n := len(nm.playlists.tracks); n != 1 {
		t.Fatalf("expected 1 track remaining, got %d", n)
	}
	if nm.playlists.scrollOffset > 0 {
		t.Fatalf("scrollOffset %d must be 0 after bulk remove with 1 remaining track", nm.playlists.scrollOffset)
	}
}

// TestBulkRemovePlaylistTracksSoftDeletesAndSchedulesPurge verifies that
// actionBulkRemovePlaylistTracks calls RemovePlaylistTrackSoft for each
// selected track when a playlistDeleter is wired.
func TestBulkRemovePlaylistTracksSoftDeletesAndSchedulesPurge(t *testing.T) {
	pd := &stubPlaylistDeleter{}
	m := New(context.Background(), WithPlaylistDeleter(pd))
	m.activeView = viewPlaylists

	m.playlists.level = pltui.TrackLevel
	m.playlists.tracks = []db.PlaylistTrackRow{{ID: 10, Position: 1, Title: "A", Artist: "X", DurationMs: 1000}, {ID: 20, Position: 2, Title: "B", Artist: "Y", DurationMs: 2000}, {ID: 30, Position: 3, Title: "C", Artist: "Z", DurationMs: 3000}}
	m.playlists.selectedTrackIDs = map[int64]struct{}{10: {}, 30: {}}

	next, _ := m.handleKeyAction(actionPlaylistDelete)
	_ = next.(Model)

	if got := len(pd.removePlaylistTrackSoftCalls); got != 2 {
		t.Fatalf("expected 2 RemovePlaylistTrackSoft calls, got %d", got)
	}
	softDeleted := map[int64]bool{}
	for _, id := range pd.removePlaylistTrackSoftCalls {
		softDeleted[id] = true
	}
	if !softDeleted[10] || !softDeleted[30] {
		t.Fatalf("expected IDs 10 and 30 to be soft-deleted, got %v", pd.removePlaylistTrackSoftCalls)
	}
}

// TestPlaylistDeleteNoPanicWhenCursorOutOfBounds verifies that
// actionPlaylistDelete does not panic when the cursor exceeds the playlist slice length.
func TestPlaylistDeleteNoPanicWhenCursorOutOfBounds(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewPlaylists
	m.playlists.playlists = []db.PlaylistRow{{ID: 1, Name: "A"}}
	m.playlists.cursor = 5

	next, _ := m.handleKeyAction(actionPlaylistDelete)
	_ = next
}

// TestPlaylistTrackDeleteNoPanicWhenCursorOutOfBounds verifies that
// actionPlaylistDelete at track level does not panic when the cursor exceeds the tracks slice length.
func TestPlaylistTrackDeleteNoPanicWhenCursorOutOfBounds(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewPlaylists
	m.playlists.level = pltui.TrackLevel
	m.playlists.tracks = []db.PlaylistTrackRow{{ID: 1, Title: "A"}}
	m.playlists.cursor = 5

	next, _ := m.handleKeyAction(actionPlaylistDelete)
	_ = next
}

// TestBulkActionInPlaylistsRemovesImmediately verifies that pressing 'a' in
// Playlists at TrackLevel with selected tracks immediately removes them —
// no two-press confirmation required since the undo window handles reversibility.
func TestBulkActionInPlaylistsRemovesImmediately(t *testing.T) {
	pd := &stubPlaylistDeleter{}
	m := New(context.Background(), WithPlaylistDeleter(pd))
	m.activeView = viewPlaylists

	m.playlists.level = pltui.TrackLevel
	m.playlists.tracks = []db.PlaylistTrackRow{{ID: 10, Position: 1, Title: "A", Artist: "X", DurationMs: 1000}, {ID: 20, Position: 2, Title: "B", Artist: "Y", DurationMs: 2000}, {ID: 30, Position: 3, Title: "C", Artist: "Z", DurationMs: 3000}}
	m.playlists.selectedTrackIDs = map[int64]struct{}{10: {}, 20: {}}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	nm := next.(Model)

	if len(nm.playlists.tracks) != 1 {
		t.Fatalf("expected 1 track remaining after removal, got %d", len(nm.playlists.tracks))
	}
	if nm.playlists.tracks[0].ID != 30 {
		t.Fatalf("expected remaining track ID=30, got ID=%d", nm.playlists.tracks[0].ID)
	}
	if nm.playlists.selectedTrackIDs != nil {
		t.Fatalf("expected selectedTrackIDs to be nil after removal, got %v", nm.playlists.selectedTrackIDs)
	}

	if _, ok := nm.undo.slot.action.(undoBulkRemovePlaylistTracksAction); !ok {
		t.Fatalf("expected undoBulkRemovePlaylistTracksAction, got %T", nm.undo.slot.action)
	}
	nm2 := nm.handleUndo()
	if len(nm2.playlists.tracks) != 3 {
		t.Fatalf("expected 3 tracks after undo, got %d", len(nm2.playlists.tracks))
	}
	wantIDs := []int64{10, 20, 30}
	for i, want := range wantIDs {
		if nm2.playlists.tracks[i].ID != want {
			t.Errorf("track[%d]: expected ID=%d, got ID=%d", i, want, nm2.playlists.tracks[i].ID)
		}
	}
}

// TestBulkActionRemovalResetsSortState verifies that the bulk action removal
// path resets the playlists sort state.
func TestBulkActionRemovalResetsSortState(t *testing.T) {
	pd := &stubPlaylistDeleter{}
	m := New(context.Background(), WithPlaylistDeleter(pd))
	m.activeView = viewPlaylists
	m.playlists.level = pltui.TrackLevel
	m.playlists.tracks = []db.PlaylistTrackRow{{ID: 10, Position: 1, Title: "A", Artist: "X", DurationMs: 1000}, {ID: 20, Position: 2, Title: "B", Artist: "Y", DurationMs: 2000}, {ID: 30, Position: 3, Title: "C", Artist: "Z", DurationMs: 3000}}
	m.playlists.selectedTrackIDs = map[int64]struct{}{10: {}, 20: {}}

	m.playlists.sortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc}
	m.playlists = m.playlists.recomputePlaylistSort()
	if m.playlists.sortedTrackOrder == nil {
		t.Fatal("precondition: sortedTrackOrder should be non-nil after sort")
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	nm := next.(Model)

	if len(nm.playlists.tracks) != 1 {
		t.Fatalf("expected 1 track remaining, got %d", len(nm.playlists.tracks))
	}
	if nm.playlists.sortState.Field != sorting.SortTitle {
		t.Errorf("expected sort field preserved as SortTitle, got %v", nm.playlists.sortState.Field)
	}
	if nm.playlists.sortedTrackOrder == nil {
		t.Error("expected sortedTrackOrder to be recomputed after bulk removal with sort active")
	}
}

// TestPlaylistDeleteWithSortActiveDeletesCorrectEntry verifies that deleting a
// playlist when sort is active removes the correct entry.
func TestPlaylistDeleteWithSortActiveDeletesCorrectEntry(t *testing.T) {
	pd := &stubPlaylistDeleter{}
	m := New(context.Background(), WithPlaylistDeleter(pd), WithPlaylists([]db.PlaylistRow{{ID: 1, Name: "Zebra"}, {ID: 2, Name: "Alpha"}, {ID: 3, Name: "Middle"}}))
	m.activeView = viewPlaylists
	m.playlists.sortState = sorting.SortState{Field: sorting.SortName, Dir: sorting.SortAsc}
	m.playlists = m.playlists.recomputePlaylistSort()
	m.playlists.cursor = 0

	next, _ := m.handleKeyAction(actionPlaylistDelete)
	nm := next.(Model)

	if len(pd.deletePlaylistSoftCalls) != 1 {
		t.Fatalf("expected 1 soft-delete call, got %d", len(pd.deletePlaylistSoftCalls))
	}
	if pd.deletePlaylistSoftCalls[0] != 2 {
		t.Fatalf("soft-delete should target ID=2 (Alpha), got ID=%d", pd.deletePlaylistSoftCalls[0])
	}

	if len(nm.playlists.playlists) != 2 {
		t.Fatalf("expected 2 playlists remaining, got %d", len(nm.playlists.playlists))
	}
	for _, p := range nm.playlists.playlists {
		if p.ID == 2 {
			t.Fatal("Alpha (ID=2) should have been removed from the canonical slice")
		}
	}
}

// TestPlaylistDeleteWithSortActiveUndoRestoresCorrectEntry verifies that
// undoing a sorted-view playlist deletion restores the entry at the correct position.
func TestPlaylistDeleteWithSortActiveUndoRestoresCorrectEntry(t *testing.T) {
	pd := &stubPlaylistDeleter{}
	m := New(context.Background(), WithPlaylistDeleter(pd), WithPlaylists([]db.PlaylistRow{{ID: 1, Name: "Zebra"}, {ID: 2, Name: "Alpha"}, {ID: 3, Name: "Middle"}}))
	m.activeView = viewPlaylists
	m.playlists.sortState = sorting.SortState{Field: sorting.SortName, Dir: sorting.SortAsc}
	m.playlists = m.playlists.recomputePlaylistSort()
	m.playlists.cursor = 0

	next, _ := m.handleKeyAction(actionPlaylistDelete)
	nm := next.(Model)
	if len(nm.playlists.playlists) != 2 {
		t.Fatalf("expected 2 playlists after delete, got %d", len(nm.playlists.playlists))
	}

	nm = nm.handleUndo()
	if len(nm.playlists.playlists) != 3 {
		t.Fatalf("expected 3 playlists after undo, got %d", len(nm.playlists.playlists))
	}
	if nm.playlists.playlists[1].ID != 2 {
		t.Fatalf("expected Alpha (ID=2) restored at canonical index 1, got ID=%d", nm.playlists.playlists[1].ID)
	}
}
