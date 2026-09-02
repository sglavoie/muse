package playlists

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/layout"
)

// TestPlaylistsViewEnsureCursorVisibleAdjustsScroll verifies that
// EnsureCursorVisible moves ScrollOffset when the cursor is beyond the
// visible window.
func TestPlaylistsViewEnsureCursorVisibleAdjustsScroll(t *testing.T) {
	t.Run("cursor below visible window", func(t *testing.T) {
		v := PlaylistsView{Cursor: 9, ScrollOffset: 0, Height: 6}
		v = v.EnsureCursorVisible()
		if v.ScrollOffset != 7 {
			t.Errorf("expected scrollOffset=7, got %d", v.ScrollOffset)
		}
	})

	t.Run("cursor above visible window", func(t *testing.T) {
		v := PlaylistsView{Cursor: 1, ScrollOffset: 5, Height: 6}
		v = v.EnsureCursorVisible()
		if v.ScrollOffset != 1 {
			t.Errorf("expected scrollOffset=1, got %d", v.ScrollOffset)
		}
	})

	t.Run("cursor already visible", func(t *testing.T) {
		v := PlaylistsView{Cursor: 2, ScrollOffset: 1, Height: 6}
		v = v.EnsureCursorVisible()
		if v.ScrollOffset != 1 {
			t.Errorf("expected scrollOffset=1 (unchanged), got %d", v.ScrollOffset)
		}
	})
}

// TestPlaylistsViewBackToListRestoresScrollOffset verifies that navigating
// from TrackLevel back to ListLevel restores the scroll offset the user had
// at list level.
func TestPlaylistsViewBackToListRestoresScrollOffset(t *testing.T) {
	v := PlaylistsView{
		Level:        ListLevel,
		Cursor:       15,
		ScrollOffset: 10,
		Height:       10,
		Playlists:    make([]db.PlaylistRow, 20),
	}

	v = v.OpenTracks([]db.PlaylistTrackRow{{Title: "T1"}, {Title: "T2"}})
	if v.ScrollOffset != 0 {
		t.Fatalf("expected track-level scrollOffset=0, got %d", v.ScrollOffset)
	}

	v = v.BackToList()
	if v.ScrollOffset != 10 {
		t.Errorf("expected restored scrollOffset=10 after backToList, got %d", v.ScrollOffset)
	}
	if v.Cursor != 15 {
		t.Errorf("expected restored cursor=15 after backToList, got %d", v.Cursor)
	}
}

func TestPlaylistsViewBackToListClampsStaleParentCursor(t *testing.T) {
	v := PlaylistsView{
		Level:            TrackLevel,
		ListCursor:       5,
		listScrollOffset: 4,
		Height:           6,
		Playlists:        []db.PlaylistRow{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}},
		Tracks:           []db.PlaylistTrackRow{{ID: 10, Title: "T1"}},
	}

	v = v.BackToList()

	if v.Cursor != 1 {
		t.Fatalf("expected cursor clamped to last playlist, got %d", v.Cursor)
	}
	if v.ScrollOffset > v.Cursor {
		t.Fatalf("expected scrollOffset clamped to cursor or above-visible range, got %d", v.ScrollOffset)
	}
}

// TestPlaylistsViewOpenTracksClearsStaleSelection verifies that opening a new
// playlist clears track-level bulk selections from a previously viewed playlist.
func TestPlaylistsViewOpenTracksClearsStaleSelection(t *testing.T) {
	v := PlaylistsView{
		Level:     TrackLevel,
		Playlists: make([]db.PlaylistRow, 5),
		Tracks:    []db.PlaylistTrackRow{{ID: 100, Title: "Old"}},
		SelectedTrackIDs: map[int64]struct{}{
			100: {},
		},
	}

	v = v.BackToList()
	v = v.OpenTracks([]db.PlaylistTrackRow{{ID: 200, Title: "New"}})

	if v.SelectedTrackIDs != nil {
		t.Errorf("expected selectedTrackIDs to be nil after openTracks, got %v", v.SelectedTrackIDs)
	}
}

