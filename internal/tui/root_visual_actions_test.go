package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	"github.com/sglavoie/muse/internal/tui/keys"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

func TestBulkSelectionToggleExitsVisualMode(t *testing.T) {
	m := newLibraryModel(5)
	m.library.Cursor = 0

	m = doVisual(m)
	m = doNav(m, keys.ActionCursorDown)
	if !m.visual.active {
		t.Fatal("expected visual mode active before toggle")
	}

	m, _, _ = m.handleNavigationAction(keys.ActionBulkSelectionToggle)
	if m.visual.active {
		t.Fatal("expected visual mode to exit on bulk-selection toggle")
	}
	assertSelectedHashesExact(t, m.library.SelectedHashes, "h0")
}

func TestSortedIntSetKeysOrder(t *testing.T) {
	s := map[int]struct{}{5: {}, 1: {}, 3: {}, 0: {}, 9: {}}
	keys := sortedIntSetKeys(s)
	want := []int{0, 1, 3, 5, 9}
	if len(keys) != len(want) {
		t.Fatalf("len = %d, want %d", len(keys), len(want))
	}
	for i, k := range keys {
		if k != want[i] {
			t.Errorf("keys[%d] = %d, want %d", i, k, want[i])
		}
	}
}

func TestSortedIntSetKeysEmpty(t *testing.T) {
	keys := sortedIntSetKeys(nil)
	if len(keys) != 0 {
		t.Fatalf("expected empty slice, got %v", keys)
	}
}

func TestBulkFavoriteTracksReturnsDeterministicOrder(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryTracks
	m.favorites.tracks = []library.Track{
		{Hash: "h0", Title: "T0", Artist: "A"},
		{Hash: "h1", Title: "T1", Artist: "A"},
		{Hash: "h2", Title: "T2", Artist: "A"},
		{Hash: "h3", Title: "T3", Artist: "A"},
		{Hash: "h4", Title: "T4", Artist: "A"},
	}
	m.favorites.selectedItems = map[int]struct{}{4: {}, 1: {}, 3: {}}

	for i := 0; i < 50; i++ {
		tracks := m.bulkFavoriteTracks()
		if len(tracks) != 3 {
			t.Fatalf("iter %d: expected 3 tracks, got %d", i, len(tracks))
		}
		if tracks[0].Hash != "h1" || tracks[1].Hash != "h3" || tracks[2].Hash != "h4" {
			t.Fatalf("iter %d: wrong order: %s, %s, %s (want h1, h3, h4)", i, tracks[0].Hash, tracks[1].Hash, tracks[2].Hash)
		}
	}
}

func TestPlaylistBulkDeleteExitsVisualMode(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewPlaylists
	m.playlists.level = pltui.TrackLevel
	m.playlists.tracks = []db.PlaylistTrackRow{
		{ID: 1, Title: "T1", Artist: "A", DurationMs: 1000},
		{ID: 2, Title: "T2", Artist: "A", DurationMs: 1000},
		{ID: 3, Title: "T3", Artist: "A", DurationMs: 1000},
	}
	m.playlists.cursor = 0

	m = doVisual(m)
	m = doNav(m, keys.ActionCursorDown)
	if !m.visual.active {
		t.Fatal("expected visual mode active before delete")
	}

	m.playlists.selectedTrackIDs = map[int64]struct{}{1: {}, 2: {}}
	m, _ = m.handlePlaylistDelete()

	if m.visual.active {
		t.Fatal("expected visual mode to exit after playlist bulk delete")
	}
	if len(m.playlists.tracks) != 1 {
		t.Fatalf("expected 1 track remaining, got %d", len(m.playlists.tracks))
	}
}

func TestFavoritesBulkActionExitsVisualMode(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryTracks
	m.favorites.tracks = []library.Track{
		{Hash: "h0", Title: "T0", Artist: "A"},
		{Hash: "h1", Title: "T1", Artist: "A"},
		{Hash: "h2", Title: "T2", Artist: "A"},
	}
	m.favorites.cursor = 0

	m = doVisual(m)
	m = doNav(m, keys.ActionCursorDown)
	if !m.visual.active {
		t.Fatal("expected visual mode active before bulk action")
	}

	m.favoriteWriter = &stubFavoriteWriter{}
	m, _, _ = m.handleFavoritesBulkAction()

	if m.visual.active {
		t.Fatal("expected visual mode to exit after favorites bulk action")
	}
}

func TestVisualSelectRangeFavoritesBoundsClamp(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryTracks
	m.favorites.tracks = []library.Track{
		{Hash: "h0", Title: "T0", Artist: "A"},
		{Hash: "h1", Title: "T1", Artist: "A"},
	}
	m.visual = visualState{active: true, view: viewFavorites}

	m = selectRange(m, 0, 10)

	for idx := range m.favorites.selectedItems {
		if idx < 0 || idx >= len(m.favorites.tracks) {
			t.Fatalf("selectedItems contains out-of-range index %d (list len=%d)", idx, len(m.favorites.tracks))
		}
	}
	if len(m.favorites.selectedItems) != len(m.favorites.tracks) {
		t.Fatalf("expected exactly %d clamped selections, got %d", len(m.favorites.tracks), len(m.favorites.selectedItems))
	}
}
