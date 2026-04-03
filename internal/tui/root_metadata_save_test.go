package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
)

// TestMetadataSaveUsesTrackDisplayForQueueEntryDisplay verifies that saving
// a metadata edit uses the same display format as trackDisplay for queue
// entries, handling missing artist/title gracefully.
func TestMetadataSaveUsesTrackDisplayForQueueEntryDisplay(t *testing.T) {
	tests := []struct {
		name          string
		artist, title string
		wantDisplay   string
	}{
		{"both", "Alice", "Song", "Alice – Song"},
		{"only title", "", "Song", "Song"},
		{"only artist", "Alice", "", "Alice"},
		{"neither", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(context.Background())
			m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Old", Title: "Old", Album: "Old"})
			m.queue.entries = []QueueEntry{
				{ID: 1, Hash: "h1", Path: "/a.flac", Display: "Old – Old", Artist: "Old", Title: "Old", Album: "Old"},
			}
			ed := NewMetadataEditor("h1", tt.title, tt.artist, "Album", "", 0)
			m.overlay.metadataEditor = &ed
			result, _ := m.handleMetadataSave()
			nm := result.(Model)
			if nm.queue.entries[0].Display != tt.wantDisplay {
				t.Errorf("expected Display=%q, got %q", tt.wantDisplay, nm.queue.entries[0].Display)
			}
		})
	}
}

// TestSingleTrackMetadataSaveRefreshesDerivedViews verifies that saving a
// single-track metadata edit refreshes the Artists and Albums views.
func TestSingleTrackMetadataSaveRefreshesDerivedViews(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")
	m = pressKey(m, "1")

	if len(m.artists.artists) != 3 {
		t.Fatalf("precondition: expected 3 artists, got %d", len(m.artists.artists))
	}

	ed := NewMetadataEditor("h1", "Track A1", "Alice", "Alpha", "", 0)
	m.overlay.metadataEditor = &ed

	ed2, _, _ := m.overlay.metadataEditor.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	m.overlay.metadataEditor = &ed2
	m = backspaceInEditor(m, 5)
	m = typeIntoEditor(m, "Zara")

	result, _ := m.handleMetadataSave()
	nm := result.(Model)

	tr, ok := nm.library.index.FindByHash("h1")
	if !ok {
		t.Fatal("track h1 not found in library after save")
	}
	if tr.Artist != "Zara" {
		t.Errorf("expected artist=%q, got %q", "Zara", tr.Artist)
	}

	foundZara := false
	for _, a := range nm.artists.artists {
		if a == "Zara" {
			foundZara = true
		}
	}
	if !foundZara {
		t.Errorf("expected 'Zara' in artists list after single-track save; got: %v", nm.artists.artists)
	}
}

// TestSingleTrackMetadataSaveCreatesUndoSlot verifies that saving a
// single-track metadata edit populates the undo slot.
func TestSingleTrackMetadataSaveCreatesUndoSlot(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Old Artist", Title: "Old Title", Album: "Old Album"})
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a.flac", Display: "Old Artist – Old Title", Artist: "Old Artist", Title: "Old Title", Album: "Old Album"},
	}

	ed := NewMetadataEditor("h1", "New Title", "New Artist", "New Album", "", 0)
	m.overlay.metadataEditor = &ed

	result, _ := m.handleMetadataSave()
	nm := result.(Model)

	if nm.undo.slot.action == nil {
		t.Fatal("expected undo slot to be populated after single-track metadata save")
	}
	if nm.undo.slot.action.kind() != undoKindBulkEditMetadata {
		t.Errorf("expected undo kind %d, got %d", undoKindBulkEditMetadata, nm.undo.slot.action.kind())
	}

	nm = nm.handleUndo()

	tr, ok := nm.library.index.FindByHash("h1")
	if !ok {
		t.Fatal("track h1 not found after undo")
	}
	if tr.Title != "Old Title" {
		t.Errorf("after undo: expected title=%q, got %q", "Old Title", tr.Title)
	}
	if tr.Artist != "Old Artist" {
		t.Errorf("after undo: expected artist=%q, got %q", "Old Artist", tr.Artist)
	}
	if tr.Album != "Old Album" {
		t.Errorf("after undo: expected album=%q, got %q", "Old Album", tr.Album)
	}
	if nm.queue.entries[0].Title != "Old Title" {
		t.Errorf("after undo: queue title=%q, want %q", nm.queue.entries[0].Title, "Old Title")
	}
}
