package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

// TestPlaylistLocalCFiresCreateNotGlobalAction verifies that pressing 'c' in
// the Playlists view dispatches PlaylistCreateMsg.
func TestPlaylistLocalCFiresCreateNotGlobalAction(t *testing.T) {
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
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
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
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
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
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
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
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

// TestPlaylistCreateDuplicateNameShowsNotification verifies that attempting to
// create a playlist with a name that already exists shows a duplicate warning
// instead of calling CreatePlaylist.
func TestPlaylistCreateDuplicateNameShowsNotification(t *testing.T) {
	creator := &stubPlaylistCreator{nextID: 42}
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
	m.activeView = viewPlaylists
	m.SetPlaylistWriter(creator)
	m.SetPlaylists([]db.PlaylistRow{
		{ID: 1, Name: "Existing"},
	})
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
	if !strings.Contains(m.statusBar.notification.msg, "already exists") {
		t.Fatalf("expected duplicate warning, got %q", m.statusBar.notification.msg)
	}
}

// TestView2RendersPlaylistsNameAndCount verifies that switching to view 2 and
// calling SetPlaylists causes each playlist's name and track count to appear
// in the rendered output.
func TestView2RendersPlaylistsNameAndCount(t *testing.T) {
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
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
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
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
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
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
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
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
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
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
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
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
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)

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

	// Press 'd' — should trigger actionPlaylistDelete which bulk-removes when tracks are selected.
	next2, _ := nm.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
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

	// Undo — should restore the removed tracks.
	if nm2.undoSlot.kind != undoKindBulkRemovePlaylistTracks {
		t.Fatalf("expected undo kind %d, got %d", undoKindBulkRemovePlaylistTracks, nm2.undoSlot.kind)
	}
	nm3 := nm2.handleUndo()
	if len(nm3.playlists.tracks) != 3 {
		t.Fatalf("expected 3 tracks after undo, got %d", len(nm3.playlists.tracks))
	}
	wantIDs := []int64{10, 20, 30}
	for i, want := range wantIDs {
		if nm3.playlists.tracks[i].ID != want {
			t.Errorf("track[%d]: expected ID=%d, got ID=%d", i, want, nm3.playlists.tracks[i].ID)
		}
	}
}

// TestPlaylistDeleteClampsScrollOffset verifies that deleting a playlist when
// the list is scrolled clamps scrollOffset so it does not point past the end.
func TestPlaylistDeleteClampsScrollOffset(t *testing.T) {
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
	m.activeView = viewPlaylists
	pd := &stubPlaylistDeleter{}
	m.SetPlaylistDeleter(pd)

	m.SetPlaylists([]db.PlaylistRow{
		{ID: 1, Name: "A"},
		{ID: 2, Name: "B"},
	})
	// Simulate the view being scrolled past what will remain after deletion.
	m.playlists.cursor = 1
	m.playlists.scrollOffset = 1

	next, _ := m.handleKeyAction(actionPlaylistDelete)
	nm := next.(Model)

	if n := len(nm.playlists.playlists); n != 1 {
		t.Fatalf("expected 1 playlist remaining, got %d", n)
	}
	if nm.playlists.scrollOffset > len(nm.playlists.playlists)-1 {
		t.Fatalf("scrollOffset %d exceeds max valid offset %d after playlist delete",
			nm.playlists.scrollOffset, len(nm.playlists.playlists)-1)
	}
}

// TestPlaylistDeleteAllClampsScrollOffsetToZero verifies that deleting the last
// playlist resets scrollOffset to 0.
func TestPlaylistDeleteAllClampsScrollOffsetToZero(t *testing.T) {
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
	m.activeView = viewPlaylists
	pd := &stubPlaylistDeleter{}
	m.SetPlaylistDeleter(pd)

	m.SetPlaylists([]db.PlaylistRow{
		{ID: 1, Name: "A"},
	})
	m.playlists.cursor = 0
	m.playlists.scrollOffset = 0

	next, _ := m.handleKeyAction(actionPlaylistDelete)
	nm := next.(Model)

	if nm.playlists.scrollOffset != 0 {
		t.Fatalf("scrollOffset must be 0 after deleting all playlists, got %d",
			nm.playlists.scrollOffset)
	}
}

