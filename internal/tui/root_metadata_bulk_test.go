package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
)

// TestBulkMetadataEditFromArtistsTrackLevel verifies that bulkSelectedHashes
// returns hashes from artists.selectedHashes when the Artists view is active.
func TestBulkMetadataEditFromArtistsTrackLevel(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")
	m = pressEnter(m)
	m = pressEnter(m)

	m.artists.selectedHashes = map[string]struct{}{"h1": {}, "h2": {}}

	hashes := m.bulkSelectedHashes()
	if len(hashes) != 2 {
		t.Fatalf("bulkSelectedHashes: expected 2, got %d", len(hashes))
	}

	m = pressMKey(m)
	if m.overlay.metadataEditor == nil {
		t.Fatal("expected bulk metadataEditor to open with 2 selected tracks")
	}
	if !m.overlay.metadataEditor.IsBulk() {
		t.Error("expected IsBulk=true for 2 selected tracks")
	}
	if len(m.overlay.metadataEditor.Hashes()) != 2 {
		t.Errorf("expected 2 hashes in editor, got %d", len(m.overlay.metadataEditor.Hashes()))
	}
}

// TestBulkMetadataEditClearsArtistsSelectionOnSave verifies that saving the
// bulk metadata editor clears artists.selectedHashes.
func TestBulkMetadataEditClearsArtistsSelectionOnSave(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")
	m = pressEnter(m)
	m = pressEnter(m)

	m.artists.selectedHashes = map[string]struct{}{"h1": {}, "h2": {}}
	m = pressMKey(m)
	if m.overlay.metadataEditor == nil {
		t.Fatal("expected editor open")
	}

	m = typeIntoEditor(m, "X")

	result, _ := m.handleMetadataSave()
	m = result.(Model)

	if len(m.artists.selectedHashes) != 0 {
		t.Errorf("after bulk save: expected selectedHashes cleared, got %d", len(m.artists.selectedHashes))
	}
}

// TestBulkMetadataEditFromAlbumsTrackLevel verifies that bulkSelectedHashes
// returns hashes from albums.selectedHashes when the Albums view is active.
func TestBulkMetadataEditFromAlbumsTrackLevel(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "5")
	m = pressEnter(m)

	m.albums.selectedHashes = map[string]struct{}{"h1": {}, "h2": {}}

	hashes := m.bulkSelectedHashes()
	if len(hashes) != 2 {
		t.Fatalf("bulkSelectedHashes (albums): expected 2, got %d", len(hashes))
	}

	m = pressMKey(m)
	if m.overlay.metadataEditor == nil {
		t.Fatal("expected bulk metadataEditor to open with 2 selected tracks")
	}
	if !m.overlay.metadataEditor.IsBulk() {
		t.Error("expected IsBulk=true for 2 selected tracks")
	}
}

// TestBulkMetadataEditClearsAlbumsSelectionOnSave verifies that saving the
// bulk metadata editor clears albums.selectedHashes.
func TestBulkMetadataEditClearsAlbumsSelectionOnSave(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "5")
	m = pressEnter(m)

	m.albums.selectedHashes = map[string]struct{}{"h1": {}, "h2": {}}
	m = pressMKey(m)
	if m.overlay.metadataEditor == nil {
		t.Fatal("expected editor open")
	}

	m = typeIntoEditor(m, "X")

	result, _ := m.handleMetadataSave()
	m = result.(Model)

	if len(m.albums.selectedHashes) != 0 {
		t.Errorf("after bulk save: expected selectedHashes cleared, got %d", len(m.albums.selectedHashes))
	}
}

// TestBulkSelectedHashesEmptyWhenNoSelection verifies that bulkSelectedHashes
// returns nil when artists/albums have no selection.
func TestBulkSelectedHashesEmptyWhenNoSelection(t *testing.T) {
	m := newModelWithTracks()

	m = pressKey(m, "4")
	if hashes := m.bulkSelectedHashes(); hashes != nil {
		t.Errorf("artists with no selection: expected nil, got %v", hashes)
	}

	m = pressKey(m, "5")
	if hashes := m.bulkSelectedHashes(); hashes != nil {
		t.Errorf("albums with no selection: expected nil, got %v", hashes)
	}
}

