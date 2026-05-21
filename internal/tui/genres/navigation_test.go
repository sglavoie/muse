package genres

import "testing"

func TestOpenArtistsPreservesCursor(t *testing.T) {
	v := View{
		Level:        GenreLevel,
		Genres:       []string{"Rock", "Jazz", "Pop"},
		Cursor:       2,
		ScrollOffset: 1,
	}
	v2 := v.OpenArtists([]string{"Alice", "Bob"})
	if v2.Level != ArtistLevel {
		t.Errorf("expected ArtistLevel, got %v", v2.Level)
	}
	if v2.GenreCursor != 2 {
		t.Errorf("expected GenreCursor=2, got %d", v2.GenreCursor)
	}
	if v2.GenreScrollOffset != 1 {
		t.Errorf("expected GenreScrollOffset=1, got %d", v2.GenreScrollOffset)
	}
	if v2.Cursor != 0 || v2.ScrollOffset != 0 {
		t.Errorf("expected cursor/scroll reset, got cursor=%d scroll=%d", v2.Cursor, v2.ScrollOffset)
	}
}

func TestOpenTracksPreservesCursor(t *testing.T) {
	v := View{
		Level:        ArtistLevel,
		Artists:      []string{"Alice", "Bob"},
		Cursor:       1,
		ScrollOffset: 0,
	}
	ts := tracks(struct{ genre, artist, title string }{"Rock", "Alice", "t1"})
	v2 := v.OpenTracks(ts)
	if v2.Level != TrackLevel {
		t.Errorf("expected TrackLevel, got %v", v2.Level)
	}
	if v2.ArtistCursor != 1 {
		t.Errorf("expected ArtistCursor=1, got %d", v2.ArtistCursor)
	}
	if v2.Cursor != 0 {
		t.Errorf("expected cursor reset to 0, got %d", v2.Cursor)
	}
}

func TestBackFromArtistLevel(t *testing.T) {
	v := View{
		Level:             ArtistLevel,
		Genres:            []string{"Rock", "Jazz", "Pop"},
		Artists:           []string{"Alice"},
		Cursor:            0,
		GenreCursor:       2,
		GenreScrollOffset: 1,
	}
	v2 := v.Back()
	if v2.Level != GenreLevel {
		t.Errorf("expected GenreLevel, got %v", v2.Level)
	}
	if v2.Cursor != 2 {
		t.Errorf("expected cursor=2, got %d", v2.Cursor)
	}
	if v2.ScrollOffset != 1 {
		t.Errorf("expected scrollOffset=1, got %d", v2.ScrollOffset)
	}
	if v2.Artists != nil {
		t.Errorf("expected Artists cleared, got %v", v2.Artists)
	}
}

func TestBackFromTrackLevel(t *testing.T) {
	ts := tracks(struct{ genre, artist, title string }{"Rock", "Alice", "t1"})
	v := View{
		Level:              TrackLevel,
		Artists:            []string{"Alice", "Bob"},
		Tracks:             ts,
		Cursor:             0,
		ArtistCursor:       1,
		ArtistScrollOffset: 0,
	}
	v2 := v.Back()
	if v2.Level != ArtistLevel {
		t.Errorf("expected ArtistLevel, got %v", v2.Level)
	}
	if v2.Cursor != 1 {
		t.Errorf("expected cursor=1, got %d", v2.Cursor)
	}
	if v2.Tracks != nil {
		t.Errorf("expected Tracks cleared, got %v", v2.Tracks)
	}
}

func TestBackRoundTrip(t *testing.T) {
	ts := tracks(struct{ genre, artist, title string }{"Rock", "Alice", "t1"})
	v := View{
		Level:        GenreLevel,
		Genres:       []string{"Rock", "Jazz"},
		Cursor:       1,
		ScrollOffset: 0,
	}
	v = v.OpenArtists([]string{"Alice"})
	v = v.OpenTracks(ts)
	v = v.Back()
	v = v.Back()
	if v.Level != GenreLevel {
		t.Errorf("expected GenreLevel, got %v", v.Level)
	}
	if v.Cursor != 1 {
		t.Errorf("expected cursor=1, got %d", v.Cursor)
	}
}

func TestVisibleRowsGenreLevel(t *testing.T) {
	v := View{Level: GenreLevel, Height: 20}
	if got := v.visibleRows(); got != 19 {
		t.Errorf("want 19 (20-1 for strip), got %d", got)
	}
}

func TestVisibleRowsGenreLevelWithFilter(t *testing.T) {
	// Strip absorbs the filter chunk so reserve stays at 1 regardless
	// of filter state.
	v := View{Level: GenreLevel, Height: 20, FilterActive: true}
	if got := v.visibleRows(); got != 19 {
		t.Errorf("want 19, got %d", got)
	}
}

func TestVisibleRowsArtistLevel(t *testing.T) {
	v := View{Level: ArtistLevel, Height: 20}
	if got := v.visibleRows(); got != 19 {
		t.Errorf("want 19 (20-1 for strip; breadcrumb replaces heading), got %d", got)
	}
}

