package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

// TestPlaylistLocalCFiresCreateNotGlobalAction verifies that pressing 'c' in
// the Playlists view dispatches PlaylistCreateMsg.
func TestPlaylistLocalCFiresCreateNotGlobalAction(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)
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
	m := New("", false, nil, nil, "", nil, nil)
	m.activeView = viewPlaylists
	m.SetPlaylistWriter(creator)
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
	if m.playlists.cursor != 0 {
		t.Errorf("expected cursor=0 on new entry, got %d", m.playlists.cursor)
	}
}

// TestPlaylistCreateFlowEscCancels verifies that pressing Esc during inline
// creation discards the input without persisting anything.
func TestPlaylistCreateFlowEscCancels(t *testing.T) {
	creator := &stubPlaylistCreator{nextID: 1}
	m := New("", false, nil, nil, "", nil, nil)
	m.activeView = viewPlaylists
	m.SetPlaylistWriter(creator)

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

	if m.playlists.isCreating() {
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
	m := New("", false, nil, nil, "", nil, nil)
	m.activeView = viewPlaylists
	m.SetPlaylistWriter(creator)

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

	if m.playlists.isCreating() {
		t.Error("expected isCreating=false after error")
	}
	if !strings.Contains(m.statusBar.notification.msg, "db locked") {
		t.Errorf("expected error message in status bar, got %q", m.statusBar.notification.msg)
	}
}

// TestView2RendersPlaylistsNameAndCount verifies that switching to view 2 and
// calling SetPlaylists causes each playlist's name and track count to appear
// in the rendered output.
func TestView2RendersPlaylistsNameAndCount(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)
	m.SetPlaylists([]db.PlaylistRow{
		{ID: 1, Name: "Chill", TrackCount: 3},
		{ID: 2, Name: "Workout", TrackCount: 8},
	})

	// Give the model a realistic terminal width so names are not truncated.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	nm := next.(Model)

	// Switch to the playlists view.
	next, _ = nm.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	nm = next.(Model)

	v := nm.View()
	for _, want := range []string{"Chill", "(3)", "Workout", "(8)"} {
		if !strings.Contains(v.Content, want) {
			t.Errorf("expected %q in view 2 output, got:\n%s", want, v.Content)
		}
	}
}

// TestPlaylistEscFromTrackLevelReturnsToList verifies that pressing Esc while
// in TrackLevel transitions back to ListLevel and preserves the list cursor.
func TestPlaylistEscFromTrackLevelReturnsToList(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)
	m.SetPlaylists([]db.PlaylistRow{
		{ID: 1, Name: "Alpha", TrackCount: 1},
		{ID: 2, Name: "Beta", TrackCount: 1},
		{ID: 3, Name: "Gamma", TrackCount: 1},
	})
	reader := &stubPlaylistTrackReader{
		rows: []db.PlaylistTrackRow{
			{Position: 1, Title: "Track One", Artist: "Artist", DurationMs: 60000},
		},
	}
	m.SetPlaylistTrackReader(reader)

	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	nm := next.(Model)
	next, _ = nm.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	nm = next.(Model)

	// Move cursor to the second playlist (index 1).
	next, _ = nm.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	nm = next.(Model)
	if nm.playlists.cursor != 1 {
		t.Fatalf("precondition: expected cursor at 1, got %d", nm.playlists.cursor)
	}

	// Press Enter to open the playlist and execute the async load.
	next, cmd := nm.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	nm = next.(Model)
	if cmd == nil {
		t.Fatal("expected non-nil Cmd from actionPlaylistOpen")
	}
	next, _ = nm.Update(cmd())
	nm = next.(Model)

	// Verify we are now at TrackLevel showing the track.
	v := nm.View()
	if !strings.Contains(v.Content, "Track One") {
		t.Fatalf("expected TrackLevel output with 'Track One', got:\n%s", v.Content)
	}

	// Press Esc — should return to ListLevel.
	next, _ = nm.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	nm = next.(Model)

	// Verify we are back at ListLevel and the list cursor is preserved at 1.
	v = nm.View()
	if !strings.Contains(v.Content, "Alpha") || !strings.Contains(v.Content, "Beta") {
		t.Fatalf("expected ListLevel output with playlist names, got:\n%s", v.Content)
	}
	if nm.playlists.cursor != 1 {
		t.Errorf("expected list cursor preserved at 1 after Esc, got %d", nm.playlists.cursor)
	}
}

