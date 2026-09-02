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
	m.favorites.Level = favtui.ListLevel
	m.favorites.Category = favtui.CategoryArtists
	m.favorites.Artists = []string{"Alice", "Bob", "Charlie", "David"}
	m.favorites.Filter.Text = "a"
	m.favorites.Cursor = 3

	m.favorites = m.favorites.ClampCursor()

	if m.favorites.Cursor != 2 {
		t.Errorf("expected cursor=2 (last filtered index), got %d", m.favorites.Cursor)
	}
}

// TestFavoritesClampCursor_ScrollOffsetClamped verifies that clampCursor brings
// scrollOffset down to cursor when scrollOffset > cursor after clamping.
func TestFavoritesClampCursor_ScrollOffsetClamped(t *testing.T) {
	m := New(context.Background())
	m.favorites.Level = favtui.ListLevel
	m.favorites.Category = favtui.CategoryAlbums
	m.favorites.Albums = []string{"Alpha"}
	m.favorites.Cursor = 5
	m.favorites.ScrollOffset = 10

	m.favorites = m.favorites.ClampCursor()

	if m.favorites.Cursor != 0 {
		t.Errorf("expected cursor=0, got %d", m.favorites.Cursor)
	}
	if m.favorites.ScrollOffset != 0 {
		t.Errorf("expected scrollOffset=0, got %d", m.favorites.ScrollOffset)
	}
}

// TestFavoritesClampCursor_TrackLevelWithFilter verifies clampCursor at
// TrackLevel with an active filter.
func TestFavoritesClampCursor_TrackLevelWithFilter(t *testing.T) {
	m := New(context.Background())
	m.favorites.Level = favtui.TrackLevel
	m.favorites.DrilldownTracks = []library.Track{
		{Hash: "h1", Title: "Alpha", Artist: "A"},
		{Hash: "h2", Title: "Beta", Artist: "B"},
		{Hash: "h3", Title: "Gamma", Artist: "G"},
	}
	m.favorites.Filter.Text = "beta"
	m.favorites.Cursor = 2
	m.favorites.ScrollOffset = 2

	m.favorites = m.favorites.ClampCursor()

	if m.favorites.Cursor != 0 {
		t.Errorf("expected cursor=0 (only 1 filtered item), got %d", m.favorites.Cursor)
	}
	if m.favorites.ScrollOffset != 0 {
		t.Errorf("expected scrollOffset=0, got %d", m.favorites.ScrollOffset)
	}
}

// TestFavoritesClampCursor_EmptyFilteredList verifies clampCursor resets both
// cursor and scrollOffset when the filter eliminates all items.
func TestFavoritesClampCursor_EmptyFilteredList(t *testing.T) {
	m := New(context.Background())
	m.favorites.Level = favtui.ListLevel
	m.favorites.Category = favtui.CategoryTracks
	m.favorites.Tracks = []library.Track{
		{Hash: "h1", Title: "Song A"},
	}
	m.favorites.Filter.Text = "zzz"
	m.favorites.Cursor = 1
	m.favorites.ScrollOffset = 1

	m.favorites = m.favorites.ClampCursor()

	if m.favorites.Cursor != 0 {
		t.Errorf("expected cursor=0, got %d", m.favorites.Cursor)
	}
	if m.favorites.ScrollOffset != 0 {
		t.Errorf("expected scrollOffset=0, got %d", m.favorites.ScrollOffset)
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

	v := favtui.FavoritesView{
		Level:    favtui.ListLevel,
		Category: favtui.CategoryArtists,
		Cursor:   0,
		Artists:  artists,
		Height:   12,
	}
	v = v.HandleAction(keys.ActionGoToBottom)
	if v.Cursor != 19 {
		t.Fatalf("GoToBottom without sort: expected cursor=19, got %d", v.Cursor)
	}
	scrollNoSort := v.ScrollOffset

	v2 := favtui.FavoritesView{
		Level:     favtui.ListLevel,
		Category:  favtui.CategoryArtists,
		Cursor:    0,
		Artists:   artists,
		Height:    12,
		SortState: sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc},
	}
	v2 = v2.HandleAction(keys.ActionGoToBottom)
	if v2.Cursor != 19 {
		t.Fatalf("GoToBottom with sort: expected cursor=19, got %d", v2.Cursor)
	}
	if v2.ScrollOffset <= scrollNoSort {
		t.Errorf("GoToBottom with sort: expected scrollOffset > %d (no-sort), got %d",
			scrollNoSort, v2.ScrollOffset)
	}
}