// TestPlaylistTrackDeleteClampsScrollOffset verifies that removing a track at
// TrackLevel clamps scrollOffset so it does not point past the end.
func TestPlaylistTrackDeleteClampsScrollOffset(t *testing.T) {
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
	m.activeView = viewPlaylists
	pd := &stubPlaylistDeleter{}
	m.SetPlaylistDeleter(pd)

	m.playlists.level = pltui.TrackLevel
	m.playlists.tracks = []db.PlaylistTrackRow{
		{ID: 10, Position: 1, Title: "A", Artist: "X", DurationMs: 1000},
		{ID: 20, Position: 2, Title: "B", Artist: "Y", DurationMs: 2000},
	}
	m.playlists.cursor = 1
	m.playlists.scrollOffset = 1

	next, _ := m.handleKeyAction(actionPlaylistDelete)
	nm := next.(Model)

	if n := len(nm.playlists.tracks); n != 1 {
		t.Fatalf("expected 1 track remaining, got %d", n)
	}
	if nm.playlists.scrollOffset > len(nm.playlists.tracks)-1 {
		t.Fatalf("scrollOffset %d exceeds max valid offset %d after track delete",
			nm.playlists.scrollOffset, len(nm.playlists.tracks)-1)
	}
}

// TestBulkRemovePlaylistTracksClampsScrollOffset verifies that bulk-removing
// tracks clamps scrollOffset so it does not point past the end.
func TestBulkRemovePlaylistTracksClampsScrollOffset(t *testing.T) {
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
	m.activeView = viewPlaylists
	pd := &stubPlaylistDeleter{}
	m.SetPlaylistDeleter(pd)

	m.playlists.level = pltui.TrackLevel
	m.playlists.tracks = []db.PlaylistTrackRow{
		{ID: 10, Position: 1, Title: "A", Artist: "X", DurationMs: 1000},
		{ID: 20, Position: 2, Title: "B", Artist: "Y", DurationMs: 2000},
		{ID: 30, Position: 3, Title: "C", Artist: "Z", DurationMs: 3000},
	}
	m.playlists.selectedTrackIDs = map[int64]struct{}{10: {}, 20: {}}
	m.playlists.cursor = 2
	m.playlists.scrollOffset = 2

	next, _ := m.handleKeyAction(actionPlaylistDelete)
	nm := next.(Model)

	if n := len(nm.playlists.tracks); n != 1 {
		t.Fatalf("expected 1 track remaining, got %d", n)
	}
	if nm.playlists.scrollOffset > 0 {
		t.Fatalf("scrollOffset %d must be 0 after bulk remove with 1 remaining track",
			nm.playlists.scrollOffset)
	}
}

// TestBulkRemovePlaylistTracksSoftDeletesAndSchedulesPurge verifies that
// actionBulkRemovePlaylistTracks calls RemovePlaylistTrackSoft for each
// selected track when a playlistDeleter is wired. Prior to the fix,
// soft-deleted rows were never purged because no purge timer was scheduled.
func TestBulkRemovePlaylistTracksSoftDeletesAndSchedulesPurge(t *testing.T) {
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
	m.activeView = viewPlaylists
	pd := &stubPlaylistDeleter{}
	m.SetPlaylistDeleter(pd)

	m.playlists.level = pltui.TrackLevel
	m.playlists.tracks = []db.PlaylistTrackRow{
		{ID: 10, Position: 1, Title: "A", Artist: "X", DurationMs: 1000},
		{ID: 20, Position: 2, Title: "B", Artist: "Y", DurationMs: 2000},
		{ID: 30, Position: 3, Title: "C", Artist: "Z", DurationMs: 3000},
	}
	m.playlists.selectedTrackIDs = map[int64]struct{}{10: {}, 30: {}}

	next, _ := m.handleKeyAction(actionPlaylistDelete)
	_ = next.(Model)

	if got := len(pd.removePlaylistTrackSoftCalls); got != 2 {
		t.Fatalf("expected 2 RemovePlaylistTrackSoft calls, got %d", got)
	}
	// Verify both selected IDs were soft-deleted (order depends on track slice iteration).
	softDeleted := map[int64]bool{}
	for _, id := range pd.removePlaylistTrackSoftCalls {
		softDeleted[id] = true
	}
	if !softDeleted[10] || !softDeleted[30] {
		t.Fatalf("expected IDs 10 and 30 to be soft-deleted, got %v", pd.removePlaylistTrackSoftCalls)
	}
}