// TestPlaylistEscAtListLevelIsNoOp verifies that pressing Esc while already at
// ListLevel produces no visible state change and no error.
func TestPlaylistEscAtListLevelIsNoOp(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)
	m.SetPlaylists([]db.PlaylistRow{
		{ID: 1, Name: "Chill", TrackCount: 5},
	})

	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	nm := next.(Model)
	next, _ = nm.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	nm = next.(Model)

	beforeView := nm.View()
	beforeLevel := nm.playlists.level
	beforeCursor := nm.playlists.cursor

	// Press Esc at ListLevel — must be a no-op.
	next, _ = nm.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	nm = next.(Model)

	if nm.playlists.level != beforeLevel {
		t.Errorf("Esc at ListLevel changed level: before=%v after=%v", beforeLevel, nm.playlists.level)
	}
	if nm.playlists.cursor != beforeCursor {
		t.Errorf("Esc at ListLevel changed cursor: before=%d after=%d", beforeCursor, nm.playlists.cursor)
	}
	afterView := nm.View()
	if beforeView.Content != afterView.Content {
		t.Errorf("Esc at ListLevel changed rendered output:\nbefore:\n%s\nafter:\n%s", beforeView.Content, afterView.Content)
	}
}

// TestPlaylistRAtListLevelActivatesRenameInput verifies the full flow: pressing
// 'r' at ListLevel with a playlist present returns a PlaylistRenameMsg cmd,
// and processing that msg activates the inline rename input pre-populated with
// the focused playlist's name.
func TestPlaylistRAtListLevelActivatesRenameInput(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)
	m.SetPlaylists([]db.PlaylistRow{
		{ID: 1, Name: "Chill Mix", TrackCount: 3},
	})

	// Navigate to playlists view.
	next, _ := m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	nm := next.(Model)

	// Press 'r' — must return a PlaylistRenameMsg cmd.
	next, cmd := nm.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	nm = next.(Model)
	if cmd == nil {
		t.Fatal("pressing 'r' at ListLevel must return a non-nil cmd")
	}
	msg := cmd()
	if _, ok := msg.(PlaylistRenameMsg); !ok {
		t.Fatalf("expected PlaylistRenameMsg, got %T", msg)
	}

	// Process the PlaylistRenameMsg — rename input must activate.
	next, _ = nm.Update(msg)
	nm = next.(Model)

	if !nm.playlists.isRenaming() {
		t.Fatal("expected isRenaming() to be true after PlaylistRenameMsg")
	}
	if got := nm.playlists.renameInputValue(); got != "Chill Mix" {
		t.Fatalf("expected rename input pre-populated with %q, got %q", "Chill Mix", got)
	}
}

// TestPlaylistRAtTrackLevelIsNoOp verifies that pressing 'r' while the
// playlists view is at TrackLevel does not activate the rename input — rename
// only applies at list level.
func TestPlaylistRAtTrackLevelIsNoOp(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)
	m.SetPlaylists([]db.PlaylistRow{
		{ID: 1, Name: "Chill Mix", TrackCount: 1},
	})
	reader := &stubPlaylistTrackReader{
		rows: []db.PlaylistTrackRow{
			{Position: 1, Title: "Track One", Artist: "Artist", DurationMs: 60000},
		},
	}
	m.SetPlaylistTrackReader(reader)

	// Navigate to playlists view and open the playlist to reach TrackLevel.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	nm := next.(Model)
	next, _ = nm.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	nm = next.(Model)
	next, cmd := nm.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	nm = next.(Model)
	if cmd != nil {
		next, _ = nm.Update(cmd())
		nm = next.(Model)
	}

	// Press 'r' at TrackLevel — rename must not activate.
	next, _ = nm.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	nm = next.(Model)

	if nm.playlists.isRenaming() {
		t.Fatal("isRenaming() must be false when 'r' is pressed at TrackLevel")
	}
}

