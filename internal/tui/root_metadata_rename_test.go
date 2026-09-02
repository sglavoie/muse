package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/metadata"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// TestRenameArtistFromArtistsView verifies that pressing M at ArtistLevel opens
// an artist rename editor, and that saving updates the library index, refreshes
// the artist list, and repositions the cursor on the new name.
func TestRenameArtistFromArtistsView(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4") // switch to Artists view (ArtistLevel, cursor=0 -> Alice)

	if m.browse.Artists.Level != arttui.ArtistLevel {
		t.Fatalf("expected ArtistLevel, got %v", m.browse.Artists.Level)
	}

	m = pressMKey(m)
	if m.overlay.metadataEditor == nil {
		t.Fatal("expected metadataEditor to be open after M at ArtistLevel")
	}
	ed := m.overlay.metadataEditor
	if ed.Mode() != metadata.ModeRenameArtist {
		t.Errorf("expected editorModeRenameArtist, got %v", ed.Mode())
	}
	if len(ed.Hashes()) != 3 {
		t.Errorf("expected 3 hashes for Alice, got %d", len(ed.Hashes()))
	}
	if ed.Artist() != "Alice" {
		t.Errorf("expected artist=%q, got %q", "Alice", ed.Artist())
	}

	m = backspaceInEditor(m, 5)
	m = typeIntoEditor(m, "Alicia")

	result, _ := m.handleMetadataSave()
	m = result.(Model)

	if m.overlay.metadataEditor != nil {
		t.Error("expected metadataEditor to be closed after save")
	}

	for _, h := range []string{"h1", "h2", "h3"} {
		tr, ok := m.library.Index.FindByHash(h)
		if !ok {
			t.Fatalf("track %s not found in library", h)
		}
		if tr.Artist != "Alicia" {
			t.Errorf("track %s: expected artist=%q, got %q", h, "Alicia", tr.Artist)
		}
	}

	found := false
	for _, a := range m.browse.Artists.Artists {
		if a == "Alicia" {
			found = true
		}
		if a == "Alice" {
			t.Error("old name 'Alice' should no longer appear in artists list")
		}
	}
	if !found {
		t.Errorf("expected 'Alicia' in artists list; got: %v", m.browse.Artists.Artists)
	}

	if m.browse.Artists.Cursor < 0 || m.browse.Artists.Cursor >= len(m.browse.Artists.Artists) {
		t.Fatalf("cursor out of bounds: %d (len=%d)", m.browse.Artists.Cursor, len(m.browse.Artists.Artists))
	}
	if m.browse.Artists.Artists[m.browse.Artists.Cursor] != "Alicia" {
		t.Errorf("expected cursor on 'Alicia', got %q", m.browse.Artists.Artists[m.browse.Artists.Cursor])
	}

	if !strings.Contains(m.statusBar.TopNotification().Msg, "Renamed") {
		t.Errorf("expected 'Renamed' notification, got %q", m.statusBar.TopNotification().Msg)
	}
}

