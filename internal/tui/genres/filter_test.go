package genres

import "testing"

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

func TestEffectiveArtists(t *testing.T) {
	v := View{
		Level:   ArtistLevel,
		Artists: []string{"Alice", "Bob", "Carol"},
	}
	got := v.EffectiveArtists()
	if len(got) != 3 {
		t.Errorf("expected 3, got %d", len(got))
	}
}

func TestEffectiveArtistsFiltered(t *testing.T) {
	v := View{
		Level:      ArtistLevel,
		Artists:    []string{"Alice", "Bob", "Carol"},
		FilterText: "bob",
	}
	got := v.EffectiveArtists()
	if len(got) != 1 || got[0] != "Bob" {
		t.Errorf("expected [Bob], got %v", got)
	}
}

func TestEffectiveTracksFiltered(t *testing.T) {
	ts := tracks(
		struct{ genre, artist, title string }{"Rock", "Alice", "hello"},
		struct{ genre, artist, title string }{"Rock", "Bob", "world"},
	)
	v := View{
		Level:      TrackLevel,
		Tracks:     ts,
		FilterText: "hel",
	}
	got := v.EffectiveTracks()
	if len(got) != 1 || got[0].Title != "hello" {
		t.Errorf("expected [hello], got %v", got)
	}
}
