package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

// TestPlaylistsViewEnsureCursorVisibleAdjustsScroll verifies that
// ensureCursorVisible moves scrollOffset when the cursor is beyond the
// visible window.
func TestPlaylistsViewEnsureCursorVisibleAdjustsScroll(t *testing.T) {
	t.Run("cursor below visible window", func(t *testing.T) {
		v := PlaylistsView{cursor: 9, scrollOffset: 0, height: 6}
		v = v.ensureCursorVisible()
		if v.scrollOffset != 7 {
			t.Errorf("expected scrollOffset=7, got %d", v.scrollOffset)
		}
	})

	t.Run("cursor above visible window", func(t *testing.T) {
		v := PlaylistsView{cursor: 1, scrollOffset: 5, height: 6}
		v = v.ensureCursorVisible()
		if v.scrollOffset != 1 {
			t.Errorf("expected scrollOffset=1, got %d", v.scrollOffset)
		}
	})

	t.Run("cursor already visible", func(t *testing.T) {
		v := PlaylistsView{cursor: 2, scrollOffset: 1, height: 6}
		v = v.ensureCursorVisible()
		if v.scrollOffset != 1 {
			t.Errorf("expected scrollOffset=1 (unchanged), got %d", v.scrollOffset)
		}
	})
}

// TestPlaylistsViewBackToListRestoresScrollOffset verifies that navigating
// from TrackLevel back to ListLevel restores the scroll offset the user had
// at list level.
func TestPlaylistsViewBackToListRestoresScrollOffset(t *testing.T) {
	v := PlaylistsView{
		level:        pltui.ListLevel,
		cursor:       15,
		scrollOffset: 10,
		height:       10,
		playlists:    make([]db.PlaylistRow, 20),
	}

	v = v.openTracks([]db.PlaylistTrackRow{{Title: "T1"}, {Title: "T2"}})
	if v.scrollOffset != 0 {
		t.Fatalf("expected track-level scrollOffset=0, got %d", v.scrollOffset)
	}

	v = v.backToList()
	if v.scrollOffset != 10 {
		t.Errorf("expected restored scrollOffset=10 after backToList, got %d", v.scrollOffset)
	}
	if v.cursor != 15 {
		t.Errorf("expected restored cursor=15 after backToList, got %d", v.cursor)
	}
}

// TestPlaylistsViewOpenTracksClearsStaleSelection verifies that opening a new
// playlist clears track-level bulk selections from a previously viewed playlist.
func TestPlaylistsViewOpenTracksClearsStaleSelection(t *testing.T) {
	v := PlaylistsView{
		level:     pltui.TrackLevel,
		playlists: make([]db.PlaylistRow, 5),
		tracks:    []db.PlaylistTrackRow{{ID: 100, Title: "Old"}},
		selectedTrackIDs: map[int64]struct{}{
			100: {},
		},
	}

	v = v.backToList()
	v = v.openTracks([]db.PlaylistTrackRow{{ID: 200, Title: "New"}})

	if v.selectedTrackIDs != nil {
		t.Errorf("expected selectedTrackIDs to be nil after openTracks, got %v", v.selectedTrackIDs)
	}
}

// TestPlaylistsViewBackToListClearsStaleSelection verifies that navigating
// back to ListLevel clears track-level selections.
func TestPlaylistsViewBackToListClearsStaleSelection(t *testing.T) {
	v := PlaylistsView{
		level:     pltui.TrackLevel,
		playlists: make([]db.PlaylistRow, 5),
		tracks:    []db.PlaylistTrackRow{{ID: 42, Title: "T1"}},
		selectedTrackIDs: map[int64]struct{}{
			42: {},
		},
	}

	if !v.hasTrackSelection() {
		t.Fatal("expected hasTrackSelection() == true before backToList")
	}

	v = v.backToList()

	if v.hasTrackSelection() {
		t.Errorf("expected hasTrackSelection() == false after backToList, got true (selectedTrackIDs=%v)", v.selectedTrackIDs)
	}
}

// TestFocusedPlaylistReturnsFalseWhenCursorOutOfBounds verifies that
// focusedPlaylist returns false when the cursor exceeds the playlist slice
// length.
func TestFocusedPlaylistReturnsFalseWhenCursorOutOfBounds(t *testing.T) {
	v := PlaylistsView{level: pltui.ListLevel, playlists: []db.PlaylistRow{{ID: 1, Name: "A"}}, cursor: 5}
	_, ok := v.focusedPlaylist()
	if ok {
		t.Fatal("focusedPlaylist must return false when cursor exceeds slice length")
	}
}

// TestFocusedPlaylistReturnsFalseWhenCursorNegative verifies that
// focusedPlaylist returns false when the cursor is negative.
func TestFocusedPlaylistReturnsFalseWhenCursorNegative(t *testing.T) {
	v := PlaylistsView{level: pltui.ListLevel, playlists: []db.PlaylistRow{{ID: 1, Name: "A"}}, cursor: -1}
	_, ok := v.focusedPlaylist()
	if ok {
		t.Fatal("focusedPlaylist must return false when cursor is negative")
	}
}