// TestPlaylistCreateDuplicateNameCaseInsensitive verifies that creating a
// playlist with a name differing only in case from an existing one is rejected.
func TestPlaylistCreateDuplicateNameCaseInsensitive(t *testing.T) {
	creator := &stubPlaylistCreator{nextID: 42}
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
	m.activeView = viewPlaylists
	m.SetPlaylistWriter(creator)
	m.SetPlaylists([]db.PlaylistRow{
		{ID: 1, Name: "Jazz"},
	})
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
	if !strings.Contains(m.statusBar.notification.msg, "already exists") {
		t.Fatalf("expected duplicate warning, got %q", m.statusBar.notification.msg)
	}
}

// TestPlaylistRenameDuplicateNameCaseInsensitive verifies that renaming a
// playlist to a name differing only in case from another existing one is rejected.
func TestPlaylistRenameDuplicateNameCaseInsensitive(t *testing.T) {
	renamer := &stubPlaylistRenamer{}
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
	m.activeView = viewPlaylists
	m.SetPlaylistRenamer(renamer)
	m.SetPlaylists([]db.PlaylistRow{
		{ID: 1, Name: "Rock"},
		{ID: 2, Name: "Jazz"},
	})
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
	if !strings.Contains(m.statusBar.notification.msg, "already exists") {
		t.Fatalf("expected duplicate warning, got %q", m.statusBar.notification.msg)
	}
}

// TestUndoDeletePlaylistScrollsToRestoredEntry verifies that undoing a playlist
// deletion adjusts scrollOffset so the restored entry is visible.
func TestUndoDeletePlaylistScrollsToRestoredEntry(t *testing.T) {
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
	m.activeView = viewPlaylists
	pd := &stubPlaylistDeleter{}
	m.SetPlaylistDeleter(pd)

	m.SetPlaylists([]db.PlaylistRow{
		{ID: 1, Name: "A"},
		{ID: 2, Name: "B"},
		{ID: 3, Name: "C"},
		{ID: 4, Name: "D"},
		{ID: 5, Name: "E"},
	})
	// Delete the first playlist.
	m.playlists.cursor = 0
	m.playlists.scrollOffset = 0
	next, _ := m.handleKeyAction(actionPlaylistDelete)
	nm := next.(Model)

	// Simulate scrolling down so scrollOffset moves past the original index.
	nm.playlists.scrollOffset = 3

	// Undo — the restored entry should make scrollOffset adjust if cursor < scrollOffset.
	nm2 := nm.handleUndo()
	if nm2.playlists.cursor != 0 {
		t.Fatalf("expected cursor at 0 after undo, got %d", nm2.playlists.cursor)
	}
	if nm2.playlists.scrollOffset > nm2.playlists.cursor {
		t.Fatalf("scrollOffset %d should be <= cursor %d so restored entry is visible",
			nm2.playlists.scrollOffset, nm2.playlists.cursor)
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
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
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
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
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
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
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
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
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
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
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
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
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

// TestPlaylistRenamedMsgAdjustsScrollOffset verifies that after a playlist
// rename causes the entry to sort to a different position (because the list is
// sorted alphabetically), scrollOffset is adjusted so the cursor is visible.
func TestPlaylistRenamedMsgAdjustsScrollOffset(t *testing.T) {
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
	m.activeView = viewPlaylists
	// Use a small terminal so visibleRows is small (height=6 - 3 reserve = 3 visible rows).
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 6})
	m = next.(Model)

	// Populate 10 playlists named A through J. Cursor on "A" (index 0).
	var playlists []db.PlaylistRow
	for i := range 10 {
		playlists = append(playlists, db.PlaylistRow{ID: int64(i + 1), Name: string(rune('A' + i))})
	}
	m.SetPlaylists(playlists)
	m.playlists.cursor = 0
	m.playlists.scrollOffset = 0

	// Simulate renaming "A" (ID=1) to "Z", which sorts it last (index 9).
	renamedPlaylists := make([]db.PlaylistRow, 0, 10)
	for i := 1; i < 10; i++ {
		renamedPlaylists = append(renamedPlaylists, db.PlaylistRow{ID: int64(i + 1), Name: string(rune('A' + i))})
	}
	renamedPlaylists = append(renamedPlaylists, db.PlaylistRow{ID: 1, Name: "Z"})

	next, _ = m.Update(playlistRenamedMsg{id: 1, playlists: renamedPlaylists})
	m = next.(Model)

	// Cursor should be at index 9 (where "Z" now is).
	if m.playlists.cursor != 9 {
		t.Fatalf("expected cursor=9 (renamed entry), got %d", m.playlists.cursor)
	}
	// scrollOffset must be adjusted so cursor is visible.
	// visibleRows = max(1, 6-3) = 3, so scrollOffset should be at least 9-3+1 = 7.
	if m.playlists.scrollOffset+3 <= m.playlists.cursor {
		t.Fatalf("cursor %d is not visible: scrollOffset=%d, visibleRows=3",
			m.playlists.cursor, m.playlists.scrollOffset)
	}
}

