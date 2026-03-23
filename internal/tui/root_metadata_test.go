package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
)

// pressMKey sends an uppercase 'M' key to the model, triggering actionEditMetadata.
func pressMKey(m Model) Model {
	next, _ := m.Update(tea.KeyPressMsg{Code: 'M', Text: "M"})
	return next.(Model)
}

// typeIntoEditor sends a string of characters to the editor in m.overlay.metadataEditor
// and returns the updated Model.
func typeIntoEditor(m Model, text string) Model {
	for _, ch := range text {
		ed, _, _ := m.overlay.metadataEditor.HandleKey(tea.KeyPressMsg{Code: ch, Text: string(ch)})
		m.overlay.metadataEditor = &ed
	}
	return m
}

// backspaceInEditor sends n backspace presses to the focused editor field.
func backspaceInEditor(m Model, n int) Model {
	for range n {
		ed, _, _ := m.overlay.metadataEditor.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
		m.overlay.metadataEditor = &ed
	}
	return m
}

// --- TestRenameArtistFromArtistsView ---

// TestRenameArtistFromArtistsView verifies that pressing M at ArtistLevel opens
// an artist rename editor, and that saving updates the library index, refreshes
// the artist list, and repositions the cursor on the new name.
func TestRenameArtistFromArtistsView(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4") // switch to Artists view (ArtistLevel, cursor=0 → Alice)

	if m.artists.level != arttui.ArtistLevel {
		t.Fatalf("expected ArtistLevel, got %v", m.artists.level)
	}

	m = pressMKey(m)
	if m.overlay.metadataEditor == nil {
		t.Fatal("expected metadataEditor to be open after M at ArtistLevel")
	}
	ed := m.overlay.metadataEditor
	if ed.Mode() != editorModeRenameArtist {
		t.Errorf("expected editorModeRenameArtist, got %v", ed.Mode())
	}
	// Alice has 3 tracks: h1, h2, h3.
	if len(ed.Hashes()) != 3 {
		t.Errorf("expected 3 hashes for Alice, got %d", len(ed.Hashes()))
	}
	if ed.Artist() != "Alice" {
		t.Errorf("expected artist=%q, got %q", "Alice", ed.Artist())
	}

	// Erase "Alice" (5 chars) then type "Alicia".
	m = backspaceInEditor(m, 5)
	m = typeIntoEditor(m, "Alicia")

	result, _ := m.handleMetadataSave()
	m = result.(Model)

	if m.overlay.metadataEditor != nil {
		t.Error("expected metadataEditor to be closed after save")
	}

	// Library index should reflect the rename for all Alice tracks.
	for _, h := range []string{"h1", "h2", "h3"} {
		tr, ok := m.library.index.FindByHash(h)
		if !ok {
			t.Fatalf("track %s not found in library", h)
		}
		if tr.Artist != "Alicia" {
			t.Errorf("track %s: expected artist=%q, got %q", h, "Alicia", tr.Artist)
		}
	}

	// Artists list should be refreshed: "Alicia" present, "Alice" absent.
	found := false
	for _, a := range m.artists.artists {
		if a == "Alicia" {
			found = true
		}
		if a == "Alice" {
			t.Error("old name 'Alice' should no longer appear in artists list")
		}
	}
	if !found {
		t.Errorf("expected 'Alicia' in artists list; got: %v", m.artists.artists)
	}

	// Cursor should be positioned on "Alicia".
	if m.artists.cursor < 0 || m.artists.cursor >= len(m.artists.artists) {
		t.Fatalf("cursor out of bounds: %d (len=%d)", m.artists.cursor, len(m.artists.artists))
	}
	if m.artists.artists[m.artists.cursor] != "Alicia" {
		t.Errorf("expected cursor on 'Alicia', got %q", m.artists.artists[m.artists.cursor])
	}

	// Notification should mention renamed tracks.
	if !strings.Contains(m.statusBar.notification.msg, "Renamed") {
		t.Errorf("expected 'Renamed' notification, got %q", m.statusBar.notification.msg)
	}
}

