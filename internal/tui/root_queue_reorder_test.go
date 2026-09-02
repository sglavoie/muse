package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// TestKWhileSortedProducesTransientNotification verifies that pressing 'K'
// while the queue is sorted shows a "Cannot reorder" notification instead of
// silently doing nothing.
func TestKWhileSortedProducesTransientNotification(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewQueue
	m.queue.Entries = []queue.Entry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}
	m.queue.Cursor = 1
	m.queue.DisplayOrder = []int{0, 1}

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'K', Text: "K"})
	nm := next.(Model)
	if cmd == nil {
		t.Fatal("'K' while sorted: expected a cmd producing TransientNotifyMsg")
	}
	next2, _ := nm.Update(cmd())
	nm2 := next2.(Model)
	if nm2.statusBar.TopNotification().Msg == "" {
		t.Fatal("'K' while sorted: expected a transient notification to be set")
	}
	if !strings.Contains(nm2.statusBar.TopNotification().Msg, "Cannot reorder") {
		t.Fatalf("'K' while sorted: unexpected notification %q", nm2.statusBar.TopNotification().Msg)
	}
}

// TestMoveTrackUpMsgCallsReordererWithCorrectIDAndDirection verifies that
// MoveTrackUpMsg swaps the TUI slice and calls ReorderQueueTrack with the
// moved entry's ID and direction -1.
func TestMoveTrackUpMsgCallsReordererWithCorrectIDAndDirection(t *testing.T) {
	r := &stubQueueReorderer{}
	m := New(context.Background(), WithQueueReorderer(r))
	m.queue.Entries = []queue.Entry{
		{ID: 10, Path: "/music/a.flac", Display: "A"},
		{ID: 20, Path: "/music/b.flac", Display: "B"},
		{ID: 30, Path: "/music/c.flac", Display: "C"},
	}

	next, cmd := m.Update(queue.MoveTrackUpMsg{Index: 1})
	nm := next.(Model)

	if nm.queue.Entries[0].ID != 20 || nm.queue.Entries[1].ID != 10 {
		t.Fatalf("expected entries swapped: got IDs %d %d", nm.queue.Entries[0].ID, nm.queue.Entries[1].ID)
	}
	if nm.queue.Cursor != 0 {
		t.Fatalf("expected cursor 0, got %d", nm.queue.Cursor)
	}
	if cmd == nil {
		t.Fatal("MoveTrackUpMsg with reorderer: expected non-nil cmd")
	}
	cmd()

	if len(r.calls) != 1 {
		t.Fatalf("expected 1 reorderer call, got %d", len(r.calls))
	}
	if r.calls[0].id != 20 {
		t.Fatalf("expected entry ID 20, got %d", r.calls[0].id)
	}
	if r.calls[0].direction != -1 {
		t.Fatalf("expected direction -1, got %d", r.calls[0].direction)
	}
}

// TestMoveTrackDownMsgCallsReordererWithCorrectIDAndDirection verifies that
// MoveTrackDownMsg swaps the TUI slice and calls ReorderQueueTrack with the
// moved entry's ID and direction +1.
func TestMoveTrackDownMsgCallsReordererWithCorrectIDAndDirection(t *testing.T) {
	r := &stubQueueReorderer{}
	m := New(context.Background(), WithQueueReorderer(r))
	m.queue.Entries = []queue.Entry{
		{ID: 10, Path: "/music/a.flac", Display: "A"},
		{ID: 20, Path: "/music/b.flac", Display: "B"},
		{ID: 30, Path: "/music/c.flac", Display: "C"},
	}

	next, cmd := m.Update(queue.MoveTrackDownMsg{Index: 1})
	nm := next.(Model)

	if nm.queue.Entries[1].ID != 30 || nm.queue.Entries[2].ID != 20 {
		t.Fatalf("expected entries swapped: got IDs %d %d", nm.queue.Entries[1].ID, nm.queue.Entries[2].ID)
	}
	if nm.queue.Cursor != 2 {
		t.Fatalf("expected cursor 2, got %d", nm.queue.Cursor)
	}
	if cmd == nil {
		t.Fatal("MoveTrackDownMsg with reorderer: expected non-nil cmd")
	}
	cmd()

	if len(r.calls) != 1 {
		t.Fatalf("expected 1 reorderer call, got %d", len(r.calls))
	}
	if r.calls[0].id != 20 {
		t.Fatalf("expected entry ID 20, got %d", r.calls[0].id)
	}
	if r.calls[0].direction != +1 {
		t.Fatalf("expected direction +1, got %d", r.calls[0].direction)
	}
}