// TestPlaylistOpenRendersTrackTitles is an integration test that verifies the
// full path from actionPlaylistOpen through playlistTracksLoadedMsg to the
// rendered output. After opening a playlist the track titles must appear in
// the playlists panel.
func TestPlaylistOpenRendersTrackTitles(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)
	m.SetPlaylists([]db.PlaylistRow{
		{ID: 1, Name: "Chill", TrackCount: 2},
	})
	reader := &stubPlaylistTrackReader{
		rows: []db.PlaylistTrackRow{
			{Position: 1, Title: "Ocean Waves", Artist: "Ambient Co", DurationMs: 240000},
			{Position: 2, Title: "Forest Rain", Artist: "Nature Sounds", DurationMs: 180000},
		},
	}
	m.SetPlaylistTrackReader(reader)

	// Switch to the playlists view at a realistic width.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	nm := next.(Model)
	next, _ = nm.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	nm = next.(Model)

	// Press Enter to open the playlist — should return a Cmd.
	next, cmd := nm.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	nm = next.(Model)
	if cmd == nil {
		t.Fatal("actionPlaylistOpen: expected a non-nil Cmd to load tracks")
	}

	// Execute the Cmd and feed the result back into Update.
	loadMsg := cmd()
	next, _ = nm.Update(loadMsg)
	nm = next.(Model)

	v := nm.View()
	for _, want := range []string{"Ocean Waves", "Forest Rain"} {
		if !strings.Contains(v.Content, want) {
			t.Errorf("expected track title %q in view after playlist open, got:\n%s", want, v.Content)
		}
	}
}

// TestPlaylistsBulkRemoveAtTrackLevel verifies the 'a' key at TrackLevel with
// a non-empty bulk selection: selected tracks are removed from the in-memory
// slice, the cursor is clamped, and selectedTrackIDs is cleared. The nil
// playlistDeleter means no DB calls are made — the nil-guard in the handler
// makes this safe without a stub.
func TestPlaylistsBulkRemoveAtTrackLevel(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)

	// Navigate to playlists view.
	next, _ := m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	nm := next.(Model)

	// Seed the playlists panel directly at TrackLevel with three tracks.
	nm.playlists.level = pltui.TrackLevel
	nm.playlists.tracks = []db.PlaylistTrackRow{
		{ID: 10, Position: 1, Title: "Track One", Artist: "A", DurationMs: 60000},
		{ID: 20, Position: 2, Title: "Track Two", Artist: "B", DurationMs: 60000},
		{ID: 30, Position: 3, Title: "Track Three", Artist: "C", DurationMs: 60000},
	}
	// Bulk-select the first two tracks.
	nm.playlists.selectedTrackIDs = map[int64]struct{}{10: {}, 20: {}}
	// Place cursor beyond the surviving track to exercise cursor clamping.
	nm.playlists.cursor = 2

	// Press 'a' — should trigger actionAddToPlaylist which bulk-removes.
	next2, _ := nm.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	nm2 := next2.(Model)

	if len(nm2.playlists.tracks) != 1 {
		t.Fatalf("expected 1 track remaining, got %d", len(nm2.playlists.tracks))
	}
	if nm2.playlists.tracks[0].ID != 30 {
		t.Fatalf("expected remaining track ID=30, got ID=%d", nm2.playlists.tracks[0].ID)
	}
	if nm2.playlists.cursor >= len(nm2.playlists.tracks) {
		t.Fatalf("cursor %d out of range after bulk remove (len=%d)", nm2.playlists.cursor, len(nm2.playlists.tracks))
	}
	if nm2.playlists.selectedTrackIDs != nil {
		t.Fatalf("expected selectedTrackIDs to be nil after bulk remove, got %v", nm2.playlists.selectedTrackIDs)
	}
}

// drillIntoPlaylistTrackLevel puts m into Playlists view at TrackLevel with
// the provided tracks seeded directly, returning a ready-to-use model.
func drillIntoPlaylistTrackLevel(m Model, tracks []db.PlaylistTrackRow) Model {
	m.activeView = viewPlaylists
	m.playlists.level = pltui.TrackLevel
	m.playlists.tracks = tracks
	m.playlists.cursor = 0
	return m
}

