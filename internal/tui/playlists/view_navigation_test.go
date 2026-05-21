package playlists

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

// --- navigation: CursorUp / CursorDown ---

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

// --- navigation: GoToTop / GoToBottom ---

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

// --- navigation: page movements ---

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

// --- moveCursor empty list ---

func TestMoveCursorEmptyListIsNoOp(t *testing.T) {
	v := View{Level: ListLevel, Cursor: 0}
	v2 := v.moveCursor(5)
	if v2.Cursor != 0 {
		t.Errorf("moveCursor empty: expected Cursor=0, got %d", v2.Cursor)
	}
}

// --- itemCount ---

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

// --- visibleRows ---

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

// TestGoToBottomClampsScrollOffsetWhenListShrank guards a regression where
// GoToBottom kept a stale ScrollOffset above the new last index after a
// delete shrank the list, leaving the cursor rendering above the visible
// window.
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
