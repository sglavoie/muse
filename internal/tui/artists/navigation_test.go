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
		Height:       1, // TrackLevel reserves two rows, leaving no content rows.
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

func TestGoToTopResetsCursorAndScroll(t *testing.T) {
	v := View{
		Level:        ArtistLevel,
		Cursor:       5,
		ScrollOffset: 3,
		Artists:      make([]string, 10),
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
		Level:   ArtistLevel,
		Cursor:  0,
		Artists: make([]string, 10),
	}
	v = v.GoToBottom()
	if v.Cursor != 9 {
		t.Errorf("GoToBottom: expected Cursor=9, got %d", v.Cursor)
	}
}

func TestGoToBottomEmptyList(t *testing.T) {
	v := View{Level: ArtistLevel, Cursor: 0}
	v = v.GoToBottom()
	if v.Cursor != 0 {
		t.Errorf("GoToBottom empty: expected Cursor=0, got %d", v.Cursor)
	}
}

func TestHalfPageDownMovesByHalfVisibleRows(t *testing.T) {
	v := View{
		Level:   ArtistLevel,
		Cursor:  0,
		Height:  13,
		Artists: make([]string, 20),
	}
	v = v.HalfPageDown()
	if v.Cursor != 6 {
		t.Errorf("HalfPageDown: expected Cursor=6, got %d", v.Cursor)
	}
}

func TestFullPageDownMovesByVisibleRows(t *testing.T) {
	v := View{
		Level:   ArtistLevel,
		Cursor:  0,
		Height:  13,
		Artists: make([]string, 20),
	}
	v = v.FullPageDown()
	// visibleRows = height(13) - reserve(1, strip) = 12
	if v.Cursor != 12 {
		t.Errorf("FullPageDown: expected Cursor=12, got %d", v.Cursor)
	}
}

func TestMoveCursorEmptyListIsNoOp(t *testing.T) {
	v := View{Level: ArtistLevel, Cursor: 0}
	v2 := v.moveCursor(5)
	if v2.Cursor != 0 {
		t.Errorf("moveCursor empty: expected Cursor=0, got %d", v2.Cursor)
	}
}