// TestMoveTrackUpSyncsPlayerQueue verifies that MoveTrackUpMsg rebuilds the
// player's in-memory queue via SetQueue so that auto-advance uses the new
// ordering after a K reorder.
func TestMoveTrackUpSyncsPlayerQueue(t *testing.T) {
	p := &stubPlayerQueueSyncer{}
	r := &stubQueueReorderer{}
	m := New(context.Background(), WithPlayer(p), WithQueueReorderer(r))
	m.queue.Entries = []queue.Entry{
		{ID: 10, Path: "/a.flac", Hash: "ha", Artist: "A", Title: "Ta"},
		{ID: 20, Path: "/b.flac", Hash: "hb", Artist: "B", Title: "Tb"},
		{ID: 30, Path: "/c.flac", Hash: "hc", Artist: "C", Title: "Tc"},
	}

	next, cmd := m.Update(queue.MoveTrackUpMsg{Index: 1})
	_ = next.(Model)

	if len(p.setQueueCalls) != 1 {
		t.Fatalf("MoveTrackUpMsg must call SetQueue once, got %d calls", len(p.setQueueCalls))
	}
	synced := p.setQueueCalls[0]
	if len(synced) != 3 {
		t.Fatalf("SetQueue called with %d tracks, want 3", len(synced))
	}
	if synced[0].Hash != "hb" || synced[1].Hash != "ha" || synced[2].Hash != "hc" {
		t.Errorf("player queue order after move-up: got hashes %q %q %q, want hb ha hc",
			synced[0].Hash, synced[1].Hash, synced[2].Hash)
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd for DB reorder")
	}
}

// TestMoveTrackDownSyncsPlayerQueue verifies that MoveTrackDownMsg rebuilds
// the player's in-memory queue via SetQueue so that auto-advance uses the
// new ordering after a J reorder.
func TestMoveTrackDownSyncsPlayerQueue(t *testing.T) {
	p := &stubPlayerQueueSyncer{}
	r := &stubQueueReorderer{}
	m := New(context.Background(), WithPlayer(p), WithQueueReorderer(r))
	m.queue.Entries = []queue.Entry{
		{ID: 10, Path: "/a.flac", Hash: "ha", Artist: "A", Title: "Ta"},
		{ID: 20, Path: "/b.flac", Hash: "hb", Artist: "B", Title: "Tb"},
		{ID: 30, Path: "/c.flac", Hash: "hc", Artist: "C", Title: "Tc"},
	}

	next, cmd := m.Update(queue.MoveTrackDownMsg{Index: 1})
	_ = next.(Model)

	if len(p.setQueueCalls) != 1 {
		t.Fatalf("MoveTrackDownMsg must call SetQueue once, got %d calls", len(p.setQueueCalls))
	}
	synced := p.setQueueCalls[0]
	if len(synced) != 3 {
		t.Fatalf("SetQueue called with %d tracks, want 3", len(synced))
	}
	if synced[0].Hash != "ha" || synced[1].Hash != "hc" || synced[2].Hash != "hb" {
		t.Errorf("player queue order after move-down: got hashes %q %q %q, want ha hc hb",
			synced[0].Hash, synced[1].Hash, synced[2].Hash)
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd for DB reorder")
	}
}