// TestPlaylistCreatedMsgAdjustsScrollOffset verifies that after a playlist
// creation causes the entry to sort to a position beyond the visible window,
// scrollOffset is adjusted so the cursor is visible.
func TestPlaylistCreatedMsgAdjustsScrollOffset(t *testing.T) {
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
	m.activeView = viewPlaylists
	// Small terminal: visibleRows = max(1, 6-3) = 3.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 6})
	m = next.(Model)

	// Pre-populate 5 playlists named A through E.
	var playlists []db.PlaylistRow
	for i := range 5 {
		playlists = append(playlists, db.PlaylistRow{ID: int64(i + 1), Name: string(rune('A' + i))})
	}
	m.SetPlaylists(playlists)
	m.playlists.cursor = 0
	m.playlists.scrollOffset = 0

	// Simulate creating a playlist named "Z" (ID=6), which sorts last.
	createdPlaylists := make([]db.PlaylistRow, len(playlists))
	copy(createdPlaylists, playlists)
	createdPlaylists = append(createdPlaylists, db.PlaylistRow{ID: 6, Name: "Z"})

	next, _ = m.Update(playlistCreatedMsg{id: 6, playlists: createdPlaylists})
	m = next.(Model)

	// Cursor should be at index 5 (where "Z" is).
	if m.playlists.cursor != 5 {
		t.Fatalf("expected cursor=5 (new entry), got %d", m.playlists.cursor)
	}
	// scrollOffset must be adjusted so cursor is visible.
	if m.playlists.scrollOffset+3 <= m.playlists.cursor {
		t.Fatalf("cursor %d is not visible: scrollOffset=%d, visibleRows=3",
			m.playlists.cursor, m.playlists.scrollOffset)
	}
}

// TestPlaylistAddErrorMsgShowsNotification verifies that playlistAddErrorMsg
// sets a transient error notification on the status bar so the user is informed
// when AddTracksToPlaylist fails.
func TestPlaylistAddErrorMsgShowsNotification(t *testing.T) {
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
	next, _ := m.Update(playlistAddErrorMsg{err: errors.New("disk full")})
	nm := next.(Model)

	if nm.statusBar.notification.msg == "" {
		t.Fatal("expected a status bar notification for playlistAddErrorMsg, got empty")
	}
	if !strings.Contains(nm.statusBar.notification.msg, "disk full") {
		t.Errorf("notification should contain error text, got %q", nm.statusBar.notification.msg)
	}
	if nm.statusBar.notification.expiresAt.Before(time.Now()) {
		t.Error("notification should expire in the future")
	}
}

