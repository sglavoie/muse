package albums

import (
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

func sampleTracks() []library.Track {
	return []library.Track{
		{Artist: "Alice", Album: "Alpha", Title: "Track A1", Duration: 183},
		{Artist: "Alice", Album: "Alpha", Title: "Track A2", Duration: 245},
		{Artist: "Alice", Album: "Beta", Title: "Track B1", Duration: 300},
		{Artist: "Bob", Album: "Gamma", Title: "Track G1", Duration: 120},
		{Artist: "Bob", Album: "Gamma", Title: "Track G2", Duration: 60},
		{Artist: "Charlie", Album: "Delta", Title: "Track D1", Duration: 3661},
	}
}

func TestDeriveAllAlbumsUniqueSorted(t *testing.T) {
	albums := DeriveAllAlbums(sampleTracks())
	want := []string{"Alpha", "Beta", "Delta", "Gamma"}
	if len(albums) != len(want) {
		t.Fatalf("got %d albums, want %d: %v", len(albums), len(want), albums)
	}
	for i, album := range albums {
		if album != want[i] {
			t.Errorf("albums[%d] = %q, want %q", i, album, want[i])
		}
	}
}

func TestDeriveAllAlbumsEmpty(t *testing.T) {
	albums := DeriveAllAlbums(nil)
	if len(albums) != 0 {
		t.Errorf("expected empty, got %v", albums)
	}
}

func TestDeriveAllAlbumCounts(t *testing.T) {
	counts := DeriveAllAlbumCounts(sampleTracks())
	want := map[string]int{"Alpha": 2, "Beta": 1, "Gamma": 2, "Delta": 1}
	if len(counts) != len(want) {
		t.Fatalf("DeriveAllAlbumCounts: expected %d entries, got %d (%v)", len(want), len(counts), counts)
	}
	for name, wantCount := range want {
		if got := counts[name]; got != wantCount {
			t.Errorf("DeriveAllAlbumCounts[%q]: expected %d, got %d", name, wantCount, got)
		}
	}
}

func TestDeriveAllAlbumCountsEmptyAndUnknown(t *testing.T) {
	if got := DeriveAllAlbumCounts(nil); got != nil {
		t.Errorf("DeriveAllAlbumCounts(nil): expected nil, got %v", got)
	}

	counts := DeriveAllAlbumCounts([]library.Track{
		{Album: "Alpha"},
		{Album: ""},
		{Album: ""},
	})
	if counts["(unknown album)"] != 2 {
		t.Errorf("expected unknown-album count = 2, got %d", counts["(unknown album)"])
	}
}

func TestDeriveAlbumTracks(t *testing.T) {
	tracks := DeriveAlbumTracks(sampleTracks(), "Alpha")
	if len(tracks) != 2 {
		t.Fatalf("got %d tracks, want 2", len(tracks))
	}
	if tracks[0].Title != "Track A1" || tracks[1].Title != "Track A2" {
		t.Errorf("unexpected tracks: %v", tracks)
	}
}

func TestDeriveAlbumTracksUnknownAlbumIncludesLiteralSentinel(t *testing.T) {
	tracks := []library.Track{
		{Album: "", Title: "Empty album"},
		{Album: "(unknown album)", Title: "Literal sentinel album"},
		{Album: "Alpha", Title: "Named album"},
	}

	got := DeriveAlbumTracks(tracks, "(unknown album)")
	if len(got) != 2 {
		t.Fatalf("got %d tracks, want 2: %v", len(got), got)
	}
	if got[0].Title != "Empty album" || got[1].Title != "Literal sentinel album" {
		t.Errorf("unexpected tracks: %v", got)
	}
}

func TestDeriveAlbumTracksNoMatch(t *testing.T) {
	tracks := DeriveAlbumTracks(sampleTracks(), "Nonexistent")
	if len(tracks) != 0 {
		t.Errorf("expected empty, got %v", tracks)
	}
}