// TestRenameAlbumFromArtistsView verifies that pressing M at AlbumLevel within
// the Artists view opens an album rename editor with the correct hashes.
func TestRenameAlbumFromArtistsView(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")
	m = pressEnter(m)

	if m.browse.Artists.Level != arttui.AlbumLevel {
		t.Fatalf("expected AlbumLevel, got %v", m.browse.Artists.Level)
	}

	m = pressMKey(m)
	if m.overlay.metadataEditor == nil {
		t.Fatal("expected metadataEditor after M at AlbumLevel in artists view")
	}
	ed := m.overlay.metadataEditor
	if ed.Mode() != metadata.ModeRenameAlbum {
		t.Errorf("expected editorModeRenameAlbum, got %v", ed.Mode())
	}
	if ed.Album() != "Alpha" {
		t.Errorf("expected album=%q, got %q", "Alpha", ed.Album())
	}
	if len(ed.Hashes()) != 2 {
		t.Errorf("expected 2 hashes for Alpha, got %d", len(ed.Hashes()))
	}

	m = backspaceInEditor(m, 5)
	m = typeIntoEditor(m, "AlphaRenamed")

	result, _ := m.handleMetadataSave()
	m = result.(Model)

	if m.overlay.metadataEditor != nil {
		t.Error("expected editor closed after save")
	}

	for _, h := range []string{"h1", "h2"} {
		tr, ok := m.library.Index.FindByHash(h)
		if !ok {
			t.Fatalf("track %s not found", h)
		}
		if tr.Album != "AlphaRenamed" {
			t.Errorf("track %s: expected album=%q, got %q", h, "AlphaRenamed", tr.Album)
		}
	}

	found := false
	for _, a := range m.browse.Artists.Albums {
		if a == "AlphaRenamed" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'AlphaRenamed' in artists.albums; got: %v", m.browse.Artists.Albums)
	}

	if m.browse.Artists.Cursor < 0 || m.browse.Artists.Cursor >= len(m.browse.Artists.Albums) {
		t.Fatalf("cursor out of bounds: %d (len=%d)", m.browse.Artists.Cursor, len(m.browse.Artists.Albums))
	}
	if m.browse.Artists.Albums[m.browse.Artists.Cursor] != "AlphaRenamed" {
		t.Errorf("expected cursor on 'AlphaRenamed', got %q", m.browse.Artists.Albums[m.browse.Artists.Cursor])
	}
}

// TestRenameAlbumFromAlbumsView verifies that pressing M at AlbumLevel in the
// Albums view opens an album rename editor and refreshes albums.albums.
func TestRenameAlbumFromAlbumsView(t *testing.T) {
	m := newModelWithTracks()
	m = switchToAlbumsMode(m)

	if m.activeView != viewBrowse {
		t.Fatalf("expected viewBrowse, got %d", m.activeView)
	}
	if m.browse.Albums.Level != albtui.AlbumLevel {
		t.Fatalf("expected AlbumLevel, got %v", m.browse.Albums.Level)
	}

	m = pressMKey(m)
	if m.overlay.metadataEditor == nil {
		t.Fatal("expected metadataEditor after M at AlbumLevel in albums view")
	}
	ed := m.overlay.metadataEditor
	if ed.Mode() != metadata.ModeRenameAlbum {
		t.Errorf("expected editorModeRenameAlbum, got %v", ed.Mode())
	}
	if ed.Album() != "Alpha" {
		t.Errorf("expected album=%q, got %q", "Alpha", ed.Album())
	}
	if len(ed.Hashes()) != 2 {
		t.Errorf("expected 2 hashes for Alpha, got %d", len(ed.Hashes()))
	}

	m = backspaceInEditor(m, 5)
	m = typeIntoEditor(m, "AlphaPrime")

	result, _ := m.handleMetadataSave()
	m = result.(Model)

	for _, h := range []string{"h1", "h2"} {
		tr, ok := m.library.Index.FindByHash(h)
		if !ok {
			t.Fatalf("track %s not found", h)
		}
		if tr.Album != "AlphaPrime" {
			t.Errorf("track %s: expected album=%q, got %q", h, "AlphaPrime", tr.Album)
		}
	}

	found := false
	for _, a := range m.browse.Albums.Albums {
		if a == "AlphaPrime" {
			found = true
		}
		if a == "Alpha" {
			t.Error("old name 'Alpha' should not remain in albums list")
		}
	}
	if !found {
		t.Errorf("expected 'AlphaPrime' in albums.albums; got: %v", m.browse.Albums.Albums)
	}

	if m.browse.Albums.Cursor < 0 || m.browse.Albums.Cursor >= len(m.browse.Albums.Albums) {
		t.Fatalf("cursor out of bounds: %d (len=%d)", m.browse.Albums.Cursor, len(m.browse.Albums.Albums))
	}
	if m.browse.Albums.Albums[m.browse.Albums.Cursor] != "AlphaPrime" {
		t.Errorf("expected cursor on 'AlphaPrime', got %q", m.browse.Albums.Albums[m.browse.Albums.Cursor])
	}
}