func TestPlaylistsHandleActionCursorUpDown(t *testing.T) {
	v := PlaylistsView{level: pltui.ListLevel, cursor: 0, playlists: make([]db.PlaylistRow, 5)}
	v = v.HandleAction(actionCursorDown)
	if v.cursor != 1 {
		t.Errorf("CursorDown: expected 1, got %d", v.cursor)
	}
	v = v.HandleAction(actionCursorUp)
	if v.cursor != 0 {
		t.Errorf("CursorUp: expected 0, got %d", v.cursor)
	}
}

func TestPlaylistsHandleActionGoToTopBottom(t *testing.T) {
	v := PlaylistsView{level: pltui.ListLevel, cursor: 0, playlists: make([]db.PlaylistRow, 10)}
	v = v.HandleAction(actionGoToBottom)
	if v.cursor != 9 {
		t.Errorf("GoToBottom: expected 9, got %d", v.cursor)
	}
	v = v.HandleAction(actionGoToTop)
	if v.cursor != 0 {
		t.Errorf("GoToTop: expected 0, got %d", v.cursor)
	}
}

func TestPlaylistsHandleActionHalfPage(t *testing.T) {
	v := PlaylistsView{level: pltui.ListLevel, cursor: 0, height: 13, playlists: make([]db.PlaylistRow, 20)}
	v = v.HandleAction(actionHalfPageDown)
	if v.cursor != 6 {
		t.Errorf("HalfPageDown: expected 6, got %d", v.cursor)
	}
}

func TestPlaylistsHandleActionFullPage(t *testing.T) {
	v := PlaylistsView{level: pltui.ListLevel, cursor: 0, height: 13, playlists: make([]db.PlaylistRow, 20)}
	v = v.HandleAction(actionFullPageDown)
	if v.cursor != 12 {
		t.Errorf("FullPageDown: expected 12, got %d", v.cursor)
	}
}

// TestOpenTracksSavesCanonicalCursorWhenSorted verifies that openTracks saves
// the canonical playlist index as listCursor, not the display cursor position.
func TestOpenTracksSavesCanonicalCursorWhenSorted(t *testing.T) {
	v := PlaylistsView{
		playlists: []db.PlaylistRow{
			{ID: 10, Name: "Chill"},
			{ID: 20, Name: "Ambient"},
			{ID: 30, Name: "Beats"},
		},
		sortedPlaylists: []db.PlaylistRow{
			{ID: 20, Name: "Ambient"},
			{ID: 30, Name: "Beats"},
			{ID: 10, Name: "Chill"},
		},
		cursor: 2,
		level:  pltui.ListLevel,
	}

	tracks := []db.PlaylistTrackRow{{ID: 100, Hash: "h1", Title: "T1", Artist: "A1"}}
	v = v.openTracks(tracks)

	if v.listCursor != 0 {
		t.Errorf("expected listCursor=0 (canonical index of Chill), got %d", v.listCursor)
	}

	if v.listCursor >= 0 && v.listCursor < len(v.playlists) {
		if v.playlists[v.listCursor].Name != "Chill" {
			t.Errorf("expected playlist name 'Chill' at listCursor, got %q", v.playlists[v.listCursor].Name)
		}
	} else {
		t.Fatalf("listCursor %d out of bounds (len=%d)", v.listCursor, len(v.playlists))
	}
}

// TestClampAfterRemovalScrollOffsetAboveCursor verifies that clampAfterRemoval
// pulls scrollOffset down to cursor when the removal leaves scrollOffset above
// the cursor row.
func TestClampAfterRemovalScrollOffsetAboveCursor(t *testing.T) {
	cursor, scrollOffset := clampAfterRemoval(0, 5, 6)
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
	cursor, scrollOffset := clampAfterRemoval(3, 3, 10)
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
	cursor, scrollOffset := clampAfterRemoval(5, 3, 0)
	if cursor != 0 {
		t.Errorf("expected cursor=0, got %d", cursor)
	}
	if scrollOffset != 0 {
		t.Errorf("expected scrollOffset=0, got %d", scrollOffset)
	}
}

// TestOpenTracksSavesDisplayCursorWhenUnsorted verifies that openTracks saves
// the display cursor directly when no sort is active.
func TestOpenTracksSavesDisplayCursorWhenUnsorted(t *testing.T) {
	v := PlaylistsView{
		playlists: []db.PlaylistRow{
			{ID: 10, Name: "Chill"},
			{ID: 20, Name: "Ambient"},
			{ID: 30, Name: "Beats"},
		},
		cursor: 1,
		level:  pltui.ListLevel,
	}

	tracks := []db.PlaylistTrackRow{{ID: 100, Hash: "h1", Title: "T1", Artist: "A1"}}
	v = v.openTracks(tracks)

	if v.listCursor != 1 {
		t.Errorf("expected listCursor=1, got %d", v.listCursor)
	}
}
