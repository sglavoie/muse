package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
)

// TestMetadataEditUpdatesStatusBarForCurrentTrack verifies that editing
// the metadata of the currently-playing track immediately updates the
// status bar artist and title.
func TestMetadataEditUpdatesStatusBarForCurrentTrack(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "playing", Path: "/p.flac", Artist: "OldArt", Title: "OldTitle", Album: "A"})
	m.playback.currentHash = "playing"
	m.statusBar.artist = "OldArt"
	m.statusBar.title = "OldTitle"

	ed := NewMetadataEditor("playing", "NewTitle", "NewArt", "A", "", 0)
	m.overlay.metadataEditor = &ed

	result, _ := m.handleMetadataSave()
	nm := result.(Model)

	if nm.statusBar.artist != "NewArt" {
		t.Errorf("status bar artist = %q, want %q", nm.statusBar.artist, "NewArt")
	}
	if nm.statusBar.title != "NewTitle" {
		t.Errorf("status bar title = %q, want %q", nm.statusBar.title, "NewTitle")
	}
}

// TestBulkMetadataEditUpdatesStatusBarForCurrentTrack verifies that a
// bulk metadata edit also updates the status bar when one of the edited
// tracks is the currently-playing track.
func TestBulkMetadataEditUpdatesStatusBarForCurrentTrack(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "OldA", Title: "OldT", Album: "A"})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "OldA", Title: "OldT", Album: "A"})
	m.playback.currentHash = "h2"
	m.statusBar.artist = "OldA"
	m.statusBar.title = "OldT"

	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Artist: "OldA", Title: "OldT", Selected: true},
		{ID: 2, Hash: "h2", Artist: "OldA", Title: "OldT", Selected: true},
	}
	m.activeView = viewQueue

	ed := NewBulkMetadataEditor([]string{"h1", "h2"}, "OldT", "OldA", "A", "", "")
	ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: 'X', Text: "X"})
	m.overlay.metadataEditor = &ed

	result, _ := m.handleMetadataSave()
	nm := result.(Model)

	if nm.statusBar.artist != "OldA" {
		t.Errorf("status bar artist = %q, want %q (unchanged)", nm.statusBar.artist, "OldA")
	}
	if nm.statusBar.title != "OldTX" {
		t.Errorf("status bar title = %q, want %q", nm.statusBar.title, "OldTX")
	}
}

// TestMetadataUndoUpdatesStatusBarForCurrentTrack verifies that undoing
// a metadata edit restores the status bar to the original values.
func TestMetadataUndoUpdatesStatusBarForCurrentTrack(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "playing", Path: "/p.flac", Artist: "OrigArt", Title: "OrigTitle", Album: "A"})
	m.playback.currentHash = "playing"
	m.statusBar.artist = "OrigArt"
	m.statusBar.title = "OrigTitle"

	ed := NewMetadataEditor("playing", "EditedTitle", "EditedArt", "A", "", 0)
	m.overlay.metadataEditor = &ed

	result, _ := m.handleMetadataSave()
	nm := result.(Model)

	if nm.statusBar.artist != "EditedArt" {
		t.Fatalf("after save: status bar artist = %q, want %q", nm.statusBar.artist, "EditedArt")
	}

	nm = nm.handleUndo()

	if nm.statusBar.artist != "OrigArt" {
		t.Errorf("after undo: status bar artist = %q, want %q", nm.statusBar.artist, "OrigArt")
	}
	if nm.statusBar.title != "OrigTitle" {
		t.Errorf("after undo: status bar title = %q, want %q", nm.statusBar.title, "OrigTitle")
	}
}

// TestMetadataEditDoesNotAffectStatusBarForOtherTrack verifies that editing
// a track that is not the currently-playing one does not change the status bar.
func TestMetadataEditDoesNotAffectStatusBarForOtherTrack(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "other", Path: "/o.flac", Artist: "X", Title: "Y", Album: "A"})
	m.playback.currentHash = "playing"
	m.statusBar.artist = "NowArt"
	m.statusBar.title = "NowTitle"

	ed := NewMetadataEditor("other", "NewY", "NewX", "A", "", 0)
	m.overlay.metadataEditor = &ed

	result, _ := m.handleMetadataSave()
	nm := result.(Model)

	if nm.statusBar.artist != "NowArt" {
		t.Errorf("status bar artist changed to %q, should remain %q", nm.statusBar.artist, "NowArt")
	}
	if nm.statusBar.title != "NowTitle" {
		t.Errorf("status bar title changed to %q, should remain %q", nm.statusBar.title, "NowTitle")
	}
}
