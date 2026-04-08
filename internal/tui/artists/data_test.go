package artists

import "testing"

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
