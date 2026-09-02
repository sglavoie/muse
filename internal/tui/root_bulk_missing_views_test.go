package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/sorting"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// TestBulkActionTagsTrackLevel verifies that pressing 'a' at Tags TrackLevel
// opens the destination picker with the focused track.
func TestBulkActionTagsTrackLevel(t *testing.T) {
	m := New(context.Background(), WithLibraryTracks(sampleLibraryTracks()))
	m.activeView = viewTags
	m.tagsView.level = tagtui.TrackLevel
	m.tagsView.tracks = []db.PlaylistTrackRow{
		{Hash: "h1", Title: "Track A1", Artist: "Alice", Album: "Alpha", DurationMs: 183000, Path: "/a1.flac"},
		{Hash: "h4", Title: "Track G1", Artist: "Bob", Album: "Gamma", DurationMs: 120000, Path: "/g1.flac"},
	}
	m.tagsView.cursor = 0
	m.tagsView.width = 80
	m.tagsView.height = 40

	next, _, handled := m.handleLibraryKeyAction(keys.ActionBulkAction)
	if !handled {
		t.Fatal("actionBulkAction should be handled")
	}
	nm := next.(Model)
	if nm.overlay.destPicker == nil {
		t.Fatal("expected destPicker to be non-nil at Tags TrackLevel")
	}
	if len(nm.overlay.destPickerTracks) != 1 {
		t.Fatalf("expected 1 destPickerTrack (focused), got %d", len(nm.overlay.destPickerTracks))
	}
	if nm.overlay.destPickerTracks[0].Hash != "h1" {
		t.Errorf("expected hash h1, got %q", nm.overlay.destPickerTracks[0].Hash)
	}
}

// TestBulkActionTagsTrackLevelWithSelection verifies that bulk action from Tags
// with a selection collects all selected tracks.
func TestBulkActionTagsTrackLevelWithSelection(t *testing.T) {
	m := New(context.Background(), WithLibraryTracks(sampleLibraryTracks()))
	m.activeView = viewTags
	m.tagsView.level = tagtui.TrackLevel
	m.tagsView.tracks = []db.PlaylistTrackRow{
		{Hash: "h1", Title: "Track A1", Artist: "Alice", Album: "Alpha", DurationMs: 183000, Path: "/a1.flac"},
		{Hash: "h4", Title: "Track G1", Artist: "Bob", Album: "Gamma", DurationMs: 120000, Path: "/g1.flac"},
	}
	m.tagsView.selectedHashes = map[string]struct{}{"h1": {}, "h4": {}}
	m.tagsView.width = 80
	m.tagsView.height = 40

	next, _, handled := m.handleLibraryKeyAction(keys.ActionBulkAction)
	if !handled {
		t.Fatal("actionBulkAction should be handled")
	}
	nm := next.(Model)
	if nm.overlay.destPicker == nil {
		t.Fatal("expected destPicker to be non-nil with selection")
	}
	if len(nm.overlay.destPickerTracks) != 2 {
		t.Fatalf("expected 2 destPickerTracks, got %d", len(nm.overlay.destPickerTracks))
	}
}

// TestBulkActionFavoritesTrackLevel verifies that pressing 'a' at Favorites
// TrackLevel opens the destination picker with the focused track.
func TestBulkActionFavoritesTrackLevel(t *testing.T) {
	tracks := sampleLibraryTracks()
	m := New(context.Background(), WithLibraryTracks(tracks))
	m.activeView = viewFavorites
	m.favorites.level = favtui.TrackLevel
	m.favorites.drilldownTracks = tracks[:2]
	m.favorites.cursor = 0
	m.favorites.width = 80
	m.favorites.height = 40

	next, _, handled := m.handleLibraryKeyAction(keys.ActionBulkAction)
	if !handled {
		t.Fatal("actionBulkAction should be handled")
	}
	nm := next.(Model)
	if nm.overlay.destPicker == nil {
		t.Fatal("expected destPicker to be non-nil at Favorites TrackLevel")
	}
	if len(nm.overlay.destPickerTracks) != 1 {
		t.Fatalf("expected 1 destPickerTrack (focused), got %d", len(nm.overlay.destPickerTracks))
	}
	if nm.overlay.destPickerTracks[0].Hash != "h1" {
		t.Errorf("expected hash h1, got %q", nm.overlay.destPickerTracks[0].Hash)
	}
}