// TestPlaylistTrackCursorDownJKey verifies that pressing "j" while the
// playlist track-level view is active advances the cursor by one.
func TestPlaylistTrackCursorDownJKey(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)
	m = drillIntoPlaylistTrackLevel(m, []db.PlaylistTrackRow{
		{ID: 1, Position: 1, Title: "Alpha", Artist: "A", DurationMs: 60000},
		{ID: 2, Position: 2, Title: "Beta", Artist: "B", DurationMs: 60000},
	})

	next, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	nm := next.(Model)
	if nm.playlists.cursor != 1 {
		t.Errorf("j key at TrackLevel: expected cursor 1, got %d", nm.playlists.cursor)
	}
}

// TestPlaylistTrackCursorUpKKey verifies that pressing "k" while the playlist
// track-level view is active moves the cursor up by one.
func TestPlaylistTrackCursorUpKKey(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)
	m = drillIntoPlaylistTrackLevel(m, []db.PlaylistTrackRow{
		{ID: 1, Position: 1, Title: "Alpha", Artist: "A", DurationMs: 60000},
		{ID: 2, Position: 2, Title: "Beta", Artist: "B", DurationMs: 60000},
	})
	m.playlists.cursor = 1

	next, _ := m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	nm := next.(Model)
	if nm.playlists.cursor != 0 {
		t.Errorf("k key at TrackLevel: expected cursor 0, got %d", nm.playlists.cursor)
	}
}

// TestPlaylistTrackCursorDownArrow verifies that the down arrow key advances
// the cursor in the playlist track-level view, behaving identically to "j".
func TestPlaylistTrackCursorDownArrow(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)
	m = drillIntoPlaylistTrackLevel(m, []db.PlaylistTrackRow{
		{ID: 1, Position: 1, Title: "Alpha", Artist: "A", DurationMs: 60000},
		{ID: 2, Position: 2, Title: "Beta", Artist: "B", DurationMs: 60000},
	})

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown, Text: "down"})
	nm := next.(Model)
	if nm.playlists.cursor != 1 {
		t.Errorf("down arrow at TrackLevel: expected cursor 1, got %d", nm.playlists.cursor)
	}
}

// TestPlaylistTrackCursorUpArrow verifies that the up arrow key moves the
// cursor up in the playlist track-level view, behaving identically to "k".
func TestPlaylistTrackCursorUpArrow(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)
	m = drillIntoPlaylistTrackLevel(m, []db.PlaylistTrackRow{
		{ID: 1, Position: 1, Title: "Alpha", Artist: "A", DurationMs: 60000},
		{ID: 2, Position: 2, Title: "Beta", Artist: "B", DurationMs: 60000},
	})
	m.playlists.cursor = 1

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp, Text: "up"})
	nm := next.(Model)
	if nm.playlists.cursor != 0 {
		t.Errorf("up arrow at TrackLevel: expected cursor 0, got %d", nm.playlists.cursor)
	}
}

// TestPlaylistTrackCursorClampAtBottom verifies that the cursor stops at the
// last track without wrapping when the playlist track-level view is active.
func TestPlaylistTrackCursorClampAtBottom(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)
	m = drillIntoPlaylistTrackLevel(m, []db.PlaylistTrackRow{
		{ID: 1, Position: 1, Title: "Alpha", Artist: "A", DurationMs: 60000},
		{ID: 2, Position: 2, Title: "Beta", Artist: "B", DurationMs: 60000},
	})
	m.playlists.cursor = 1 // already at last item

	next, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	nm := next.(Model)
	if nm.playlists.cursor != 1 {
		t.Errorf("j at last track: expected cursor 1 (clamped), got %d", nm.playlists.cursor)
	}
}

// TestPlaylistTrackCursorClampAtTop verifies that the cursor stops at the
// first track without wrapping when the playlist track-level view is active.
func TestPlaylistTrackCursorClampAtTop(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)
	m = drillIntoPlaylistTrackLevel(m, []db.PlaylistTrackRow{
		{ID: 1, Position: 1, Title: "Alpha", Artist: "A", DurationMs: 60000},
		{ID: 2, Position: 2, Title: "Beta", Artist: "B", DurationMs: 60000},
	})
	// cursor starts at 0

	next, _ := m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	nm := next.(Model)
	if nm.playlists.cursor != 0 {
		t.Errorf("k at first track: expected cursor 0 (clamped), got %d", nm.playlists.cursor)
	}
}
