package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
)

// TestView2RendersPlaylistsNameAndCount verifies that switching to view 2 and
// calling SetPlaylists causes each playlist's name and track count to appear
// in the rendered output.
func TestView2RendersPlaylistsNameAndCount(t *testing.T) {
	m := New(context.Background(), WithPlaylists([]db.PlaylistRow{{ID: 1, Name: "Chill", TrackCount: 3}, {ID: 2, Name: "Workout", TrackCount: 8}}))

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
	reader := &stubPlaylistTrackReader{rows: []db.PlaylistTrackRow{{Position: 1, Title: "Track One", Artist: "Artist", DurationMs: 60000}}}
	m := New(context.Background(), WithPlaylists([]db.PlaylistRow{{ID: 1, Name: "Alpha", TrackCount: 1}, {ID: 2, Name: "Beta", TrackCount: 1}, {ID: 3, Name: "Gamma", TrackCount: 1}}), WithPlaylistTrackReader(reader))

	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	nm := next.(Model)
	next, _ = nm.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	nm = next.(Model)

	// Move cursor to the second playlist (index 1).
	next, _ = nm.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	nm = next.(Model)
	if nm.playlists.Cursor != 1 {
		t.Fatalf("precondition: expected cursor at 1, got %d", nm.playlists.Cursor)
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
	if nm.playlists.Cursor != 1 {
		t.Errorf("expected list cursor preserved at 1 after Esc, got %d", nm.playlists.Cursor)
	}
}

// TestPlaylistEscAtListLevelIsNoOp verifies that pressing Esc while already at
// ListLevel produces no visible state change and no error.
func TestPlaylistEscAtListLevelIsNoOp(t *testing.T) {
	m := New(context.Background(), WithPlaylists([]db.PlaylistRow{{ID: 1, Name: "Chill", TrackCount: 5}}))

	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	nm := next.(Model)
	next, _ = nm.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	nm = next.(Model)

	beforeView := nm.View()
	beforeLevel := nm.playlists.Level
	beforeCursor := nm.playlists.Cursor

	// Press Esc at ListLevel — must be a no-op.
	next, _ = nm.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	nm = next.(Model)

	if nm.playlists.Level != beforeLevel {
		t.Errorf("Esc at ListLevel changed level: before=%v after=%v", beforeLevel, nm.playlists.Level)
	}
	if nm.playlists.Cursor != beforeCursor {
		t.Errorf("Esc at ListLevel changed cursor: before=%d after=%d", beforeCursor, nm.playlists.Cursor)
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
	m := New(context.Background(), WithPlaylists([]db.PlaylistRow{{ID: 1, Name: "Chill Mix", TrackCount: 3}}))

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

	if !nm.playlists.IsRenaming() {
		t.Fatal("expected isRenaming() to be true after PlaylistRenameMsg")
	}
	if got := nm.playlists.RenameInputValue(); got != "Chill Mix" {
		t.Fatalf("expected rename input pre-populated with %q, got %q", "Chill Mix", got)
	}
}

// TestPlaylistRAtTrackLevelIsNoOp verifies that pressing 'r' while the
// playlists view is at TrackLevel does not activate the rename input — rename
// only applies at list level.
func TestPlaylistRAtTrackLevelIsNoOp(t *testing.T) {
	reader := &stubPlaylistTrackReader{rows: []db.PlaylistTrackRow{{Position: 1, Title: "Track One", Artist: "Artist", DurationMs: 60000}}}
	m := New(context.Background(), WithPlaylists([]db.PlaylistRow{{ID: 1, Name: "Chill Mix", TrackCount: 1}}), WithPlaylistTrackReader(reader))

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

	if nm.playlists.IsRenaming() {
		t.Fatal("isRenaming() must be false when 'r' is pressed at TrackLevel")
	}
}

// TestPlaylistOpenRendersTrackTitles is an integration test that verifies the
// full path from actionPlaylistOpen through playlistTracksLoadedMsg to the
// rendered output. After opening a playlist the track titles must appear in
// the playlists panel.
func TestPlaylistOpenRendersTrackTitles(t *testing.T) {
	reader := &stubPlaylistTrackReader{rows: []db.PlaylistTrackRow{{Position: 1, Title: "Ocean Waves", Artist: "Ambient Co", DurationMs: 240000}, {Position: 2, Title: "Forest Rain", Artist: "Nature Sounds", DurationMs: 180000}}}
	m := New(context.Background(), WithPlaylists([]db.PlaylistRow{{ID: 1, Name: "Chill", TrackCount: 2}}), WithPlaylistTrackReader(reader))

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

// TestPlaylistTrackCursorDownJKey verifies that pressing "j" while the
// playlist track-level view is active advances the cursor by one.
func TestPlaylistTrackCursorDownJKey(t *testing.T) {
	m := New(context.Background())
	m = drillIntoPlaylistTrackLevel(m, []db.PlaylistTrackRow{{ID: 1, Position: 1, Title: "Alpha", Artist: "A", DurationMs: 60000}, {ID: 2, Position: 2, Title: "Beta", Artist: "B", DurationMs: 60000}})

	next, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	nm := next.(Model)
	if nm.playlists.Cursor != 1 {
		t.Errorf("j key at TrackLevel: expected cursor 1, got %d", nm.playlists.Cursor)
	}
}

// TestPlaylistTrackCursorUpKKey verifies that pressing "k" while the playlist
// track-level view is active moves the cursor up by one.
func TestPlaylistTrackCursorUpKKey(t *testing.T) {
	m := New(context.Background())
	m = drillIntoPlaylistTrackLevel(m, []db.PlaylistTrackRow{{ID: 1, Position: 1, Title: "Alpha", Artist: "A", DurationMs: 60000}, {ID: 2, Position: 2, Title: "Beta", Artist: "B", DurationMs: 60000}})
	m.playlists.Cursor = 1

	next, _ := m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	nm := next.(Model)
	if nm.playlists.Cursor != 0 {
		t.Errorf("k key at TrackLevel: expected cursor 0, got %d", nm.playlists.Cursor)
	}
}

// TestPlaylistTrackCursorDownArrow verifies that the down arrow key advances
// the cursor in the playlist track-level view, behaving identically to "j".
func TestPlaylistTrackCursorDownArrow(t *testing.T) {
	m := New(context.Background())
	m = drillIntoPlaylistTrackLevel(m, []db.PlaylistTrackRow{{ID: 1, Position: 1, Title: "Alpha", Artist: "A", DurationMs: 60000}, {ID: 2, Position: 2, Title: "Beta", Artist: "B", DurationMs: 60000}})

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown, Text: "down"})
	nm := next.(Model)
	if nm.playlists.Cursor != 1 {
		t.Errorf("down arrow at TrackLevel: expected cursor 1, got %d", nm.playlists.Cursor)
	}
}

// TestPlaylistTrackCursorUpArrow verifies that the up arrow key moves the
// cursor up in the playlist track-level view, behaving identically to "k".
func TestPlaylistTrackCursorUpArrow(t *testing.T) {
	m := New(context.Background())
	m = drillIntoPlaylistTrackLevel(m, []db.PlaylistTrackRow{{ID: 1, Position: 1, Title: "Alpha", Artist: "A", DurationMs: 60000}, {ID: 2, Position: 2, Title: "Beta", Artist: "B", DurationMs: 60000}})
	m.playlists.Cursor = 1

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp, Text: "up"})
	nm := next.(Model)
	if nm.playlists.Cursor != 0 {
		t.Errorf("up arrow at TrackLevel: expected cursor 0, got %d", nm.playlists.Cursor)
	}
}

