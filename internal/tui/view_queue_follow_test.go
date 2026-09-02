package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// makeFollowQueue builds a queue with n entries and a fixed height so
// visibleRows() returns a known value for follow-scroll math tests.
func makeFollowQueue(n, height int) queue.View {
	entries := make([]queue.Entry, n)
	for i := range entries {
		entries[i] = queue.Entry{
			Path:    "/music/track.flac",
			Hash:    string(rune('a'+i%26)) + string(rune('a'+i/26)),
			Display: "T",
		}
	}
	return queue.View{Entries: entries, Height: height}
}

// TestScrollToNowPlayingPlacesRowAtOneThird verifies that the now-playing
// row is positioned ~⅓ from the top of the visible window.
func TestScrollToNowPlayingPlacesRowAtOneThird(t *testing.T) {
	q := makeFollowQueue(30, 12) // visibleRows = 9, maxRows/3 = 3
	q2 := q.ScrollToNowPlaying(10)
	if q2.ScrollOffset != 10-3 {
		t.Errorf("scrollOffset: expected %d, got %d", 10-3, q2.ScrollOffset)
	}
}

// TestScrollToNowPlayingClampsToZero verifies that when the now-playing
// row is near the top of the queue the offset clamps to zero.
func TestScrollToNowPlayingClampsToZero(t *testing.T) {
	q := makeFollowQueue(30, 12)
	q2 := q.ScrollToNowPlaying(1)
	if q2.ScrollOffset != 0 {
		t.Errorf("near-top: expected scrollOffset 0, got %d", q2.ScrollOffset)
	}
}

// TestScrollToNowPlayingClampsToMaxOffset verifies that when the
// now-playing row is near the tail of the queue the offset clamps so the
// list does not scroll past the last row.
func TestScrollToNowPlayingClampsToMaxOffset(t *testing.T) {
	q := makeFollowQueue(30, 12) // visibleRows = 9
	q2 := q.ScrollToNowPlaying(29)
	// maxOffset = 30 - 9 = 21
	if q2.ScrollOffset != 21 {
		t.Errorf("near-tail: expected scrollOffset 21, got %d", q2.ScrollOffset)
	}
}

// TestScrollToNowPlayingNegativeIndex verifies that a negative
// currentPlayingIndex is a no-op (no playing track).
func TestScrollToNowPlayingNegativeIndex(t *testing.T) {
	q := makeFollowQueue(30, 12)
	q.ScrollOffset = 5
	q2 := q.ScrollToNowPlaying(-1)
	if q2.ScrollOffset != 5 {
		t.Errorf("negative idx: expected scrollOffset unchanged at 5, got %d", q2.ScrollOffset)
	}
}

// TestScrollToNowPlayingDoesNotMoveCursor verifies follow scrolling only
// adjusts scrollOffset, not the cursor.
func TestScrollToNowPlayingDoesNotMoveCursor(t *testing.T) {
	q := makeFollowQueue(30, 12)
	q.Cursor = 0
	q2 := q.ScrollToNowPlaying(15)
	if q2.Cursor != 0 {
		t.Errorf("cursor moved: expected 0, got %d", q2.Cursor)
	}
}

// TestFocusNowPlayingClampsScrollToMaxOffset verifies that focusing a row near
// the tail does not scroll beyond the last full viewport.
func TestFocusNowPlayingClampsScrollToMaxOffset(t *testing.T) {
	q := makeFollowQueue(11, 12) // visibleRows = 9, maxOffset = 2
	q2 := q.FocusNowPlaying(10)
	if q2.Cursor != 10 {
		t.Fatalf("cursor: expected 10, got %d", q2.Cursor)
	}
	if q2.ScrollOffset != 2 {
		t.Errorf("scrollOffset: expected 2, got %d", q2.ScrollOffset)
	}
}

// TestTrackChangedScrollsWhenFollowingAndQueueActive verifies the on-track-change
// hook adjusts scrollOffset when follow mode is on and the Queue view is active.
func TestTrackChangedScrollsWhenFollowingAndQueueActive(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewQueue
	m.followNowPlaying = true
	m.queue = makeFollowQueue(30, 12)
	// Pre-populate hashes so the index match works.
	for i := range m.queue.Entries {
		m.queue.Entries[i].Hash = "h" + string(rune('A'+i))
	}

	msg := playback.TrackChangedMsg{
		Hash:  m.queue.Entries[15].Hash,
		Index: 15,
	}
	m2, _ := m.handleTrackChangedMsg(msg)

	// visibleRows = 9; target = 15 - 3 = 12
	if m2.queue.ScrollOffset != 12 {
		t.Errorf("scroll on track change: expected scrollOffset 12, got %d", m2.queue.ScrollOffset)
	}
}

// TestTrackChangedDoesNotScrollWhenFollowDisabled verifies that with follow
// mode off, scrollOffset is preserved across a track change.
func TestTrackChangedDoesNotScrollWhenFollowDisabled(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewQueue
	m.followNowPlaying = false
	m.queue = makeFollowQueue(30, 12)
	for i := range m.queue.Entries {
		m.queue.Entries[i].Hash = "h" + string(rune('A'+i))
	}
	m.queue.ScrollOffset = 4

	msg := playback.TrackChangedMsg{
		Hash:  m.queue.Entries[15].Hash,
		Index: 15,
	}
	m2, _ := m.handleTrackChangedMsg(msg)

	if m2.queue.ScrollOffset != 4 {
		t.Errorf("scroll preserved off: expected scrollOffset 4, got %d", m2.queue.ScrollOffset)
	}
}

