package tui

import (
	"context"
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/layout"
	libtui "github.com/sglavoie/muse/internal/tui/library"
)

// TestRapidWindowSizeMsgDoesNotPanic verifies that sending many successive
// WindowSizeMsg events in a tight loop does not cause a panic and always
// returns a valid model.
func TestRapidWindowSizeMsgDoesNotPanic(t *testing.T) {
	m := New(context.Background())

	sizes := [][2]int{
		{80, 24}, {120, 40}, {40, 10}, {200, 50}, {60, 20},
		{80, 24}, {100, 30}, {50, 15}, {180, 45}, {70, 22},
	}
	for range 5 {
		for _, s := range sizes {
			next, _ := m.Update(tea.WindowSizeMsg{Width: s[0], Height: s[1]})
			m = next.(Model)
		}
	}

	// If we reach here, no panic occurred. Confirm final termWidth is the last sent value.
	lastWidth := sizes[len(sizes)-1][0]
	if m.termWidth != lastWidth {
		t.Fatalf("after rapid resize: expected termWidth=%d, got %d", lastWidth, m.termWidth)
	}
}

// TestWindowSizeMsgPreservesApplicationState verifies that a terminal resize
// does not alter cursor position, playback state, or scan state.
func TestWindowSizeMsgPreservesApplicationState(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
		{Path: "/music/c.flac", Display: "C"},
	}
	m.queue.cursor = 2
	m.playback.state = PlaybackPlaying
	m.scanning = true
	m.activeView = viewQueue

	next, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 50})
	nm := next.(Model)

	if nm.queue.cursor != 2 {
		t.Fatalf("resize: queue cursor changed; expected 2, got %d", nm.queue.cursor)
	}
	if nm.playback.state != PlaybackPlaying {
		t.Fatalf("resize: playbackState changed; expected PlaybackPlaying, got %v", nm.playback.state)
	}
	if !nm.scanning {
		t.Fatal("resize: scanning changed; expected true, got false")
	}
	if nm.activeView != viewQueue {
		t.Fatalf("resize: activeView changed; expected viewQueue (%d), got %d", viewQueue, nm.activeView)
	}
}

// TestWindowSizeMsgUpdatesTermWidthAndRendersContent verifies that after a
// WindowSizeMsg the model stores the new terminal width and View() produces
// non-empty rendered content.
func TestWindowSizeMsgUpdatesTermWidthAndRendersContent(t *testing.T) {
	m := New(context.Background())

	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	nm := next.(Model)

	if nm.termWidth != 120 {
		t.Fatalf("termWidth: expected 120, got %d", nm.termWidth)
	}

	v := nm.View()
	if v.Content == "" {
		t.Fatal("View() after WindowSizeMsg must produce non-empty content")
	}
}

// TestWindowSizeMsgPropagatesWidthToChildViews verifies that a WindowSizeMsg
// updates the width fields of both the library and playlists child views, and
// that column widths computed from the updated width are proportionally larger
// than those computed from a smaller width.
func TestWindowSizeMsgPropagatesWidthToChildViews(t *testing.T) {
	m := New(context.Background())

	const newWidth = 160
	next, _ := m.Update(tea.WindowSizeMsg{Width: newWidth, Height: 40})
	nm := next.(Model)

	if nm.library.width != newWidth {
		t.Fatalf("library.width: expected %d, got %d", newWidth, nm.library.width)
	}
	if nm.playlists.width != newWidth {
		t.Fatalf("playlists.width: expected %d, got %d", newWidth, nm.playlists.width)
	}

	wide := libtui.Compute(120)
	narrow := libtui.Compute(80)

	if wide.Artist <= narrow.Artist {
		t.Fatalf("Artist column: expected Compute(120).Artist (%d) > Compute(80).Artist (%d)", wide.Artist, narrow.Artist)
	}
	if wide.Title <= narrow.Title {
		t.Fatalf("Title column: expected Compute(120).Title (%d) > Compute(80).Title (%d)", wide.Title, narrow.Title)
	}
}

// TestWindowSizeShrinkClampsCursorVisibility verifies that when the terminal
// shrinks, the scroll offset is adjusted so the cursor row remains visible
// in all views. Without clamping, the cursor could disappear below the
// viewport until the user presses a cursor key.
func TestWindowSizeShrinkClampsCursorVisibility(t *testing.T) {
	m := New(context.Background())
	// Start with a tall terminal so we can position cursors far down.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 80})
	m = next.(Model)

	// Populate the queue with enough entries and position cursor far down.
	entries := make([]QueueEntry, 100)
	for i := range entries {
		entries[i] = QueueEntry{Path: "/music/" + string(rune('a'+i%26)) + ".flac", Display: "Track " + string(rune('a'+i%26))}
	}
	m.queue.entries = entries
	m.queue.cursor = 70
	m.queue.scrollOffset = 60
	m.activeView = viewQueue

	// Shrink the terminal significantly.
	next, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	nm := next.(Model)

	// viewHeight = 12-4 = 8, visibleRows = 8-2 = 6.
	maxRows := nm.queue.visibleRows()
	if nm.queue.cursor < nm.queue.scrollOffset || nm.queue.cursor >= nm.queue.scrollOffset+maxRows {
		t.Fatalf("after shrink: queue cursor %d not visible with scrollOffset=%d maxRows=%d",
			nm.queue.cursor, nm.queue.scrollOffset, maxRows)
	}
}

// TestWindowSizeShrinkClampsRecentlyAddedScroll verifies that when the terminal
// shrinks, the library (recently added mode) scroll offset is adjusted so the
// cursor remains visible.
func TestWindowSizeShrinkClampsRecentlyAddedScroll(t *testing.T) {
	m := New(context.Background())
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 80})
	m = next.(Model)

	// Populate library (recently added mode) with enough tracks.
	tracks := make([]library.Track, 100)
	for i := range tracks {
		tracks[i] = library.Track{Hash: fmt.Sprintf("h%d", i), Title: fmt.Sprintf("Track %d", i)}
	}
	m.library.mode = LibraryModeRecent
	m.library.sourceTracks = tracks
	m.library = m.library.recomputeSortedTracks()
	m.library.cursor = 70
	m.library.scrollOffset = 60
	m.activeView = viewLibrary

	// Shrink the terminal significantly.
	next, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	nm := next.(Model)

	maxRows := layout.VisibleRows(nm.library.height, 1)
	if nm.library.cursor < nm.library.scrollOffset ||
		nm.library.cursor >= nm.library.scrollOffset+maxRows {
		t.Fatalf("after shrink: library cursor %d not visible with scrollOffset=%d maxRows=%d",
			nm.library.cursor, nm.library.scrollOffset, maxRows)
	}
}