// --- TestRenameAlbumFromArtistsView ---

// TestRenameAlbumFromArtistsView verifies that pressing M at AlbumLevel within
// the Artists view opens an album rename editor with the correct hashes.
func TestRenameAlbumFromArtistsView(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4") // Artists view, ArtistLevel
	m = pressEnter(m)    // Drill down to AlbumLevel for Alice

	if m.artists.level != arttui.AlbumLevel {
		t.Fatalf("expected AlbumLevel, got %v", m.artists.level)
	}
	// Cursor on Alice's first album: "Alpha".

	m = pressMKey(m)
	if m.overlay.metadataEditor == nil {
		t.Fatal("expected metadataEditor after M at AlbumLevel in artists view")
	}
	ed := m.overlay.metadataEditor
	if ed.Mode() != editorModeRenameAlbum {
		t.Errorf("expected editorModeRenameAlbum, got %v", ed.Mode())
	}
	if ed.Album() != "Alpha" {
		t.Errorf("expected album=%q, got %q", "Alpha", ed.Album())
	}
	// Alpha has 2 tracks: h1, h2.
	if len(ed.Hashes()) != 2 {
		t.Errorf("expected 2 hashes for Alpha, got %d", len(ed.Hashes()))
	}

	// Rename "Alpha" to "AlphaRenamed".
	m = backspaceInEditor(m, 5)
	m = typeIntoEditor(m, "AlphaRenamed")

	result, _ := m.handleMetadataSave()
	m = result.(Model)

	if m.overlay.metadataEditor != nil {
		t.Error("expected editor closed after save")
	}

	// Library tracks h1, h2 should have Album="AlphaRenamed".
	for _, h := range []string{"h1", "h2"} {
		tr, ok := m.library.index.FindByHash(h)
		if !ok {
			t.Fatalf("track %s not found", h)
		}
		if tr.Album != "AlphaRenamed" {
			t.Errorf("track %s: expected album=%q, got %q", h, "AlphaRenamed", tr.Album)
		}
	}

	// Albums list for Alice should be refreshed.
	found := false
	for _, a := range m.artists.albums {
		if a == "AlphaRenamed" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'AlphaRenamed' in artists.albums; got: %v", m.artists.albums)
	}

	// Cursor repositioned on "AlphaRenamed".
	if m.artists.cursor < 0 || m.artists.cursor >= len(m.artists.albums) {
		t.Fatalf("cursor out of bounds: %d (len=%d)", m.artists.cursor, len(m.artists.albums))
	}
	if m.artists.albums[m.artists.cursor] != "AlphaRenamed" {
		t.Errorf("expected cursor on 'AlphaRenamed', got %q", m.artists.albums[m.artists.cursor])
	}
}

// --- TestRenameAlbumFromAlbumsView ---

