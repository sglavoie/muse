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

func TestBulkFavoriteTracksReturnsDeterministicOrder(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewFavorites
	m.favorites.Level = favtui.ListLevel
	m.favorites.Category = favtui.CategoryTracks
	m.favorites.Tracks = []library.Track{
		{Hash: "h0", Title: "T0", Artist: "A"},
		{Hash: "h1", Title: "T1", Artist: "A"},
		{Hash: "h2", Title: "T2", Artist: "A"},
		{Hash: "h3", Title: "T3", Artist: "A"},
		{Hash: "h4", Title: "T4", Artist: "A"},
	}
	m.favorites = m.favorites.ClearSelection().ToggleSelection(4).ToggleSelection(1).ToggleSelection(3)

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
	m.ActiveView = viewPlaylists
	m.playlists.Level = pltui.TrackLevel
	m.playlists.Tracks = []db.PlaylistTrackRow{
		{ID: 1, Title: "T1", Artist: "A", DurationMs: 1000},
		{ID: 2, Title: "T2", Artist: "A", DurationMs: 1000},
		{ID: 3, Title: "T3", Artist: "A", DurationMs: 1000},
	}
	m.playlists.Cursor = 0

	m = doVisual(m)
	m = doNav(m, keys.ActionCursorDown)
	if !m.visual.active {
		t.Fatal("expected visual mode active before delete")
	}

	m.playlists.SelectedTrackIDs = map[int64]struct{}{1: {}, 2: {}}
	m, _ = m.handlePlaylistDelete()

	if m.visual.active {
		t.Fatal("expected visual mode to exit after playlist bulk delete")
	}
	if len(m.playlists.Tracks) != 1 {
		t.Fatalf("expected 1 track remaining, got %d", len(m.playlists.Tracks))
	}
}

func TestFavoritesBulkActionExitsVisualMode(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewFavorites
	m.favorites.Level = favtui.ListLevel
	m.favorites.Category = favtui.CategoryTracks
	m.favorites.Tracks = []library.Track{
		{Hash: "h0", Title: "T0", Artist: "A"},
		{Hash: "h1", Title: "T1", Artist: "A"},
		{Hash: "h2", Title: "T2", Artist: "A"},
	}
	m.favorites.Cursor = 0

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
	m.ActiveView = viewFavorites
	m.favorites.Level = favtui.ListLevel
	m.favorites.Category = favtui.CategoryTracks
	m.favorites.Tracks = []library.Track{
		{Hash: "h0", Title: "T0", Artist: "A"},
		{Hash: "h1", Title: "T1", Artist: "A"},
	}
	m.visual = visualState{active: true, view: viewFavorites}

	m = selectRange(m, 0, 10)

	for key := range m.favorites.SelectedItems {
		if key != "h0" && key != "h1" {
			t.Fatalf("unexpected selected key %q", key)
		}
	}

	if len(m.favorites.SelectedItems) != len(m.favorites.Tracks) {
		t.Fatalf("expected exactly %d clamped selections, got %d", len(m.favorites.Tracks), len(m.favorites.SelectedItems))
	}
}
