package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
)

// TestLibraryCursorDownJKey verifies that pressing "j" while the library view
// is active advances the cursor by one.
func TestLibraryCursorDownJKey(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "A", Title: "One"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Two"})

	next, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	nm := next.(Model)
	if nm.library.cursor != 1 {
		t.Errorf("j key: expected cursor 1, got %d", nm.library.cursor)
	}
}

// TestLibraryCursorUpKKey verifies that pressing "k" while the library view
// is active moves the cursor up by one.
func TestLibraryCursorUpKKey(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "A", Title: "One"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Two"})
	m.library.cursor = 1

	next, _ := m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	nm := next.(Model)
	if nm.library.cursor != 0 {
		t.Errorf("k key: expected cursor 0, got %d", nm.library.cursor)
	}
}

// TestLibraryCursorDownArrow verifies that the down arrow key advances the
// cursor in the library view, behaving identically to "j".
func TestLibraryCursorDownArrow(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "A", Title: "One"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Two"})

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown, Text: "down"})
	nm := next.(Model)
	if nm.library.cursor != 1 {
		t.Errorf("down arrow: expected cursor 1, got %d", nm.library.cursor)
	}
}

// TestLibraryCursorUpArrow verifies that the up arrow key moves the cursor up
// in the library view, behaving identically to "k".
func TestLibraryCursorUpArrow(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "A", Title: "One"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Two"})
	m.library.cursor = 1

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp, Text: "up"})
	nm := next.(Model)
	if nm.library.cursor != 0 {
		t.Errorf("up arrow: expected cursor 0, got %d", nm.library.cursor)
	}
}

// TestLibraryCursorClampAtBottom verifies that the cursor stops at the last
// track without wrapping when the library view is active.
func TestLibraryCursorClampAtBottom(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "A", Title: "One"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Two"})
	m.library.cursor = 1 // already at last item

	next, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	nm := next.(Model)
	if nm.library.cursor != 1 {
		t.Errorf("j at last: expected cursor 1 (clamped), got %d", nm.library.cursor)
	}
}

// TestLibraryCursorClampAtTop verifies that the cursor stops at the first
// track without wrapping when the library view is active.
func TestLibraryCursorClampAtTop(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "A", Title: "One"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Two"})
	// cursor starts at 0

	next, _ := m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	nm := next.(Model)
	if nm.library.cursor != 0 {
		t.Errorf("k at first: expected cursor 0 (clamped), got %d", nm.library.cursor)
	}
}

// TestLibraryCursorNotMovedWhenQueueActive verifies that j/k in the queue view
// do not affect the library cursor.
func TestLibraryCursorNotMovedWhenQueueActive(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "A", Title: "One"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Two"})
	m.activeView = viewQueue

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown, Text: "down"})
	nm := next.(Model)
	if nm.library.cursor != 0 {
		t.Errorf("down in queue view must not move library cursor, got %d", nm.library.cursor)
	}
}

