package tui

import (
	"context"
	"testing"

	favtui "github.com/sglavoie/muse/internal/tui/favorites"
)

func newFavoritesCategoryModel(t *testing.T) Model {
	t.Helper()
	r := &stubFavoriteReader{favorites: map[string][]string{
		"artist": {"Bach", "Davis"},
		"album":  {"Blue", "Kind", "Time"},
	}}
	m := New(context.Background(), WithFavoriteReader(r))
	m.ActiveView = viewFavorites
	m.favorites.Level = favtui.CategoryLevel
	return m
}

// TestFavoritesDoubleEnterDropsSupersededLoad verifies that pressing Enter
// twice on a category applies only the latest load, so the second result
// cannot wipe cursor and filter changes made after the first one landed.
func TestFavoritesDoubleEnterDropsSupersededLoad(t *testing.T) {
	m := newFavoritesCategoryModel(t)
	m.favorites.Cursor = int(favtui.CategoryAlbums)

	m, first, _ := m.handleFavoritesOpen()
	m, second, _ := m.handleFavoritesOpen()

	// The superseded first load is dropped.
	next, _ := m.Update(first())
	m = next.(Model)
	if m.favorites.Level != favtui.CategoryLevel {
		t.Fatalf("superseded load should be dropped, got level %d", m.favorites.Level)
	}

	next, _ = m.Update(second())
	m = next.(Model)
	if m.favorites.Level != favtui.ListLevel {
		t.Fatalf("latest load should open the list, got level %d", m.favorites.Level)
	}
	m.favorites.Cursor = 2

	// A replay of either result after the list opened changes nothing.
	next, _ = m.Update(second())
	m = next.(Model)
	if m.favorites.Cursor != 2 {
		t.Errorf("late result reset the list cursor to %d", m.favorites.Cursor)
	}

	m.favorites = m.favorites.Back()
	if m.favorites.Cursor != int(favtui.CategoryAlbums) {
		t.Errorf("Back should return to Albums, got cursor %d", m.favorites.Cursor)
	}
}

// TestFavoritesLoadSavesRequestedCategory verifies that the category cursor
// saved for Back is the category requested, even when the cursor moved while
// the load was in flight.
func TestFavoritesLoadSavesRequestedCategory(t *testing.T) {
	m := newFavoritesCategoryModel(t)
	m.favorites.Cursor = int(favtui.CategoryArtists)

	m, cmd, _ := m.handleFavoritesOpen()
	m.favorites.Cursor = int(favtui.CategoryTracks) // cursor moved before arrival

	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.favorites.Category != favtui.CategoryArtists {
		t.Fatalf("expected Artists list, got category %d", m.favorites.Category)
	}

	m.favorites = m.favorites.Back()
	if m.favorites.Cursor != int(favtui.CategoryArtists) {
		t.Errorf("Back should return to Artists, got cursor %d", m.favorites.Cursor)
	}
}
