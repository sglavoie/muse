package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/keys"
)

// TestUndoDeletePlaylistScrollsToRestoredEntry verifies that undoing a playlist
// deletion adjusts scrollOffset so the restored entry is visible.
func TestUndoDeletePlaylistScrollsToRestoredEntry(t *testing.T) {
	pd := &stubPlaylistDeleter{}
	m := New(context.Background(), WithPlaylistDeleter(pd), WithPlaylists([]db.PlaylistRow{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}, {ID: 3, Name: "C"}, {ID: 4, Name: "D"}, {ID: 5, Name: "E"}}))
	m.ActiveView = viewPlaylists
	m.playlists.Cursor = 0
	m.playlists.ScrollOffset = 0
	next, _ := m.handleKeyAction(keys.ActionPlaylistDelete)
	nm := next.(Model)

	nm.playlists.ScrollOffset = 3

	nm2 := nm.handleUndo()
	if nm2.playlists.Cursor != 0 {
		t.Fatalf("expected cursor at 0 after undo, got %d", nm2.playlists.Cursor)
	}
	if nm2.playlists.ScrollOffset > nm2.playlists.Cursor {
		t.Fatalf("scrollOffset %d should be <= cursor %d so restored entry is visible", nm2.playlists.ScrollOffset, nm2.playlists.Cursor)
	}
}

// TestUndoDeletePlaylistShowsNotification verifies that undoing a playlist
// deletion sets a confirmation notification in the status bar.
func TestUndoDeletePlaylistShowsNotification(t *testing.T) {
	pd := &stubPlaylistDeleter{}
	m := New(context.Background(), WithPlaylistDeleter(pd), WithPlaylists([]db.PlaylistRow{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}}))
	m.ActiveView = viewPlaylists
	m.playlists.Cursor = 0

	next, _ := m.handleKeyAction(keys.ActionPlaylistDelete)
	nm := next.(Model)

	nm2 := nm.handleUndo()
	if nm2.statusBar.TopNotification().Msg == "" {
		t.Fatal("undo delete playlist must set a status bar notification")
	}
	if !strings.Contains(nm2.statusBar.TopNotification().Msg, "Undone") {
		t.Errorf("expected undo notification containing 'Undone', got %q", nm2.statusBar.TopNotification().Msg)
	}
}

// TestUndoRemovePlaylistTrackShowsNotification verifies that undoing a single
// playlist track removal sets a confirmation notification in the status bar.
func TestUndoRemovePlaylistTrackShowsNotification(t *testing.T) {
	pd := &stubPlaylistDeleter{}
	m := New(context.Background(), WithPlaylistDeleter(pd))
	m = drillIntoPlaylistTrackLevel(m, []db.PlaylistTrackRow{{ID: 10, Position: 1, Title: "Song A"}, {ID: 20, Position: 2, Title: "Song B"}})

	next, _ := m.handleKeyAction(keys.ActionPlaylistDelete)
	nm := next.(Model)

	nm2 := nm.handleUndo()
	if nm2.statusBar.TopNotification().Msg == "" {
		t.Fatal("undo remove playlist track must set a status bar notification")
	}
	if !strings.Contains(nm2.statusBar.TopNotification().Msg, "Undone") {
		t.Errorf("expected undo notification containing 'Undone', got %q", nm2.statusBar.TopNotification().Msg)
	}
}

// TestUndoBulkRemovePlaylistTracksShowsNotification verifies that undoing a
// bulk playlist track removal sets a confirmation notification in the status bar.
func TestUndoBulkRemovePlaylistTracksShowsNotification(t *testing.T) {
	pd := &stubPlaylistDeleter{}
	m := New(context.Background(), WithPlaylistDeleter(pd))
	m = drillIntoPlaylistTrackLevel(m, []db.PlaylistTrackRow{{ID: 10, Position: 1, Title: "Song A"}, {ID: 20, Position: 2, Title: "Song B"}, {ID: 30, Position: 3, Title: "Song C"}})
	m.playlists.SelectedTrackIDs = map[int64]struct{}{10: {}, 20: {}}

	m.setUndoSlot(undoBulkRemovePlaylistTracksAction{
		entries: []db.PlaylistTrackRow{{ID: 10, Position: 1, Title: "Song A"}, {ID: 20, Position: 2, Title: "Song B"}},
		indices: []int{0, 1},
	}, nil)
	m.playlists.Tracks = []db.PlaylistTrackRow{{ID: 30, Position: 3, Title: "Song C"}}

	nm := m.handleUndo()
	if nm.statusBar.TopNotification().Msg == "" {
		t.Fatal("undo bulk remove playlist tracks must set a status bar notification")
	}
	if !strings.Contains(nm.statusBar.TopNotification().Msg, "Undone") {
		t.Errorf("expected undo notification containing 'Undone', got %q", nm.statusBar.TopNotification().Msg)
	}
	if !strings.Contains(nm.statusBar.TopNotification().Msg, "2") {
		t.Errorf("expected undo notification to mention count, got %q", nm.statusBar.TopNotification().Msg)
	}
}