func TestPageMovementDoesNotAdvanceScrollWhenNoRowsVisible(t *testing.T) {
	v := View{
		Level:        AlbumLevel,
		Cursor:       1,
		ScrollOffset: 0,
		Height:       1, // AlbumLevel reserves one row, leaving no content rows.
		Albums:       []string{"Alpha", "Beta", "Gamma"},
	}
	if rows := v.visibleRows(); rows != 0 {
		t.Fatalf("precondition: expected no visible rows, got %d", rows)
	}

	v = v.FullPageDown()
	if v.Cursor != 1 {
		t.Errorf("FullPageDown: expected Cursor=1, got %d", v.Cursor)
	}
	if v.ScrollOffset > v.Cursor {
		t.Errorf("FullPageDown: expected ScrollOffset<=Cursor, got %d > %d", v.ScrollOffset, v.Cursor)
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

func TestOpenAlbumsTransitionsToAlbumLevel(t *testing.T) {
	v := View{
		Level:   ArtistLevel,
		Cursor:  2,
		Artists: []string{"Alice", "Bob", "Charlie"},
	}
	v2 := v.OpenAlbums([]string{"Alpha", "Beta"})

	if v2.Level != AlbumLevel {
		t.Errorf("OpenAlbums: expected Level=AlbumLevel, got %v", v2.Level)
	}
	if v2.Cursor != 0 {
		t.Errorf("OpenAlbums: expected Cursor=0, got %d", v2.Cursor)
	}
	if v2.ArtistCursor != 2 {
		t.Errorf("OpenAlbums: expected ArtistCursor=2 (saved), got %d", v2.ArtistCursor)
	}
	if len(v2.Albums) != 2 {
		t.Errorf("OpenAlbums: expected 2 albums, got %d", len(v2.Albums))
	}
}

func TestOpenTracksTransitionsToTrackLevel(t *testing.T) {
	tracks := []library.Track{
		{Title: "Song A"},
		{Title: "Song B"},
	}
	v := View{
		Level:  AlbumLevel,
		Cursor: 1,
		Albums: []string{"Alpha", "Beta"},
	}
	v2 := v.OpenTracks(tracks)

	if v2.Level != TrackLevel {
		t.Errorf("OpenTracks: expected Level=TrackLevel, got %v", v2.Level)
	}
	if v2.Cursor != 0 {
		t.Errorf("OpenTracks: expected Cursor=0, got %d", v2.Cursor)
	}
	if v2.AlbumCursor != 1 {
		t.Errorf("OpenTracks: expected AlbumCursor=1 (saved), got %d", v2.AlbumCursor)
	}
	if len(v2.Tracks) != 2 {
		t.Errorf("OpenTracks: expected 2 tracks, got %d", len(v2.Tracks))
	}
}

func TestBackFromAlbumLevelRestoresArtistCursor(t *testing.T) {
	v := View{
		Level:        ArtistLevel,
		Cursor:       3,
		ScrollOffset: 2,
		Artists:      make([]string, 10),
	}

	v = v.OpenAlbums([]string{"Alpha", "Beta"})
	if v.Cursor != 0 {
		t.Fatalf("precondition: expected album cursor=0, got %d", v.Cursor)
	}

	v.Cursor = 1
	v.ScrollOffset = 1

	v = v.Back()
	if v.Level != ArtistLevel {
		t.Errorf("Back: expected Level=ArtistLevel, got %v", v.Level)
	}
	if v.Cursor != 3 {
		t.Errorf("Back: expected Cursor=3 (restored), got %d", v.Cursor)
	}
	if v.ScrollOffset != 2 {
		t.Errorf("Back: expected ScrollOffset=2 (restored), got %d", v.ScrollOffset)
	}
	if v.Albums != nil {
		t.Errorf("Back: expected Albums=nil, got %v", v.Albums)
	}
}

func TestBackFromTrackLevelRestoresAlbumCursor(t *testing.T) {
	v := View{
		Level:        AlbumLevel,
		Cursor:       2,
		ScrollOffset: 1,
		Albums:       []string{"Alpha", "Beta", "Gamma"},
	}

	tracks := []library.Track{{Title: "Song A"}, {Title: "Song B"}}
	v = v.OpenTracks(tracks)
	if v.Cursor != 0 {
		t.Fatalf("precondition: expected track cursor=0, got %d", v.Cursor)
	}

	v.Cursor = 1
	v.ScrollOffset = 0

	v = v.Back()
	if v.Level != AlbumLevel {
		t.Errorf("Back: expected Level=AlbumLevel, got %v", v.Level)
	}
	if v.Cursor != 2 {
		t.Errorf("Back: expected Cursor=2 (restored), got %d", v.Cursor)
	}
	if v.ScrollOffset != 1 {
		t.Errorf("Back: expected ScrollOffset=1 (restored), got %d", v.ScrollOffset)
	}
	if v.Tracks != nil {
		t.Errorf("Back: expected Tracks=nil, got %v", v.Tracks)
	}
}

func TestBackAtArtistLevelIsNoOp(t *testing.T) {
	v := View{
		Level:   ArtistLevel,
		Cursor:  1,
		Artists: []string{"Alice", "Bob"},
	}

	if v.Level != ArtistLevel {
		v = v.Back()
	}

	if v.Level != ArtistLevel {
		t.Errorf("Back at ArtistLevel: expected Level=ArtistLevel, got %v", v.Level)
	}
	if v.Cursor != 1 {
		t.Errorf("Back at ArtistLevel: expected Cursor=1 (unchanged), got %d", v.Cursor)
	}
}

func TestBackClampsCursorWhenParentShrank(t *testing.T) {
	v := View{
		Level:        ArtistLevel,
		Cursor:       5,
		ScrollOffset: 3,
		Artists:      []string{"a", "b", "c", "d", "e", "f"},
	}
	v = v.OpenAlbums([]string{"X", "Y"})
	// Mutate parent list to simulate artist deletion while drilled down.
	v.Artists = []string{"a", "b"}
	v = v.Back()
	if v.Level != ArtistLevel {
		t.Fatalf("expected ArtistLevel, got %v", v.Level)
	}
	if v.Cursor != 1 {
		t.Errorf("Back: expected clamped Cursor=1, got %d", v.Cursor)
	}
	if v.ScrollOffset > v.Cursor {
		t.Errorf("Back: expected ScrollOffset <= Cursor, got %d > %d", v.ScrollOffset, v.Cursor)
	}
}

func TestBackClampsCursorWhenParentEmpty(t *testing.T) {
	v := View{
		Level:        AlbumLevel,
		Cursor:       3,
		ScrollOffset: 1,
		Albums:       []string{"X", "Y", "Z", "W"},
	}
	v = v.OpenTracks([]library.Track{{Title: "t"}})
	// All albums gone while drilled down (e.g. all tracks for the artist removed).
	v.Albums = nil
	v = v.Back()
	if v.Level != AlbumLevel {
		t.Fatalf("expected AlbumLevel, got %v", v.Level)
	}
	if v.Cursor != 0 || v.ScrollOffset != 0 {
		t.Errorf("Back with empty parent: expected cursor=0 scroll=0, got cursor=%d scroll=%d", v.Cursor, v.ScrollOffset)
	}
}

func TestBackKeepsRestoredCursorVisibleAfterResize(t *testing.T) {
	v := View{
		Level:        ArtistLevel,
		Cursor:       8,
		ScrollOffset: 0,
		Height:       20,
		Artists:      []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"},
	}

	v = v.OpenAlbums([]string{"X"})
	v.Height = 3 // visibleRows at ArtistLevel becomes 2
	v = v.Back()

	if v.Level != ArtistLevel {
		t.Fatalf("expected ArtistLevel, got %v", v.Level)
	}
	if v.Cursor != 8 {
		t.Fatalf("expected restored cursor=8, got %d", v.Cursor)
	}
	if v.ScrollOffset != 7 {
		t.Errorf("expected scroll offset=7 to keep cursor visible, got %d", v.ScrollOffset)
	}
}

func TestBackDoesNotAdvanceScrollWhenNoRowsVisible(t *testing.T) {
	v := View{
		Level:        AlbumLevel,
		Cursor:       1,
		ScrollOffset: 0,
		Height:       10,
		Albums:       []string{"Alpha", "Beta", "Gamma"},
	}
	v = v.OpenTracks([]library.Track{{Title: "Song"}})
	v.Height = 1 // AlbumLevel reserves one row, leaving no content rows after Back.

	v = v.Back()
	if v.Level != AlbumLevel {
		t.Fatalf("expected AlbumLevel, got %v", v.Level)
	}
	if v.Cursor != 1 {
		t.Errorf("Back: expected Cursor=1, got %d", v.Cursor)
	}
	if v.ScrollOffset > v.Cursor {
		t.Errorf("Back: expected ScrollOffset<=Cursor, got %d > %d", v.ScrollOffset, v.Cursor)
	}
}

func TestFullDrillDownCycle(t *testing.T) {
	v := View{
		Level:   ArtistLevel,
		Cursor:  2,
		Artists: []string{"Alice", "Bob", "Charlie"},
		Width:   80,
	}

	v = v.OpenAlbums([]string{"Alpha", "Beta"})
	if v.Level != AlbumLevel || v.Cursor != 0 {
		t.Fatalf("after OpenAlbums: Level=%v Cursor=%d", v.Level, v.Cursor)
	}

	v.Cursor = 1

	tracks := []library.Track{{Title: "Song A"}, {Title: "Song B"}}
	v = v.OpenTracks(tracks)
	if v.Level != TrackLevel || v.Cursor != 0 {
		t.Fatalf("after OpenTracks: Level=%v Cursor=%d", v.Level, v.Cursor)
	}

	v = v.Back()
	if v.Level != AlbumLevel {
		t.Errorf("first Back: expected AlbumLevel, got %v", v.Level)
	}
	if v.Cursor != 1 {
		t.Errorf("first Back: expected album Cursor=1, got %d", v.Cursor)
	}

	v = v.Back()
	if v.Level != ArtistLevel {
		t.Errorf("second Back: expected ArtistLevel, got %v", v.Level)
	}
	if v.Cursor != 2 {
		t.Errorf("second Back: expected artist Cursor=2, got %d", v.Cursor)
	}
}

// TestGoToBottomClampsScrollOffsetWhenListShrank guards a regression where
// GoToBottom kept a stale ScrollOffset above the new last index after a
// filter/delete shrank the list, leaving the cursor rendering above the
// visible window.
func TestGoToBottomClampsScrollOffsetWhenListShrank(t *testing.T) {
	v := View{
		Level:        ArtistLevel,
		Artists:      []string{"A", "B", "C"},
		Cursor:       0,
		ScrollOffset: 80,
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