// TestRenameUpdatesQueueAlbum verifies that renaming an album also updates the
// Album field in matching queue entries.
func TestRenameUpdatesQueueAlbum(t *testing.T) {
	m := newModelWithTracks()
	m = switchToAlbumsMode(m)

	m.queue.Entries = []queue.Entry{
		{Hash: "h1", Title: "Track A1", Artist: "Alice", Album: "Alpha", Display: "Alice - Track A1"},
		{Hash: "h4", Title: "Track G1", Artist: "Bob", Album: "Gamma", Display: "Bob - Track G1"},
	}

	m = pressMKey(m)
	if m.overlay.metadataEditor == nil {
		t.Fatal("expected metadataEditor")
	}

	m = backspaceInEditor(m, 5)
	m = typeIntoEditor(m, "Renamedalbum")

	result, _ := m.handleMetadataSave()
	m = result.(Model)

	found := false
	for _, e := range m.queue.Entries {
		if e.Hash == "h1" {
			found = true
			if e.Album != "Renamedalbum" {
				t.Errorf("queue entry h1: expected Album=%q, got %q", "Renamedalbum", e.Album)
			}
		}
	}
	if !found {
		t.Error("queue entry for h1 not found")
	}

	for _, e := range m.queue.Entries {
		if e.Hash == "h4" && e.Album != "Gamma" {
			t.Errorf("queue entry h4: expected Album=%q (unchanged), got %q", "Gamma", e.Album)
		}
	}
}

// TestSingleTrackMetadataSaveUpdatesQueueAlbum verifies that saving a
// single-track metadata edit updates the Album field on matching queue entries.
func TestSingleTrackMetadataSaveUpdatesQueueAlbum(t *testing.T) {
	m := newModelWithTracks()
	m.activeView = viewQueue
	m.queue.Entries = []queue.Entry{
		{Hash: "h1", Title: "Track A1", Artist: "Alice", Album: "Alpha", Display: "Alice - Track A1"},
		{Hash: "h4", Title: "Track G1", Artist: "Bob", Album: "Gamma", Display: "Bob - Track G1"},
	}
	m.queue.Cursor = 0

	ed := metadata.NewEditor("h1", "Track A1", "Alice", "Alpha", "", 0)
	m.overlay.metadataEditor = &ed

	ed2, _, _ := m.overlay.metadataEditor.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	m.overlay.metadataEditor = &ed2
	ed3, _, _ := m.overlay.metadataEditor.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	m.overlay.metadataEditor = &ed3
	m = backspaceInEditor(m, 5)
	m = typeIntoEditor(m, "NewAlbum")

	result, _ := m.handleMetadataSave()
	nm := result.(Model)

	if nm.queue.Entries[0].Album != "NewAlbum" {
		t.Errorf("queue entry h1: expected Album=%q, got %q", "NewAlbum", nm.queue.Entries[0].Album)
	}
	if nm.queue.Entries[1].Album != "Gamma" {
		t.Errorf("queue entry h4: expected Album=%q (unchanged), got %q", "Gamma", nm.queue.Entries[1].Album)
	}
}

