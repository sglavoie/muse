package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/statusbar"
)

// stubPlaylistDeleterReorderer implements both playlistDeleter and
// playlistTrackReorderer so the type assertion in handlePlaylistMoveTrack
// succeeds during tests.
type stubPlaylistDeleterReorderer struct {
	stubPlaylistDeleter
	reorderCalls []struct {
		playlistID int64
		trackID    int64
		direction  int
	}
}

func (s *stubPlaylistDeleterReorderer) ReorderPlaylistTrack(playlistID, trackID int64, direction int) error {
	s.reorderCalls = append(s.reorderCalls, struct {
		playlistID int64
		trackID    int64
		direction  int
	}{playlistID, trackID, direction})
	return nil
}

func samplePlaylistTracks() []db.PlaylistTrackRow {
	return []db.PlaylistTrackRow{
		{ID: 10, Hash: "ha", Title: "A"},
		{ID: 20, Hash: "hb", Title: "B"},
		{ID: 30, Hash: "hc", Title: "C"},
	}
}

func playlistModelAtTrackLevel(m Model, tracks []db.PlaylistTrackRow, playlistID int64) Model {
	m.activeView = viewPlaylists
	m.playlists.Level = pltui.TrackLevel
	m.playlists.Tracks = tracks
	m.playlists.PendingLoadID = playlistID
	m.playlists.Cursor = 0
	return m
}

// --- Move Up ---

func TestPlaylistMoveTrackUp_SwapsAndCursorMoves(t *testing.T) {
	pd := &stubPlaylistDeleterReorderer{}
	m := New(context.Background(), WithPlaylistDeleter(pd))
	m = playlistModelAtTrackLevel(m, samplePlaylistTracks(), 42)
	m.playlists.Cursor = 1

	m, cmd := m.handlePlaylistMoveTrackUp()

	if m.playlists.Tracks[0].ID != 20 || m.playlists.Tracks[1].ID != 10 {
		t.Fatalf("expected tracks swapped: got IDs %d %d", m.playlists.Tracks[0].ID, m.playlists.Tracks[1].ID)
	}
	if m.playlists.Cursor != 0 {
		t.Fatalf("expected cursor 0, got %d", m.playlists.Cursor)
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd for DB persist")
	}
	cmd()
	if len(pd.reorderCalls) != 1 {
		t.Fatalf("expected 1 reorder call, got %d", len(pd.reorderCalls))
	}
	if pd.reorderCalls[0].trackID != 20 || pd.reorderCalls[0].direction != -1 {
		t.Fatalf("expected reorder(20, -1), got (%d, %d)", pd.reorderCalls[0].trackID, pd.reorderCalls[0].direction)
	}
	if pd.reorderCalls[0].playlistID != 42 {
		t.Fatalf("expected playlistID 42, got %d", pd.reorderCalls[0].playlistID)
	}
}

func TestPlaylistMoveTrackUp_AtTopIsNoOp(t *testing.T) {
	m := New(context.Background())
	m = playlistModelAtTrackLevel(m, samplePlaylistTracks(), 1)
	m.playlists.Cursor = 0

	m, cmd := m.handlePlaylistMoveTrackUp()

	if cmd != nil {
		t.Fatal("expected nil cmd when cursor at top")
	}
	if m.playlists.Tracks[0].ID != 10 {
		t.Fatal("tracks should not have changed")
	}
}

// --- Move Down ---

func TestPlaylistMoveTrackDown_SwapsAndCursorMoves(t *testing.T) {
	pd := &stubPlaylistDeleterReorderer{}
	m := New(context.Background(), WithPlaylistDeleter(pd))
	m = playlistModelAtTrackLevel(m, samplePlaylistTracks(), 42)
	m.playlists.Cursor = 1

	m, cmd := m.handlePlaylistMoveTrackDown()

	if m.playlists.Tracks[1].ID != 30 || m.playlists.Tracks[2].ID != 20 {
		t.Fatalf("expected tracks swapped: got IDs %d %d", m.playlists.Tracks[1].ID, m.playlists.Tracks[2].ID)
	}
	if m.playlists.Cursor != 2 {
		t.Fatalf("expected cursor 2, got %d", m.playlists.Cursor)
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd for DB persist")
	}
	cmd()
	if len(pd.reorderCalls) != 1 {
		t.Fatalf("expected 1 reorder call, got %d", len(pd.reorderCalls))
	}
	if pd.reorderCalls[0].trackID != 20 || pd.reorderCalls[0].direction != +1 {
		t.Fatalf("expected reorder(20, +1), got (%d, %d)", pd.reorderCalls[0].trackID, pd.reorderCalls[0].direction)
	}
}

func TestPlaylistMoveTrackDown_AtBottomIsNoOp(t *testing.T) {
	m := New(context.Background())
	m = playlistModelAtTrackLevel(m, samplePlaylistTracks(), 1)
	m.playlists.Cursor = 2

	m, cmd := m.handlePlaylistMoveTrackDown()

	if cmd != nil {
		t.Fatal("expected nil cmd when cursor at bottom")
	}
	if m.playlists.Tracks[2].ID != 30 {
		t.Fatal("tracks should not have changed")
	}
}

