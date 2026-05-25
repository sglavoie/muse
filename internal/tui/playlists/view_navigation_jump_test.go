package playlists

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

func TestGoToTopResetsCursorAndScroll(t *testing.T) {
	v := View{
		Level:        ListLevel,
		Cursor:       5,
		ScrollOffset: 3,
		Playlists:    make([]db.PlaylistRow, 10),
	}
	v = v.GoToTop()
	if v.Cursor != 0 {
		t.Errorf("GoToTop: expected Cursor=0, got %d", v.Cursor)
	}
	if v.ScrollOffset != 0 {
		t.Errorf("GoToTop: expected ScrollOffset=0, got %d", v.ScrollOffset)
	}
}

func TestGoToBottomMovesCursorToEnd(t *testing.T) {
	v := View{
		Level:     ListLevel,
		Cursor:    0,
		Playlists: make([]db.PlaylistRow, 10),
	}
	v = v.GoToBottom()
	if v.Cursor != 9 {
		t.Errorf("GoToBottom: expected Cursor=9, got %d", v.Cursor)
	}
}

func TestGoToBottomTrackLevel(t *testing.T) {
	v := View{
		Level:  TrackLevel,
		Cursor: 0,
		Tracks: make([]db.PlaylistTrackRow, 5),
	}
	v = v.GoToBottom()
	if v.Cursor != 4 {
		t.Errorf("GoToBottom TrackLevel: expected Cursor=4, got %d", v.Cursor)
	}
}

func TestGoToBottomEmptyList(t *testing.T) {
	v := View{Level: ListLevel, Cursor: 0}
	v = v.GoToBottom()
	if v.Cursor != 0 {
		t.Errorf("GoToBottom empty: expected Cursor=0, got %d", v.Cursor)
	}
}

func TestGoToBottomAdjustsScrollOffset(t *testing.T) {
	v := View{
		Level:     ListLevel,
		Cursor:    0,
		Height:    6,
		Playlists: make([]db.PlaylistRow, 10),
	}
	v = v.GoToBottom()
	if v.Cursor != 9 {
		t.Errorf("GoToBottom: expected Cursor=9, got %d", v.Cursor)
	}
	if v.ScrollOffset != 5 {
		t.Errorf("GoToBottom: expected ScrollOffset=5, got %d", v.ScrollOffset)
	}
}
