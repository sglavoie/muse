package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/library"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
)

// TestFavoritesRemoveWithFilter_ArtistUsesRawIndex verifies that removing a
// single favorite artist while a filter is active stores the raw slice index
// (not the filtered cursor position) so that undo re-inserts at the correct
// position.
func TestFavoritesRemoveWithFilter_ArtistUsesRawIndex(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{
		"artist": {"Alpha", "Beta", "Charlie", "Delta"},
	}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryArtists
	m.favorites.artists = []string{"Alpha", "Beta", "Charlie", "Delta"}
	m.favorites.artistCount = 4
	m.favorites.filter.Text = "l"
	m.favorites.cursor = 2

	nm, cmd, handled := m.handleFavoritesRemove()
	if !handled {
		t.Fatal("handleFavoritesRemove should handle the action")
	}
	_ = nm
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd().(favoriteToggledMsg)
	if msg.removedIdx != 3 {
		t.Errorf("removedIdx: expected 3 (raw index of Delta), got %d", msg.removedIdx)
	}
	if msg.value != "Delta" {
		t.Errorf("value: expected 'Delta', got %q", msg.value)
	}
}

// TestFavoritesRemoveWithFilter_TrackUsesRawIndex verifies that removing a
// single favorite track while a filter is active stores the raw slice index.
func TestFavoritesRemoveWithFilter_TrackUsesRawIndex(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{
		"track": {"h-alpha", "h-beta", "h-charlie"},
	}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryTracks
	m.favorites.tracks = []library.Track{
		{Hash: "h-alpha", Title: "Alpha", Artist: "art1"},
		{Hash: "h-beta", Title: "Beta", Artist: "art2"},
		{Hash: "h-charlie", Title: "Charlie", Artist: "art3"},
	}
	m.favorites.trackCount = 3
	m.favorites.filter.Text = "charlie"
	m.favorites.cursor = 0

	nm, cmd, handled := m.handleFavoritesRemove()
	if !handled {
		t.Fatal("handleFavoritesRemove should handle the action")
	}
	_ = nm
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd().(favoriteToggledMsg)
	if msg.removedIdx != 2 {
		t.Errorf("removedIdx: expected 2 (raw index of h-charlie), got %d", msg.removedIdx)
	}
}

// TestFavoritesRemoveWithFilter_DrilldownTrackUsesRawIndex verifies that
// removing a favorite track at TrackLevel (drill-down) with an active filter
// uses the raw drilldownTracks index, not the filtered cursor position.
func TestFavoritesRemoveWithFilter_DrilldownTrackUsesRawIndex(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{
		"track": {"h1", "h2", "h3", "h4"},
	}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.activeView = viewFavorites
	m.favorites.level = favtui.TrackLevel
	m.favorites.drilldownTracks = []library.Track{
		{Hash: "h1", Title: "Andante", Artist: "X"},
		{Hash: "h2", Title: "Bolero", Artist: "Y"},
		{Hash: "h3", Title: "Allegro", Artist: "Z"},
		{Hash: "h4", Title: "Adagio", Artist: "W"},
	}
	m.favorites.filter.Text = "a"
	m.favorites.cursor = 2

	nm, cmd, handled := m.handleFavoritesRemove()
	if !handled {
		t.Fatal("handleFavoritesRemove should handle the action")
	}
	_ = nm
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd().(favoriteToggledMsg)
	if msg.removedIdx != 3 {
		t.Errorf("removedIdx: expected 3 (raw index of h4/Adagio), got %d", msg.removedIdx)
	}
}

// TestFavoritesRemoveWithFilter_AlbumUsesRawIndex verifies that removing a
// single favorite album while a filter is active stores the raw slice index.
func TestFavoritesRemoveWithFilter_AlbumUsesRawIndex(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{
		"album": {"Jazz", "Rock", "Blues"},
	}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryAlbums
	m.favorites.albums = []string{"Jazz", "Rock", "Blues"}
	m.favorites.albumCount = 3
	m.favorites.filter.Text = "lu"
	m.favorites.cursor = 0

	nm, cmd, handled := m.handleFavoritesRemove()
	if !handled {
		t.Fatal("handleFavoritesRemove should handle the action")
	}
	_ = nm
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd().(favoriteToggledMsg)
	if msg.removedIdx != 2 {
		t.Errorf("removedIdx: expected 2 (raw index of Blues), got %d", msg.removedIdx)
	}
}