// TestRenameUndo verifies that undoing a rename restores original names in the
// library index and refreshes the derived artists list.
func TestRenameUndo(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")

	m = pressMKey(m)
	if m.overlay.metadataEditor == nil {
		t.Fatal("expected editor open")
	}

	m = backspaceInEditor(m, 5)
	m = typeIntoEditor(m, "Renamed")

	result, _ := m.handleMetadataSave()
	m = result.(Model)

	tr, _ := m.library.Index.FindByHash("h1")
	if tr.Artist != "Renamed" {
		t.Fatalf("expected artist='Renamed' after save, got %q", tr.Artist)
	}
	if m.undo.slot.action == nil {
		t.Fatal("expected undo slot to be populated after rename")
	}

	m = m.handleUndo()

	tr, ok := m.library.Index.FindByHash("h1")
	if !ok {
		t.Fatal("track h1 not found after undo")
	}
	if tr.Artist != "Alice" {
		t.Errorf("after undo: expected artist='Alice', got %q", tr.Artist)
	}

	foundAlice := false
	for _, a := range m.browse.Artists.Artists {
		if a == "Alice" {
			foundAlice = true
		}
		if a == "Renamed" {
			t.Error("after undo: 'Renamed' should not appear in artists list")
		}
	}
	if !foundAlice {
		t.Errorf("after undo: expected 'Alice' in artists list; got: %v", m.browse.Artists.Artists)
	}
}

// TestRenameNoOpWhenEditorHasNoChanges verifies that saving without typing
// anything results in a "No changes" notification.
func TestRenameNoOpWhenEditorHasNoChanges(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")
	m = pressMKey(m)
	if m.overlay.metadataEditor == nil {
		t.Fatal("expected editor open")
	}

	result, _ := m.handleMetadataSave()
	m = result.(Model)

	if m.overlay.metadataEditor != nil {
		t.Error("expected editor to close after save")
	}
	if !strings.Contains(m.statusBar.TopNotification().Msg, "No changes") {
		t.Errorf("expected 'No changes' notification, got %q", m.statusBar.TopNotification().Msg)
	}
}

// TestRenameNotOpenedAtTrackLevel verifies that M at TrackLevel in the artists
// view opens the single-track editor, not a rename editor.
func TestRenameNotOpenedAtTrackLevel(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")
	m = pressEnter(m)
	m = pressEnter(m)

	if m.browse.Artists.Level != arttui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.browse.Artists.Level)
	}

	m = pressMKey(m)
	if m.overlay.metadataEditor == nil {
		t.Fatal("expected editor open at TrackLevel")
	}
	if m.overlay.metadataEditor.Mode() != metadata.ModeDefault {
		t.Errorf("expected editorModeDefault at TrackLevel, got %v", m.overlay.metadataEditor.Mode())
	}
}

// TestRenameUnknownArtistSentinel verifies that tracks with an empty artist
// are correctly gathered and included in the rename editor.
func TestRenameUnknownArtistSentinel(t *testing.T) {
	tracks := []library.Track{
		{Artist: "", Album: "Mystery", Title: "Unknown Track", Hash: "ux1", Path: "/ux1.flac"},
		{Artist: "", Album: "Mystery", Title: "Unknown Track 2", Hash: "ux2", Path: "/ux2.flac"},
		{Artist: "Known", Album: "KnownAlbum", Title: "Known Track", Hash: "kx1", Path: "/kx1.flac"},
	}
	m := New(context.Background(), WithLibraryTracks(tracks))
	m = pressKey(m, "4")

	unknownIdx := -1
	for i, a := range m.browse.Artists.Artists {
		if a == arttui.UnknownArtist {
			unknownIdx = i
			break
		}
	}
	if unknownIdx == -1 {
		t.Fatalf("expected %q in artists list; got: %v", arttui.UnknownArtist, m.browse.Artists.Artists)
	}

	m.browse.Artists.Cursor = unknownIdx

	m = pressMKey(m)
	if m.overlay.metadataEditor == nil {
		t.Fatal("expected editor open for unknown artist")
	}
	ed := m.overlay.metadataEditor
	if ed.Mode() != metadata.ModeRenameArtist {
		t.Errorf("expected editorModeRenameArtist, got %v", ed.Mode())
	}
	if len(ed.Hashes()) != 2 {
		t.Errorf("expected 2 hashes for unknown artist, got %d", len(ed.Hashes()))
	}
	if ed.Artist() != arttui.UnknownArtist {
		t.Errorf("expected artist=%q, got %q", arttui.UnknownArtist, ed.Artist())
	}
}
