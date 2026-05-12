package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
)

// TestScanBatchRefreshesArtistList verifies that receiving a ScanBatchMsg
// while at ArtistLevel re-derives the artist list.
func TestScanBatchRefreshesArtistList(t *testing.T) {
	m := New(context.Background(), WithLibraryTracks([]library.Track{{Artist: "Alice", Album: "Alpha", Title: "T1", Hash: "h1"}}))

	m = pressKey(m, "4")
	if len(m.browse.artists.artists) != 1 {
		t.Fatalf("expected 1 artist initially, got %d", len(m.browse.artists.artists))
	}

	next, _ := m.Update(library.ScanBatchMsg{Tracks: []library.Track{{Artist: "Bob", Album: "Beta", Title: "T2", Hash: "h2"}}})
	m = next.(Model)

	if len(m.browse.artists.artists) != 2 {
		t.Fatalf("expected 2 artists after scan batch, got %d", len(m.browse.artists.artists))
	}
}

// TestWatchBatchRefreshesArtistList verifies that receiving a watchBatchMsg
// while at ArtistLevel re-derives the artist list.
func TestWatchBatchRefreshesArtistList(t *testing.T) {
	m := New(context.Background(), WithLibraryTracks([]library.Track{
		{Artist: "Alice", Album: "Alpha", Title: "T1", Hash: "h1", Path: "/a.flac"},
		{Artist: "Bob", Album: "Beta", Title: "T2", Hash: "h2", Path: "/b.flac"},
	}))

	m = pressKey(m, "4")
	if len(m.browse.artists.artists) != 2 {
		t.Fatalf("expected 2 artists initially, got %d", len(m.browse.artists.artists))
	}

	next, _ := m.Update(watchBatchMsg{deleted: []string{"/b.flac"}})
	m = next.(Model)

	if len(m.browse.artists.artists) != 1 {
		t.Fatalf("expected 1 artist after watch batch, got %d", len(m.browse.artists.artists))
	}
	if m.browse.artists.artists[0] != "Alice" {
		t.Errorf("expected remaining artist='Alice', got %q", m.browse.artists.artists[0])
	}
}

// TestArtistsClampCursorAfterShrink verifies that the cursor is clamped when
// the artist list shrinks during a library update.
func TestArtistsClampCursorAfterShrink(t *testing.T) {
	m := New(context.Background(), WithLibraryTracks([]library.Track{
		{Artist: "Alice", Album: "Alpha", Title: "T1", Hash: "h1", Path: "/a.flac"},
		{Artist: "Bob", Album: "Beta", Title: "T2", Hash: "h2", Path: "/b.flac"},
		{Artist: "Charlie", Album: "Gamma", Title: "T3", Hash: "h3", Path: "/c.flac"},
	}))

	m = pressKey(m, "4")
	m = pressKey(m, "j")
	m = pressKey(m, "j")
	if m.browse.artists.cursor != 2 {
		t.Fatalf("expected cursor=2, got %d", m.browse.artists.cursor)
	}

	next, _ := m.Update(watchBatchMsg{deleted: []string{"/c.flac"}})
	m = next.(Model)

	if m.browse.artists.cursor != 1 {
		t.Errorf("expected cursor clamped to 1, got %d", m.browse.artists.cursor)
	}
}

// TestRescanClearsArtistsView verifies that actionRescan resets the artists
// view state so it gets re-derived from the new library.
func TestRescanClearsArtistsView(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")
	if len(m.browse.artists.artists) != 3 {
		t.Fatalf("expected 3 artists, got %d", len(m.browse.artists.artists))
	}

	m.libraryRoots = []string{"/tmp/music"}
	m.extractor = stubExtractor{}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m = next.(Model)

	if len(m.browse.artists.artists) != 0 {
		t.Errorf("expected empty artist list after rescan, got %d", len(m.browse.artists.artists))
	}
	if m.browse.artists.level != arttui.ArtistLevel {
		t.Errorf("expected ArtistLevel after rescan, got %v", m.browse.artists.level)
	}
}
