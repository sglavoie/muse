package tui

import (
	"context"
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

// TestRescanHotkeyReturnsCmd verifies that pressing ctrl+r with a
// configured library root returns a non-nil background cmd and does not block.
func TestRescanHotkeyReturnsCmd(t *testing.T) {
	root := t.TempDir()
	m := New(context.Background(), WithLibraryRoots([]string{root}), WithExtractor(stubExtractor{}))
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("R key: expected non-nil cmd for rescan, got nil")
	}
}

// TestRescanHotkeyIsNoOpWhenAlreadyScanning verifies that pressing R during
// an in-progress scan does nothing.
func TestRescanHotkeyIsNoOpWhenScanning(t *testing.T) {
	ch := make(chan library.Track)
	defer close(ch)
	root := t.TempDir()
	m := New(context.Background(), WithScanChannel(ch), WithLibraryRoots([]string{root}), WithExtractor(stubExtractor{}))
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	if cmd != nil {
		t.Fatal("R key during active scan must return nil cmd")
	}
}

// TestRescanHotkeyIsNoOpWithoutRoot verifies that pressing R when no library
// root is configured returns nil.
func TestRescanHotkeyIsNoOpWithoutRoot(t *testing.T) {
	m := New(context.Background())
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	if cmd != nil {
		t.Fatal("R key without library root must return nil cmd")
	}
}

// TestRescanResetsIndexAndAddsNewTracks verifies that triggering a rescan
// clears the existing index and that subsequent ScanBatchMsg messages
// populate it with only the new tracks, effectively removing deleted ones.
func TestRescanResetsIndexAndAddsNewTracks(t *testing.T) {
	root := t.TempDir()
	m := New(context.Background(), WithLibraryRoots([]string{root}), WithExtractor(stubExtractor{}))

	// Seed two initial tracks.
	next, _ := m.Update(library.ScanBatchMsg{Tracks: []library.Track{
		{Artist: "A", Title: "kept track"},
		{Artist: "B", Title: "deleted track"},
	}})
	nm := next.(Model)
	if nm.library.Index.Len() != 2 {
		t.Fatalf("seed: expected 2 tracks, got %d", nm.library.Index.Len())
	}

	// Trigger rescan — index must be reset immediately.
	next, cmd := nm.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	nm = next.(Model)
	if cmd == nil {
		t.Fatal("rescan: expected non-nil cmd")
	}
	if nm.library.Index.Len() != 0 {
		t.Fatalf("rescan: expected empty index after reset, got %d", nm.library.Index.Len())
	}

	// Only the surviving track arrives; the deleted track is absent.
	next, _ = nm.Update(library.ScanBatchMsg{Tracks: []library.Track{
		{Artist: "A", Title: "kept track"},
	}})
	nm = next.(Model)
	if nm.library.Index.Len() != 1 {
		t.Fatalf("after rescan batch: expected 1 track, got %d", nm.library.Index.Len())
	}
	if nm.library.Index.Tracks()[0].Title != "kept track" {
		t.Fatalf("expected 'kept track', got %q", nm.library.Index.Tracks()[0].Title)
	}
}

// TestRescanResetsCursorScrollOffsetAndFilter verifies that triggering a rescan
// resets the library cursor, scroll offset, and filter state so that the UI
// does not render with a stale cursor pointing past the end of the now-empty
// index.
func TestRescanResetsCursorScrollOffsetAndFilter(t *testing.T) {
	root := t.TempDir()
	m := New(context.Background(), WithLibraryRoots([]string{root}), WithExtractor(stubExtractor{}))

	// Seed tracks and move the cursor down.
	tracks := make([]library.Track, 20)
	for i := range tracks {
		tracks[i] = library.Track{Artist: "A", Title: fmt.Sprintf("T%02d", i), Hash: fmt.Sprintf("h%d", i)}
	}
	next, _ := m.Update(library.ScanBatchMsg{Tracks: tracks})
	nm := next.(Model)
	// Move cursor to position 10.
	for range 10 {
		nm.library = nm.library.HandleAction(keys.ActionCursorDown)
	}
	if nm.library.Cursor != 10 {
		t.Fatalf("setup: expected cursor=10, got %d", nm.library.Cursor)
	}
	// Activate a filter.
	nm.library.Filter, _ = nm.library.Filter.Activate()
	nm.library.Filter, _ = nm.library.Filter.Update(tea.KeyPressMsg{Text: "T"})
	nm.library.Cursor = 0
	nm.library.ScrollOffset = 0
	nm.library = nm.library.RebuildFilterCache()
	nm.library.Filter = nm.library.Filter.Commit()
	if nm.library.Filter.Text != "T" {
		t.Fatalf("setup: expected filterText='T', got %q", nm.library.Filter.Text)
	}

	// Trigger rescan.
	next, cmd := nm.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	nm = next.(Model)
	if cmd == nil {
		t.Fatal("rescan: expected non-nil cmd")
	}
	if nm.library.Cursor != 0 {
		t.Fatalf("rescan: expected cursor=0, got %d", nm.library.Cursor)
	}
	if nm.library.ScrollOffset != 0 {
		t.Fatalf("rescan: expected scrollOffset=0, got %d", nm.library.ScrollOffset)
	}
	if nm.library.Filter.Active {
		t.Fatal("rescan: expected filterActive=false")
	}
	if nm.library.Filter.Text != "" {
		t.Fatalf("rescan: expected filterText='', got %q", nm.library.Filter.Text)
	}
	if nm.library.FilteredMatches != nil {
		t.Fatal("rescan: expected filteredMatches=nil")
	}
}