// TestTrackChangedDoesNotScrollWhenQueueInactive verifies follow scrolling
// is gated by the active view — track changes while in Library or Browse
// must not touch the queue scroll position.
func TestTrackChangedDoesNotScrollWhenQueueInactive(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewLibrary
	m.followNowPlaying = true
	m.queue = makeFollowQueue(30, 12)
	for i := range m.queue.Entries {
		m.queue.Entries[i].Hash = "h" + string(rune('A'+i))
	}
	m.queue.ScrollOffset = 4

	msg := playback.TrackChangedMsg{
		Hash:  m.queue.Entries[15].Hash,
		Index: 15,
	}
	m2, _ := m.handleTrackChangedMsg(msg)

	if m2.queue.ScrollOffset != 4 {
		t.Errorf("scroll preserved when queue inactive: expected 4, got %d", m2.queue.ScrollOffset)
	}
}

// TestSwitchToQueueScrollsWhenFollowing verifies that entering the Queue
// view re-anchors the scroll on the now-playing row.
func TestSwitchToQueueScrollsWhenFollowing(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewLibrary
	m.followNowPlaying = true
	m.queue = makeFollowQueue(30, 12)
	for i := range m.queue.Entries {
		m.queue.Entries[i].Hash = "h" + string(rune('A'+i))
	}
	m.playback.currentIndex = 20
	m.queue.ScrollOffset = 0

	m2 := m.switchToView(viewQueue)

	// target = 20 - 3 = 17
	if m2.queue.ScrollOffset != 17 {
		t.Errorf("switchToView(viewQueue): expected scrollOffset 17, got %d", m2.queue.ScrollOffset)
	}
}

// TestSwitchToQueueDoesNotScrollWhenFollowDisabled verifies that switching
// to the Queue view with follow disabled preserves the user's scroll.
func TestSwitchToQueueDoesNotScrollWhenFollowDisabled(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewLibrary
	m.followNowPlaying = false
	m.queue = makeFollowQueue(30, 12)
	for i := range m.queue.Entries {
		m.queue.Entries[i].Hash = "h" + string(rune('A'+i))
	}
	m.playback.currentIndex = 20
	m.queue.ScrollOffset = 5

	m2 := m.switchToView(viewQueue)

	if m2.queue.ScrollOffset != 5 {
		t.Errorf("scroll preserved when follow off: expected 5, got %d", m2.queue.ScrollOffset)
	}
}

// TestToggleQueueFollowFlipsState verifies the palette toggle handler
// mirrors the followNowPlaying field with cfg.QueueFollowNowPlaying.
func TestToggleQueueFollowFlipsState(t *testing.T) {
	m := New(context.Background())
	m.followNowPlaying = true
	m.cfg.QueueFollowNowPlaying = true

	m2, _ := m.handleToggleQueueFollowMsg()
	if m2.followNowPlaying {
		t.Errorf("after toggle: expected followNowPlaying=false")
	}
	if m2.cfg.QueueFollowNowPlaying {
		t.Errorf("after toggle: expected cfg.QueueFollowNowPlaying=false")
	}

	m3, _ := m2.handleToggleQueueFollowMsg()
	if !m3.followNowPlaying {
		t.Errorf("after second toggle: expected followNowPlaying=true")
	}
	if !m3.cfg.QueueFollowNowPlaying {
		t.Errorf("after second toggle: expected cfg.QueueFollowNowPlaying=true")
	}
}

// TestToggleQueueFollowReanchorsScrollWhenReenabled verifies that
// re-enabling follow mode while the Queue is active immediately scrolls
// the now-playing row back into the target position.
func TestToggleQueueFollowReanchorsScrollWhenReenabled(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewQueue
	m.followNowPlaying = false
	m.queue = makeFollowQueue(30, 12)
	for i := range m.queue.Entries {
		m.queue.Entries[i].Hash = "h" + string(rune('A'+i))
	}
	m.playback.currentIndex = 18
	m.queue.ScrollOffset = 0

	m2, _ := m.handleToggleQueueFollowMsg()
	if !m2.followNowPlaying {
		t.Fatalf("expected follow enabled after toggle")
	}
	// target = 18 - 3 = 15
	if m2.queue.ScrollOffset != 15 {
		t.Errorf("re-enable scroll: expected scrollOffset 15, got %d", m2.queue.ScrollOffset)
	}
}

// TestQueueFollowPersistsAcrossRestart verifies that the toggle state is
// captured in cfg.QueueFollowNowPlaying so a new Model built from that cfg
// (simulating a restart) restores the runtime field.
func TestQueueFollowPersistsAcrossRestart(t *testing.T) {
	m := New(context.Background())
	if !m.followNowPlaying {
		t.Fatalf("default: expected followNowPlaying=true")
	}

	m2, _ := m.handleToggleQueueFollowMsg()
	if m2.followNowPlaying {
		t.Fatalf("after toggle: expected followNowPlaying=false")
	}

	// Simulate a restart: a fresh Model that loads the persisted config.
	cfg := config.Defaults()
	cfg.QueueFollowNowPlaying = m2.cfg.QueueFollowNowPlaying
	restarted := New(context.Background(), WithConfig(cfg))

	if restarted.followNowPlaying {
		t.Errorf("after restart: expected followNowPlaying=false (persisted), got true")
	}
}