// TestUndoDeletePlaylistCursorVisibleBelowViewport verifies that undoing a
// playlist deletion adjusts scrollOffset when the restored cursor position
// falls below the visible viewport.
func TestUndoDeletePlaylistCursorVisibleBelowViewport(t *testing.T) {
	rows := make([]db.PlaylistRow, 20)
	for i := range rows {
		rows[i] = db.PlaylistRow{ID: int64(i + 1), Name: fmt.Sprintf("Playlist %02d", i+1)}
	}

	pd := &stubPlaylistDeleter{}
	m := New(context.Background(), WithPlaylistDeleter(pd), WithPlaylists(rows))
	m.ActiveView = viewPlaylists
	m.playlists.Height = 8

	m.playlists.Cursor = 19
	m.playlists.ScrollOffset = 14
	next, _ := m.handleKeyAction(keys.ActionPlaylistDelete)
	nm := next.(Model)

	nm.playlists.Cursor = 0
	nm.playlists.ScrollOffset = 0

	nm2 := nm.handleUndo()

	if nm2.playlists.Cursor != 19 {
		t.Fatalf("expected cursor at 19 after undo, got %d", nm2.playlists.Cursor)
	}

	reserve := 3
	maxRows := max(1, nm2.playlists.Height-reserve)
	if nm2.playlists.Cursor >= nm2.playlists.ScrollOffset+maxRows {
		t.Errorf("cursor %d not visible: scrollOffset=%d, maxRows=%d", nm2.playlists.Cursor, nm2.playlists.ScrollOffset, maxRows)
	}
}

// TestUndoRemovePlaylistTrackCursorVisibleBelowViewport verifies that undoing
// a playlist track removal adjusts scrollOffset when the restored cursor
// position falls below the visible viewport.
func TestUndoRemovePlaylistTrackCursorVisibleBelowViewport(t *testing.T) {
	tracks := make([]db.PlaylistTrackRow, 20)
	for i := range tracks {
		tracks[i] = db.PlaylistTrackRow{ID: int64(i + 1), Position: int64(i + 1), Title: fmt.Sprintf("Track %02d", i+1), Hash: fmt.Sprintf("hash%d", i+1)}
	}

	pd := &stubPlaylistDeleter{}
	m := New(context.Background(), WithPlaylistDeleter(pd), WithPlaylists([]db.PlaylistRow{{ID: 1, Name: "Test"}}))
	m.ActiveView = viewPlaylists
	m = drillIntoPlaylistTrackLevel(m, tracks)
	m.playlists.Height = 8

	m.playlists.Cursor = 19
	m.playlists.ScrollOffset = 14
	next, _ := m.handleKeyAction(keys.ActionPlaylistDelete)
	nm := next.(Model)

	nm.playlists.Cursor = 0
	nm.playlists.ScrollOffset = 0

	nm2 := nm.handleUndo()

	if nm2.playlists.Cursor != 19 {
		t.Fatalf("expected cursor at 19 after undo, got %d", nm2.playlists.Cursor)
	}

	reserve := 3
	maxRows := max(1, nm2.playlists.Height-reserve)
	if nm2.playlists.Cursor >= nm2.playlists.ScrollOffset+maxRows {
		t.Errorf("cursor %d not visible: scrollOffset=%d, maxRows=%d", nm2.playlists.Cursor, nm2.playlists.ScrollOffset, maxRows)
	}
}
