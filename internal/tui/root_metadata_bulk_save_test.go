package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
)

// TestBulkMetadataEditClearsArtistsSelectionOnSave verifies that saving the
// bulk metadata editor clears artists.selectedHashes.
func TestBulkMetadataEditClearsArtistsSelectionOnSave(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")
	m = pressEnter(m)
	m = pressEnter(m)

	m.browse.artists.selectedHashes = map[string]struct{}{"h1": {}, "h2": {}}
	m = pressMKey(m)
	if m.overlay.metadataEditor == nil {
		t.Fatal("expected editor open")
	}

	m = typeIntoEditor(m, "X")

	result, _ := m.handleMetadataSave()
	m = result.(Model)

	if len(m.browse.artists.selectedHashes) != 0 {
		t.Errorf("after bulk save: expected selectedHashes cleared, got %d", len(m.browse.artists.selectedHashes))
	}
}

// TestBulkMetadataEditClearsAlbumsSelectionOnSave verifies that saving the
// bulk metadata editor clears albums.selectedHashes.
func TestBulkMetadataEditClearsAlbumsSelectionOnSave(t *testing.T) {
	m := newModelWithTracks()
	m = switchToAlbumsMode(m)
	m = pressEnter(m)

	m.browse.albums.selectedHashes = map[string]struct{}{"h1": {}, "h2": {}}
	m = pressMKey(m)
	if m.overlay.metadataEditor == nil {
		t.Fatal("expected editor open")
	}

	m = typeIntoEditor(m, "X")

	result, _ := m.handleMetadataSave()
	m = result.(Model)

	if len(m.browse.albums.selectedHashes) != 0 {
		t.Errorf("after bulk save: expected selectedHashes cleared, got %d", len(m.browse.albums.selectedHashes))
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

func TestBulkMetadataSaveInvalidDurationKeepsEditorOpen(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Old", Title: "One", Duration: 60})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "Old", Title: "Two", Duration: 90})

	ed := NewBulkMetadataEditor([]string{"h1", "h2"}, "", "Old", "", "", "")
	ed.fields[fieldDuration].SetValue("1:99")
	ed.dirty[fieldDuration] = true
	m.overlay.metadataEditor = &ed

	result, cmd := m.handleMetadataSave()
	nm := result.(Model)
	if cmd != nil {
		t.Fatal("invalid duration must not return a persistence command")
	}
	if nm.overlay.metadataEditor == nil {
		t.Fatal("invalid duration should keep the bulk metadata editor open")
	}
	if got := nm.overlay.metadataEditor.Duration(); got != "1:99" {
		t.Fatalf("duration input = %q, want %q", got, "1:99")
	}
	if tr, _ := nm.library.index.FindByHash("h1"); tr.Duration != 60 {
		t.Fatalf("h1 duration changed to %v, want 60", tr.Duration)
	}
	if tr, _ := nm.library.index.FindByHash("h2"); tr.Duration != 90 {
		t.Fatalf("h2 duration changed to %v, want 90", tr.Duration)
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
	m.activeView = viewBrowse
	m.browse.mode = BrowseModeGenres
	m.browse.genres.selectedHashes = map[string]struct{}{"g1": {}, "g2": {}}

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

	if len(m.browse.genres.selectedHashes) != 0 {
		t.Errorf("expected genres.selectedHashes to be cleared after bulk metadata save, got %v", m.browse.genres.selectedHashes)
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