// TestFavoritesFilterChangeClearsSelection verifies that changing the filter
// text in the Favorites view clears the selectedItems map so that stale
// display indices cannot refer to wrong items after the filtered list changes.
func TestFavoritesFilterChangeClearsSelection(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryArtists
	m.favorites.artists = []string{"Alpha", "Beta", "Gamma"}
	m.favorites.selectedItems = map[int]struct{}{0: {}, 2: {}}

	m, _ = m.handleFilterBarMsg()
	if !m.favorites.filter.Active {
		t.Fatal("filter should be active")
	}

	ops := m.activeFilterOps()
	if ops == nil {
		t.Fatal("expected active filter ops for favorites")
	}
	m, _ = ops.update(tea.KeyPressMsg{Text: "a"})
	if m.favorites.selectedItems != nil {
		t.Fatal("selectedItems should be nil after filter text change")
	}
}

// TestFavoritesFilterDeactivateClearsSelection verifies that deactivating the
// filter (Esc) clears the selectedItems map.
func TestFavoritesFilterDeactivateClearsSelection(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryTracks
	m.favorites.tracks = []library.Track{
		{Hash: "a", Title: "Alpha"},
		{Hash: "b", Title: "Beta"},
	}
	m.favorites.filter.Active = true
	m.favorites.filter.Text = "al"
	m.favorites.selectedItems = map[int]struct{}{0: {}}

	ops := m.activeFilterOps()
	if ops == nil {
		t.Fatal("expected active filter ops")
	}
	m = ops.deactivate()
	if m.favorites.selectedItems != nil {
		t.Fatal("selectedItems should be nil after filter deactivation")
	}
}

// TestFavoritesFilterBackspaceClearsSelection verifies that pressing backspace
// in the filter clears the selectedItems map when the text changes.
func TestFavoritesFilterBackspaceClearsSelection(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryAlbums
	m.favorites.albums = []string{"Foo", "Bar", "Baz"}
	m.favorites.filter.Active = true
	m.favorites.filter.Text = "ba"
	m.favorites.filter, _ = m.favorites.filter.Activate()
	m.favorites.selectedItems = map[int]struct{}{0: {}, 1: {}}

	ops := m.activeFilterOps()
	if ops == nil {
		t.Fatal("expected active filter ops")
	}
	m, _ = ops.update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if m.favorites.selectedItems != nil {
		t.Fatal("selectedItems should be nil after filter backspace")
	}
}

// TestFavoritesSortCycleClearsSelection verifies that cycling the sort field
// in the Favorites view clears the selectedItems map so that stale display
// indices cannot refer to wrong items after the list is re-sorted.
func TestFavoritesSortCycleClearsSelection(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryArtists
	m.favorites.artists = []string{"Charlie", "Alpha", "Bravo"}
	m.favorites.height = 20
	m.favorites.selectedItems = map[int]struct{}{0: {}, 2: {}}

	result, _, handled := m.handleSortCycle()
	if !handled {
		t.Fatal("handleSortCycle should handle favorites view")
	}
	rm := result.(Model)
	if rm.favorites.selectedItems != nil {
		t.Fatal("selectedItems should be nil after sort cycle")
	}
}

// TestFavoritesSortDirChangeClearsSelection verifies that changing the sort
// direction (asc/desc) in the Favorites view clears the selectedItems map.
func TestFavoritesSortDirChangeClearsSelection(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryAlbums
	m.favorites.albums = []string{"Zulu", "Mike", "Alpha"}
	m.favorites.height = 20
	m.favorites.sortState = SortState{Field: SortTitle, Dir: SortAsc}
	m.favorites.selectedItems = map[int]struct{}{1: {}}

	result, _, handled := m.handleSortDir(SortAsc)
	if !handled {
		t.Fatal("handleSortDir(SortAsc) should handle favorites view with active sort")
	}
	rm := result.(Model)
	if rm.favorites.selectedItems != nil {
		t.Fatal("selectedItems should be nil after sort asc")
	}

	rm.favorites.selectedItems = map[int]struct{}{0: {}}
	result2, _, handled2 := rm.handleSortDir(SortDesc)
	if !handled2 {
		t.Fatal("handleSortDir(SortDesc) should handle favorites view with active sort")
	}
	rm2 := result2.(Model)
	if rm2.favorites.selectedItems != nil {
		t.Fatal("selectedItems should be nil after sort desc")
	}
}
