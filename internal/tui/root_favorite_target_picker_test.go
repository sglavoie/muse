package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

func openFavoritePickerForTest(t *testing.T, m Model) Model {
	t.Helper()
	next, cmd, handled := m.handleToggleFavorite()
	if !handled {
		t.Fatal("favorite action was not handled")
	}
	if cmd != nil {
		t.Fatal("opening favorite picker unexpectedly returned a mutation command")
	}
	if next.overlay.favoriteTargetPicker == nil {
		t.Fatal("favorite target picker did not open")
	}
	return next
}

func confirmFavoritePickerForTest(t *testing.T, m Model, key string) (Model, favoriteToggledMsg) {
	t.Helper()
	updated, pickedCmd := m.overlay.favoriteTargetPicker.HandleKey(key)
	m.overlay.favoriteTargetPicker = &updated
	if pickedCmd == nil {
		t.Fatalf("favorite target key %q did not select", key)
	}
	picked := pickedCmd().(favtui.TargetPickedMsg)
	next, writeCmd := m.handleFavoriteTargetPicked(picked)
	if writeCmd == nil {
		t.Fatal("favorite target selection did not produce write command")
	}
	result := writeCmd().(favoriteToggledMsg)
	return next, result
}

func TestFavoritePickerBulkSelectionLifecycleAndNormalization(t *testing.T) {
	store := newUITestFavorites(map[string][]string{"track": {"h1"}})
	m := New(context.Background(), WithFavoriteReader(store), WithFavoriteWriter(store))
	m.activeView = viewLibrary
	m.library.SourceTracks = []library.Track{
		{Hash: "h1", Title: "One", Artist: "A", Album: "X"},
		{Hash: "h2", Title: "Two", Artist: "B", Album: "Y"},
	}
	m.library = m.library.RecomputeSortedTracks()
	m.library.SelectedHashes = map[string]struct{}{"h1": {}, "h2": {}}

	opened := openFavoritePickerForTest(t, m)
	if len(opened.library.SelectedHashes) != 2 {
		t.Fatal("opening picker cleared selection")
	}
	dismissed := opened
	dismissed.overlay.favoriteTargetPicker = nil
	if len(dismissed.library.SelectedHashes) != 2 {
		t.Fatal("canceling picker cleared selection")
	}

	confirmed, result := confirmFavoritePickerForTest(t, opened, "t")
	if confirmed.overlay.favoriteTargetPicker != nil || len(confirmed.library.SelectedHashes) != 0 {
		t.Fatal("confirming picker did not close overlay and clear selection")
	}
	if result.err != nil || !result.added || result.count != 1 || result.value != "h2" {
		t.Fatalf("normalized result = %+v", result)
	}
	confirmed = confirmed.handleFavoriteToggledMsg(result)
	if _, ok := confirmed.favoriteHashes["h1"]; !ok {
		t.Fatal("pre-existing favorite was removed during mixed normalization")
	}
	if _, ok := confirmed.favoriteHashes["h2"]; !ok {
		t.Fatal("missing favorite was not added")
	}
}

func TestFavoritePickerAllFavoriteBulkRemovesAndUndoRestores(t *testing.T) {
	store := newUITestFavorites(map[string][]string{"artist": {"A", "B"}})
	m := New(context.Background(), WithFavoriteReader(store), WithFavoriteWriter(store))
	m.activeView = viewLibrary
	m.library.SourceTracks = []library.Track{
		{Hash: "h1", Title: "One", Artist: "A", Album: "X"},
		{Hash: "h2", Title: "Two", Artist: "B", Album: "Y"},
	}
	m.library = m.library.RecomputeSortedTracks()
	m.library.SelectedHashes = map[string]struct{}{"h1": {}, "h2": {}}

	opened := openFavoritePickerForTest(t, m)
	confirmed, result := confirmFavoritePickerForTest(t, opened, "r")
	if result.err != nil || result.added || result.count != 2 {
		t.Fatalf("remove result = %+v", result)
	}
	confirmed = confirmed.handleFavoriteToggledMsg(result)
	if len(confirmed.favoriteArtists) != 0 {
		t.Fatalf("artist cache after removal = %v", confirmed.favoriteArtists)
	}
	undone := confirmed.handleUndo()
	if _, ok := undone.favoriteArtists["A"]; !ok {
		t.Fatal("undo did not restore artist A")
	}
	if _, ok := undone.favoriteArtists["B"]; !ok {
		t.Fatal("undo did not restore artist B")
	}
}

func TestFavoritePickerMixesTrackLevel(t *testing.T) {
	store := newUITestFavorites(nil)
	m := New(context.Background(), WithFavoriteReader(store), WithFavoriteWriter(store))
	m.activeView = viewMixes
	m.mixes.level = 1
	m.mixes.trackView = libtui.View{SourceTracks: []library.Track{{Hash: "mx", Title: "Mix", Artist: "A", Album: "B"}}}
	m.mixes.trackView = m.mixes.trackView.RefreshView()

	opened := openFavoritePickerForTest(t, m)
	if got := opened.overlay.favoriteTargetPicker.Tracks[0].Hash; got != "mx" {
		t.Fatalf("mix picker hash = %q", got)
	}
}

