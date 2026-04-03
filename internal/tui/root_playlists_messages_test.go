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