// TestPlaylistTrackCursorClampAtBottom verifies that the cursor stops at the
// last track without wrapping when the playlist track-level view is active.
func TestPlaylistTrackCursorClampAtBottom(t *testing.T) {
	m := New(context.Background())
	m = drillIntoPlaylistTrackLevel(m, []db.PlaylistTrackRow{{ID: 1, Position: 1, Title: "Alpha", Artist: "A", DurationMs: 60000}, {ID: 2, Position: 2, Title: "Beta", Artist: "B", DurationMs: 60000}})
	m.playlists.Cursor = 1

	next, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	nm := next.(Model)
	if nm.playlists.Cursor != 1 {
		t.Errorf("j at last track: expected cursor 1 (clamped), got %d", nm.playlists.Cursor)
	}
}

// TestPlaylistTrackCursorClampAtTop verifies that the cursor stops at the
// first track without wrapping when the playlist track-level view is active.
func TestPlaylistTrackCursorClampAtTop(t *testing.T) {
	m := New(context.Background())
	m = drillIntoPlaylistTrackLevel(m, []db.PlaylistTrackRow{{ID: 1, Position: 1, Title: "Alpha", Artist: "A", DurationMs: 60000}, {ID: 2, Position: 2, Title: "Beta", Artist: "B", DurationMs: 60000}})

	next, _ := m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	nm := next.(Model)
	if nm.playlists.Cursor != 0 {
		t.Errorf("k at first track: expected cursor 0 (clamped), got %d", nm.playlists.Cursor)
	}
}