// TestBulkMetadataSaveUsesTrackDisplayForQueueEntryDisplay verifies that bulk
// metadata save also uses the trackDisplay format for queue entry displays.
func TestBulkMetadataSaveUsesTrackDisplayForQueueEntryDisplay(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Old", Title: "Old", Album: "A"})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "Old", Title: "Old", Album: "A"})
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a.flac", Display: "Old - Old", Artist: "Old", Title: "Old"},
		{ID: 2, Hash: "h2", Path: "/b.flac", Display: "Old - Old", Artist: "Old", Title: "Old"},
	}
	m.activeView = viewQueue
	m.queue.entries[0].Selected = true
	m.queue.entries[1].Selected = true

	ed := NewBulkMetadataEditor([]string{"h1", "h2"}, "Old", "Old", "A", "", "")
	ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	for range 3 {
		ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	m.overlay.metadataEditor = &ed

	result, _ := m.handleMetadataSave()
	nm := result.(Model)
	for _, e := range nm.queue.entries {
		if e.Display != "Old" {
			t.Errorf("expected Display=%q (title only), got %q", "Old", e.Display)
		}
	}
}

// TestBulkMetadataSaveClearsGenresSelection verifies that after a bulk
// metadata edit from the genres view, genres.selectedHashes is cleared.
func TestBulkMetadataSaveClearsGenresSelection(t *testing.T) {
	tracks := []library.Track{
		{Artist: "Alice", Album: "Alpha", Title: "Song 1", Genre: "Rock", Hash: "g1", Path: "/g1.flac"},
		{Artist: "Alice", Album: "Alpha", Title: "Song 2", Genre: "Rock", Hash: "g2", Path: "/g2.flac"},
	}
	m := New(context.Background(), WithLibraryTracks(tracks))
	m.activeView = viewGenres
	m.genres.selectedHashes = map[string]struct{}{"g1": {}, "g2": {}}

	ed := NewBulkMetadataEditor([]string{"g1", "g2"}, "Song 1", "Alice", "Alpha", "Rock", "")
	genreMsg := tea.KeyPressMsg{Code: tea.KeyTab}
	ed, _, _ = ed.HandleKey(genreMsg)
	ed, _, _ = ed.HandleKey(genreMsg)
	ed, _, _ = ed.HandleKey(genreMsg)
	ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: 'P', Text: "P"})
	ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: 'o', Text: "o"})
	ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: 'p', Text: "p"})
	m.overlay.metadataEditor = &ed

	result, _ := m.handleMetadataSave()
	m = result.(Model)

	if len(m.genres.selectedHashes) != 0 {
		t.Errorf("expected genres.selectedHashes to be cleared after bulk metadata save, got %v", m.genres.selectedHashes)
	}
}

// TestBulkSelectedHashesFavoritesListLevelTracks verifies that
// bulkSelectedHashes returns hashes for selected tracks at favorites ListLevel.
func TestBulkSelectedHashesFavoritesListLevelTracks(t *testing.T) {
	m := New(context.Background(), WithLibraryTracks(sampleLibraryTracks()))
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryTracks
	m.favorites.tracks = []library.Track{
		{Hash: "h1", Artist: "Alice", Title: "Track A1"},
		{Hash: "h2", Artist: "Alice", Title: "Track A2"},
		{Hash: "h3", Artist: "Alice", Title: "Track B1"},
	}
	m.favorites.selectedItems = map[int]struct{}{0: {}, 2: {}}

	hashes := m.bulkSelectedHashes()
	if len(hashes) != 2 {
		t.Fatalf("expected 2 hashes, got %d", len(hashes))
	}
	hashSet := make(map[string]struct{}, len(hashes))
	for _, h := range hashes {
		hashSet[h] = struct{}{}
	}
	if _, ok := hashSet["h1"]; !ok {
		t.Error("expected h1 in hashes")
	}
	if _, ok := hashSet["h3"]; !ok {
		t.Error("expected h3 in hashes")
	}
}

