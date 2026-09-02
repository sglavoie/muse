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
	m.ActiveView = viewTags
	m.tagsView.Level = tagtui.TrackLevel
	m.tagsView.Tracks = []db.PlaylistTrackRow{
		{Hash: "h1", Title: "Track A1", Artist: "Alice", Album: "Alpha", DurationMs: 183000, Path: "/a1.flac"},
		{Hash: "h4", Title: "Track G1", Artist: "Bob", Album: "Gamma", DurationMs: 120000, Path: "/g1.flac"},
	}
	m.tagsView.Cursor = 0
	m.tagsView.Width = 80
	m.tagsView.Height = 40

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
	m.ActiveView = viewTags
	m.tagsView.Level = tagtui.TrackLevel
	m.tagsView.Tracks = []db.PlaylistTrackRow{
		{Hash: "h1", Title: "Track A1", Artist: "Alice", Album: "Alpha", DurationMs: 183000, Path: "/a1.flac"},
		{Hash: "h4", Title: "Track G1", Artist: "Bob", Album: "Gamma", DurationMs: 120000, Path: "/g1.flac"},
	}
	m.tagsView.SelectedHashes = map[string]struct{}{"h1": {}, "h4": {}}
	m.tagsView.Width = 80
	m.tagsView.Height = 40

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
	m.ActiveView = viewFavorites
	m.favorites.Level = favtui.TrackLevel
	m.favorites.DrilldownTracks = tracks[:2]
	m.favorites.Cursor = 0
	m.favorites.Width = 80
	m.favorites.Height = 40

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
	m.ActiveView = viewFavorites
	m.favorites.Level = favtui.ListLevel
	m.favorites.Category = favtui.CategoryTracks
	m.favorites.Tracks = tracks[:3]
	m.favorites.SelectedItems = map[int]struct{}{0: {}, 2: {}}
	m.favorites.Width = 80
	m.favorites.Height = 40

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
	m.ActiveView = viewTags
	m.tagsView.Level = tagtui.TrackLevel
	m.tagsView.Tracks = []db.PlaylistTrackRow{
		{Hash: "h1", Title: "Track A1", Artist: "Alice", Album: "Alpha", DurationMs: 183000, Path: "/a1.flac"},
	}
	m.tagsView.Cursor = 0
	m.tagsView.Width = 80
	m.tagsView.Height = 40

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
	m.tagsView.SortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortDesc}

	m = m.resetAllSorts()

	if m.tagsView.SortState.Field != sorting.SortDefault {
		t.Errorf("expected tags sort field to be SortDefault, got %v", m.tagsView.SortState.Field)
	}
	if m.tagsView.SortState.Dir != sorting.SortAsc {
		t.Errorf("expected tags sort dir to be SortAsc, got %v", m.tagsView.SortState.Dir)
	}
}

// TestBulkActionTagsNoOpAtTagLevel verifies that pressing 'a' at Tags
// TagLevel returns nil tracks (no dest picker should open).
func TestBulkActionTagsNoOpAtTagLevel(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewTags
	m.tagsView.Level = tagtui.TagLevel
	m.tagsView.Tags = []db.TagRow{{ID: 1, Name: "T1"}}

	tracks := m.bulkTagTracks()
	if tracks != nil {
		t.Errorf("expected nil tracks at TagLevel, got %d", len(tracks))
	}
}

// TestBulkActionFavoritesNoOpAtCategoryLevel verifies that pressing 'a' at
// Favorites CategoryLevel returns nil tracks.
func TestBulkActionFavoritesNoOpAtCategoryLevel(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewFavorites
	m.favorites.Level = favtui.CategoryLevel

	tracks := m.bulkFavoriteTracks()
	if tracks != nil {
		t.Errorf("expected nil tracks at CategoryLevel, got %d", len(tracks))
	}
}

// TestBulkActionFavoritesNoOpAtListLevelArtists verifies that bulk action at
// Favorites ListLevel with CategoryArtists returns nil (not a track list).
func TestBulkActionFavoritesNoOpAtListLevelArtists(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewFavorites
	m.favorites.Level = favtui.ListLevel
	m.favorites.Category = favtui.CategoryArtists
	m.favorites.Artists = []string{"Alice", "Bob"}

	tracks := m.bulkFavoriteTracks()
	if tracks != nil {
		t.Errorf("expected nil tracks at ListLevel/Artists, got %d", len(tracks))
	}
}