// TestMoveTrackUpAdjustsScrollOffset verifies that moving a track up when the
// cursor is at the top of the visible area adjusts scrollOffset so the cursor
// remains visible.
func TestMoveTrackUpAdjustsScrollOffset(t *testing.T) {
	m := New(context.Background())
	m.queue.Entries = make([]queue.Entry, 10)
	for i := range m.queue.Entries {
		m.queue.Entries[i] = queue.Entry{
			ID:      int64(i + 1),
			Path:    "/track" + string(rune('a'+i)) + ".flac",
			Hash:    "hash" + string(rune('a'+i)),
			Display: "Track " + string(rune('A'+i)),
		}
	}
	m.queue.Height = 7
	m.queue.Width = 80
	m.queue.Cursor = 5
	m.queue.ScrollOffset = 5

	m, _ = m.handleMoveTrackUpMsg(queue.MoveTrackUpMsg{Index: 5})

	if m.queue.Cursor != 4 {
		t.Errorf("expected cursor=4 after move up, got %d", m.queue.Cursor)
	}
	if m.queue.ScrollOffset > m.queue.Cursor {
		t.Errorf("scrollOffset (%d) must be <= cursor (%d) after move up",
			m.queue.ScrollOffset, m.queue.Cursor)
	}
}

// TestMoveTrackDownAdjustsScrollOffset verifies that moving a track down when
// the cursor would go below the visible area adjusts scrollOffset so the
// cursor remains visible.
func TestMoveTrackDownAdjustsScrollOffset(t *testing.T) {
	m := New(context.Background())
	m.queue.Entries = make([]queue.Entry, 10)
	for i := range m.queue.Entries {
		m.queue.Entries[i] = queue.Entry{
			ID:      int64(i + 1),
			Path:    "/track" + string(rune('a'+i)) + ".flac",
			Hash:    "hash" + string(rune('a'+i)),
			Display: "Track " + string(rune('A'+i)),
		}
	}
	m.queue.Height = 7
	m.queue.Width = 80
	m.queue.ScrollOffset = 0

	maxRows := m.queue.VisibleRows()
	m.queue.Cursor = maxRows - 1

	m, _ = m.handleMoveTrackDownMsg(queue.MoveTrackDownMsg{Index: m.queue.Cursor})

	if m.queue.Cursor != maxRows {
		t.Errorf("expected cursor=%d after move down, got %d", maxRows, m.queue.Cursor)
	}
	if m.queue.Cursor >= m.queue.ScrollOffset+maxRows {
		t.Errorf("cursor (%d) must be < scrollOffset+maxRows (%d) after move down",
			m.queue.Cursor, m.queue.ScrollOffset+maxRows)
	}
}

// TestMoveTrackUpExitsVisualMode verifies that moving a track up exits
// visual mode so the index-based selection snapshot does not become stale.
func TestMoveTrackUpExitsVisualMode(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.ActiveView = viewQueue
	m.queue.Entries = sampleQueueEntries()
	m.queue.Cursor = 1

	m = doVisual(m)
	if !m.visual.active {
		t.Fatal("expected visual mode active after entering")
	}

	m, _ = m.handleMoveTrackUpMsg(queue.MoveTrackUpMsg{Index: 1})

	if m.visual.active {
		t.Fatal("expected visual mode to exit after move-up")
	}
}

// TestMoveTrackDownExitsVisualMode verifies that moving a track down exits
// visual mode so the index-based selection snapshot does not become stale.
func TestMoveTrackDownExitsVisualMode(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.ActiveView = viewQueue
	m.queue.Entries = sampleQueueEntries()
	m.queue.Cursor = 1

	m = doVisual(m)
	if !m.visual.active {
		t.Fatal("expected visual mode active after entering")
	}

	m, _ = m.handleMoveTrackDownMsg(queue.MoveTrackDownMsg{Index: 1})

	if m.visual.active {
		t.Fatal("expected visual mode to exit after move-down")
	}
}