// TestPlaylistsViewBackToListClearsStaleSelection verifies that navigating
// back to ListLevel clears track-level selections.
func TestPlaylistsViewBackToListClearsStaleSelection(t *testing.T) {
	v := PlaylistsView{
		Level:     TrackLevel,
		Playlists: make([]db.PlaylistRow, 5),
		Tracks:    []db.PlaylistTrackRow{{ID: 42, Title: "T1"}},
		SelectedTrackIDs: map[int64]struct{}{
			42: {},
		},
	}

	if !v.HasTrackSelection() {
		t.Fatal("expected hasTrackSelection() == true before backToList")
	}

	v = v.BackToList()

	if v.HasTrackSelection() {
		t.Errorf("expected hasTrackSelection() == false after backToList, got true (selectedTrackIDs=%v)", v.SelectedTrackIDs)
	}
}

// TestFocusedPlaylistReturnsFalseWhenCursorOutOfBounds verifies that
// FocusedPlaylist returns false when the cursor exceeds the playlist slice
// length.
func TestFocusedPlaylistReturnsFalseWhenCursorOutOfBounds(t *testing.T) {
	v := PlaylistsView{Level: ListLevel, Playlists: []db.PlaylistRow{{ID: 1, Name: "A"}}, Cursor: 5}
	_, ok := v.FocusedPlaylist()
	if ok {
		t.Fatal("focusedPlaylist must return false when cursor exceeds slice length")
	}
}

// TestFocusedPlaylistReturnsFalseWhenCursorNegative verifies that
// FocusedPlaylist returns false when the cursor is negative.
func TestFocusedPlaylistReturnsFalseWhenCursorNegative(t *testing.T) {
	v := PlaylistsView{Level: ListLevel, Playlists: []db.PlaylistRow{{ID: 1, Name: "A"}}, Cursor: -1}
	_, ok := v.FocusedPlaylist()
	if ok {
		t.Fatal("focusedPlaylist must return false when cursor is negative")
	}
}

func TestPlaylistsHandleActionCursorUpDown(t *testing.T) {
	v := PlaylistsView{Level: ListLevel, Cursor: 0, Playlists: make([]db.PlaylistRow, 5)}
	v = v.HandleAction(keys.ActionCursorDown)
	if v.Cursor != 1 {
		t.Errorf("CursorDown: expected 1, got %d", v.Cursor)
	}
	v = v.HandleAction(keys.ActionCursorUp)
	if v.Cursor != 0 {
		t.Errorf("CursorUp: expected 0, got %d", v.Cursor)
	}
}

func TestPlaylistsHandleActionGoToTopBottom(t *testing.T) {
	v := PlaylistsView{Level: ListLevel, Cursor: 0, Playlists: make([]db.PlaylistRow, 10)}
	v = v.HandleAction(keys.ActionGoToBottom)
	if v.Cursor != 9 {
		t.Errorf("GoToBottom: expected 9, got %d", v.Cursor)
	}
	v = v.HandleAction(keys.ActionGoToTop)
	if v.Cursor != 0 {
		t.Errorf("GoToTop: expected 0, got %d", v.Cursor)
	}
}

func TestPlaylistsHandleActionHalfPage(t *testing.T) {
	v := PlaylistsView{Level: ListLevel, Cursor: 0, Height: 13, Playlists: make([]db.PlaylistRow, 20)}
	v = v.HandleAction(keys.ActionHalfPageDown)
	if v.Cursor != 6 {
		t.Errorf("HalfPageDown: expected 6, got %d", v.Cursor)
	}
}

func TestPlaylistsHandleActionFullPage(t *testing.T) {
	v := PlaylistsView{Level: ListLevel, Cursor: 0, Height: 13, Playlists: make([]db.PlaylistRow, 20)}
	v = v.HandleAction(keys.ActionFullPageDown)
	if v.Cursor != 12 {
		t.Errorf("FullPageDown: expected 12, got %d", v.Cursor)
	}
}

