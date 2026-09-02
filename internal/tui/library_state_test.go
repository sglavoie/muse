package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/text"
)

// TestClampCursorReserveMatchesRenderer verifies that clampCursor uses the same
// visible-row reserve as the renderer (libraryViewVisibleReserve). A previous
// version hard-coded reserve=1 (or 2 with filter) which over-estimated the
// number of visible rows by one when width>0 and no filter was active, leaving
// the focused row just below the rendered window.
func TestClampCursorReserveMatchesRenderer(t *testing.T) {
	tracks := make([]library.Track, 100)
	for i := range tracks {
		tracks[i] = library.Track{
			Hash:   string(rune('a' + (i % 26))),
			Title:  "t",
			Artist: "a",
			Path:   "/p",
		}
		// Ensure unique hashes.
		tracks[i].Hash = tracks[i].Hash + "-" + string(rune('0'+i%10)) + "-" + string(rune('0'+(i/10)%10))
	}
	var idx library.LibraryIndex
	idx.BulkLoad(tracks)

	v := LibraryView{index: idx, width: 80, height: 10}
	v.cursor = 10
	v.scrollOffset = 0
	v = v.clampCursor()

	// Renderer uses reserve=2 when width>0, so visible rows = 8. The cursor at
	// row 10 must therefore satisfy scrollOffset <= 10 < scrollOffset+8.
	if v.scrollOffset > v.cursor {
		t.Fatalf("scrollOffset %d > cursor %d", v.scrollOffset, v.cursor)
	}
	rendererReserve := libraryViewVisibleReserve(v)
	visibleRows := v.height - rendererReserve
	if v.cursor >= v.scrollOffset+visibleRows {
		t.Fatalf("cursor %d falls outside rendered window [%d, %d) (visibleRows=%d)",
			v.cursor, v.scrollOffset, v.scrollOffset+visibleRows, visibleRows)
	}
}

// TestClampCursorEmptyResetsScroll verifies that clampCursor resets both
// cursor and scrollOffset to 0 when the visible track list is empty.
func TestClampCursorEmptyResetsScroll(t *testing.T) {
	v := LibraryView{width: 80, height: 24}
	v.cursor = 5
	v.scrollOffset = 5
	v = v.clampCursor()
	if v.cursor != 0 || v.scrollOffset != 0 {
		t.Fatalf("empty clamp: want (0,0), got (%d,%d)", v.cursor, v.scrollOffset)
	}
}

// TestClampCursorBeyondEndIsClampedToLast verifies cursor past the last index
// is brought back to count-1 and scrollOffset is adjusted to keep it visible.
func TestClampCursorBeyondEndIsClampedToLast(t *testing.T) {
	tracks := make([]library.Track, 5)
	for i := range tracks {
		tracks[i] = library.Track{Hash: string(rune('a' + i)), Path: "/p"}
	}
	var idx library.LibraryIndex
	idx.BulkLoad(tracks)

	v := LibraryView{index: idx, width: 80, height: 24}
	v.cursor = 999
	v.scrollOffset = 999
	v = v.clampCursor()
	if v.cursor != 4 {
		t.Fatalf("cursor: want 4, got %d", v.cursor)
	}
	if v.scrollOffset < 0 {
		t.Fatalf("scrollOffset: want >=0, got %d", v.scrollOffset)
	}
}

// TestSwitchModeRestoresSnapshot verifies switchMode saves and restores
// per-mode cursor/scroll/filter snapshots when cycling back to a previously
// visited mode.
func TestSwitchModeRestoresSnapshot(t *testing.T) {
	tracks := make([]library.Track, 50)
	for i := range tracks {
		tracks[i] = library.Track{Hash: "h" + string(rune('0'+i%10)) + string(rune('0'+i/10)), Path: "/p"}
	}
	var idx library.LibraryIndex
	idx.BulkLoad(tracks)
	v := LibraryView{index: idx, width: 80, height: 24}

	// Park in All at cursor 7.
	v.cursor = 7
	v.scrollOffset = 3

	// Switch to Recent, move cursor, then back to All.
	v = v.switchMode(LibraryModeRecent)
	if v.cursor != 0 || v.scrollOffset != 0 {
		t.Fatalf("recent snapshot start: want (0,0), got (%d,%d)", v.cursor, v.scrollOffset)
	}
	v.cursor = 12
	v.scrollOffset = 5

	v = v.switchMode(LibraryModeAll)
	if v.cursor != 7 || v.scrollOffset != 3 {
		t.Fatalf("returned to All: want (7,3), got (%d,%d)", v.cursor, v.scrollOffset)
	}

	// Back to Recent: snapshot should restore.
	v = v.switchMode(LibraryModeRecent)
	if v.cursor != 12 || v.scrollOffset != 5 {
		t.Fatalf("returned to Recent: want (12,5), got (%d,%d)", v.cursor, v.scrollOffset)
	}
}

// TestFormatCountNegative verifies negative numbers retain the sign prefix.
func TestFormatCountNegative(t *testing.T) {
	cases := map[int]string{
		0:       "0",
		5:       "5",
		1234:    "1,234",
		1000000: "1,000,000",
		-1234:   "-1,234",
		-100:    "-100",
	}
	for in, want := range cases {
		got := text.FormatCount(in)
		if got != want {
			t.Errorf("formatCount(%d) = %q, want %q", in, got, want)
		}
	}
}
