package artists

import (
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

func TestDeriveArtistsFromSortedTracks(t *testing.T) {
	tracks := sampleTracks()
	artists := DeriveArtists(tracks)
	want := []string{"Alice", "Bob", "Charlie"}
	if len(artists) != len(want) {
		t.Fatalf("DeriveArtists: expected %d artists, got %d", len(want), len(artists))
	}
	for i, w := range want {
		if artists[i] != w {
			t.Errorf("DeriveArtists[%d]: expected %q, got %q", i, w, artists[i])
		}
	}
}

func TestDeriveArtistsEmpty(t *testing.T) {
	artists := DeriveArtists(nil)
	if len(artists) != 0 {
		t.Errorf("DeriveArtists(nil): expected empty, got %v", artists)
	}
	artists = DeriveArtists(sampleTracks()[:0])
	if len(artists) != 0 {
		t.Errorf("DeriveArtists([]): expected empty, got %v", artists)
	}
}

func TestDeriveArtistsNormalizesLibraryUnknownArtist(t *testing.T) {
	tracks := []library.Track{
		{Artist: "Alice", Album: "A", Title: "A1"},
		{Artist: "", Album: "Mystery", Title: "Blank artist"},
		{Artist: libraryUnknownArtist, Album: "Fallback", Title: "Fallback artist"},
	}

	artists := DeriveArtists(tracks)
	want := []string{"Alice", UnknownArtist}
	if len(artists) != len(want) {
		t.Fatalf("DeriveArtists: expected %d artists, got %d: %v", len(want), len(artists), artists)
	}
	for i, w := range want {
		if artists[i] != w {
			t.Errorf("DeriveArtists[%d]: expected %q, got %q", i, w, artists[i])
		}
	}
}

func TestDeriveArtistsDeduplicatesUnknownAcrossNonAdjacentSentinels(t *testing.T) {
	tracks := []library.Track{
		{Artist: "", Album: "Mystery", Title: "Blank artist"},
		{Artist: "Alice", Album: "A", Title: "A1"},
		{Artist: libraryUnknownArtist, Album: "Fallback", Title: "Fallback artist"},
	}

	artists := DeriveArtists(tracks)
	want := []string{UnknownArtist, "Alice"}
	if len(artists) != len(want) {
		t.Fatalf("DeriveArtists: expected %d artists, got %d: %v", len(want), len(artists), artists)
	}
	for i, w := range want {
		if artists[i] != w {
			t.Errorf("DeriveArtists[%d]: expected %q, got %q", i, w, artists[i])
		}
	}
}

func TestDeriveArtistCounts(t *testing.T) {
	counts := DeriveArtistCounts(sampleTracks())
	want := map[string]int{"Alice": 3, "Bob": 2, "Charlie": 1}
	if len(counts) != len(want) {
		t.Fatalf("DeriveArtistCounts: expected %d entries, got %d (%v)", len(want), len(counts), counts)
	}
	for name, wantN := range want {
		if got := counts[name]; got != wantN {
			t.Errorf("DeriveArtistCounts[%q]: expected %d, got %d", name, wantN, got)
		}
	}
}

func TestDeriveArtistCountsUnknown(t *testing.T) {
	counts := DeriveArtistCounts([]library.Track{
		{Artist: "Alice", Title: "A1"},
		{Artist: "", Title: "?1"},
		{Artist: "", Title: "?2"},
		{Artist: libraryUnknownArtist, Title: "?3"},
	})
	if counts[UnknownArtist] != 3 {
		t.Errorf("expected UnknownArtist count = 3, got %d", counts[UnknownArtist])
	}
	if counts["Alice"] != 1 {
		t.Errorf("expected Alice count = 1, got %d", counts["Alice"])
	}
}

func TestDeriveArtistCountsEmpty(t *testing.T) {
	if got := DeriveArtistCounts(nil); got != nil {
		t.Errorf("DeriveArtistCounts(nil): expected nil, got %v", got)
	}
}

func TestDeriveAlbumsForArtist(t *testing.T) {
	tracks := sampleTracks()
	albums := DeriveAlbums(tracks, "Alice")
	want := []string{"Alpha", "Beta"}
	if len(albums) != len(want) {
		t.Fatalf("DeriveAlbums: expected %d albums, got %d", len(want), len(albums))
	}
	for i, w := range want {
		if albums[i] != w {
			t.Errorf("DeriveAlbums[%d]: expected %q, got %q", i, w, albums[i])
		}
	}
}

func TestDeriveAlbumsNoMatch(t *testing.T) {
	tracks := sampleTracks()
	albums := DeriveAlbums(tracks, "Nobody")
	if len(albums) != 0 {
		t.Errorf("DeriveAlbums no match: expected empty, got %v", albums)
	}
}

func TestDeriveAlbumsForUnknownArtistIncludesLibrarySentinel(t *testing.T) {
	tracks := []library.Track{
		{Artist: "", Album: "Blank", Title: "Blank artist"},
		{Artist: libraryUnknownArtist, Album: "Fallback", Title: "Fallback artist"},
	}
	albums := DeriveAlbums(tracks, UnknownArtist)
	want := []string{"Blank", "Fallback"}
	if len(albums) != len(want) {
		t.Fatalf("DeriveAlbums: expected %d albums, got %d: %v", len(want), len(albums), albums)
	}
	for i, w := range want {
		if albums[i] != w {
			t.Errorf("DeriveAlbums[%d]: expected %q, got %q", i, w, albums[i])
		}
	}
}

func TestDeriveTracksForArtistAndAlbum(t *testing.T) {
	tracks := sampleTracks()
	result := DeriveTracks(tracks, "Alice", "Alpha")
	if len(result) != 2 {
		t.Fatalf("DeriveTracks: expected 2 tracks, got %d", len(result))
	}
	if result[0].Title != "Track A1" {
		t.Errorf("DeriveTracks[0]: expected 'Track A1', got %q", result[0].Title)
	}
	if result[1].Title != "Track A2" {
		t.Errorf("DeriveTracks[1]: expected 'Track A2', got %q", result[1].Title)
	}
}

func TestDeriveTracksNoMatch(t *testing.T) {
	tracks := sampleTracks()
	result := DeriveTracks(tracks, "Alice", "NonExistent")
	if len(result) != 0 {
		t.Errorf("DeriveTracks no match: expected empty, got %v", result)
	}
}

func TestDeriveTracksForUnknownArtistIncludesLibrarySentinel(t *testing.T) {
	tracks := []library.Track{
		{Artist: "", Album: "Mystery", Title: "Blank artist"},
		{Artist: libraryUnknownArtist, Album: "Mystery", Title: "Fallback artist"},
		{Artist: "Alice", Album: "Mystery", Title: "Named artist"},
	}

	result := DeriveTracks(tracks, UnknownArtist, "Mystery")
	if len(result) != 2 {
		t.Fatalf("DeriveTracks: expected 2 unknown-artist tracks, got %d: %v", len(result), result)
	}
	for _, track := range result {
		if track.Artist == "Alice" {
			t.Fatalf("DeriveTracks included named artist track: %+v", track)
		}
	}
}

func TestDeriveTracksForUnknownArtistIncludesDisplaySentinel(t *testing.T) {
	tracks := []library.Track{
		{Artist: "", Album: "Mystery", Title: "Blank artist"},
		{Artist: UnknownArtist, Album: "Mystery", Title: "Display sentinel artist"},
		{Artist: "Alice", Album: "Mystery", Title: "Named artist"},
	}

	result := DeriveTracks(tracks, UnknownArtist, "Mystery")
	if len(result) != 2 {
		t.Fatalf("DeriveTracks: expected 2 unknown-artist tracks, got %d: %v", len(result), result)
	}
	if result[0].Title != "Blank artist" || result[1].Title != "Display sentinel artist" {
		t.Fatalf("DeriveTracks returned unexpected tracks: %v", result)
	}
}

func TestDeriveTracksForUnknownAlbumIncludesDisplaySentinel(t *testing.T) {
	tracks := []library.Track{
		{Artist: "Alice", Album: "", Title: "Blank album"},
		{Artist: "Alice", Album: UnknownAlbum, Title: "Display sentinel album"},
		{Artist: "Alice", Album: "Alpha", Title: "Named album"},
	}

	result := DeriveTracks(tracks, "Alice", UnknownAlbum)
	if len(result) != 2 {
		t.Fatalf("DeriveTracks: expected 2 unknown-album tracks, got %d: %v", len(result), result)
	}
	if result[0].Title != "Blank album" || result[1].Title != "Display sentinel album" {
		t.Fatalf("DeriveTracks returned unexpected tracks: %v", result)
	}
}
