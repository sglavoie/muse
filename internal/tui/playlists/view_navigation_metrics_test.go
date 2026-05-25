package playlists

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

func TestItemCountListLevel(t *testing.T) {
	v := View{Level: ListLevel, Playlists: make([]db.PlaylistRow, 7)}
	if v.itemCount() != 7 {
		t.Errorf("itemCount ListLevel: expected 7, got %d", v.itemCount())
	}
}

func TestItemCountTrackLevel(t *testing.T) {
	v := View{Level: TrackLevel, Tracks: make([]db.PlaylistTrackRow, 3)}
	if v.itemCount() != 3 {
		t.Errorf("itemCount TrackLevel: expected 3, got %d", v.itemCount())
	}
}

func TestVisibleRowsZeroHeight(t *testing.T) {
	v := View{Height: 0}
	if v.visibleRows() != 50 {
		t.Errorf("visibleRows Height=0: expected 50, got %d", v.visibleRows())
	}
}

func TestVisibleRowsSmallHeight(t *testing.T) {
	v := View{Height: 4}
	if v.visibleRows() != 3 {
		t.Errorf("visibleRows Height=4: expected 3, got %d", v.visibleRows())
	}
}

func TestVisibleRowsTrackLevel(t *testing.T) {
	v := View{Level: TrackLevel, Height: 10}
	if v.visibleRows() != 6 {
		t.Errorf("visibleRows TrackLevel Height=10: expected 6, got %d", v.visibleRows())
	}
}
