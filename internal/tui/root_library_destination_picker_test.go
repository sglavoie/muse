package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
)

// TestRootLibraryAOpensDestinationPickerNoSelection verifies that pressing 'a'
// in the library view with no bulk selection opens the destination picker modal
// for the focused track.
func TestRootLibraryAOpensDestinationPickerNoSelection(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "A", Title: "One", Hash: "hash-one"})

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	nm := next.(Model)

	if cmd != nil {
		t.Fatalf("'a' with no selection: expected nil cmd (modal open is synchronous), got %T", cmd)
	}
	if nm.overlay.destPicker == nil {
		t.Fatal("'a' with no selection: expected destPicker to be non-nil (modal open)")
	}
	if len(nm.overlay.destPickerTracks) != 1 || nm.overlay.destPickerTracks[0].Hash != "hash-one" {
		t.Errorf("'a' with no selection: expected destPickerTracks to contain focused track, got %v", nm.overlay.destPickerTracks)
	}
}

// TestRootLibraryAOpensDestinationPickerWithSelection verifies that pressing
// 'a' while tracks are bulk-selected opens the destination picker modal for
// all selected tracks.
func TestRootLibraryAOpensDestinationPickerWithSelection(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "A", Title: "One", Hash: "hash-one"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Two", Hash: "hash-two"})
	m.library.selectedHashes = map[string]struct{}{"hash-one": {}, "hash-two": {}}

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	nm := next.(Model)

	if cmd != nil {
		t.Fatalf("'a' with selection: expected nil cmd (modal open is synchronous), got %T", cmd)
	}
	if nm.overlay.destPicker == nil {
		t.Fatal("'a' with selection: expected destPicker to be non-nil (modal open)")
	}
	if len(nm.overlay.destPickerTracks) != 2 {
		t.Errorf("'a' with selection: expected 2 destPickerTracks, got %d", len(nm.overlay.destPickerTracks))
	}
}

// TestRootLibraryAWithSelectionEnqueuesAndClears verifies that pressing 'a' in
// library view, then confirming the queue destination, appends tracks to the
// queue and clears the bulk selection.
func TestRootLibraryAWithSelectionEnqueuesAndClears(t *testing.T) {
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithBatchEnqueuer(be))
	m.library.index.Insert(library.Track{Artist: "A", Title: "One", Hash: "hash-one"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Two", Hash: "hash-two"})
	m.library.selectedHashes = map[string]struct{}{"hash-one": {}}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	nm := next.(Model)
	if nm.overlay.destPicker == nil {
		t.Fatal("expected destPicker to be open after 'a'")
	}

	nm2, enterCmd := nm.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	_ = nm2

	if enterCmd == nil {
		t.Fatal("Enter on modal: expected non-nil cmd (DestinationPickedMsg)")
	}
	pickedMsg := enterCmd()

	next3, enqueueCmd := nm.Update(pickedMsg)
	nm3 := next3.(Model)

	if enqueueCmd == nil {
		t.Fatal("DestinationPickedMsg: expected non-nil enqueue cmd")
	}
	batchMsg := enqueueCmd()
	if _, ok := batchMsg.(BatchEnqueuedMsg); !ok {
		t.Fatalf("expected BatchEnqueuedMsg, got %T", batchMsg)
	}
	if nm3.overlay.destPicker != nil {
		t.Error("expected destPicker to be nil after confirmation")
	}
	if len(nm3.library.selectedHashes) != 0 {
		t.Errorf("expected selectedHashes to be cleared after confirmation, got %v", nm3.library.selectedHashes)
	}
}

// TestRootLibraryAEscDismissesModal verifies that pressing Esc while the
// destination picker is open dismisses the modal without enqueueing.
func TestRootLibraryAEscDismissesModal(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "A", Title: "One", Hash: "hash-one"})

	next, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	nm := next.(Model)
	if nm.overlay.destPicker == nil {
		t.Fatal("expected destPicker to be open after 'a'")
	}

	_, escCmd := nm.Update(tea.KeyPressMsg{Code: tea.KeyEscape, Text: "esc"})
	if escCmd == nil {
		t.Fatal("Esc: expected a dismiss cmd")
	}
	dismissMsg := escCmd()
	next2, _ := nm.Update(dismissMsg)
	nm2 := next2.(Model)
	if nm2.overlay.destPicker != nil {
		t.Error("expected destPicker to be nil after dismiss")
	}
}