func TestFavoritePickerQueuePlaylistAndFavoritesTracks(t *testing.T) {
	newModel := func() Model {
		store := newUITestFavorites(nil)
		return New(context.Background(), WithFavoriteReader(store), WithFavoriteWriter(store))
	}

	t.Run("queue", func(t *testing.T) {
		m := newModel()
		m.activeView = viewQueue
		m.queue.entries = []QueueEntry{{Hash: "q1", Title: "Queued", Artist: "A", Album: "B"}}
		opened := openFavoritePickerForTest(t, m)
		if got := opened.overlay.favoriteTargetPicker.Tracks[0].Hash; got != "q1" {
			t.Fatalf("queue picker hash = %q", got)
		}
	})

	t.Run("playlist", func(t *testing.T) {
		m := newModel()
		m.activeView = viewPlaylists
		m.playlists.Level = pltui.TrackLevel
		m.playlists.Tracks = []db.PlaylistTrackRow{{ID: 1, Hash: "p1", Title: "Playlist", Artist: "A", Album: "B"}}
		opened := openFavoritePickerForTest(t, m)
		if got := opened.overlay.favoriteTargetPicker.Tracks[0].Hash; got != "p1" {
			t.Fatalf("playlist picker hash = %q", got)
		}
	})

	t.Run("favorites", func(t *testing.T) {
		m := newModel()
		m.activeView = viewFavorites
		m.favorites.Level = favtui.ListLevel
		m.favorites.Category = favtui.CategoryTracks
		m.favorites.Tracks = []library.Track{{Hash: "f1", Title: "Favorite", Artist: "A", Album: "B"}}
		opened := openFavoritePickerForTest(t, m)
		if got := opened.overlay.favoriteTargetPicker.Tracks[0].Hash; got != "f1" {
			t.Fatalf("favorites picker hash = %q", got)
		}
	})
}

func TestFavoriteEntityRowsKeepDirectToggle(t *testing.T) {
	store := newUITestFavorites(map[string][]string{"artist": {"A"}})
	m := New(context.Background(), WithFavoriteReader(store), WithFavoriteWriter(store))
	m.activeView = viewFavorites
	m.favorites.Level = favtui.ListLevel
	m.favorites.Category = favtui.CategoryArtists
	m.favorites.Artists = []string{"A"}

	next, cmd, handled := m.handleToggleFavorite()
	if !handled || cmd == nil {
		t.Fatal("favorite artist entity did not retain direct toggle")
	}
	if next.overlay.favoriteTargetPicker != nil {
		t.Fatal("favorite artist entity opened target picker")
	}
	result := cmd().(favoriteToggledMsg)
	if result.typ != "artist" || result.added || result.value != "A" {
		t.Fatalf("entity toggle result = %+v", result)
	}
}

func TestFavoritePickerRemovingDrilldownParentReturnsToList(t *testing.T) {
	store := newUITestFavorites(map[string][]string{"artist": {"A", "B"}})
	m := New(context.Background(), WithFavoriteReader(store), WithFavoriteWriter(store))
	m.activeView = viewFavorites
	m.favorites.Level = favtui.TrackLevel
	m.favorites.Category = favtui.CategoryArtists
	m.favorites.Artists = []string{"A", "B"}
	m.favorites.DrilldownName = "A"
	m.favorites.DrilldownTracks = []library.Track{{Hash: "h1", Title: "One", Artist: "A", Album: "X"}}

	opened := openFavoritePickerForTest(t, m)
	confirmed, result := confirmFavoritePickerForTest(t, opened, "r")
	confirmed = confirmed.handleFavoriteToggledMsg(result)
	if confirmed.favorites.Level != favtui.ListLevel {
		t.Fatalf("favorites level = %v, want ListLevel", confirmed.favorites.Level)
	}
	if len(confirmed.favorites.Artists) != 1 || confirmed.favorites.Artists[0] != "B" {
		t.Fatalf("parent list = %v, want [B]", confirmed.favorites.Artists)
	}

	undone := confirmed.handleUndo()
	if len(undone.favorites.Artists) != 2 || undone.favorites.Artists[0] != "A" {
		t.Fatalf("parent list after undo = %v", undone.favorites.Artists)
	}
}

func TestFavoritePickerFavoritesTrackRemovalAndNonParentNavigation(t *testing.T) {
	t.Run("title removal updates track list", func(t *testing.T) {
		store := newUITestFavorites(map[string][]string{"track": {"h1", "h2"}})
		m := New(context.Background(), WithFavoriteReader(store), WithFavoriteWriter(store))
		m.activeView = viewFavorites
		m.favorites.Level = favtui.ListLevel
		m.favorites.Category = favtui.CategoryTracks
		m.favorites.Tracks = []library.Track{
			{Hash: "h1", Title: "One", Artist: "A", Album: "X"},
			{Hash: "h2", Title: "Two", Artist: "B", Album: "Y"},
		}

		opened := openFavoritePickerForTest(t, m)
		confirmed, result := confirmFavoritePickerForTest(t, opened, "t")
		confirmed = confirmed.handleFavoriteToggledMsg(result)
		if len(confirmed.favorites.Tracks) != 1 || confirmed.favorites.Tracks[0].Hash != "h2" {
			t.Fatalf("favorites tracks after removal = %+v", confirmed.favorites.Tracks)
		}
	})

	t.Run("unrelated removal stays in drilldown", func(t *testing.T) {
		store := newUITestFavorites(map[string][]string{"artist": {"A"}, "album": {"X"}})
		m := New(context.Background(), WithFavoriteReader(store), WithFavoriteWriter(store))
		m.activeView = viewFavorites
		m.favorites.Level = favtui.TrackLevel
		m.favorites.Category = favtui.CategoryAlbums
		m.favorites.Albums = []string{"X"}
		m.favorites.DrilldownName = "X"
		m.favorites.DrilldownTracks = []library.Track{{Hash: "h1", Title: "One", Artist: "A", Album: "X"}}

		opened := openFavoritePickerForTest(t, m)
		confirmed, result := confirmFavoritePickerForTest(t, opened, "r")
		confirmed = confirmed.handleFavoriteToggledMsg(result)
		if confirmed.favorites.Level != favtui.TrackLevel {
			t.Fatalf("favorites level = %v, want TrackLevel", confirmed.favorites.Level)
		}
	})
}
