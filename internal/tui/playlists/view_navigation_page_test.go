package playlists

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

func TestHalfPageDownMovesByHalfVisibleRows(t *testing.T) {
	v := View{
		Level:     ListLevel,
		Cursor:    0,
		Height:    13,
		Playlists: make([]db.PlaylistRow, 20),
	}
	v = v.HalfPageDown()
	if v.Cursor != 6 {
		t.Errorf("HalfPageDown: expected Cursor=6, got %d", v.Cursor)
	}
}

func TestHalfPageUpMovesByHalfVisibleRows(t *testing.T) {
	v := View{
		Level:     ListLevel,
		Cursor:    10,
		Height:    13,
		Playlists: make([]db.PlaylistRow, 20),
	}
	v = v.HalfPageUp()
	if v.Cursor != 4 {
		t.Errorf("HalfPageUp: expected Cursor=4, got %d", v.Cursor)
	}
}

func TestFullPageDownMovesByVisibleRows(t *testing.T) {
	v := View{
		Level:     ListLevel,
		Cursor:    0,
		Height:    13,
		Playlists: make([]db.PlaylistRow, 20),
	}
	v = v.FullPageDown()
	if v.Cursor != 12 {
		t.Errorf("FullPageDown: expected Cursor=12, got %d", v.Cursor)
	}
}

func TestFullPageUpMovesByVisibleRows(t *testing.T) {
	v := View{
		Level:     ListLevel,
		Cursor:    15,
		Height:    13,
		Playlists: make([]db.PlaylistRow, 20),
	}
	v = v.FullPageUp()
	if v.Cursor != 3 {
		t.Errorf("FullPageUp: expected Cursor=3, got %d", v.Cursor)
	}
}

func TestPageMovementsClampsToEnd(t *testing.T) {
	v := View{
		Level:     ListLevel,
		Cursor:    8,
		Height:    13,
		Playlists: make([]db.PlaylistRow, 10),
	}
	v = v.FullPageDown()
	if v.Cursor != 9 {
		t.Errorf("FullPageDown clamp: expected Cursor=9, got %d", v.Cursor)
	}
}

func TestPageMovementsClampsToStart(t *testing.T) {
	v := View{
		Level:     ListLevel,
		Cursor:    2,
		Height:    13,
		Playlists: make([]db.PlaylistRow, 10),
	}
	v = v.FullPageUp()
	if v.Cursor != 0 {
		t.Errorf("FullPageUp clamp: expected Cursor=0, got %d", v.Cursor)
	}
}

func TestMoveCursorEmptyListIsNoOp(t *testing.T) {
	v := View{Level: ListLevel, Cursor: 0}
	v2 := v.moveCursor(5)
	if v2.Cursor != 0 {
		t.Errorf("moveCursor empty: expected Cursor=0, got %d", v2.Cursor)
	}
}
