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

func TestOpenArtistsTranslatesFilteredCursorToUnfilteredIndex(t *testing.T) {
	v := View{
		Level:        GenreLevel,
		Genres:       []string{"Ambient", "Blues", "Classical", "Dub", "Electronic"},
		FilterActive: true,
		FilterText:   "d",
		Cursor:       0, // filtered list = ["Dub"], cursor=0 points at Dub
	}

	v = v.OpenArtists([]string{"Alice"})
	v = v.Back()
	if v.Level != GenreLevel {
		t.Fatalf("expected GenreLevel after Back, got %v", v.Level)
	}
	if v.Cursor != 3 {
		t.Errorf("Back after filtered OpenArtists: expected Cursor=3 (Dub's index in unfiltered list), got %d", v.Cursor)
	}
}

func TestOpenArtistsTranslatesClampedFilteredCursorToUnfilteredIndex(t *testing.T) {
	v := View{
		Level:        GenreLevel,
		Genres:       []string{"Ambient", "Blues", "Classical", "Dub", "Electronic"},
		FilterActive: true,
		FilterText:   "d",
		Cursor:       99, // filtered list = ["Dub"], render clamps this to Dub
	}

	v = v.OpenArtists([]string{"Alice"})
	v = v.Back()
	if v.Level != GenreLevel {
		t.Fatalf("expected GenreLevel after Back, got %v", v.Level)
	}
	if v.Cursor != 3 {
		t.Errorf("Back after filtered OpenArtists: expected Cursor=3 (Dub's index in unfiltered list), got %d", v.Cursor)
	}
}

func TestOpenTracksTranslatesFilteredArtistCursorToUnfilteredIndex(t *testing.T) {
	v := View{
		Level:        ArtistLevel,
		Artists:      []string{"Alice", "Bob", "Carol", "Dave"},
		FilterActive: true,
		FilterText:   "d",
		Cursor:       0, // filtered list = ["Dave"], cursor=0 points at Dave
	}

	v = v.OpenTracks(nil)
	v = v.Back()
	if v.Level != ArtistLevel {
		t.Fatalf("expected ArtistLevel after Back, got %v", v.Level)
	}
	if v.Cursor != 3 {
		t.Errorf("Back after filtered OpenTracks: expected Cursor=3 (Dave's index in unfiltered list), got %d", v.Cursor)
	}
}

func TestOpenTracksTranslatesClampedFilteredArtistCursorToUnfilteredIndex(t *testing.T) {
	v := View{
		Level:        ArtistLevel,
		Artists:      []string{"Alice", "Bob", "Carol", "Dave"},
		FilterActive: true,
		FilterText:   "d",
		Cursor:       99, // filtered list = ["Dave"], render clamps this to Dave
	}

	v = v.OpenTracks(nil)
	v = v.Back()
	if v.Level != ArtistLevel {
		t.Fatalf("expected ArtistLevel after Back, got %v", v.Level)
	}
	if v.Cursor != 3 {
		t.Errorf("Back after filtered OpenTracks: expected Cursor=3 (Dave's index in unfiltered list), got %d", v.Cursor)
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
