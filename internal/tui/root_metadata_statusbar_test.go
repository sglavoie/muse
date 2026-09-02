package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/metadata"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// TestMetadataEditUpdatesStatusBarForCurrentTrack verifies that editing
// the metadata of the currently-playing track immediately updates the
// status bar artist and title.
func TestMetadataEditUpdatesStatusBarForCurrentTrack(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "playing", Path: "/p.flac", Artist: "OldArt", Title: "OldTitle", Album: "A"})
	m.playback.currentHash = "playing"
	m.statusBar.Artist = "OldArt"
	m.statusBar.Title = "OldTitle"

	ed := metadata.NewEditor("playing", "NewTitle", "NewArt", "A", "", 0)
	m.overlay.metadataEditor = &ed

	result, _ := m.handleMetadataSave()
	nm := result.(Model)

	if nm.statusBar.Artist != "NewArt" {
		t.Errorf("status bar artist = %q, want %q", nm.statusBar.Artist, "NewArt")
	}
	if nm.statusBar.Title != "NewTitle" {
		t.Errorf("status bar title = %q, want %q", nm.statusBar.Title, "NewTitle")
	}
}

// TestBulkMetadataEditUpdatesStatusBarForCurrentTrack verifies that a
// bulk metadata edit also updates the status bar when one of the edited
// tracks is the currently-playing track.
func TestBulkMetadataEditUpdatesStatusBarForCurrentTrack(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "OldA", Title: "OldT", Album: "A"})
	m.library.Index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "OldA", Title: "OldT", Album: "A"})
	m.playback.currentHash = "h2"
	m.statusBar.Artist = "OldA"
	m.statusBar.Title = "OldT"

	m.queue.Entries = []queue.Entry{
		{ID: 1, Hash: "h1", Artist: "OldA", Title: "OldT", Selected: true},
		{ID: 2, Hash: "h2", Artist: "OldA", Title: "OldT", Selected: true},
	}
	m.activeView = viewQueue

	ed := metadata.NewBulkEditor([]string{"h1", "h2"}, "OldT", "OldA", "A", "", "")
	ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: 'X', Text: "X"})
	m.overlay.metadataEditor = &ed

	result, _ := m.handleMetadataSave()
	nm := result.(Model)

	if nm.statusBar.Artist != "OldA" {
		t.Errorf("status bar artist = %q, want %q (unchanged)", nm.statusBar.Artist, "OldA")
	}
	if nm.statusBar.Title != "OldTX" {
		t.Errorf("status bar title = %q, want %q", nm.statusBar.Title, "OldTX")
	}
}

// TestMetadataUndoUpdatesStatusBarForCurrentTrack verifies that undoing
// a metadata edit restores the status bar to the original values.
func TestMetadataUndoUpdatesStatusBarForCurrentTrack(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "playing", Path: "/p.flac", Artist: "OrigArt", Title: "OrigTitle", Album: "A"})
	m.playback.currentHash = "playing"
	m.statusBar.Artist = "OrigArt"
	m.statusBar.Title = "OrigTitle"

	ed := metadata.NewEditor("playing", "EditedTitle", "EditedArt", "A", "", 0)
	m.overlay.metadataEditor = &ed

	result, _ := m.handleMetadataSave()
	nm := result.(Model)

	if nm.statusBar.Artist != "EditedArt" {
		t.Fatalf("after save: status bar artist = %q, want %q", nm.statusBar.Artist, "EditedArt")
	}

	nm = nm.handleUndo()

	if nm.statusBar.Artist != "OrigArt" {
		t.Errorf("after undo: status bar artist = %q, want %q", nm.statusBar.Artist, "OrigArt")
	}
	if nm.statusBar.Title != "OrigTitle" {
		t.Errorf("after undo: status bar title = %q, want %q", nm.statusBar.Title, "OrigTitle")
	}
}

// TestMetadataEditDoesNotAffectStatusBarForOtherTrack verifies that editing
// a track that is not the currently-playing one does not change the status bar.
func TestMetadataEditDoesNotAffectStatusBarForOtherTrack(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "other", Path: "/o.flac", Artist: "X", Title: "Y", Album: "A"})
	m.playback.currentHash = "playing"
	m.statusBar.Artist = "NowArt"
	m.statusBar.Title = "NowTitle"

	ed := metadata.NewEditor("other", "NewY", "NewX", "A", "", 0)
	m.overlay.metadataEditor = &ed

	result, _ := m.handleMetadataSave()
	nm := result.(Model)

	if nm.statusBar.Artist != "NowArt" {
		t.Errorf("status bar artist changed to %q, should remain %q", nm.statusBar.Artist, "NowArt")
	}
	if nm.statusBar.Title != "NowTitle" {
		t.Errorf("status bar title changed to %q, should remain %q", nm.statusBar.Title, "NowTitle")
	}
}
