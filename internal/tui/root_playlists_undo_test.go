package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

// TestUndoDeletePlaylistScrollsToRestoredEntry verifies that undoing a playlist
// deletion adjusts scrollOffset so the restored entry is visible.
func TestUndoDeletePlaylistScrollsToRestoredEntry(t *testing.T) {
	pd := &stubPlaylistDeleter{}
	m := New(context.Background(), WithPlaylistDeleter(pd), WithPlaylists([]db.PlaylistRow{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}, {ID: 3, Name: "C"}, {ID: 4, Name: "D"}, {ID: 5, Name: "E"}}))
	m.activeView = viewPlaylists
	m.playlists.cursor = 0
	m.playlists.scrollOffset = 0
	next, _ := m.handleKeyAction(actionPlaylistDelete)
	nm := next.(Model)

	nm.playlists.scrollOffset = 3

	nm2 := nm.handleUndo()
	if nm2.playlists.cursor != 0 {
		t.Fatalf("expected cursor at 0 after undo, got %d", nm2.playlists.cursor)
	}
	if nm2.playlists.scrollOffset > nm2.playlists.cursor {
		t.Fatalf("scrollOffset %d should be <= cursor %d so restored entry is visible", nm2.playlists.scrollOffset, nm2.playlists.cursor)
	}
}

// TestUndoDeletePlaylistShowsNotification verifies that undoing a playlist
// deletion sets a confirmation notification in the status bar.
func TestUndoDeletePlaylistShowsNotification(t *testing.T) {
	pd := &stubPlaylistDeleter{}
	m := New(context.Background(), WithPlaylistDeleter(pd), WithPlaylists([]db.PlaylistRow{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}}))
	m.activeView = viewPlaylists
	m.playlists.cursor = 0

	next, _ := m.handleKeyAction(actionPlaylistDelete)
	nm := next.(Model)

	nm2 := nm.handleUndo()
	if nm2.statusBar.notification.msg == "" {
		t.Fatal("undo delete playlist must set a status bar notification")
	}
	if !strings.Contains(nm2.statusBar.notification.msg, "Undone") {
		t.Errorf("expected undo notification containing 'Undone', got %q", nm2.statusBar.notification.msg)
	}
}

// TestUndoRemovePlaylistTrackShowsNotification verifies that undoing a single
// playlist track removal sets a confirmation notification in the status bar.
func TestUndoRemovePlaylistTrackShowsNotification(t *testing.T) {
	pd := &stubPlaylistDeleter{}
	m := New(context.Background(), WithPlaylistDeleter(pd))
	m = drillIntoPlaylistTrackLevel(m, []db.PlaylistTrackRow{{ID: 10, Position: 1, Title: "Song A"}, {ID: 20, Position: 2, Title: "Song B"}})

	next, _ := m.handleKeyAction(actionPlaylistDelete)
	nm := next.(Model)

	nm2 := nm.handleUndo()
	if nm2.statusBar.notification.msg == "" {
		t.Fatal("undo remove playlist track must set a status bar notification")
	}
	if !strings.Contains(nm2.statusBar.notification.msg, "Undone") {
		t.Errorf("expected undo notification containing 'Undone', got %q", nm2.statusBar.notification.msg)
	}
}

// TestUndoBulkRemovePlaylistTracksShowsNotification verifies that undoing a
// bulk playlist track removal sets a confirmation notification in the status bar.
func TestUndoBulkRemovePlaylistTracksShowsNotification(t *testing.T) {
	pd := &stubPlaylistDeleter{}
	m := New(context.Background(), WithPlaylistDeleter(pd))
	m = drillIntoPlaylistTrackLevel(m, []db.PlaylistTrackRow{{ID: 10, Position: 1, Title: "Song A"}, {ID: 20, Position: 2, Title: "Song B"}, {ID: 30, Position: 3, Title: "Song C"}})
	m.playlists.selectedTrackIDs = map[int64]struct{}{10: {}, 20: {}}

	m.setUndoSlot(undoBulkRemovePlaylistTracksAction{
		entries: []db.PlaylistTrackRow{{ID: 10, Position: 1, Title: "Song A"}, {ID: 20, Position: 2, Title: "Song B"}},
		indices: []int{0, 1},
	}, nil)
	m.playlists.tracks = []db.PlaylistTrackRow{{ID: 30, Position: 3, Title: "Song C"}}

	nm := m.handleUndo()
	if nm.statusBar.notification.msg == "" {
		t.Fatal("undo bulk remove playlist tracks must set a status bar notification")
	}
	if !strings.Contains(nm.statusBar.notification.msg, "Undone") {
		t.Errorf("expected undo notification containing 'Undone', got %q", nm.statusBar.notification.msg)
	}
	if !strings.Contains(nm.statusBar.notification.msg, "2") {
		t.Errorf("expected undo notification to mention count, got %q", nm.statusBar.notification.msg)
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
	m.activeView = viewPlaylists
	m.playlists.height = 8

	m.playlists.cursor = 19
	m.playlists.scrollOffset = 14
	next, _ := m.handleKeyAction(actionPlaylistDelete)
	nm := next.(Model)

	nm.playlists.cursor = 0
	nm.playlists.scrollOffset = 0

	nm2 := nm.handleUndo()

	if nm2.playlists.cursor != 19 {
		t.Fatalf("expected cursor at 19 after undo, got %d", nm2.playlists.cursor)
	}

	reserve := 3
	maxRows := max(1, nm2.playlists.height-reserve)
	if nm2.playlists.cursor >= nm2.playlists.scrollOffset+maxRows {
		t.Errorf("cursor %d not visible: scrollOffset=%d, maxRows=%d", nm2.playlists.cursor, nm2.playlists.scrollOffset, maxRows)
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
	m.activeView = viewPlaylists
	m = drillIntoPlaylistTrackLevel(m, tracks)
	m.playlists.height = 8

	m.playlists.cursor = 19
	m.playlists.scrollOffset = 14
	next, _ := m.handleKeyAction(actionPlaylistDelete)
	nm := next.(Model)

	nm.playlists.cursor = 0
	nm.playlists.scrollOffset = 0

	nm2 := nm.handleUndo()

	if nm2.playlists.cursor != 19 {
		t.Fatalf("expected cursor at 19 after undo, got %d", nm2.playlists.cursor)
	}

	reserve := 3
	maxRows := max(1, nm2.playlists.height-reserve)
	if nm2.playlists.cursor >= nm2.playlists.scrollOffset+maxRows {
		t.Errorf("cursor %d not visible: scrollOffset=%d, maxRows=%d", nm2.playlists.cursor, nm2.playlists.scrollOffset, maxRows)
	}
}
