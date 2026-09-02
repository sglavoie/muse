package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// TestPlaylistLocalCFiresCreateNotGlobalAction verifies that pressing 'c' in
// the Playlists view dispatches PlaylistCreateMsg.
func TestPlaylistLocalCFiresCreateNotGlobalAction(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewPlaylists

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if cmd == nil {
		t.Fatal("c key in Playlists: expected non-nil cmd for create playlist")
	}
	msg := cmd()
	if _, ok := msg.(PlaylistCreateMsg); !ok {
		t.Fatalf("c key in Playlists: expected PlaylistCreateMsg, got %T", msg)
	}
}

// TestPlaylistCreateFlowSuccess exercises the full inline create flow:
// 'c' activates the input, typing a name and pressing Enter calls CreatePlaylist,
// feeds playlistCreatedMsg back to Update, and verifies the name appears in the
// rendered view with the cursor positioned on the new entry.
func TestPlaylistCreateFlowSuccess(t *testing.T) {
	creator := &stubPlaylistCreator{nextID: 42}
	m := New(context.Background(), WithPlaylistWriter(creator))
	m.activeView = viewPlaylists
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(Model)

	// 'c' dispatches PlaylistCreateMsg.
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("c: expected non-nil cmd")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)

	// Type the playlist name character by character.
	for _, ch := range "MyList" {
		next, _ = m.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
		m = next.(Model)
	}

	// Press Enter — should return the async create cmd.
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("Enter: expected non-nil cmd for CreatePlaylist")
	}

	// Execute the cmd and feed the resulting playlistCreatedMsg back.
	result := cmd()
	if _, ok := result.(playlistCreatedMsg); !ok {
		t.Fatalf("cmd result: expected playlistCreatedMsg, got %T", result)
	}
	next, _ = m.Update(result)
	m = next.(Model)

	// Verify the new playlist name appears in the view.
	v := m.View()
	if !strings.Contains(v.Content, "MyList") {
		t.Errorf("expected 'MyList' in view after creation, got:\n%s", v.Content)
	}

	// Verify cursor is on the new entry (index 0, the only entry).
	if m.playlists.Cursor != 0 {
		t.Errorf("expected cursor=0 on new entry, got %d", m.playlists.Cursor)
	}
}

// TestPlaylistCreateFlowEscCancels verifies that pressing Esc during inline
// creation discards the input without persisting anything.
func TestPlaylistCreateFlowEscCancels(t *testing.T) {
	creator := &stubPlaylistCreator{nextID: 1}
	m := New(context.Background(), WithPlaylistWriter(creator))
	m.activeView = viewPlaylists

	// Activate create mode.
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = next.(Model)
	next, _ = m.Update(cmd())
	m = next.(Model)

	// Type something, then press Esc.
	for _, ch := range "Oops" {
		next, _ = m.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
		m = next.(Model)
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = next.(Model)

	if m.playlists.IsCreating() {
		t.Error("expected isCreating=false after Esc")
	}
	if len(creator.rows) != 0 {
		t.Errorf("expected no playlists persisted after Esc, got %d", len(creator.rows))
	}
}

// TestPlaylistCreateFlowErrorShowsNotification verifies that a CreatePlaylist
// error cancels the input and surfaces a status bar notification.
func TestPlaylistCreateFlowErrorShowsNotification(t *testing.T) {
	creator := &stubPlaylistCreator{createErr: &errPlaylistCreate{msg: "db locked"}, nextID: 1}
	m := New(context.Background(), WithPlaylistWriter(creator))
	m.activeView = viewPlaylists

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = next.(Model)
	next, _ = m.Update(cmd())
	m = next.(Model)

	for _, ch := range "Oops" {
		next, _ = m.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
		m = next.(Model)
	}
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("Enter: expected non-nil cmd")
	}

	result := cmd()
	if _, ok := result.(playlistCreateErrorMsg); !ok {
		t.Fatalf("cmd result: expected playlistCreateErrorMsg, got %T", result)
	}
	next, _ = m.Update(result)
	m = next.(Model)

	if m.playlists.IsCreating() {
		t.Error("expected isCreating=false after error")
	}
	if !strings.Contains(m.statusBar.TopNotification().Msg, "db locked") {
		t.Errorf("expected error message in status bar, got %q", m.statusBar.TopNotification().Msg)
	}
}

