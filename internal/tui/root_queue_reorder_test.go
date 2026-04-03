package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestKWhileSortedProducesTransientNotification verifies that pressing 'K'
// while the queue is sorted shows a "Cannot reorder" notification instead of
// silently doing nothing.
func TestKWhileSortedProducesTransientNotification(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}
	m.queue.cursor = 1
	m.queue.displayOrder = []int{0, 1}

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'K', Text: "K"})
	nm := next.(Model)
	if cmd == nil {
		t.Fatal("'K' while sorted: expected a cmd producing TransientNotifyMsg")
	}
	next2, _ := nm.Update(cmd())
	nm2 := next2.(Model)
	if nm2.statusBar.notification.msg == "" {
		t.Fatal("'K' while sorted: expected a transient notification to be set")
	}
	if !strings.Contains(nm2.statusBar.notification.msg, "Cannot reorder") {
		t.Fatalf("'K' while sorted: unexpected notification %q", nm2.statusBar.notification.msg)
	}
}

// TestMoveTrackUpMsgCallsReordererWithCorrectIDAndDirection verifies that
// MoveTrackUpMsg swaps the TUI slice and calls ReorderQueueTrack with the
// moved entry's ID and direction -1.
func TestMoveTrackUpMsgCallsReordererWithCorrectIDAndDirection(t *testing.T) {
	r := &stubQueueReorderer{}
	m := New(context.Background(), WithQueueReorderer(r))
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A"},
		{ID: 20, Path: "/music/b.flac", Display: "B"},
		{ID: 30, Path: "/music/c.flac", Display: "C"},
	}

	next, cmd := m.Update(MoveTrackUpMsg{Index: 1})
	nm := next.(Model)

	if nm.queue.entries[0].ID != 20 || nm.queue.entries[1].ID != 10 {
		t.Fatalf("expected entries swapped: got IDs %d %d", nm.queue.entries[0].ID, nm.queue.entries[1].ID)
	}
	if nm.queue.cursor != 0 {
		t.Fatalf("expected cursor 0, got %d", nm.queue.cursor)
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
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A"},
		{ID: 20, Path: "/music/b.flac", Display: "B"},
		{ID: 30, Path: "/music/c.flac", Display: "C"},
	}

	next, cmd := m.Update(MoveTrackDownMsg{Index: 1})
	nm := next.(Model)

	if nm.queue.entries[1].ID != 30 || nm.queue.entries[2].ID != 20 {
		t.Fatalf("expected entries swapped: got IDs %d %d", nm.queue.entries[1].ID, nm.queue.entries[2].ID)
	}
	if nm.queue.cursor != 2 {
		t.Fatalf("expected cursor 2, got %d", nm.queue.cursor)
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
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/a.flac", Hash: "ha", Artist: "A", Title: "Ta"},
		{ID: 20, Path: "/b.flac", Hash: "hb", Artist: "B", Title: "Tb"},
		{ID: 30, Path: "/c.flac", Hash: "hc", Artist: "C", Title: "Tc"},
	}

	next, cmd := m.Update(MoveTrackUpMsg{Index: 1})
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
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/a.flac", Hash: "ha", Artist: "A", Title: "Ta"},
		{ID: 20, Path: "/b.flac", Hash: "hb", Artist: "B", Title: "Tb"},
		{ID: 30, Path: "/c.flac", Hash: "hc", Artist: "C", Title: "Tc"},
	}

	next, cmd := m.Update(MoveTrackDownMsg{Index: 1})
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
	m.queue.entries = make([]QueueEntry, 10)
	for i := range m.queue.entries {
		m.queue.entries[i] = QueueEntry{
			ID:      int64(i + 1),
			Path:    "/track" + string(rune('a'+i)) + ".flac",
			Hash:    "hash" + string(rune('a'+i)),
			Display: "Track " + string(rune('A'+i)),
		}
	}
	m.queue.height = 7
	m.queue.width = 80
	m.queue.cursor = 5
	m.queue.scrollOffset = 5

	m, _ = m.handleMoveTrackUpMsg(MoveTrackUpMsg{Index: 5})

	if m.queue.cursor != 4 {
		t.Errorf("expected cursor=4 after move up, got %d", m.queue.cursor)
	}
	if m.queue.scrollOffset > m.queue.cursor {
		t.Errorf("scrollOffset (%d) must be <= cursor (%d) after move up",
			m.queue.scrollOffset, m.queue.cursor)
	}
}

// TestMoveTrackDownAdjustsScrollOffset verifies that moving a track down when
// the cursor would go below the visible area adjusts scrollOffset so the
// cursor remains visible.
func TestMoveTrackDownAdjustsScrollOffset(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = make([]QueueEntry, 10)
	for i := range m.queue.entries {
		m.queue.entries[i] = QueueEntry{
			ID:      int64(i + 1),
			Path:    "/track" + string(rune('a'+i)) + ".flac",
			Hash:    "hash" + string(rune('a'+i)),
			Display: "Track " + string(rune('A'+i)),
		}
	}
	m.queue.height = 7
	m.queue.width = 80
	m.queue.scrollOffset = 0

	maxRows := m.queue.visibleRows()
	m.queue.cursor = maxRows - 1

	m, _ = m.handleMoveTrackDownMsg(MoveTrackDownMsg{Index: m.queue.cursor})

	if m.queue.cursor != maxRows {
		t.Errorf("expected cursor=%d after move down, got %d", maxRows, m.queue.cursor)
	}
	if m.queue.cursor >= m.queue.scrollOffset+maxRows {
		t.Errorf("cursor (%d) must be < scrollOffset+maxRows (%d) after move down",
			m.queue.cursor, m.queue.scrollOffset+maxRows)
	}
}