// TestOpenTracksSavesCanonicalCursorWhenSorted verifies that OpenTracks saves
// the canonical playlist index as ListCursor, not the display cursor position.
func TestOpenTracksSavesCanonicalCursorWhenSorted(t *testing.T) {
	v := PlaylistsView{
		Playlists: []db.PlaylistRow{
			{ID: 10, Name: "Chill"},
			{ID: 20, Name: "Ambient"},
			{ID: 30, Name: "Beats"},
		},
		SortedPlaylists: []db.PlaylistRow{
			{ID: 20, Name: "Ambient"},
			{ID: 30, Name: "Beats"},
			{ID: 10, Name: "Chill"},
		},
		Cursor: 2,
		Level:  ListLevel,
	}

	tracks := []db.PlaylistTrackRow{{ID: 100, Hash: "h1", Title: "T1", Artist: "A1"}}
	v = v.OpenTracks(tracks)

	if v.ListCursor != 0 {
		t.Errorf("expected listCursor=0 (canonical index of Chill), got %d", v.ListCursor)
	}

	if v.ListCursor >= 0 && v.ListCursor < len(v.Playlists) {
		if v.Playlists[v.ListCursor].Name != "Chill" {
			t.Errorf("expected playlist name 'Chill' at listCursor, got %q", v.Playlists[v.ListCursor].Name)
		}
	} else {
		t.Fatalf("listCursor %d out of bounds (len=%d)", v.ListCursor, len(v.Playlists))
	}
}

// TestClampAfterRemovalScrollOffsetAboveCursor verifies that clampAfterRemoval
// pulls scrollOffset down to cursor when the removal leaves scrollOffset above
// the cursor row.
func TestClampAfterRemovalScrollOffsetAboveCursor(t *testing.T) {
	cursor, scrollOffset := layout.ClampAfterRemoval(0, 5, 6)
	if cursor != 0 {
		t.Errorf("expected cursor=0, got %d", cursor)
	}
	if scrollOffset != 0 {
		t.Errorf("expected scrollOffset=0 (clamped to cursor), got %d", scrollOffset)
	}
}

// TestClampAfterRemovalScrollOffsetAtCursor verifies that when scrollOffset
// equals cursor, no adjustment is needed.
func TestClampAfterRemovalScrollOffsetAtCursor(t *testing.T) {
	cursor, scrollOffset := layout.ClampAfterRemoval(3, 3, 10)
	if cursor != 3 {
		t.Errorf("expected cursor=3, got %d", cursor)
	}
	if scrollOffset != 3 {
		t.Errorf("expected scrollOffset=3, got %d", scrollOffset)
	}
}

// TestClampAfterRemovalEmptyList verifies both cursor and scrollOffset reset
// to 0 for an empty list.
func TestClampAfterRemovalEmptyList(t *testing.T) {
	cursor, scrollOffset := layout.ClampAfterRemoval(5, 3, 0)
	if cursor != 0 {
		t.Errorf("expected cursor=0, got %d", cursor)
	}
	if scrollOffset != 0 {
		t.Errorf("expected scrollOffset=0, got %d", scrollOffset)
	}
}

// TestOpenTracksSavesDisplayCursorWhenUnsorted verifies that OpenTracks saves
// the display cursor directly when no sort is active.
func TestOpenTracksSavesDisplayCursorWhenUnsorted(t *testing.T) {
	v := PlaylistsView{
		Playlists: []db.PlaylistRow{
			{ID: 10, Name: "Chill"},
			{ID: 20, Name: "Ambient"},
			{ID: 30, Name: "Beats"},
		},
		Cursor: 1,
		Level:  ListLevel,
	}

	tracks := []db.PlaylistTrackRow{{ID: 100, Hash: "h1", Title: "T1", Artist: "A1"}}
	v = v.OpenTracks(tracks)

	if v.ListCursor != 1 {
		t.Errorf("expected listCursor=1, got %d", v.ListCursor)
	}
}
