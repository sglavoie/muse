package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// TestQueueCursorDownJKey verifies that pressing "j" while the queue view is
// active advances the cursor by one.
func TestQueueCursorDownJKey(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewQueue
	m.queue.Entries = []queue.Entry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	nm := next.(Model)
	if nm.queue.Cursor != 1 {
		t.Errorf("j key: expected cursor 1, got %d", nm.queue.Cursor)
	}
}

// TestQueueCursorUpKKey verifies that pressing "k" while the queue view is
// active moves the cursor up by one.
func TestQueueCursorUpKKey(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewQueue
	m.queue.Entries = []queue.Entry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}
	m.queue.Cursor = 1

	next, _ := m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	nm := next.(Model)
	if nm.queue.Cursor != 0 {
		t.Errorf("k key: expected cursor 0, got %d", nm.queue.Cursor)
	}
}

// TestQueueCursorDownArrow verifies that the down arrow key advances the
// cursor in the queue view, behaving identically to "j".
func TestQueueCursorDownArrow(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewQueue
	m.queue.Entries = []queue.Entry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown, Text: "down"})
	nm := next.(Model)
	if nm.queue.Cursor != 1 {
		t.Errorf("down arrow: expected cursor 1, got %d", nm.queue.Cursor)
	}
}

// TestQueueCursorUpArrow verifies that the up arrow key moves the cursor up
// in the queue view, behaving identically to "k".
func TestQueueCursorUpArrow(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewQueue
	m.queue.Entries = []queue.Entry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}
	m.queue.Cursor = 1

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp, Text: "up"})
	nm := next.(Model)
	if nm.queue.Cursor != 0 {
		t.Errorf("up arrow: expected cursor 0, got %d", nm.queue.Cursor)
	}
}

// TestQueueCursorClampAtBottom verifies that the cursor stops at the last
// entry without wrapping when the queue view is active.
func TestQueueCursorClampAtBottom(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewQueue
	m.queue.Entries = []queue.Entry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}
	m.queue.Cursor = 1

	next, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	nm := next.(Model)
	if nm.queue.Cursor != 1 {
		t.Errorf("j at last: expected cursor 1 (clamped), got %d", nm.queue.Cursor)
	}
}

// TestQueueCursorClampAtTop verifies that the cursor stops at the first entry
// without wrapping when the queue view is active.
func TestQueueCursorClampAtTop(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewQueue
	m.queue.Entries = []queue.Entry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	nm := next.(Model)
	if nm.queue.Cursor != 0 {
		t.Errorf("k at first: expected cursor 0 (clamped), got %d", nm.queue.Cursor)
	}
}

// TestFocusNowPlayingMovesCursorToCurrentTrack verifies that pressing "f" in
// the queue view moves the cursor to the entry matching currentTrackHash.
func TestFocusNowPlayingMovesCursorToCurrentTrack(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewQueue
	m.queue.Entries = []queue.Entry{
		{Path: "/music/a.flac", Hash: "aaa", Display: "A"},
		{Path: "/music/b.flac", Hash: "bbb", Display: "B"},
		{Path: "/music/c.flac", Hash: "ccc", Display: "C"},
		{Path: "/music/d.flac", Hash: "ddd", Display: "D"},
		{Path: "/music/e.flac", Hash: "eee", Display: "E"},
	}
	m.playback.currentHash = "ccc"
	m.playback.currentIndex = 2
	m.queue.Cursor = 0

	next, _ := m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	nm := next.(Model)

	if nm.queue.Cursor != 2 {
		t.Errorf("f key: expected cursor 2 (track ccc), got %d", nm.queue.Cursor)
	}
}

// TestFocusNowPlayingFromOtherView verifies that pressing "f" from a non-Queue
// view switches to the Queue view and focuses the currently playing track.
func TestFocusNowPlayingFromOtherView(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewLibrary
	m.queue.Entries = []queue.Entry{
		{Path: "/music/a.flac", Hash: "aaa", Display: "A"},
		{Path: "/music/b.flac", Hash: "bbb", Display: "B"},
		{Path: "/music/c.flac", Hash: "ccc", Display: "C"},
	}
	m.playback.currentHash = "bbb"
	m.playback.currentIndex = 1
	m.queue.Cursor = 0

	next, _ := m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	nm := next.(Model)

	if nm.ActiveView != viewQueue {
		t.Errorf("f from Library: expected activeView=viewQueue, got %d", nm.ActiveView)
	}
	if nm.queue.Cursor != 1 {
		t.Errorf("f from Library: expected cursor 1 (track bbb), got %d", nm.queue.Cursor)
	}
}

// TestFocusNowPlayingNoOpWhenNothingPlaying verifies that pressing "f" does
// nothing when no track is currently playing (empty currentTrackHash).
func TestFocusNowPlayingNoOpWhenNothingPlaying(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewQueue
	m.queue.Entries = []queue.Entry{
		{Path: "/music/a.flac", Hash: "aaa", Display: "A"},
		{Path: "/music/b.flac", Hash: "bbb", Display: "B"},
	}
	m.playback.currentHash = ""
	m.queue.Cursor = 1

	next, _ := m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	nm := next.(Model)

	if nm.queue.Cursor != 1 {
		t.Errorf("f key with no playing track: expected cursor unchanged at 1, got %d", nm.queue.Cursor)
	}
	if got := nm.statusBar.TopNotification().Msg; got != "Load a track first" {
		t.Errorf("f key with no playing track: notification = %q, want %q", got, "Load a track first")
	}
}

// TestFocusNowPlayingExplainsFilteredOutTrack verifies that pressing "f" while
// the queue filter hides the playing row leaves the cursor and says why.
func TestFocusNowPlayingExplainsFilteredOutTrack(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewQueue
	m.queue.Entries = []queue.Entry{
		{Path: "/music/a.flac", Hash: "aaa", Display: "A"},
		{Path: "/music/b.flac", Hash: "bbb", Display: "B"},
	}
	m.queue.TagFilterHashes = map[string]struct{}{"aaa": {}}
	m.playback.currentHash = "bbb"
	m.playback.currentIndex = 1

	next, _ := m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	nm := next.(Model)

	if nm.queue.Cursor != 0 {
		t.Errorf("expected cursor unchanged at 0, got %d", nm.queue.Cursor)
	}
	if got := nm.statusBar.TopNotification().Msg; got != "Now playing is hidden by the queue filter" {
		t.Errorf("notification = %q, want the filtered-out hint", got)
	}
}