// TestPlaylistDeleteNoPanicWhenCursorOutOfBounds verifies that
// actionPlaylistDelete does not panic when the cursor exceeds the playlist
// slice length (e.g. due to a race between deletion and cursor movement).
func TestPlaylistDeleteNoPanicWhenCursorOutOfBounds(t *testing.T) {
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
	m.activeView = viewPlaylists
	m.playlists.playlists = []db.PlaylistRow{
		{ID: 1, Name: "A"},
	}
	m.playlists.cursor = 5 // out of bounds

	// Must not panic.
	next, _ := m.handleKeyAction(actionPlaylistDelete)
	_ = next
}

// TestPlaylistTrackDeleteNoPanicWhenCursorOutOfBounds verifies that
// actionPlaylistDelete at track level does not panic when the cursor
// exceeds the tracks slice length.
func TestPlaylistTrackDeleteNoPanicWhenCursorOutOfBounds(t *testing.T) {
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
	m.activeView = viewPlaylists
	m.playlists.level = pltui.TrackLevel
	m.playlists.tracks = []db.PlaylistTrackRow{
		{ID: 1, Title: "A"},
	}
	m.playlists.cursor = 5 // out of bounds

	// Must not panic.
	next, _ := m.handleKeyAction(actionPlaylistDelete)
	_ = next
}

// TestBulkActionInPlaylistsRequiresConfirmation verifies that pressing 'a' in
// Playlists at TrackLevel with selected tracks shows a confirmation notification
// on the first press, and executes the removal on the second press.
func TestBulkActionInPlaylistsRequiresConfirmation(t *testing.T) {
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
	m.activeView = viewPlaylists
	pd := &stubPlaylistDeleter{}
	m.SetPlaylistDeleter(pd)

	m.playlists.level = pltui.TrackLevel
	m.playlists.tracks = []db.PlaylistTrackRow{
		{ID: 10, Position: 1, Title: "A", Artist: "X", DurationMs: 1000},
		{ID: 20, Position: 2, Title: "B", Artist: "Y", DurationMs: 2000},
		{ID: 30, Position: 3, Title: "C", Artist: "Z", DurationMs: 3000},
	}
	m.playlists.selectedTrackIDs = map[int64]struct{}{10: {}, 20: {}}

	// First 'a' press — should show confirmation, NOT remove tracks.
	next, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	nm := next.(Model)

	if len(nm.playlists.tracks) != 3 {
		t.Fatalf("first 'a' press should not remove tracks, got %d remaining", len(nm.playlists.tracks))
	}
	if !nm.bulkRemovePending {
		t.Fatal("expected bulkRemovePending=true after first 'a' press")
	}
	if nm.statusBar.notification.msg == "" {
		t.Fatal("expected a confirmation notification after first 'a' press")
	}

	// Second 'a' press — should execute the removal.
	next2, _ := nm.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	nm2 := next2.(Model)

	if len(nm2.playlists.tracks) != 1 {
		t.Fatalf("expected 1 track remaining after confirmed removal, got %d", len(nm2.playlists.tracks))
	}
	if nm2.playlists.tracks[0].ID != 30 {
		t.Fatalf("expected remaining track ID=30, got ID=%d", nm2.playlists.tracks[0].ID)
	}
	if nm2.bulkRemovePending {
		t.Fatal("expected bulkRemovePending=false after confirmed removal")
	}
	if nm2.playlists.selectedTrackIDs != nil {
		t.Fatalf("expected selectedTrackIDs to be nil after removal, got %v", nm2.playlists.selectedTrackIDs)
	}

	// Undo — should restore the removed tracks.
	if nm2.undoSlot.kind != undoKindBulkRemovePlaylistTracks {
		t.Fatalf("expected undoSlot kind %d, got %d", undoKindBulkRemovePlaylistTracks, nm2.undoSlot.kind)
	}
	nm3 := nm2.handleUndo()
	if len(nm3.playlists.tracks) != 3 {
		t.Fatalf("expected 3 tracks after undo, got %d", len(nm3.playlists.tracks))
	}
	// Verify original tracks restored at correct positions.
	wantIDs := []int64{10, 20, 30}
	for i, want := range wantIDs {
		if nm3.playlists.tracks[i].ID != want {
			t.Errorf("track[%d]: expected ID=%d, got ID=%d", i, want, nm3.playlists.tracks[i].ID)
		}
	}
}
