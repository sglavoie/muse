package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
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

	if len(m.browse.artists.artists) != 3 {
		t.Fatalf("precondition: expected 3 artists, got %d", len(m.browse.artists.artists))
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
	for _, a := range nm.browse.artists.artists {
		if a == "Zara" {
			foundZara = true
		}
	}
	if !foundZara {
		t.Errorf("expected 'Zara' in artists list after single-track save; got: %v", nm.browse.artists.artists)
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

// TestMetadataSavePreservesQueueSort verifies that saving a metadata edit
// recomputes the queue display order when a sort is active.
func TestMetadataSavePreservesQueueSort(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Zara", Title: "Song1"})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "Alice", Title: "Song2"})
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a.flac", Artist: "Zara", Title: "Song1"},
		{ID: 2, Hash: "h2", Path: "/b.flac", Artist: "Alice", Title: "Song2"},
	}
	// Sort queue by artist ascending: Alice, Zara → displayOrder = [1, 0]
	m.queue.sortState = SortState{Field: SortArtist, Dir: SortAsc}
	m.queue = m.queue.recomputeDisplayOrder()

	if len(m.queue.displayOrder) != 2 {
		t.Fatalf("precondition: expected displayOrder len 2, got %d", len(m.queue.displayOrder))
	}

	// Edit h1 artist from "Zara" to "Aaron" — should sort before "Alice"
	ed := NewMetadataEditor("h1", "Song1", "Aaron", "", "", 0)
	m.overlay.metadataEditor = &ed
	result, _ := m.handleMetadataSave()
	nm := result.(Model)

	if nm.queue.displayOrder == nil {
		t.Fatal("expected displayOrder to be non-nil after metadata save with active sort")
	}
	// After re-sort: Aaron(h1)=0, Alice(h2)=1 → displayOrder = [0, 1]
	if nm.queue.displayOrder[0] != 0 {
		t.Errorf("expected displayOrder[0]=0 (Aaron), got %d", nm.queue.displayOrder[0])
	}
}

// TestBulkMetadataSavePreservesQueueSort verifies that saving a bulk metadata
// edit recomputes the queue display order when a sort is active.
func TestBulkMetadataSavePreservesQueueSort(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Zara", Title: "Song1"})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "Bob", Title: "Song2"})
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a.flac", Artist: "Zara", Title: "Song1"},
		{ID: 2, Hash: "h2", Path: "/b.flac", Artist: "Bob", Title: "Song2"},
	}
	m.queue.sortState = SortState{Field: SortArtist, Dir: SortAsc}
	m.queue = m.queue.recomputeDisplayOrder()

	ed := NewBulkMetadataEditor([]string{"h1", "h2"}, "CommonTitle", "", "", "", "")
	m.overlay.metadataEditor = &ed
	result, _ := m.handleMetadataSave()
	nm := result.(Model)

	if nm.queue.displayOrder == nil {
		t.Fatal("expected displayOrder to be non-nil after bulk metadata save with active sort")
	}
}

// TestMetadataSavePreservesPlaylistSort verifies that saving a metadata edit
// recomputes playlist sort when at track level with a sort active.
func TestMetadataSavePreservesPlaylistSort(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Zara", Title: "T1"})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "Alice", Title: "T2"})
	m.playlists.level = pltui.TrackLevel
	m.playlists.tracks = []db.PlaylistTrackRow{
		{ID: 1, Hash: "h1", Path: "/a.flac", Artist: "Zara", Title: "T1"},
		{ID: 2, Hash: "h2", Path: "/b.flac", Artist: "Alice", Title: "T2"},
	}
	m.playlists.sortState = SortState{Field: SortArtist, Dir: SortAsc}
	m.playlists = m.playlists.recomputePlaylistSort()

	// Edit h1 artist — this goes through refreshDerivedViews which should
	// recompute the playlist sort.
	ed := NewMetadataEditor("h1", "T1", "Aaron", "", "", 0)
	m.overlay.metadataEditor = &ed
	result, _ := m.handleMetadataSave()
	nm := result.(Model)

	if nm.playlists.sortedTrackOrder == nil {
		t.Fatal("expected sortedTrackOrder to be non-nil after metadata save with active playlist sort")
	}
}

// TestMetadataEditedMsgErrorNotifiesUser verifies that a metadataEditedMsg
// with err != nil shows a notification instead of being silently dropped.
func TestMetadataEditedMsgErrorNotifiesUser(t *testing.T) {
	m := New(context.Background())
	msg := metadataEditedMsg{err: errors.New("disk full")}
	result, _ := m.Update(msg)
	nm := result.(Model)
	if !strings.Contains(nm.statusBar.topNotification().msg, "Metadata save failed") {
		t.Errorf("expected notification about metadata save failure, got %q", nm.statusBar.topNotification().msg)
	}
	if !strings.Contains(nm.statusBar.topNotification().msg, "disk full") {
		t.Errorf("expected error detail in notification, got %q", nm.statusBar.topNotification().msg)
	}
}

// TestMetadataEditedMsgSuccessNoNotification verifies that a metadataEditedMsg
// without an error does not produce a notification.
func TestMetadataEditedMsgSuccessNoNotification(t *testing.T) {
	m := New(context.Background())
	msg := metadataEditedMsg{err: nil}
	result, _ := m.Update(msg)
	nm := result.(Model)
	if nm.statusBar.topNotification().msg != "" {
		t.Errorf("expected no notification on success, got %q", nm.statusBar.topNotification().msg)
	}
}
