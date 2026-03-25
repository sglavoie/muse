package genres

import (
	"testing"

	"github.com/sglavoie/muse/internal/library"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
)

// helpers

func tracks(specs ...struct{ genre, artist, title string }) []library.Track {
	out := make([]library.Track, len(specs))
	for i, s := range specs {
		out[i] = library.Track{
			Hash:   s.title, // use title as a unique hash for tests
			Genre:  s.genre,
			Artist: s.artist,
			Title:  s.title,
		}
	}
	return out
}

// ---------- DeriveGenres ----------

func TestDeriveGenres_empty(t *testing.T) {
	if got := DeriveGenres(nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestDeriveGenres_multiGenreSplitAndDedup(t *testing.T) {
	ts := tracks(
		struct{ genre, artist, title string }{"Rock; Jazz", "A", "t1"},
		struct{ genre, artist, title string }{"Jazz", "B", "t2"},
		struct{ genre, artist, title string }{"Rock", "C", "t3"},
	)
	got := DeriveGenres(ts)
	want := []string{"Jazz", "Rock"}
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] want %q, got %q", i, want[i], got[i])
		}
	}
}

func TestDeriveGenres_unknownLast(t *testing.T) {
	ts := tracks(
		struct{ genre, artist, title string }{"", "A", "t1"},
		struct{ genre, artist, title string }{"Rock", "B", "t2"},
	)
	got := DeriveGenres(ts)
	want := []string{"Rock", UnknownGenre}
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] want %q, got %q", i, want[i], got[i])
		}
	}
}

func TestDeriveGenres_sorting(t *testing.T) {
	ts := tracks(
		struct{ genre, artist, title string }{"Zebra", "A", "t1"},
		struct{ genre, artist, title string }{"Apple", "B", "t2"},
		struct{ genre, artist, title string }{"Mango", "C", "t3"},
	)
	got := DeriveGenres(ts)
	want := []string{"Apple", "Mango", "Zebra"}
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] want %q, got %q", i, want[i], got[i])
		}
	}
}

// ---------- DeriveGenreArtists ----------

func TestDeriveGenreArtists(t *testing.T) {
	ts := tracks(
		struct{ genre, artist, title string }{"Rock", "Alice", "t1"},
		struct{ genre, artist, title string }{"Rock", "Bob", "t2"},
		struct{ genre, artist, title string }{"Jazz", "Alice", "t3"},
		struct{ genre, artist, title string }{"Rock", "Alice", "t4"}, // duplicate artist
	)
	got := DeriveGenreArtists(ts, "Rock")
	want := []string{"Alice", "Bob"}
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] want %q, got %q", i, want[i], got[i])
		}
	}
}

func TestDeriveGenreArtists_unknownLast(t *testing.T) {
	ts := tracks(
		struct{ genre, artist, title string }{"Rock", "", "t1"},
		struct{ genre, artist, title string }{"Rock", "Alice", "t2"},
	)
	got := DeriveGenreArtists(ts, "Rock")
	want := []string{"Alice", arttui.UnknownArtist}
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] want %q, got %q", i, want[i], got[i])
		}
	}
}

func TestDeriveGenreArtists_unknownGenre(t *testing.T) {
	ts := tracks(
		struct{ genre, artist, title string }{"", "Alice", "t1"},
		struct{ genre, artist, title string }{"Rock", "Bob", "t2"},
	)
	got := DeriveGenreArtists(ts, UnknownGenre)
	want := []string{"Alice"}
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	if got[0] != want[0] {
		t.Errorf("want %q, got %q", want[0], got[0])
	}
}

// ---------- DeriveGenreArtistTracks ----------

func TestDeriveGenreArtistTracks(t *testing.T) {
	ts := tracks(
		struct{ genre, artist, title string }{"Rock", "Alice", "t1"},
		struct{ genre, artist, title string }{"Rock", "Bob", "t2"},
		struct{ genre, artist, title string }{"Jazz", "Alice", "t3"},
		struct{ genre, artist, title string }{"Rock", "Alice", "t4"},
	)
	got := DeriveGenreArtistTracks(ts, "Rock", "Alice")
	if len(got) != 2 {
		t.Fatalf("want 2 tracks, got %d: %v", len(got), got)
	}
	if got[0].Title != "t1" || got[1].Title != "t4" {
		t.Errorf("unexpected tracks: %v", got)
	}
}

// ---------- OpenArtists / OpenTracks / Back cursor preservation ----------

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
	v = v.Back() // back to ArtistLevel
	v = v.Back() // back to GenreLevel
	if v.Level != GenreLevel {
		t.Errorf("expected GenreLevel, got %v", v.Level)
	}
	if v.Cursor != 1 {
		t.Errorf("expected cursor=1, got %d", v.Cursor)
	}
}

// ---------- EffectiveGenres with filter ----------

func TestEffectiveGenresFilter(t *testing.T) {
	v := View{
		Genres:     []string{"Rock", "Jazz", "Electronic"},
		FilterText: "jaz",
	}
	got := v.EffectiveGenres()
	if len(got) != 1 || got[0] != "Jazz" {
		t.Errorf("expected [Jazz], got %v", got)
	}
}

func TestEffectiveGenresNoFilter(t *testing.T) {
	v := View{Genres: []string{"Rock", "Jazz"}}
	got := v.EffectiveGenres()
	if len(got) != 2 {
		t.Errorf("expected 2 genres, got %v", got)
	}
}

// ---------- visibleRows reserve calculations ----------

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
