package artists

import (
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

func TestCursorUpDecrementsAndStopsAtZero(t *testing.T) {
	v := View{
		Level:   ArtistLevel,
		Cursor:  2,
		Artists: make([]string, 5),
	}

	v = v.CursorUp()
	if v.Cursor != 1 {
		t.Errorf("CursorUp: expected 1, got %d", v.Cursor)
	}
	v = v.CursorUp()
	if v.Cursor != 0 {
		t.Errorf("CursorUp: expected 0, got %d", v.Cursor)
	}
	v = v.CursorUp()
	if v.Cursor != 0 {
		t.Errorf("CursorUp at 0: expected 0, got %d", v.Cursor)
	}
}

func TestCursorDownIncrementsAndStopsAtEnd(t *testing.T) {
	v := View{
		Level:   ArtistLevel,
		Cursor:  0,
		Artists: make([]string, 3),
	}

	v = v.CursorDown()
	if v.Cursor != 1 {
		t.Errorf("CursorDown: expected 1, got %d", v.Cursor)
	}
	v = v.CursorDown()
	if v.Cursor != 2 {
		t.Errorf("CursorDown: expected 2, got %d", v.Cursor)
	}
	v = v.CursorDown()
	if v.Cursor != 2 {
		t.Errorf("CursorDown at end: expected 2, got %d", v.Cursor)
	}
}

func TestCursorDownAtAlbumLevel(t *testing.T) {
	v := View{
		Level:  AlbumLevel,
		Cursor: 0,
		Albums: make([]string, 2),
	}

	v = v.CursorDown()
	if v.Cursor != 1 {
		t.Errorf("CursorDown AlbumLevel: expected 1, got %d", v.Cursor)
	}
	v = v.CursorDown()
	if v.Cursor != 1 {
		t.Errorf("CursorDown AlbumLevel at end: expected 1, got %d", v.Cursor)
	}
}

func TestCursorDownAtTrackLevel(t *testing.T) {
	v := View{
		Level:  TrackLevel,
		Cursor: 0,
		Tracks: make([]library.Track, 2),
	}

	v = v.CursorDown()
	if v.Cursor != 1 {
		t.Errorf("CursorDown TrackLevel: expected 1, got %d", v.Cursor)
	}
	v = v.CursorDown()
	if v.Cursor != 1 {
		t.Errorf("CursorDown TrackLevel at end: expected 1, got %d", v.Cursor)
	}
}

func TestCursorUpClampsStaleCursorAfterListShrank(t *testing.T) {
	v := View{
		Level:        ArtistLevel,
		Cursor:       8,
		ScrollOffset: 8,
		Artists:      []string{"Alice", "Bob", "Charlie"},
	}

	v = v.CursorUp()
	if v.Cursor != 2 {
		t.Errorf("CursorUp: expected stale cursor clamped to 2, got %d", v.Cursor)
	}
	if v.ScrollOffset > v.Cursor {
		t.Errorf("CursorUp: expected ScrollOffset<=Cursor, got %d > %d", v.ScrollOffset, v.Cursor)
	}
}

func TestCursorDownClampsStaleCursorAfterFilterShrankList(t *testing.T) {
	v := View{
		Level:        ArtistLevel,
		Cursor:       4,
		ScrollOffset: 4,
		Artists:      []string{"Alice", "Bob", "Charlie"},
		FilterText:   "b",
	}

	v = v.CursorDown()
	if v.Cursor != 0 {
		t.Errorf("CursorDown: expected stale cursor clamped to only filtered row, got %d", v.Cursor)
	}
	if v.ScrollOffset != 0 {
		t.Errorf("CursorDown: expected ScrollOffset=0, got %d", v.ScrollOffset)
	}
}

func TestCursorMovementDoesNotAdvanceScrollWhenNoRowsVisible(t *testing.T) {
	v := View{
		Level:        TrackLevel,
		Cursor:       0,
		ScrollOffset: 0,
		Height:       1,
		Tracks:       make([]library.Track, 2),
	}
	if rows := v.visibleRows(); rows != 0 {
		t.Fatalf("precondition: expected no visible rows, got %d", rows)
	}

	v = v.CursorDown()
	if v.Cursor != 1 {
		t.Errorf("CursorDown: expected Cursor=1, got %d", v.Cursor)
	}
	if v.ScrollOffset > v.Cursor {
		t.Errorf("CursorDown: expected ScrollOffset<=Cursor, got %d > %d", v.ScrollOffset, v.Cursor)
	}
}

func TestMoveCursorEmptyListIsNoOp(t *testing.T) {
	v := View{Level: ArtistLevel, Cursor: 0}
	v2 := v.moveCursor(5)
	if v2.Cursor != 0 {
		t.Errorf("moveCursor empty: expected Cursor=0, got %d", v2.Cursor)
	}
}

func TestMoveCursorClampsNegativeScrollOffset(t *testing.T) {
	v := View{
		Level:        ArtistLevel,
		Artists:      []string{"A", "B", "C", "D", "E"},
		Cursor:       0,
		ScrollOffset: -5,
		Height:       20,
	}

	v = v.CursorDown()
	if v.ScrollOffset < 0 {
		t.Errorf("CursorDown must not preserve negative ScrollOffset, got %d", v.ScrollOffset)
	}
}

func TestItemCountAtEachLevel(t *testing.T) {
	v := View{
		Level:   ArtistLevel,
		Artists: make([]string, 7),
		Albums:  make([]string, 4),
		Tracks:  make([]library.Track, 3),
	}

	if v.itemCount() != 7 {
		t.Errorf("itemCount ArtistLevel: expected 7, got %d", v.itemCount())
	}

	v.Level = AlbumLevel
	if v.itemCount() != 4 {
		t.Errorf("itemCount AlbumLevel: expected 4, got %d", v.itemCount())
	}

	v.Level = TrackLevel
	if v.itemCount() != 3 {
		t.Errorf("itemCount TrackLevel: expected 3, got %d", v.itemCount())
	}
}
