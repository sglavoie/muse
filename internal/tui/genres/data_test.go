package genres

import (
	"testing"

	arttui "github.com/sglavoie/muse/internal/tui/artists"
)

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

func TestDeriveGenreCounts(t *testing.T) {
	ts := tracks(
		struct{ genre, artist, title string }{"Rock; Jazz", "A", "t1"},
		struct{ genre, artist, title string }{"Jazz", "B", "t2"},
		struct{ genre, artist, title string }{"Rock", "C", "t3"},
		struct{ genre, artist, title string }{"", "D", "t4"},
		struct{ genre, artist, title string }{"", "E", "t5"},
	)
	got := DeriveGenreCounts(ts)
	want := map[string]int{"Rock": 2, "Jazz": 2, UnknownGenre: 2}
	if len(got) != len(want) {
		t.Fatalf("DeriveGenreCounts: want %v, got %v", want, got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("DeriveGenreCounts[%q]: want %d, got %d", k, v, got[k])
		}
	}
}

func TestDeriveGenreCounts_empty(t *testing.T) {
	if got := DeriveGenreCounts(nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
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

func TestDeriveGenreArtists(t *testing.T) {
	ts := tracks(
		struct{ genre, artist, title string }{"Rock", "Alice", "t1"},
		struct{ genre, artist, title string }{"Rock", "Bob", "t2"},
		struct{ genre, artist, title string }{"Jazz", "Alice", "t3"},
		struct{ genre, artist, title string }{"Rock", "Alice", "t4"},
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

func TestDeriveGenreArtists_NormalizesLibraryUnknownArtist(t *testing.T) {
	ts := tracks(
		struct{ genre, artist, title string }{"Rock", "", "empty"},
		struct{ genre, artist, title string }{"Rock", libraryUnknownArtist, "fallback"},
		struct{ genre, artist, title string }{"Rock", "Alice", "known"},
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

func TestDeriveGenreArtists_UnknownGenreIncludesLiteralSentinelTag(t *testing.T) {
	ts := tracks(
		struct{ genre, artist, title string }{"", "Alice", "t1"},
		struct{ genre, artist, title string }{UnknownGenre, "Bob", "t2"},
		struct{ genre, artist, title string }{"Rock", "Carol", "t3"},
	)
	got := DeriveGenreArtists(ts, UnknownGenre)
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

func TestDeriveGenreArtistTracks_UnknownArtistIncludesLibrarySentinel(t *testing.T) {
	ts := tracks(
		struct{ genre, artist, title string }{"Rock", "", "empty"},
		struct{ genre, artist, title string }{"Rock", libraryUnknownArtist, "fallback"},
		struct{ genre, artist, title string }{"Rock", "Alice", "known"},
		struct{ genre, artist, title string }{"Jazz", libraryUnknownArtist, "other genre"},
	)
	got := DeriveGenreArtistTracks(ts, "Rock", arttui.UnknownArtist)
	if len(got) != 2 {
		t.Fatalf("want 2 tracks, got %d: %v", len(got), got)
	}
	if got[0].Title != "empty" || got[1].Title != "fallback" {
		t.Errorf("unexpected tracks: %v", got)
	}
}

func TestDeriveGenreArtistTracks_UnknownArtistIncludesDisplaySentinel(t *testing.T) {
	ts := tracks(
		struct{ genre, artist, title string }{"Rock", "", "empty"},
		struct{ genre, artist, title string }{"Rock", arttui.UnknownArtist, "display"},
		struct{ genre, artist, title string }{"Rock", "Alice", "known"},
	)
	got := DeriveGenreArtistTracks(ts, "Rock", arttui.UnknownArtist)
	if len(got) != 2 {
		t.Fatalf("want 2 tracks, got %d: %v", len(got), got)
	}
	if got[0].Title != "empty" || got[1].Title != "display" {
		t.Errorf("unexpected tracks: %v", got)
	}
}

func TestDeriveGenreArtistTracks_UnknownGenreIncludesLiteralSentinelTag(t *testing.T) {
	ts := tracks(
		struct{ genre, artist, title string }{"", "Alice", "empty"},
		struct{ genre, artist, title string }{UnknownGenre, "Alice", "literal"},
		struct{ genre, artist, title string }{"Rock", "Alice", "rock"},
	)
	got := DeriveGenreArtistTracks(ts, UnknownGenre, "Alice")
	if len(got) != 2 {
		t.Fatalf("want 2 tracks, got %d: %v", len(got), got)
	}
	if got[0].Title != "empty" || got[1].Title != "literal" {
		t.Errorf("unexpected tracks: %v", got)
	}
}

func TestDeriveGenreTracks(t *testing.T) {
	ts := tracks(
		struct{ genre, artist, title string }{"Rock", "Alice", "t1"},
		struct{ genre, artist, title string }{"Jazz", "Bob", "t2"},
		struct{ genre, artist, title string }{"Rock; Jazz", "Carol", "t3"},
		struct{ genre, artist, title string }{"Pop", "Dave", "t4"},
	)
	got := DeriveGenreTracks(ts, "Rock")
	if len(got) != 2 {
		t.Fatalf("want 2 tracks, got %d: %v", len(got), got)
	}
	if got[0].Title != "t1" || got[1].Title != "t3" {
		t.Errorf("unexpected tracks: %v", got)
	}
}

func TestDeriveGenreTracks_UnknownGenre(t *testing.T) {
	ts := tracks(
		struct{ genre, artist, title string }{"", "Alice", "t1"},
		struct{ genre, artist, title string }{"Rock", "Bob", "t2"},
		struct{ genre, artist, title string }{"", "Carol", "t3"},
	)
	got := DeriveGenreTracks(ts, UnknownGenre)
	if len(got) != 2 {
		t.Fatalf("want 2 tracks, got %d", len(got))
	}
	if got[0].Title != "t1" || got[1].Title != "t3" {
		t.Errorf("unexpected tracks: %v", got)
	}
}

func TestDeriveGenreTracks_UnknownGenreIncludesLiteralSentinelTag(t *testing.T) {
	ts := tracks(
		struct{ genre, artist, title string }{"", "Alice", "empty"},
		struct{ genre, artist, title string }{UnknownGenre, "Bob", "literal"},
		struct{ genre, artist, title string }{"Rock", "Carol", "rock"},
	)
	got := DeriveGenreTracks(ts, UnknownGenre)
	if len(got) != 2 {
		t.Fatalf("want 2 tracks, got %d: %v", len(got), got)
	}
	if got[0].Title != "empty" || got[1].Title != "literal" {
		t.Errorf("unexpected tracks: %v", got)
	}
}

func TestDeriveGenreTracks_NoMatches(t *testing.T) {
	ts := tracks(
		struct{ genre, artist, title string }{"Rock", "Alice", "t1"},
	)
	got := DeriveGenreTracks(ts, "Jazz")
	if len(got) != 0 {
		t.Errorf("want 0 tracks, got %d", len(got))
	}
}

func TestRawGenre_Unknown(t *testing.T) {
	if got := RawGenre(UnknownGenre); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestRawGenre_Normal(t *testing.T) {
	if got := RawGenre("Rock"); got != "Rock" {
		t.Errorf("expected Rock, got %q", got)
	}
}
