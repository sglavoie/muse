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

// --- TestSingleTrackMetadataSaveUpdatesQueueAlbum ---

// TestSingleTrackMetadataSaveUpdatesQueueAlbum verifies that saving a
// single-track metadata edit updates the Album field (not just Title and
// Artist) on matching queue entries.
func TestSingleTrackMetadataSaveUpdatesQueueAlbum(t *testing.T) {
	m := newModelWithTracks()
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Hash: "h1", Title: "Track A1", Artist: "Alice", Album: "Alpha", Display: "Alice – Track A1"},
		{Hash: "h4", Title: "Track G1", Artist: "Bob", Album: "Gamma", Display: "Bob – Track G1"},
	}
	m.queue.cursor = 0

	// Open single-track metadata editor for h1.
	ed := NewMetadataEditor("h1", "Track A1", "Alice", "Alpha")
	m.overlay.metadataEditor = &ed

	// Clear the album field and type a new one.
	// Switch focus to album field (tab twice: title -> artist -> album).
	ed2, _, _ := m.overlay.metadataEditor.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	m.overlay.metadataEditor = &ed2
	ed3, _, _ := m.overlay.metadataEditor.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	m.overlay.metadataEditor = &ed3
	// Erase "Alpha" (5 chars) and type new album name.
	m = backspaceInEditor(m, 5)
	m = typeIntoEditor(m, "NewAlbum")

	result, _ := m.handleMetadataSave()
	nm := result.(Model)

	// Queue entry h1 should have Album updated to "NewAlbum".
	if nm.queue.entries[0].Album != "NewAlbum" {
		t.Errorf("queue entry h1: expected Album=%q, got %q", "NewAlbum", nm.queue.entries[0].Album)
	}
	// Queue entry h4 should be unaffected.
	if nm.queue.entries[1].Album != "Gamma" {
		t.Errorf("queue entry h4: expected Album=%q (unchanged), got %q", "Gamma", nm.queue.entries[1].Album)
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

// --- TestBulkMetadataEditFromArtistsTrackLevel ---

// TestBulkMetadataEditFromArtistsTrackLevel verifies that bulkSelectedHashes
// returns hashes from artists.selectedHashes when the Artists view is active.
func TestBulkMetadataEditFromArtistsTrackLevel(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4") // Artists view
	m = pressEnter(m)    // AlbumLevel (Alice)
	m = pressEnter(m)    // TrackLevel (Alpha)

	if m.artists.level != arttui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.artists.level)
	}

	// Select h1 and h2 directly.
	m.artists.selectedHashes = map[string]struct{}{"h1": {}, "h2": {}}

	hashes := m.bulkSelectedHashes()
	if len(hashes) != 2 {
		t.Fatalf("bulkSelectedHashes: expected 2, got %d", len(hashes))
	}

	// Press M — should open bulk editor (2 hashes selected).
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

	// Make a change so the editor doesn't early-return "No changes".
	m = typeIntoEditor(m, "X")

	result, _ := m.handleMetadataSave()
	m = result.(Model)

	if len(m.artists.selectedHashes) != 0 {
		t.Errorf("after bulk save: expected selectedHashes cleared, got %d", len(m.artists.selectedHashes))
	}
}

// --- TestBulkMetadataEditFromAlbumsTrackLevel ---

// TestBulkMetadataEditFromAlbumsTrackLevel verifies that bulkSelectedHashes
// returns hashes from albums.selectedHashes when the Albums view is active.
func TestBulkMetadataEditFromAlbumsTrackLevel(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "5") // Albums view
	m = pressEnter(m)    // TrackLevel (Alpha)

	// Select h1 and h2 directly.
	m.albums.selectedHashes = map[string]struct{}{"h1": {}, "h2": {}}

	hashes := m.bulkSelectedHashes()
	if len(hashes) != 2 {
		t.Fatalf("bulkSelectedHashes (albums): expected 2, got %d", len(hashes))
	}

	// Press M — should open bulk editor.
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

	// Make a change so the editor doesn't early-return "No changes".
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

// TestMetadataSaveUsesTrackDisplayForQueueEntryDisplay verifies that saving
// a metadata edit uses the same display format as trackDisplay for queue
// entries, handling missing artist/title gracefully instead of producing
// a bare " - " separator.
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
			// Open the editor for the track.
			ed := NewMetadataEditor("h1", tt.title, tt.artist, "Album")
			m.overlay.metadataEditor = &ed
			result, _ := m.handleMetadataSave()
			nm := result.(Model)
			if nm.queue.entries[0].Display != tt.wantDisplay {
				t.Errorf("expected Display=%q, got %q", tt.wantDisplay, nm.queue.entries[0].Display)
			}
		})
	}
}

// TestBulkMetadataSaveUsesTrackDisplayForQueueEntryDisplay verifies that bulk
// metadata save also uses the trackDisplay format for queue entry displays.
func TestBulkMetadataSaveUsesTrackDisplayForQueueEntryDisplay(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Old", Title: "Old", Album: "A"})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "Old", Title: "Old", Album: "A"})
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a.flac", Display: "Old – Old", Artist: "Old", Title: "Old"},
		{ID: 2, Hash: "h2", Path: "/b.flac", Display: "Old – Old", Artist: "Old", Title: "Old"},
	}
	m.activeView = viewQueue
	m.queue.entries[0].Selected = true
	m.queue.entries[1].Selected = true

	// Open a bulk editor that clears the artist field.
	ed := NewBulkMetadataEditor([]string{"h1", "h2"}, "Old", "Old", "A")
	// Navigate to artist field, clear it.
	ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	for range 3 {
		ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	m.overlay.metadataEditor = &ed

	result, _ := m.handleMetadataSave()
	nm := result.(Model)
	// With empty artist, display should fall back to just the title.
	for _, e := range nm.queue.entries {
		if e.Display != "Old" {
			t.Errorf("expected Display=%q (title only), got %q", "Old", e.Display)
		}
	}
}