func TestVisibleRowsTrackLevel(t *testing.T) {
	v := View{Level: TrackLevel, Height: 20}
	if got := v.visibleRows(); got != 18 {
		t.Errorf("want 18 (20-2 for strip + col header; breadcrumb replaces heading), got %d", got)
	}
}

func TestVisibleRowsGenreLevelWithFilterText(t *testing.T) {
	// Strip absorbs the filter chunk so reserve stays at 1.
	v := View{Level: GenreLevel, Height: 20, FilterText: "roc"}
	if got := v.visibleRows(); got != 19 {
		t.Errorf("want 19, got %d", got)
	}
}

func TestCursorDown_AtBottom(t *testing.T) {
	v := View{
		Level:  GenreLevel,
		Genres: []string{"A", "B", "C"},
		Cursor: 2,
		Height: 20,
	}
	v2 := v.CursorDown()
	if v2.Cursor != 2 {
		t.Errorf("expected cursor=2 at bottom, got %d", v2.Cursor)
	}
}

func TestCursorUp_AtTop(t *testing.T) {
	v := View{
		Level:  GenreLevel,
		Genres: []string{"A", "B"},
		Cursor: 0,
	}
	v2 := v.CursorUp()
	if v2.Cursor != 0 {
		t.Errorf("expected cursor=0 at top, got %d", v2.Cursor)
	}
}

func TestGoToBottom_WithScroll(t *testing.T) {
	v := View{
		Level:  GenreLevel,
		Genres: []string{"A", "B", "C", "D", "E"},
		Cursor: 0,
		Height: 3,
	}
	v2 := v.GoToBottom()
	if v2.Cursor != 4 {
		t.Errorf("expected cursor=4, got %d", v2.Cursor)
	}
	if v2.ScrollOffset < 2 {
		t.Errorf("expected scrollOffset >= 2, got %d", v2.ScrollOffset)
	}
}

func TestGoToTop_ResetsScroll(t *testing.T) {
	v := View{
		Level:        GenreLevel,
		Genres:       []string{"A", "B", "C", "D"},
		Cursor:       3,
		ScrollOffset: 2,
	}
	v2 := v.GoToTop()
	if v2.Cursor != 0 || v2.ScrollOffset != 0 {
		t.Errorf("expected cursor=0 scroll=0, got cursor=%d scroll=%d", v2.Cursor, v2.ScrollOffset)
	}
}

func TestMoveCursor_EmptyGenres(t *testing.T) {
	v := View{Level: GenreLevel, Genres: nil, Height: 20}
	v2 := v.moveCursor(5)
	if v2.Cursor != 0 {
		t.Errorf("expected cursor=0 for empty, got %d", v2.Cursor)
	}
}

func TestHalfPageDown(t *testing.T) {
	v := View{
		Level:  GenreLevel,
		Genres: []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J"},
		Cursor: 0,
		Height: 6,
	}
	v2 := v.HalfPageDown()
	// visibleRows = height(6) - reserve(1, strip) = 5; HalfPageDown = 5/2 = 2
	if v2.Cursor != 2 {
		t.Errorf("expected cursor=2, got %d", v2.Cursor)
	}
}

func TestBack_ClampsCursorWhenParentShrank(t *testing.T) {
	v := View{
		Level:        GenreLevel,
		Genres:       []string{"a", "b", "c", "d", "e"},
		Cursor:       4,
		ScrollOffset: 2,
		Height:       20,
	}
	v = v.OpenArtists([]string{"X"})
	v.Genres = []string{"a", "b"}
	v = v.Back()
	if v.Level != GenreLevel {
		t.Fatalf("expected GenreLevel, got %v", v.Level)
	}
	if v.Cursor != 1 {
		t.Errorf("Back: expected clamped Cursor=1, got %d", v.Cursor)
	}
	if v.ScrollOffset > v.Cursor {
		t.Errorf("Back: ScrollOffset(%d) > Cursor(%d)", v.ScrollOffset, v.Cursor)
	}
}

func TestBack_ClampsScrollToKeepCursorVisible(t *testing.T) {
	v := View{
		Level:        GenreLevel,
		Genres:       []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"},
		Cursor:       9,
		ScrollOffset: 0,
		Height:       6,
	}
	v = v.OpenArtists([]string{"X"})
	v.Genres = []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	v = v.Back()
	if v.Level != GenreLevel {
		t.Fatalf("expected GenreLevel, got %v", v.Level)
	}
	if v.Cursor != 7 {
		t.Fatalf("expected clamped Cursor=7, got %d", v.Cursor)
	}
	if v.ScrollOffset != 3 {
		t.Fatalf("expected ScrollOffset=3 to keep cursor visible, got %d", v.ScrollOffset)
	}
}

func TestBack_ClampsCursorWhenParentEmpty(t *testing.T) {
	v := View{
		Level:        ArtistLevel,
		Artists:      []string{"a", "b", "c"},
		Cursor:       2,
		ScrollOffset: 1,
		Height:       20,
	}
	v = v.OpenTracks(nil)
	v.Artists = nil
	v = v.Back()
	if v.Cursor != 0 || v.ScrollOffset != 0 {
		t.Errorf("Back with empty parent: expected cursor=0 scroll=0, got cursor=%d scroll=%d", v.Cursor, v.ScrollOffset)
	}
}
