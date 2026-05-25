package playlists

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

func TestCursorUpClampsStaleCursorPastEnd(t *testing.T) {
	v := View{
		Level:        ListLevel,
		Cursor:       9,
		ScrollOffset: 9,
		Playlists:    make([]db.PlaylistRow, 3),
	}

	v = v.CursorUp()

	if v.Cursor != 2 {
		t.Fatalf("CursorUp with stale cursor: expected Cursor=2, got %d", v.Cursor)
	}
	if v.ScrollOffset > v.Cursor {
		t.Fatalf("CursorUp with stale cursor: expected ScrollOffset<=Cursor, got scroll=%d cursor=%d", v.ScrollOffset, v.Cursor)
	}
}

func TestCursorDownClampsStaleCursorPastEnd(t *testing.T) {
	v := View{
		Level:        TrackLevel,
		Cursor:       9,
		ScrollOffset: 9,
		Tracks:       make([]db.PlaylistTrackRow, 3),
	}

	v = v.CursorDown()

	if v.Cursor != 2 {
		t.Fatalf("CursorDown with stale cursor: expected Cursor=2, got %d", v.Cursor)
	}
	if v.ScrollOffset > v.Cursor {
		t.Fatalf("CursorDown with stale cursor: expected ScrollOffset<=Cursor, got scroll=%d cursor=%d", v.ScrollOffset, v.Cursor)
	}
}

func TestCursorDownClampsNegativeCursorToFirstRow(t *testing.T) {
	v := View{
		Level:     ListLevel,
		Cursor:    -1,
		Playlists: make([]db.PlaylistRow, 3),
	}

	v = v.CursorDown()

	if v.Cursor != 0 {
		t.Fatalf("CursorDown with negative cursor: expected Cursor=0, got %d", v.Cursor)
	}
	if v.ScrollOffset != 0 {
		t.Fatalf("CursorDown with negative cursor: expected ScrollOffset=0, got %d", v.ScrollOffset)
	}
}

func TestNavigationEmptyListClearsStaleCursorAndScroll(t *testing.T) {
	for name, move := range map[string]func(View) View{
		"CursorUp":     View.CursorUp,
		"CursorDown":   View.CursorDown,
		"HalfPageDown": View.HalfPageDown,
		"HalfPageUp":   View.HalfPageUp,
		"FullPageDown": View.FullPageDown,
		"FullPageUp":   View.FullPageUp,
	} {
		t.Run(name, func(t *testing.T) {
			v := View{
				Level:        TrackLevel,
				Cursor:       4,
				ScrollOffset: 2,
			}

			v = move(v)

			if v.Cursor != 0 || v.ScrollOffset != 0 {
				t.Fatalf("expected stale empty-list state cleared, got cursor=%d scroll=%d", v.Cursor, v.ScrollOffset)
			}
		})
	}
}

func TestGoToBottomClampsScrollOffsetWhenListShrank(t *testing.T) {
	v := View{
		Level:        ListLevel,
		Playlists:    fakePlaylistRows("A", "B", "C"),
		Cursor:       0,
		ScrollOffset: 50,
		Height:       20,
	}
	v = v.GoToBottom()
	if v.Cursor != 2 {
		t.Errorf("expected Cursor=2, got %d", v.Cursor)
	}
	if v.ScrollOffset > v.Cursor {
		t.Errorf("expected ScrollOffset<=Cursor (%d), got ScrollOffset=%d", v.Cursor, v.ScrollOffset)
	}
	if v.ScrollOffset < 0 {
		t.Errorf("ScrollOffset must not be negative, got %d", v.ScrollOffset)
	}
}

func TestMoveCursorClampsNegativeScrollOffset(t *testing.T) {
	v := View{
		Level:        ListLevel,
		Playlists:    fakePlaylistRows("A", "B", "C", "D", "E"),
		Cursor:       0,
		ScrollOffset: -5,
		Height:       20,
	}
	v = v.CursorDown()
	if v.ScrollOffset < 0 {
		t.Errorf("CursorDown must not preserve negative ScrollOffset, got %d", v.ScrollOffset)
	}
}
