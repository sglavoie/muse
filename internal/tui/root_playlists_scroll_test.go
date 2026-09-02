package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
)

// TestPlaylistRenamedMsgAdjustsScrollOffset verifies that after a playlist
// rename causes the entry to sort to a different position, scrollOffset is adjusted.
func TestPlaylistRenamedMsgAdjustsScrollOffset(t *testing.T) {
	var playlists []db.PlaylistRow
	for i := range 10 {
		playlists = append(playlists, db.PlaylistRow{ID: int64(i + 1), Name: string(rune('A' + i))})
	}
	m := New(context.Background(), WithPlaylists(playlists))
	m.ActiveView = viewPlaylists
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 6})
	m = next.(Model)
	m.playlists.Cursor = 0
	m.playlists.ScrollOffset = 0

	renamedPlaylists := make([]db.PlaylistRow, 0, 10)
	for i := 1; i < 10; i++ {
		renamedPlaylists = append(renamedPlaylists, db.PlaylistRow{ID: int64(i + 1), Name: string(rune('A' + i))})
	}
	renamedPlaylists = append(renamedPlaylists, db.PlaylistRow{ID: 1, Name: "Z"})

	next, _ = m.Update(playlistRenamedMsg{id: 1, playlists: renamedPlaylists})
	m = next.(Model)

	if m.playlists.Cursor != 9 {
		t.Fatalf("expected cursor=9 (renamed entry), got %d", m.playlists.Cursor)
	}
	if m.playlists.ScrollOffset+3 <= m.playlists.Cursor {
		t.Fatalf("cursor %d is not visible: scrollOffset=%d, visibleRows=3", m.playlists.Cursor, m.playlists.ScrollOffset)
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
	m.ActiveView = viewPlaylists
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 6})
	m = next.(Model)
	m.playlists.Cursor = 0
	m.playlists.ScrollOffset = 0

	createdPlaylists := make([]db.PlaylistRow, len(playlists))
	copy(createdPlaylists, playlists)
	createdPlaylists = append(createdPlaylists, db.PlaylistRow{ID: 6, Name: "Z"})

	next, _ = m.Update(playlistCreatedMsg{id: 6, playlists: createdPlaylists})
	m = next.(Model)

	if m.playlists.Cursor != 5 {
		t.Fatalf("expected cursor=5 (new entry), got %d", m.playlists.Cursor)
	}
	if m.playlists.ScrollOffset+3 <= m.playlists.Cursor {
		t.Fatalf("cursor %d is not visible: scrollOffset=%d, visibleRows=3", m.playlists.Cursor, m.playlists.ScrollOffset)
	}
}