// TestRootLibraryAConfirmPlaylistAddsToPlaylist verifies that pressing 'a',
// navigating to a playlist destination, and pressing Enter calls
// AddTracksToPlaylist with the correct playlist ID and hashes.
func TestRootLibraryAConfirmPlaylistAddsToPlaylist(t *testing.T) {
	pa := &stubPlaylistAdder{}
	m := New(context.Background(), WithPlaylistAdder(pa))
	m.library.index.Insert(library.Track{Artist: "A", Title: "One", Hash: "hash-one"})
	m.playlists.playlists = []db.PlaylistRow{{ID: 7, Name: "Favourites"}}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	nm := next.(Model)

	next2, _ := nm.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	nm2 := next2.(Model)

	_, enterCmd := nm2.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	if enterCmd == nil {
		t.Fatal("Enter on playlist destination: expected non-nil cmd (DestinationPickedMsg)")
	}
	pickedMsg := enterCmd()

	next3, addCmd := nm2.Update(pickedMsg)
	nm3 := next3.(Model)

	if addCmd == nil {
		t.Fatal("DestinationPickedMsg: expected non-nil add cmd")
	}
	addCmd()

	if len(pa.calls) != 1 {
		t.Fatalf("expected 1 AddTracksToPlaylist call, got %d", len(pa.calls))
	}
	if pa.calls[0].playlistID != 7 {
		t.Errorf("expected playlistID 7, got %d", pa.calls[0].playlistID)
	}
	if len(pa.calls[0].hashes) != 1 || pa.calls[0].hashes[0] != "hash-one" {
		t.Errorf("expected hashes [hash-one], got %v", pa.calls[0].hashes)
	}
	if nm3.overlay.destPicker != nil {
		t.Error("expected destPicker to be nil after confirmation")
	}
	if len(nm3.library.selectedHashes) != 0 {
		t.Errorf("expected selectedHashes to be cleared, got %v", nm3.library.selectedHashes)
	}
}

// TestRootLibraryAConfirmPlaylistClearsSelectionAndNotifies verifies the full
// playlist destination-picker confirmation path end-to-end: the stub
// PlaylistAdder receives the correct playlist ID and hashes, bulk selection is
// cleared after DestinationPickedMsg, and the status bar notification reflects
// the track count and playlist name once tracksAddedToPlaylistMsg is processed.
func TestRootLibraryAConfirmPlaylistClearsSelectionAndNotifies(t *testing.T) {
	pa := &stubPlaylistAdder{}
	m := New(context.Background(), WithPlaylistAdder(pa))
	m.library.index.Insert(library.Track{Artist: "A", Title: "One", Hash: "hash-one"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Two", Hash: "hash-two"})
	m.library.selectedHashes = map[string]struct{}{"hash-one": {}, "hash-two": {}}
	m.playlists.playlists = []db.PlaylistRow{{ID: 42, Name: "Chill Vibes"}}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	nm := next.(Model)
	if nm.overlay.destPicker == nil {
		t.Fatal("expected destPicker to be open after 'a'")
	}
	if len(nm.overlay.destPickerTracks) != 2 {
		t.Fatalf("expected 2 destPickerTracks, got %d", len(nm.overlay.destPickerTracks))
	}

	next2, _ := nm.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	nm2 := next2.(Model)

	_, enterCmd := nm2.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	if enterCmd == nil {
		t.Fatal("Enter on playlist destination: expected non-nil cmd (DestinationPickedMsg)")
	}
	pickedMsg := enterCmd()

	next3, addCmd := nm2.Update(pickedMsg)
	nm3 := next3.(Model)

	if len(nm3.library.selectedHashes) != 0 {
		t.Errorf("selectedHashes must be empty after DestinationPickedMsg, got %v", nm3.library.selectedHashes)
	}
	if addCmd == nil {
		t.Fatal("DestinationPickedMsg: expected non-nil add cmd")
	}

	addedMsg := addCmd()

	if len(pa.calls) != 1 {
		t.Fatalf("expected 1 AddTracksToPlaylist call, got %d", len(pa.calls))
	}
	if pa.calls[0].playlistID != 42 {
		t.Errorf("expected playlistID 42, got %d", pa.calls[0].playlistID)
	}
	if len(pa.calls[0].hashes) != 2 {
		t.Errorf("expected 2 hashes, got %d: %v", len(pa.calls[0].hashes), pa.calls[0].hashes)
	}

	next4, _ := nm3.Update(addedMsg)
	nm4 := next4.(Model)
	v := nm4.View()
	if !strings.Contains(v.Content, "2 tracks") {
		t.Errorf("status bar must contain track count; got:\n%s", v.Content)
	}
	if !strings.Contains(v.Content, "Chill Vibes") {
		t.Errorf("status bar must contain playlist name; got:\n%s", v.Content)
	}
}
