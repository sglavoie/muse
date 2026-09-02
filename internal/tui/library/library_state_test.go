package library

import (
	"testing"

	musiclib "github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/text"
)

// TestClampCursorReserveMatchesRenderer verifies that clampCursor uses the same
// visible-row reserve as the renderer (libraryViewVisibleReserve). A previous
// version hard-coded reserve=1 (or 2 with filter) which over-estimated the
// number of visible rows by one when width>0 and no filter was active, leaving
// the focused row just below the rendered window.
func TestClampCursorReserveMatchesRenderer(t *testing.T) {
	tracks := make([]musiclib.Track, 100)
	for i := range tracks {
		tracks[i] = musiclib.Track{
			Hash:   string(rune('a' + (i % 26))),
			Title:  "t",
			Artist: "a",
			Path:   "/p",
		}
		// Ensure unique hashes.
		tracks[i].Hash = tracks[i].Hash + "-" + string(rune('0'+i%10)) + "-" + string(rune('0'+(i/10)%10))
	}
	var idx musiclib.LibraryIndex
	idx.BulkLoad(tracks)

	v := View{Index: idx, Width: 80, Height: 10}
	v.Cursor = 10
	v.ScrollOffset = 0
	v = v.ClampCursor()

	// Renderer uses reserve=2 when width>0, so visible rows = 8. The cursor at
	// row 10 must therefore satisfy scrollOffset <= 10 < scrollOffset+8.
	if v.ScrollOffset > v.Cursor {
		t.Fatalf("scrollOffset %d > cursor %d", v.ScrollOffset, v.Cursor)
	}
	rendererReserve := VisibleReserve(v)
	visibleRows := v.Height - rendererReserve
	if v.Cursor >= v.ScrollOffset+visibleRows {
		t.Fatalf("cursor %d falls outside rendered window [%d, %d) (visibleRows=%d)",
			v.Cursor, v.ScrollOffset, v.ScrollOffset+visibleRows, visibleRows)
	}
}

// TestClampCursorEmptyResetsScroll verifies that clampCursor resets both
// cursor and scrollOffset to 0 when the visible track list is empty.
func TestClampCursorEmptyResetsScroll(t *testing.T) {
	v := View{Width: 80, Height: 24}
	v.Cursor = 5
	v.ScrollOffset = 5
	v = v.ClampCursor()
	if v.Cursor != 0 || v.ScrollOffset != 0 {
		t.Fatalf("empty clamp: want (0,0), got (%d,%d)", v.Cursor, v.ScrollOffset)
	}
}

// TestClampCursorBeyondEndIsClampedToLast verifies cursor past the last index
// is brought back to count-1 and scrollOffset is adjusted to keep it visible.
func TestClampCursorBeyondEndIsClampedToLast(t *testing.T) {
	tracks := make([]musiclib.Track, 5)
	for i := range tracks {
		tracks[i] = musiclib.Track{Hash: string(rune('a' + i)), Path: "/p"}
	}
	var idx musiclib.LibraryIndex
	idx.BulkLoad(tracks)

	v := View{Index: idx, Width: 80, Height: 24}
	v.Cursor = 999
	v.ScrollOffset = 999
	v = v.ClampCursor()
	if v.Cursor != 4 {
		t.Fatalf("cursor: want 4, got %d", v.Cursor)
	}
	if v.ScrollOffset < 0 {
		t.Fatalf("scrollOffset: want >=0, got %d", v.ScrollOffset)
	}
}

// TestSwitchModeRestoresSnapshot verifies switchMode saves and restores
// per-mode cursor/scroll/filter snapshots when cycling back to a previously
// visited mode.
func TestSwitchModeRestoresSnapshot(t *testing.T) {
	tracks := make([]musiclib.Track, 50)
	for i := range tracks {
		tracks[i] = musiclib.Track{Hash: "h" + string(rune('0'+i%10)) + string(rune('0'+i/10)), Path: "/p"}
	}
	var idx musiclib.LibraryIndex
	idx.BulkLoad(tracks)
	v := View{Index: idx, Width: 80, Height: 24}

	// Park in All at cursor 7.
	v.Cursor = 7
	v.ScrollOffset = 3

	// Switch to Recent, move cursor, then back to All.
	v = v.SwitchMode(LibraryModeRecent)
	if v.Cursor != 0 || v.ScrollOffset != 0 {
		t.Fatalf("recent snapshot start: want (0,0), got (%d,%d)", v.Cursor, v.ScrollOffset)
	}
	v.Cursor = 12
	v.ScrollOffset = 5

	v = v.SwitchMode(LibraryModeAll)
	if v.Cursor != 7 || v.ScrollOffset != 3 {
		t.Fatalf("returned to All: want (7,3), got (%d,%d)", v.Cursor, v.ScrollOffset)
	}

	// Back to Recent: snapshot should restore.
	v = v.SwitchMode(LibraryModeRecent)
	if v.Cursor != 12 || v.ScrollOffset != 5 {
		t.Fatalf("returned to Recent: want (12,5), got (%d,%d)", v.Cursor, v.ScrollOffset)
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