// TestCtrlRFiresRescan verifies that pressing ctrl+r with a configured
// library root triggers a rescan.
func TestCtrlRFiresRescan(t *testing.T) {
	root := t.TempDir()
	m := New(context.Background(), WithLibraryRoots([]string{root}), WithExtractor(stubExtractor{}))

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+r: expected non-nil rescan cmd")
	}
}

// TestRescanClearsSelectionAndSort verifies that a manual rescan resets the
// library selection, sort state, and sorted cache so stale state from the
// previous library does not carry over.
func TestRescanClearsSelectionAndSort(t *testing.T) {
	// Provide a library root and extractor so the rescan action is not a no-op.
	root := t.TempDir()
	m := New(context.Background(), WithLibraryRoots([]string{root}), WithExtractor(stubExtractor{}))
	// Seed a track, select it, and activate a sort.
	m.library.Index.Insert(library.Track{Artist: "A", Title: "Alpha", Path: "/a", Hash: "h1"})
	m.library = m.library.ToggleBulkSelection("h1")
	m.library.SortState = sorting.SortState{Field: sorting.SortArtist, Dir: sorting.SortAsc}
	m.library = m.library.RecomputeSortedTracks()

	if len(m.library.SelectedHashes) != 1 {
		t.Fatalf("setup: expected 1 selected hash, got %d", len(m.library.SelectedHashes))
	}
	if m.library.SortState.Field != sorting.SortArtist {
		t.Fatal("setup: expected SortArtist sort field")
	}
	if m.library.SortedTracks == nil {
		t.Fatal("setup: expected non-nil sortedTracks")
	}

	// Trigger rescan. We cannot actually run the full scan pipeline (needs
	// mpv + exiftool), but we can dispatch the action and verify the model
	// state was reset. The key mapping for rescan is ctrl+r.
	next, _ := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	nm := next.(Model)

	if nm.library.SelectedHashes != nil {
		t.Errorf("selectedHashes should be nil after rescan, got %v", nm.library.SelectedHashes)
	}
	if nm.library.SortState.Field != sorting.SortDefault {
		t.Errorf("sortState.Field should be SortDefault after rescan, got %v", nm.library.SortState.Field)
	}
	if nm.library.SortedTracks != nil {
		t.Errorf("sortedTracks should be nil after rescan, got %v", nm.library.SortedTracks)
	}
}

// TestRescanResetsGenresView verifies that triggering a rescan clears the
// genres view's stale drill-down state (selectedGenre, selectedArtist) so
// that stale references from the previous scan don't persist.
func TestRescanResetsGenresView(t *testing.T) {
	root := t.TempDir()
	m := New(context.Background(), WithLibraryRoots([]string{root}), WithExtractor(stubExtractor{}))

	// Set up genres view at ArtistLevel with a selected genre.
	m.browse.Genres.Level = gentui.ArtistLevel
	m.browse.Genres.SelectedGenre = "Rock"
	m.browse.Genres.Artists = []string{"Artist1", "Artist2"}
	m.browse.Genres.Cursor = 1
	m.browse.Genres.ScrollOffset = 0

	// Trigger rescan.
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	nm := next.(Model)
	if cmd == nil {
		t.Fatal("rescan: expected non-nil cmd")
	}

	// Genres view must be reset to GenreLevel with no stale selection.
	if nm.browse.Genres.Level != gentui.GenreLevel {
		t.Fatalf("rescan: expected genres level=GenreLevel, got %v", nm.browse.Genres.Level)
	}
	if nm.browse.Genres.SelectedGenre != "" {
		t.Fatalf("rescan: expected selectedGenre='', got %q", nm.browse.Genres.SelectedGenre)
	}
	if nm.browse.Genres.SelectedArtist != "" {
		t.Fatalf("rescan: expected selectedArtist='', got %q", nm.browse.Genres.SelectedArtist)
	}
	if nm.browse.Genres.Cursor != 0 {
		t.Fatalf("rescan: expected genres cursor=0, got %d", nm.browse.Genres.Cursor)
	}
	if len(nm.browse.Genres.Artists) != 0 {
		t.Fatalf("rescan: expected genres artists empty, got %d", len(nm.browse.Genres.Artists))
	}
}