// TestRootLibraryXTogglesSelectedHash verifies that pressing 'x' while the
// library view is active toggles the focused track's hash into selectedHashes.
func TestRootLibraryXTogglesSelectedHash(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "A", Title: "One", Hash: "hash-one"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Two", Hash: "hash-two"})
	m.library.cursor = 0
	// activeView is viewLibrary by default

	next, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	nm := next.(Model)

	if _, ok := nm.library.selectedHashes["hash-one"]; !ok {
		t.Errorf("'x' on focused track: expected hash-one in selectedHashes, got %v", nm.library.selectedHashes)
	}
	if _, ok := nm.library.selectedHashes["hash-two"]; ok {
		t.Errorf("'x' on focused track: hash-two must not be in selectedHashes")
	}

	// Pressing 'x' again must remove the hash (toggle off).
	next2, _ := nm.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	nm2 := next2.(Model)
	if _, ok := nm2.library.selectedHashes["hash-one"]; ok {
		t.Errorf("second 'x': expected hash-one to be deselected, but it is still in selectedHashes")
	}
}

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

	// 'a' opens the destination picker.
	next, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	nm := next.(Model)
	if nm.overlay.destPicker == nil {
		t.Fatal("expected destPicker to be open after 'a'")
	}

	// Enter confirms the focused destination (Play Queue is first).
	// The modal dispatches a DestinationPickedMsg via a Cmd.
	nm2, enterCmd := nm.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	_ = nm2

	if enterCmd == nil {
		t.Fatal("Enter on modal: expected non-nil cmd (DestinationPickedMsg)")
	}
	pickedMsg := enterCmd()

	// Processing DestinationPickedMsg triggers the batch enqueue Cmd.
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

	// Open the modal.
	next, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	nm := next.(Model)
	if nm.overlay.destPicker == nil {
		t.Fatal("expected destPicker to be open after 'a'")
	}

	// Esc dispatches DestinationPickerDismissedMsg via a Cmd.
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

	// 'a' opens the modal.
	next, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	nm := next.(Model)

	// 'j' moves cursor from Play Queue (0) to Favourites (1).
	next2, _ := nm.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	nm2 := next2.(Model)

	// Enter on modal dispatches DestinationPickedMsg via Cmd.
	_, enterCmd := nm2.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	if enterCmd == nil {
		t.Fatal("Enter on playlist destination: expected non-nil cmd (DestinationPickedMsg)")
	}
	pickedMsg := enterCmd()

	// Processing DestinationPickedMsg triggers the AddTracksToPlaylist Cmd.
	next3, addCmd := nm2.Update(pickedMsg)
	nm3 := next3.(Model)

	if addCmd == nil {
		t.Fatal("DestinationPickedMsg: expected non-nil add cmd")
	}
	// Execute the cmd to trigger the DB call.
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

	// 'a' opens the destination picker with all 2 bulk-selected tracks staged.
	next, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	nm := next.(Model)
	if nm.overlay.destPicker == nil {
		t.Fatal("expected destPicker to be open after 'a'")
	}
	if len(nm.overlay.destPickerTracks) != 2 {
		t.Fatalf("expected 2 destPickerTracks, got %d", len(nm.overlay.destPickerTracks))
	}

	// 'j' moves cursor from Play Queue (0) to Chill Vibes (1).
	next2, _ := nm.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	nm2 := next2.(Model)

	// Enter dispatches DestinationPickedMsg via Cmd.
	_, enterCmd := nm2.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	if enterCmd == nil {
		t.Fatal("Enter on playlist destination: expected non-nil cmd (DestinationPickedMsg)")
	}
	pickedMsg := enterCmd()

	// Processing DestinationPickedMsg clears bulk selection and returns the add Cmd.
	next3, addCmd := nm2.Update(pickedMsg)
	nm3 := next3.(Model)

	if len(nm3.library.selectedHashes) != 0 {
		t.Errorf("selectedHashes must be empty after DestinationPickedMsg, got %v", nm3.library.selectedHashes)
	}
	if addCmd == nil {
		t.Fatal("DestinationPickedMsg: expected non-nil add cmd")
	}

	// Execute the add cmd — stub records the call and returns tracksAddedToPlaylistMsg.
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

	// Processing tracksAddedToPlaylistMsg must update the status bar.
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

// TestLibraryEscClearsCommittedFilter verifies that pressing Esc while the
// library has a committed (non-active) filter clears the filter text and
// restores the full library listing.
func TestLibraryEscClearsCommittedFilter(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "Alice", Title: "Alpha", Hash: "h1"})
	m.library.index.Insert(library.Track{Artist: "Bob", Title: "Beta", Hash: "h2"})

	// Activate filter, type, commit.
	m = pressSlash(m)
	m = typeText(m, "ali")
	m = pressEnter(m) // commit filter

	if m.library.filter.Text != "ali" {
		t.Fatalf("expected committed filterText \"ali\", got %q", m.library.filter.Text)
	}
	if m.library.filter.Active {
		t.Fatal("expected filterActive=false after commit")
	}

	// Esc should clear the committed filter.
	m = pressEsc(m)
	if m.library.filter.Text != "" {
		t.Errorf("expected filterText cleared after Esc, got %q", m.library.filter.Text)
	}
	if m.library.filter.Active {
		t.Error("expected filterActive=false after Esc")
	}
}