// TestPlaylistCreateDuplicateNameShowsNotification verifies that attempting to
// create a playlist with a name that already exists shows a duplicate warning
// instead of calling CreatePlaylist.
func TestPlaylistCreateDuplicateNameShowsNotification(t *testing.T) {
	creator := &stubPlaylistCreator{nextID: 42}
	m := New(context.Background(), WithPlaylistWriter(creator), WithPlaylists([]db.PlaylistRow{{ID: 1, Name: "Existing"}}))
	m.activeView = viewPlaylists
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(Model)

	// Activate create mode.
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("c: expected non-nil cmd")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)

	// Type the duplicate name.
	for _, ch := range "Existing" {
		next, _ = m.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
		m = next.(Model)
	}

	// Press Enter — should NOT return an async cmd (duplicate rejected locally).
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if cmd != nil {
		t.Fatal("Enter with duplicate name: expected nil cmd (no DB call)")
	}
	if !strings.Contains(m.statusBar.TopNotification().Msg, "already exists") {
		t.Fatalf("expected duplicate warning, got %q", m.statusBar.TopNotification().Msg)
	}
}

// TestPlaylistCreateDuplicateNameCaseInsensitive verifies that creating a
// playlist with a name differing only in case from an existing one is rejected.
func TestPlaylistCreateDuplicateNameCaseInsensitive(t *testing.T) {
	creator := &stubPlaylistCreator{nextID: 42}
	m := New(context.Background(), WithPlaylistWriter(creator), WithPlaylists([]db.PlaylistRow{{ID: 1, Name: "Jazz"}}))
	m.activeView = viewPlaylists
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(Model)

	// Activate create mode.
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("c: expected non-nil cmd")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)

	// Type a case-variant of the existing name.
	for _, ch := range "jazz" {
		next, _ = m.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
		m = next.(Model)
	}

	// Press Enter — should be rejected locally (no DB call).
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if cmd != nil {
		t.Fatal("Enter with case-variant duplicate name: expected nil cmd (no DB call)")
	}
	if !strings.Contains(m.statusBar.TopNotification().Msg, "already exists") {
		t.Fatalf("expected duplicate warning, got %q", m.statusBar.TopNotification().Msg)
	}
}

// TestPlaylistRenameDuplicateNameCaseInsensitive verifies that renaming a
// playlist to a name differing only in case from another existing one is rejected.
func TestPlaylistRenameDuplicateNameCaseInsensitive(t *testing.T) {
	renamer := &stubPlaylistRenamer{}
	m := New(context.Background(), WithPlaylistRenamer(renamer), WithPlaylists([]db.PlaylistRow{{ID: 1, Name: "Rock"}, {ID: 2, Name: "Jazz"}}))
	m.activeView = viewPlaylists
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(Model)

	// Press 'r' to rename the focused playlist ("Rock").
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("r: expected non-nil cmd")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)

	// Clear the rename input and type the case-variant duplicate.
	for range "Rock" {
		next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
		m = next.(Model)
	}
	for _, ch := range "jazz" {
		next, _ = m.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
		m = next.(Model)
	}

	// Press Enter — should be rejected.
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if cmd != nil {
		t.Fatal("Enter with case-variant duplicate name: expected nil cmd (no DB call)")
	}
	if !strings.Contains(m.statusBar.TopNotification().Msg, "already exists") {
		t.Fatalf("expected duplicate warning, got %q", m.statusBar.TopNotification().Msg)
	}
}