// TestBulkSelectedHashesFavoritesTrackLevel verifies that bulkSelectedHashes
// returns hashes for selected tracks at favorites TrackLevel.
func TestBulkSelectedHashesFavoritesTrackLevel(t *testing.T) {
	m := New(context.Background(), WithLibraryTracks(sampleLibraryTracks()))
	m.activeView = viewFavorites
	m.favorites.level = favtui.TrackLevel
	m.favorites.drilldownTracks = []library.Track{
		{Hash: "h4", Artist: "Bob", Title: "Track G1"},
		{Hash: "h5", Artist: "Bob", Title: "Track G2"},
	}
	m.favorites.selectedItems = map[int]struct{}{1: {}}

	hashes := m.bulkSelectedHashes()
	if len(hashes) != 1 {
		t.Fatalf("expected 1 hash, got %d", len(hashes))
	}
	if hashes[0] != "h5" {
		t.Errorf("expected h5, got %s", hashes[0])
	}
}

// TestBulkSelectedHashesFavoritesArtistLevelReturnsNil verifies that
// bulkSelectedHashes returns nil at favorites ListLevel with CategoryArtists.
func TestBulkSelectedHashesFavoritesArtistLevelReturnsNil(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryArtists
	m.favorites.artists = []string{"Alice", "Bob"}
	m.favorites.selectedItems = map[int]struct{}{0: {}}

	if hashes := m.bulkSelectedHashes(); hashes != nil {
		t.Errorf("expected nil for artist level, got %v", hashes)
	}
}

// TestBulkMetadataSaveClearsFavoritesSelection verifies that saving a bulk
// metadata edit from the favorites view clears the selectedItems map.
func TestBulkMetadataSaveClearsFavoritesSelection(t *testing.T) {
	m := New(context.Background(), WithLibraryTracks(sampleLibraryTracks()))
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryTracks
	m.favorites.tracks = []library.Track{
		{Hash: "h1", Artist: "Alice", Title: "Track A1"},
		{Hash: "h2", Artist: "Alice", Title: "Track A2"},
	}
	m.favorites.selectedItems = map[int]struct{}{0: {}, 1: {}}

	ed := NewBulkMetadataEditor([]string{"h1", "h2"}, "", "Alice", "", "", "")
	m.overlay.metadataEditor = &ed
	m = typeIntoEditor(m, "X")

	result, _ := m.handleMetadataSave()
	nm := result.(Model)

	if nm.favorites.selectedItems != nil {
		t.Errorf("expected selectedItems to be cleared, got %v", nm.favorites.selectedItems)
	}
}

// TestBulkSelectedHashesRecentlyAdded verifies that bulkSelectedHashes
// returns hashes from recentlyAdded.selectedHashes when the Recently Added
// view is active.
func TestBulkSelectedHashesRecentlyAdded(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "8")

	if m.activeView != viewRecentlyAdded {
		t.Fatalf("expected viewRecentlyAdded, got %d", m.activeView)
	}

	m.recentlyAdded.selectedHashes = map[string]struct{}{"h1": {}, "h2": {}}

	hashes := m.bulkSelectedHashes()
	if len(hashes) != 2 {
		t.Fatalf("bulkSelectedHashes (recentlyAdded): expected 2, got %d", len(hashes))
	}
	hashSet := make(map[string]struct{}, len(hashes))
	for _, h := range hashes {
		hashSet[h] = struct{}{}
	}
	if _, ok := hashSet["h1"]; !ok {
		t.Error("expected h1 in hashes")
	}
	if _, ok := hashSet["h2"]; !ok {
		t.Error("expected h2 in hashes")
	}
}

// TestBulkSelectedHashesRecentlyAddedEmptyReturnsNil verifies that
// bulkSelectedHashes returns nil when no tracks are selected in the
// Recently Added view.
func TestBulkSelectedHashesRecentlyAddedEmptyReturnsNil(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "8")

	if hashes := m.bulkSelectedHashes(); hashes != nil {
		t.Errorf("recentlyAdded with no selection: expected nil, got %v", hashes)
	}
}