// TestLibraryEnterClearsCommittedFilter verifies that pressing Enter on a
// focused track while a committed filter is active enqueues the track and
// clears the filter.
func TestLibraryEnterClearsCommittedFilter(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.library.index.Insert(library.Track{Artist: "Alice", Title: "Alpha", Hash: "h1"})
	m.library.index.Insert(library.Track{Artist: "Bob", Title: "Beta", Hash: "h2"})

	// Activate filter, type, commit.
	m = pressSlash(m)
	m = typeText(m, "ali")
	m = pressEnter(m) // commit filter

	// Press Enter to enqueue the focused (filtered) track.
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)

	if m.library.filter.Text != "" {
		t.Errorf("expected filterText cleared after enqueue Enter, got %q", m.library.filter.Text)
	}
	if m.library.filter.Active {
		t.Error("expected filterActive=false after enqueue Enter")
	}
	if cmd == nil {
		t.Fatal("expected non-nil enqueue cmd")
	}
	cmd() // execute the enqueue command
	if len(qa.tracks) != 1 {
		t.Errorf("expected 1 enqueued track, got %d", len(qa.tracks))
	}
}

// TestFilterBarMsgIgnoredOutsideLibraryView verifies that pressing '/' while on
// the playlists view does not activate the library filter bar. Before the fix,
// FilterBarMsg always toggled the library filter regardless of the active view,
// silently leaving it in an inconsistent state.
func TestFilterBarMsgIgnoredOutsideLibraryView(t *testing.T) {
	m := New(context.Background())

	// Switch to queue view — filter IS supported here now.
	next, _ := m.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	nm := next.(Model)
	if nm.activeView != viewQueue {
		t.Fatalf("expected viewQueue, got %d", nm.activeView)
	}

	// Send FilterBarMsg while on the queue view: should activate the queue filter.
	next, _ = nm.Update(FilterBarMsg{})
	nm = next.(Model)
	if nm.library.filter.Active {
		t.Error("library filterActive should remain false when FilterBarMsg is sent on the queue view")
	}
	if !nm.queue.filter.Active {
		t.Error("queue filter should be active after FilterBarMsg on queue view")
	}

	// Check playlists view — filter is NOT supported here.
	next, _ = m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	nm = next.(Model)
	next, _ = nm.Update(FilterBarMsg{})
	nm = next.(Model)
	if nm.library.filter.Active {
		t.Error("library filterActive should remain false when FilterBarMsg is sent on playlists view")
	}
	if nm.statusBar.notification.msg == "" {
		t.Error("FilterBarMsg on playlists view should set a transient notification")
	}
}

// TestEnqueueReplaceEmptyLibraryDoesNotClearQueue verifies that
// actionLibraryEnqueueReplace in the Library view does not clear the existing
// queue when the library is empty (no focused track to replace with).
func TestEnqueueReplaceEmptyLibraryDoesNotClearQueue(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewLibrary
	// Pre-populate the queue with one entry.
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Title: "Existing", Artist: "A"},
	}

	next, _ := m.handleKeyAction(actionLibraryEnqueueReplace)
	nm := next.(Model)
	if len(nm.queue.entries) == 0 {
		t.Fatal("queue must not be cleared when library is empty")
	}
	if len(nm.queue.entries) != 1 || nm.queue.entries[0].Hash != "h1" {
		t.Errorf("queue should be unchanged; got %d entries", len(nm.queue.entries))
	}
}

// TestEnqueueReplaceFavoritesCategoryLevelDoesNotClearQueue verifies that
// actionLibraryEnqueueReplace on the Favorites view at CategoryLevel does
// not clear the queue (there is no focused track at this level).
func TestEnqueueReplaceFavoritesCategoryLevelDoesNotClearQueue(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewFavorites
	// Pre-populate the queue with one entry.
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Title: "Existing", Artist: "A"},
	}

	next, _ := m.handleKeyAction(actionLibraryEnqueueReplace)
	nm := next.(Model)
	if len(nm.queue.entries) == 0 {
		t.Fatal("queue must not be cleared at favorites CategoryLevel")
	}
	if len(nm.queue.entries) != 1 || nm.queue.entries[0].Hash != "h1" {
		t.Errorf("queue should be unchanged; got %d entries", len(nm.queue.entries))
	}
}