// TestPlaylistCreateNameTrimsWhitespace verifies that leading and trailing
// whitespace is stripped from the playlist name before persisting.
func TestPlaylistCreateNameTrimsWhitespace(t *testing.T) {
	creator := &stubPlaylistCreator{nextID: 99}
	m := New(context.Background(), WithPlaylistWriter(creator))
	m.activeView = viewPlaylists

	// Activate the create input.
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = next.(Model)
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(Model)
	}

	// Type a name with surrounding whitespace.
	for _, ch := range "  MyList  " {
		next, _ = m.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
		m = next.(Model)
	}

	// Press Enter.
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("Enter: expected non-nil cmd for CreatePlaylist")
	}

	// Execute the cmd and verify the created playlist name is trimmed.
	result := cmd()
	created, ok := result.(playlistCreatedMsg)
	if !ok {
		t.Fatalf("expected playlistCreatedMsg, got %T", result)
	}
	for _, p := range created.playlists {
		if p.ID == 99 {
			if p.Name != "MyList" {
				t.Errorf("expected trimmed name 'MyList', got %q", p.Name)
			}
			return
		}
	}
	t.Error("created playlist with ID 99 not found in playlistCreatedMsg.playlists")
}

// TestPlaylistRenameNameTrimsWhitespace verifies that leading and trailing
// whitespace is stripped from the rename value before persisting.
func TestPlaylistRenameNameTrimsWhitespace(t *testing.T) {
	renamer := &stubPlaylistRenamer{rows: []db.PlaylistRow{{ID: 1, Name: "Old"}}}
	m := New(context.Background(), WithPlaylistRenamer(renamer))
	m.playlists.Playlists = []db.PlaylistRow{{ID: 1, Name: "Old"}}
	m.playlists.Cursor = 0

	// The renameInputValue method should trim whitespace.
	m.playlists = m.playlists.StartRename("Old")
	m.playlists.RenameInput.SetValue("  NewName  ")

	got := m.playlists.RenameInputValue()
	if got != "NewName" {
		t.Errorf("expected trimmed rename value 'NewName', got %q", got)
	}
}

// TestQueueSaveInputValueTrimsWhitespace verifies that leading and trailing
// whitespace is stripped from the save-as-playlist input.
func TestQueueSaveInputValueTrimsWhitespace(t *testing.T) {
	m := New(context.Background())
	m.queue.Entries = []queue.Entry{{Path: "/a.flac", Hash: "aaa", Display: "A"}}
	m.queue = m.queue.StartSaveAsPlaylist(false)
	m.queue.SaveInput.SetValue("  SavedQueue  ")

	got := m.queue.SaveInputValue()
	if got != "SavedQueue" {
		t.Errorf("expected trimmed save value 'SavedQueue', got %q", got)
	}
}

// TestPlaylistCreateDuplicateWithWhitespace verifies that a name with
// surrounding whitespace is detected as a duplicate when a playlist with the
// trimmed name already exists.
func TestPlaylistCreateDuplicateWithWhitespace(t *testing.T) {
	creator := &stubPlaylistCreator{nextID: 50}
	m := New(context.Background(), WithPlaylistWriter(creator))
	m = m.switchToView(viewPlaylists)
	m.playlists.Playlists = []db.PlaylistRow{{ID: 1, Name: "Existing"}}

	// Activate the create input.
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = next.(Model)
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(Model)
	}

	// Type the existing name with spaces around it.
	for _, ch := range "  Existing  " {
		next, _ = m.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
		m = next.(Model)
	}

	// Press Enter — should be rejected as a duplicate.
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if cmd != nil {
		t.Fatal("Enter with duplicate name should not dispatch a create cmd")
	}
	if !strings.Contains(m.statusBar.TopNotification().Msg, "already exists") {
		t.Errorf("expected duplicate notification, got %q", m.statusBar.TopNotification().Msg)
	}
}