// TestRenameAlbumFromAlbumsView verifies that pressing M at AlbumLevel in the
// Albums view opens an album rename editor and refreshes albums.albums.
func TestRenameAlbumFromAlbumsView(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "5") // Albums view, AlbumLevel

	if m.activeView != viewAlbums {
		t.Fatalf("expected viewAlbums, got %d", m.activeView)
	}
	if m.albums.level != albtui.AlbumLevel {
		t.Fatalf("expected AlbumLevel, got %v", m.albums.level)
	}
	// First album is "Alpha" (index 0).

	m = pressMKey(m)
	if m.overlay.metadataEditor == nil {
		t.Fatal("expected metadataEditor after M at AlbumLevel in albums view")
	}
	ed := m.overlay.metadataEditor
	if ed.Mode() != editorModeRenameAlbum {
		t.Errorf("expected editorModeRenameAlbum, got %v", ed.Mode())
	}
	if ed.Album() != "Alpha" {
		t.Errorf("expected album=%q, got %q", "Alpha", ed.Album())
	}
	// Alpha has 2 tracks.
	if len(ed.Hashes()) != 2 {
		t.Errorf("expected 2 hashes for Alpha, got %d", len(ed.Hashes()))
	}

	// Rename "Alpha" to "AlphaPrime".
	m = backspaceInEditor(m, 5)
	m = typeIntoEditor(m, "AlphaPrime")

	result, _ := m.handleMetadataSave()
	m = result.(Model)

	// Library tracks updated.
	for _, h := range []string{"h1", "h2"} {
		tr, ok := m.library.index.FindByHash(h)
		if !ok {
			t.Fatalf("track %s not found", h)
		}
		if tr.Album != "AlphaPrime" {
			t.Errorf("track %s: expected album=%q, got %q", h, "AlphaPrime", tr.Album)
		}
	}

	// Albums list refreshed; "AlphaPrime" present, "Alpha" absent.
	found := false
	for _, a := range m.albums.albums {
		if a == "AlphaPrime" {
			found = true
		}
		if a == "Alpha" {
			t.Error("old name 'Alpha' should not remain in albums list")
		}
	}
	if !found {
		t.Errorf("expected 'AlphaPrime' in albums.albums; got: %v", m.albums.albums)
	}

	// Cursor on new name.
	if m.albums.cursor < 0 || m.albums.cursor >= len(m.albums.albums) {
		t.Fatalf("cursor out of bounds: %d (len=%d)", m.albums.cursor, len(m.albums.albums))
	}
	if m.albums.albums[m.albums.cursor] != "AlphaPrime" {
		t.Errorf("expected cursor on 'AlphaPrime', got %q", m.albums.albums[m.albums.cursor])
	}
}

// --- TestRenameUpdatesQueueAlbum ---

// TestRenameUpdatesQueueAlbum verifies that renaming an album also updates the
// Album field in matching queue entries (bug fix regression test).
func TestRenameUpdatesQueueAlbum(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "5") // Albums view

	// Pre-populate queue with entries including one from "Alpha".
	m.queue.entries = []QueueEntry{
		{Hash: "h1", Title: "Track A1", Artist: "Alice", Album: "Alpha", Display: "Alice – Track A1"},
		{Hash: "h4", Title: "Track G1", Artist: "Bob", Album: "Gamma", Display: "Bob – Track G1"},
	}

	// Open rename editor for "Alpha" (first album at cursor=0).
	m = pressMKey(m)
	if m.overlay.metadataEditor == nil {
		t.Fatal("expected metadataEditor")
	}

	m = backspaceInEditor(m, 5)
	m = typeIntoEditor(m, "Renamedalbum")

	result, _ := m.handleMetadataSave()
	m = result.(Model)

	// Queue entry h1 should have updated Album.
	found := false
	for _, e := range m.queue.entries {
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

	// Queue entry h4 (different album) should be unaffected.
	for _, e := range m.queue.entries {
		if e.Hash == "h4" && e.Album != "Gamma" {
			t.Errorf("queue entry h4: expected Album=%q (unchanged), got %q", "Gamma", e.Album)
		}
	}
}

// --- TestRenameUndo ---

// TestRenameUndo verifies that undoing a rename restores original names in the
// library index and refreshes the derived artists list.
func TestRenameUndo(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4") // Artists view, ArtistLevel

	m = pressMKey(m)
	if m.overlay.metadataEditor == nil {
		t.Fatal("expected editor open")
	}

	m = backspaceInEditor(m, 5) // erase "Alice"
	m = typeIntoEditor(m, "Renamed")

	result, _ := m.handleMetadataSave()
	m = result.(Model)

	// Verify rename was applied.
	tr, _ := m.library.index.FindByHash("h1")
	if tr.Artist != "Renamed" {
		t.Fatalf("expected artist='Renamed' after save, got %q", tr.Artist)
	}
	if m.undo.slot.action == nil {
		t.Fatal("expected undo slot to be populated after rename")
	}

	// Undo the rename.
	m = m.handleUndo()

	// Library should be restored to original "Alice".
	tr, ok := m.library.index.FindByHash("h1")
	if !ok {
		t.Fatal("track h1 not found after undo")
	}
	if tr.Artist != "Alice" {
		t.Errorf("after undo: expected artist='Alice', got %q", tr.Artist)
	}

	// Artists list should show "Alice" again; "Renamed" gone.
	foundAlice := false
	for _, a := range m.artists.artists {
		if a == "Alice" {
			foundAlice = true
		}
		if a == "Renamed" {
			t.Error("after undo: 'Renamed' should not appear in artists list")
		}
	}
	if !foundAlice {
		t.Errorf("after undo: expected 'Alice' in artists list; got: %v", m.artists.artists)
	}
}