// --- Guards ---

func TestPlaylistMoveTrack_SmartPlaylistIsNoOp(t *testing.T) {
	m := New(context.Background())
	m = playlistModelAtTrackLevel(m, samplePlaylistTracks(), 1)
	m.playlists.Cursor = 1
	q := db.SmartPlaylistQuery{}
	m.playlists.SmartQuery = &q

	m, cmdUp := m.handlePlaylistMoveTrackUp()
	if cmdUp != nil {
		t.Fatal("expected nil cmd for smart playlist move-up")
	}
	m, cmdDown := m.handlePlaylistMoveTrackDown()
	if cmdDown != nil {
		t.Fatal("expected nil cmd for smart playlist move-down")
	}
}

func TestPlaylistMoveTrack_SortedShowsNotification(t *testing.T) {
	m := New(context.Background())
	m = playlistModelAtTrackLevel(m, samplePlaylistTracks(), 1)
	m.playlists.Cursor = 1
	m.playlists.SortedTrackOrder = []int{0, 1, 2}

	_, cmd := m.handlePlaylistMoveTrackUp()
	if cmd == nil {
		t.Fatal("expected cmd for sorted notification")
	}
	msg := cmd()
	note, ok := msg.(statusbar.TransientNotifyMsg)
	if !ok {
		t.Fatalf("expected TransientNotifyMsg, got %T", msg)
	}
	if !strings.Contains(string(note), "Cannot reorder") {
		t.Fatalf("unexpected notification: %q", string(note))
	}
}

func TestPlaylistMoveTrack_AtListLevelIsNoOp(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewPlaylists
	// level defaults to ListLevel
	m.playlists.Cursor = 0

	_, cmdUp := m.handlePlaylistMoveTrackUp()
	if cmdUp != nil {
		t.Fatal("expected nil cmd at list level for move-up")
	}
	_, cmdDown := m.handlePlaylistMoveTrackDown()
	if cmdDown != nil {
		t.Fatal("expected nil cmd at list level for move-down")
	}
}

func TestPlaylistMoveTrackUp_ExitsVisualMode(t *testing.T) {
	m := New(context.Background())
	m = playlistModelAtTrackLevel(m, samplePlaylistTracks(), 1)
	m.playlists.Cursor = 1
	m = doVisual(m)
	if !m.visual.active {
		t.Fatal("expected visual mode active")
	}

	m, _ = m.handlePlaylistMoveTrackUp()

	if m.visual.active {
		t.Fatal("expected visual mode to exit after move-up")
	}
}

func TestPlaylistMoveTrackDown_ExitsVisualMode(t *testing.T) {
	m := New(context.Background())
	m = playlistModelAtTrackLevel(m, samplePlaylistTracks(), 1)
	m.playlists.Cursor = 1
	m = doVisual(m)
	if !m.visual.active {
		t.Fatal("expected visual mode active")
	}

	m, _ = m.handlePlaylistMoveTrackDown()

	if m.visual.active {
		t.Fatal("expected visual mode to exit after move-down")
	}
}

func TestPlaylistMoveTrackUp_AdjustsScrollOffset(t *testing.T) {
	m := New(context.Background())
	tracks := make([]db.PlaylistTrackRow, 10)
	for i := range tracks {
		tracks[i] = db.PlaylistTrackRow{ID: int64(i + 1), Hash: "h", Title: "T"}
	}
	m = playlistModelAtTrackLevel(m, tracks, 1)
	m.playlists.Cursor = 5
	m.playlists.ScrollOffset = 5

	m, _ = m.handlePlaylistMoveTrackUp()

	if m.playlists.Cursor != 4 {
		t.Fatalf("expected cursor 4, got %d", m.playlists.Cursor)
	}
	if m.playlists.ScrollOffset > m.playlists.Cursor {
		t.Fatalf("scrollOffset (%d) must be <= cursor (%d)", m.playlists.ScrollOffset, m.playlists.Cursor)
	}
}

func TestPlaylistMoveTrackDown_AdjustsScrollOffset(t *testing.T) {
	m := New(context.Background())
	tracks := make([]db.PlaylistTrackRow, 10)
	for i := range tracks {
		tracks[i] = db.PlaylistTrackRow{ID: int64(i + 1), Hash: "h", Title: "T"}
	}
	m = playlistModelAtTrackLevel(m, tracks, 1)
	m.playlists.Height = 7
	m.playlists.Cursor = 3 // will push past visible area with height=7, reserve=3 → rows=4
	m.playlists.ScrollOffset = 0

	m, _ = m.handlePlaylistMoveTrackDown()

	if m.playlists.Cursor != 4 {
		t.Fatalf("expected cursor 4, got %d", m.playlists.Cursor)
	}
	rows := max(1, m.playlists.Height-3) // reserve=3
	if m.playlists.Cursor >= m.playlists.ScrollOffset+rows {
		t.Fatalf("cursor (%d) must be < scrollOffset+rows (%d)", m.playlists.Cursor, m.playlists.ScrollOffset+rows)
	}
}
