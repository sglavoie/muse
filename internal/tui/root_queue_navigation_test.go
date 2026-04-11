package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestQueueCursorDownJKey verifies that pressing "j" while the queue view is
// active advances the cursor by one.
func TestQueueCursorDownJKey(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	nm := next.(Model)
	if nm.queue.cursor != 1 {
		t.Errorf("j key: expected cursor 1, got %d", nm.queue.cursor)
	}
}

// TestQueueCursorUpKKey verifies that pressing "k" while the queue view is
// active moves the cursor up by one.
func TestQueueCursorUpKKey(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}
	m.queue.cursor = 1

	next, _ := m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	nm := next.(Model)
	if nm.queue.cursor != 0 {
		t.Errorf("k key: expected cursor 0, got %d", nm.queue.cursor)
	}
}

// TestQueueCursorDownArrow verifies that the down arrow key advances the
// cursor in the queue view, behaving identically to "j".
func TestQueueCursorDownArrow(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown, Text: "down"})
	nm := next.(Model)
	if nm.queue.cursor != 1 {
		t.Errorf("down arrow: expected cursor 1, got %d", nm.queue.cursor)
	}
}

// TestQueueCursorUpArrow verifies that the up arrow key moves the cursor up
// in the queue view, behaving identically to "k".
func TestQueueCursorUpArrow(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}
	m.queue.cursor = 1

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp, Text: "up"})
	nm := next.(Model)
	if nm.queue.cursor != 0 {
		t.Errorf("up arrow: expected cursor 0, got %d", nm.queue.cursor)
	}
}

// TestQueueCursorClampAtBottom verifies that the cursor stops at the last
// entry without wrapping when the queue view is active.
func TestQueueCursorClampAtBottom(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}
	m.queue.cursor = 1

	next, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	nm := next.(Model)
	if nm.queue.cursor != 1 {
		t.Errorf("j at last: expected cursor 1 (clamped), got %d", nm.queue.cursor)
	}
}

// TestQueueCursorClampAtTop verifies that the cursor stops at the first entry
// without wrapping when the queue view is active.
func TestQueueCursorClampAtTop(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	nm := next.(Model)
	if nm.queue.cursor != 0 {
		t.Errorf("k at first: expected cursor 0 (clamped), got %d", nm.queue.cursor)
	}
}

// TestFocusNowPlayingMovesCursorToCurrentTrack verifies that pressing "f" in
// the queue view moves the cursor to the entry matching currentTrackHash.
func TestFocusNowPlayingMovesCursorToCurrentTrack(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Hash: "aaa", Display: "A"},
		{Path: "/music/b.flac", Hash: "bbb", Display: "B"},
		{Path: "/music/c.flac", Hash: "ccc", Display: "C"},
		{Path: "/music/d.flac", Hash: "ddd", Display: "D"},
		{Path: "/music/e.flac", Hash: "eee", Display: "E"},
	}
	m.playback.currentHash = "ccc"
	m.playback.currentIndex = 2
	m.queue.cursor = 0

	next, _ := m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	nm := next.(Model)

	if nm.queue.cursor != 2 {
		t.Errorf("f key: expected cursor 2 (track ccc), got %d", nm.queue.cursor)
	}
}

// TestFocusNowPlayingNoOpWhenNothingPlaying verifies that pressing "f" does
// nothing when no track is currently playing (empty currentTrackHash).
func TestFocusNowPlayingNoOpWhenNothingPlaying(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Hash: "aaa", Display: "A"},
		{Path: "/music/b.flac", Hash: "bbb", Display: "B"},
	}
	m.playback.currentHash = ""
	m.queue.cursor = 1

	next, _ := m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	nm := next.(Model)

	if nm.queue.cursor != 1 {
		t.Errorf("f key with no playing track: expected cursor unchanged at 1, got %d", nm.queue.cursor)
	}
}
