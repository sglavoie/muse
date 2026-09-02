package tui

import (
	"context"
	"fmt"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

// TestFavoritesClampCursor_WithActiveFilter verifies that clampCursor accounts
// for the active filter rather than the raw backing-store length. Without the
// fix, a cursor at index 2 in a filtered list of 2 items goes out of bounds.
func TestFavoritesClampCursor_WithActiveFilter(t *testing.T) {
	m := New(context.Background())
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryArtists
	m.favorites.artists = []string{"Alice", "Bob", "Charlie", "David"}
	m.favorites.filter.Text = "a"
	m.favorites.cursor = 3

	m.favorites = m.favorites.clampCursor()

	if m.favorites.cursor != 2 {
		t.Errorf("expected cursor=2 (last filtered index), got %d", m.favorites.cursor)
	}
}

// TestFavoritesClampCursor_ScrollOffsetClamped verifies that clampCursor brings
// scrollOffset down to cursor when scrollOffset > cursor after clamping.
func TestFavoritesClampCursor_ScrollOffsetClamped(t *testing.T) {
	m := New(context.Background())
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryAlbums
	m.favorites.albums = []string{"Alpha"}
	m.favorites.cursor = 5
	m.favorites.scrollOffset = 10

	m.favorites = m.favorites.clampCursor()

	if m.favorites.cursor != 0 {
		t.Errorf("expected cursor=0, got %d", m.favorites.cursor)
	}
	if m.favorites.scrollOffset != 0 {
		t.Errorf("expected scrollOffset=0, got %d", m.favorites.scrollOffset)
	}
}

// TestFavoritesClampCursor_TrackLevelWithFilter verifies clampCursor at
// TrackLevel with an active filter.
func TestFavoritesClampCursor_TrackLevelWithFilter(t *testing.T) {
	m := New(context.Background())
	m.favorites.level = favtui.TrackLevel
	m.favorites.drilldownTracks = []library.Track{
		{Hash: "h1", Title: "Alpha", Artist: "A"},
		{Hash: "h2", Title: "Beta", Artist: "B"},
		{Hash: "h3", Title: "Gamma", Artist: "G"},
	}
	m.favorites.filter.Text = "beta"
	m.favorites.cursor = 2
	m.favorites.scrollOffset = 2

	m.favorites = m.favorites.clampCursor()

	if m.favorites.cursor != 0 {
		t.Errorf("expected cursor=0 (only 1 filtered item), got %d", m.favorites.cursor)
	}
	if m.favorites.scrollOffset != 0 {
		t.Errorf("expected scrollOffset=0, got %d", m.favorites.scrollOffset)
	}
}

// TestFavoritesClampCursor_EmptyFilteredList verifies clampCursor resets both
// cursor and scrollOffset when the filter eliminates all items.
func TestFavoritesClampCursor_EmptyFilteredList(t *testing.T) {
	m := New(context.Background())
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryTracks
	m.favorites.tracks = []library.Track{
		{Hash: "h1", Title: "Song A"},
	}
	m.favorites.filter.Text = "zzz"
	m.favorites.cursor = 1
	m.favorites.scrollOffset = 1

	m.favorites = m.favorites.clampCursor()

	if m.favorites.cursor != 0 {
		t.Errorf("expected cursor=0, got %d", m.favorites.cursor)
	}
	if m.favorites.scrollOffset != 0 {
		t.Errorf("expected scrollOffset=0, got %d", m.favorites.scrollOffset)
	}
}

// TestFavoritesGoToBottomWithSortAccountsForReserve verifies that GoToBottom
// calculates the correct scroll offset when a sort is active. The sort
// indicator takes one extra row, so visibleRows is one fewer.
func TestFavoritesGoToBottomWithSortAccountsForReserve(t *testing.T) {
	artists := make([]string, 20)
	for i := range artists {
		artists[i] = fmt.Sprintf("Artist_%02d", i)
	}

	v := FavoritesView{
		level:    favtui.ListLevel,
		category: favtui.CategoryArtists,
		cursor:   0,
		artists:  artists,
		height:   12,
	}
	v = v.HandleAction(keys.ActionGoToBottom)
	if v.cursor != 19 {
		t.Fatalf("GoToBottom without sort: expected cursor=19, got %d", v.cursor)
	}
	scrollNoSort := v.scrollOffset

	v2 := FavoritesView{
		level:     favtui.ListLevel,
		category:  favtui.CategoryArtists,
		cursor:    0,
		artists:   artists,
		height:    12,
		sortState: sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc},
	}
	v2 = v2.HandleAction(keys.ActionGoToBottom)
	if v2.cursor != 19 {
		t.Fatalf("GoToBottom with sort: expected cursor=19, got %d", v2.cursor)
	}
	if v2.scrollOffset <= scrollNoSort {
		t.Errorf("GoToBottom with sort: expected scrollOffset > %d (no-sort), got %d",
			scrollNoSort, v2.scrollOffset)
	}
}