// TestBulkActionFavoritesListLevelTracks verifies that bulk action works at
// Favorites ListLevel for the Tracks category.
func TestBulkActionFavoritesListLevelTracks(t *testing.T) {
	tracks := sampleLibraryTracks()
	m := New(context.Background(), WithLibraryTracks(tracks))
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryTracks
	m.favorites.tracks = tracks[:3]
	m.favorites.selectedItems = map[int]struct{}{0: {}, 2: {}}
	m.favorites.width = 80
	m.favorites.height = 40

	next, _, handled := m.handleLibraryKeyAction(keys.ActionBulkAction)
	if !handled {
		t.Fatal("actionBulkAction should be handled")
	}
	nm := next.(Model)
	if nm.overlay.destPicker == nil {
		t.Fatal("expected destPicker to be non-nil at Favorites ListLevel/Tracks")
	}
	if len(nm.overlay.destPickerTracks) != 2 {
		t.Fatalf("expected 2 destPickerTracks, got %d", len(nm.overlay.destPickerTracks))
	}
}

// TestEnqueueReplaceFromTags verifies that the enqueue-replace action from the
// Tags view calls enqueueFocusedTrack.
func TestEnqueueReplaceFromTags(t *testing.T) {
	qa := &fakeQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa), WithLibraryTracks(sampleLibraryTracks()))
	m.activeView = viewTags
	m.tagsView.level = tagtui.TrackLevel
	m.tagsView.tracks = []db.PlaylistTrackRow{
		{Hash: "h1", Title: "Track A1", Artist: "Alice", Album: "Alpha", DurationMs: 183000, Path: "/a1.flac"},
	}
	m.tagsView.cursor = 0
	m.tagsView.width = 80
	m.tagsView.height = 40

	_, cmd, handled := m.handleLibraryEnqueueReplaceAction()
	if !handled {
		t.Fatal("enqueue-replace should be handled for viewTags")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd from enqueue-replace in Tags")
	}
}

// TestResetAllSortsClearsTagsSort verifies that resetAllSorts resets the tags
// view sort state.
func TestResetAllSortsClearsTagsSort(t *testing.T) {
	m := New(context.Background())
	m.tagsView.sortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortDesc}

	m = m.resetAllSorts()

	if m.tagsView.sortState.Field != sorting.SortDefault {
		t.Errorf("expected tags sort field to be SortDefault, got %v", m.tagsView.sortState.Field)
	}
	if m.tagsView.sortState.Dir != sorting.SortAsc {
		t.Errorf("expected tags sort dir to be SortAsc, got %v", m.tagsView.sortState.Dir)
	}
}

// TestBulkActionTagsNoOpAtTagLevel verifies that pressing 'a' at Tags
// TagLevel returns nil tracks (no dest picker should open).
func TestBulkActionTagsNoOpAtTagLevel(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewTags
	m.tagsView.level = tagtui.TagLevel
	m.tagsView.tags = []db.TagRow{{ID: 1, Name: "T1"}}

	tracks := m.bulkTagTracks()
	if tracks != nil {
		t.Errorf("expected nil tracks at TagLevel, got %d", len(tracks))
	}
}

// TestBulkActionFavoritesNoOpAtCategoryLevel verifies that pressing 'a' at
// Favorites CategoryLevel returns nil tracks.
func TestBulkActionFavoritesNoOpAtCategoryLevel(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewFavorites
	m.favorites.level = favtui.CategoryLevel

	tracks := m.bulkFavoriteTracks()
	if tracks != nil {
		t.Errorf("expected nil tracks at CategoryLevel, got %d", len(tracks))
	}
}

// TestBulkActionFavoritesNoOpAtListLevelArtists verifies that bulk action at
// Favorites ListLevel with CategoryArtists returns nil (not a track list).
func TestBulkActionFavoritesNoOpAtListLevelArtists(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryArtists
	m.favorites.artists = []string{"Alice", "Bob"}

	tracks := m.bulkFavoriteTracks()
	if tracks != nil {
		t.Errorf("expected nil tracks at ListLevel/Artists, got %d", len(tracks))
	}
}