// --- TestRenameNoOpWhenEditorHasNoChanges ---

// TestRenameNoOpWhenEditorHasNoChanges verifies that saving without typing
// anything results in a "No changes" notification.
func TestRenameNoOpWhenEditorHasNoChanges(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")
	m = pressMKey(m)
	if m.overlay.metadataEditor == nil {
		t.Fatal("expected editor open")
	}

	// Save without any edits.
	result, _ := m.handleMetadataSave()
	m = result.(Model)

	if m.overlay.metadataEditor != nil {
		t.Error("expected editor to close after save")
	}
	if !strings.Contains(m.statusBar.notification.msg, "No changes") {
		t.Errorf("expected 'No changes' notification, got %q", m.statusBar.notification.msg)
	}
}

// --- TestRenameNotOpenedAtTrackLevel ---

// TestRenameNotOpenedAtTrackLevel verifies that M at TrackLevel in the artists
// view opens the single-track editor (editorModeDefault), not a rename editor.
func TestRenameNotOpenedAtTrackLevel(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")
	m = pressEnter(m) // ArtistLevel → AlbumLevel (Alice)
	m = pressEnter(m) // AlbumLevel → TrackLevel (Alpha)

	if m.artists.level != arttui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.artists.level)
	}

	m = pressMKey(m)
	if m.overlay.metadataEditor == nil {
		t.Fatal("expected editor open at TrackLevel")
	}
	if m.overlay.metadataEditor.Mode() != editorModeDefault {
		t.Errorf("expected editorModeDefault at TrackLevel, got %v", m.overlay.metadataEditor.Mode())
	}
}

// --- TestRenameUnknownArtistSentinel ---

// TestRenameUnknownArtistSentinel verifies that tracks with an empty artist
// (shown as "(unknown artist)") are correctly gathered and included in the
// rename editor.
func TestRenameUnknownArtistSentinel(t *testing.T) {
	tracks := []library.Track{
		{Artist: "", Album: "Mystery", Title: "Unknown Track", Hash: "ux1", Path: "/ux1.flac"},
		{Artist: "", Album: "Mystery", Title: "Unknown Track 2", Hash: "ux2", Path: "/ux2.flac"},
		{Artist: "Known", Album: "KnownAlbum", Title: "Known Track", Hash: "kx1", Path: "/kx1.flac"},
	}
	m := New(context.Background(), WithLibraryTracks(tracks))
	m = pressKey(m, "4") // Artists view, ArtistLevel

	// "(unknown artist)" should appear in the list.
	unknownIdx := -1
	for i, a := range m.artists.artists {
		if a == arttui.UnknownArtist {
			unknownIdx = i
			break
		}
	}
	if unknownIdx == -1 {
		t.Fatalf("expected %q in artists list; got: %v", arttui.UnknownArtist, m.artists.artists)
	}

	// Navigate to the unknown artist.
	m.artists.cursor = unknownIdx

	m = pressMKey(m)
	if m.overlay.metadataEditor == nil {
		t.Fatal("expected editor open for unknown artist")
	}
	ed := m.overlay.metadataEditor
	if ed.Mode() != editorModeRenameArtist {
		t.Errorf("expected editorModeRenameArtist, got %v", ed.Mode())
	}
	// Both unknown tracks should be gathered.
	if len(ed.Hashes()) != 2 {
		t.Errorf("expected 2 hashes for unknown artist, got %d", len(ed.Hashes()))
	}
	// Editor displays the sentinel name.
	if ed.Artist() != arttui.UnknownArtist {
		t.Errorf("expected artist=%q, got %q", arttui.UnknownArtist, ed.Artist())
	}
}
