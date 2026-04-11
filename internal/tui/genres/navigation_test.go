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
	if got := v.visibleRows(); got != 20 {
		t.Errorf("want 20, got %d", got)
	}
}

func TestVisibleRowsGenreLevelWithFilter(t *testing.T) {
	v := View{Level: GenreLevel, Height: 20, FilterActive: true}
	if got := v.visibleRows(); got != 19 {
		t.Errorf("want 19, got %d", got)
	}
}

func TestVisibleRowsArtistLevel(t *testing.T) {
	v := View{Level: ArtistLevel, Height: 20}
	if got := v.visibleRows(); got != 17 {
		t.Errorf("want 17 (20-3), got %d", got)
	}
}

func TestVisibleRowsTrackLevel(t *testing.T) {
	v := View{Level: TrackLevel, Height: 20}
	if got := v.visibleRows(); got != 17 {
		t.Errorf("want 17 (20-3), got %d", got)
	}
}

func TestVisibleRowsSortIndicator(t *testing.T) {
	v := View{Level: GenreLevel, Height: 20, SortIndicator: "Sort: Name ▲"}
	if got := v.visibleRows(); got != 19 {
		t.Errorf("want 19 (20-1 for sort indicator), got %d", got)
	}
}

func TestVisibleRowsGenreLevelWithFilterText(t *testing.T) {
	v := View{Level: GenreLevel, Height: 20, FilterText: "roc"}
	if got := v.visibleRows(); got != 19 {
		t.Errorf("want 19 (filter text reserves 1 row), got %d", got)
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
	if v2.Cursor != 3 {
		t.Errorf("expected cursor=3, got %d", v2.Cursor)
	}
}