// TestBulkMetadataEditFromRecentlyAdded verifies that pressing M with
// multiple selected tracks in the Recently Added view opens the bulk
// metadata editor.
func TestBulkMetadataEditFromRecentlyAdded(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "8")
	m.recentlyAdded.selectedHashes = map[string]struct{}{"h1": {}, "h2": {}}

	m = pressMKey(m)
	if m.overlay.metadataEditor == nil {
		t.Fatal("expected bulk metadataEditor to open with 2 selected tracks in Recently Added")
	}
	if !m.overlay.metadataEditor.IsBulk() {
		t.Error("expected IsBulk=true for 2 selected tracks")
	}
	if len(m.overlay.metadataEditor.Hashes()) != 2 {
		t.Errorf("expected 2 hashes in editor, got %d", len(m.overlay.metadataEditor.Hashes()))
	}
}

func TestBulkSelectedHashesTags(t *testing.T) {
	m := newModelWithTracks()
	m.activeView = viewTags
	m.tagsView.selectedHashes = map[string]struct{}{"h1": {}, "h2": {}}

	hashes := m.bulkSelectedHashes()
	if len(hashes) != 2 {
		t.Fatalf("bulkSelectedHashes (tags): expected 2, got %d", len(hashes))
	}
	hashSet := make(map[string]struct{}, len(hashes))
	for _, h := range hashes {
		hashSet[h] = struct{}{}
	}
	if _, ok := hashSet["h1"]; !ok {
		t.Error("expected h1 in hashes")
	}
	if _, ok := hashSet["h2"]; !ok {
		t.Error("expected h2 in hashes")
	}
}

// TestBulkMetadataSaveClearsTagsSelection verifies that saving a bulk
// metadata edit from the Tags view clears tagsView.selectedHashes.
func TestBulkMetadataSaveClearsTagsSelection(t *testing.T) {
	m := New(context.Background(), WithLibraryTracks(sampleLibraryTracks()))
	m.activeView = viewTags
	m.tagsView.selectedHashes = map[string]struct{}{"h1": {}, "h2": {}}

	ed := NewBulkMetadataEditor([]string{"h1", "h2"}, "", "Alice", "", "", "")
	m.overlay.metadataEditor = &ed
	m = typeIntoEditor(m, "X")

	result, _ := m.handleMetadataSave()
	nm := result.(Model)

	if nm.tagsView.selectedHashes != nil {
		t.Errorf("expected tagsView.selectedHashes to be cleared, got %v", nm.tagsView.selectedHashes)
	}
}

func TestBulkSelectedHashesTagsEmptyReturnsNil(t *testing.T) {
	m := newModelWithTracks()
	m.activeView = viewTags

	if hashes := m.bulkSelectedHashes(); hashes != nil {
		t.Errorf("tags with no selection: expected nil, got %v", hashes)
	}
}

// TestBulkMetadataSaveClearsMixesSelection verifies that saving a bulk
// metadata edit from the Mixes view clears trackView.selectedHashes.
func TestBulkMetadataSaveClearsMixesSelection(t *testing.T) {
	tracks := []library.Track{
		{Hash: "mx1", Title: "Mix 1", Artist: "ArtX", Path: "/mx1.flac"},
		{Hash: "mx2", Title: "Mix 2", Artist: "ArtX", Path: "/mx2.flac"},
	}
	m := New(context.Background(), WithLibraryTracks(tracks))
	m.activeView = viewMixes
	m.mixes.level = 1
	m.mixes.trackView.selectedHashes = map[string]struct{}{"mx1": {}, "mx2": {}}

	ed := NewBulkMetadataEditor([]string{"mx1", "mx2"}, "", "ArtX", "", "", "")
	m.overlay.metadataEditor = &ed
	m = typeIntoEditor(m, "X")

	result, _ := m.handleMetadataSave()
	nm := result.(Model)

	if nm.mixes.trackView.selectedHashes != nil {
		t.Errorf("expected mixes trackView.selectedHashes to be cleared, got %v", nm.mixes.trackView.selectedHashes)
	}
}
