package playlists

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

func TestCursorUpDecrementsAndStopsAtZero(t *testing.T) {
	v := View{
		Level:     ListLevel,
		Cursor:    2,
		Playlists: make([]db.PlaylistRow, 5),
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

func TestCursorUpAdjustsScrollOffset(t *testing.T) {
	v := View{
		Level:        ListLevel,
		Cursor:       3,
		ScrollOffset: 3,
		Playlists:    make([]db.PlaylistRow, 10),
	}
	v = v.CursorUp()
	if v.ScrollOffset != 2 {
		t.Errorf("CursorUp: expected ScrollOffset=2, got %d", v.ScrollOffset)
	}
}

func TestCursorDownIncrementsAndStopsAtEnd(t *testing.T) {
	v := View{
		Level:     ListLevel,
		Cursor:    0,
		Playlists: make([]db.PlaylistRow, 3),
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

func TestCursorDownAtTrackLevel(t *testing.T) {
	v := View{
		Level:  TrackLevel,
		Cursor: 0,
		Tracks: make([]db.PlaylistTrackRow, 2),
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

func TestCursorDownAdjustsScrollOffset(t *testing.T) {
	v := View{
		Level:     ListLevel,
		Cursor:    0,
		Height:    6,
		Playlists: make([]db.PlaylistRow, 10),
	}
	for range 3 {
		v = v.CursorDown()
	}
	if v.ScrollOffset != 0 {
		t.Errorf("CursorDown: expected ScrollOffset=0, got %d", v.ScrollOffset)
	}
}

func TestCursorDownWithNoVisibleRowsKeepsScrollOnCursor(t *testing.T) {
	v := View{
		Level:     ListLevel,
		Cursor:    0,
		Height:    1,
		Playlists: make([]db.PlaylistRow, 3),
	}

	v = v.CursorDown()

	if v.Cursor != 1 {
		t.Fatalf("CursorDown: expected Cursor=1, got %d", v.Cursor)
	}
	if v.ScrollOffset != v.Cursor {
		t.Fatalf("CursorDown with no visible rows: expected ScrollOffset=%d, got %d", v.Cursor, v.ScrollOffset)
	}
}
