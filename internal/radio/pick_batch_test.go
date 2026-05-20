package radio

import (
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

func TestPickBatch_BasicCount(t *testing.T) {
	tracks := make([]library.Track, 10)
	for i := range tracks {
		tracks[i] = library.Track{Hash: string(rune('a' + i)), Artist: "ArtistX"}
	}
	scored := ScoreAll(ScoreInput{
		Seed:      SeedTrack{Hash: "seed"},
		AllTracks: tracks,
	})
	result := PickBatch(scored, 5, 0.4)
	if len(result) > 5 {
		t.Errorf("expected at most 5 tracks, got %d", len(result))
	}
	if len(result) == 0 {
		t.Errorf("expected non-empty result")
	}
}

func TestPickBatch_SmallLibrary(t *testing.T) {
	tracks := []library.Track{
		{Hash: "t1", Artist: "ArtistA"},
		{Hash: "t2", Artist: "ArtistB"},
	}
	scored := ScoreAll(ScoreInput{
		Seed:      SeedTrack{Hash: "seed"},
		AllTracks: tracks,
	})
	result := PickBatch(scored, 10, 0.4)
	if len(result) > 2 {
		t.Errorf("small library: expected at most 2 tracks, got %d", len(result))
	}
}

func TestPickBatch_ArtistCap(t *testing.T) {
	var tracks []library.Track
	for i := 0; i < 8; i++ {
		tracks = append(tracks, library.Track{Hash: string(rune('a' + i)), Artist: "ArtistA", Genre: "Rock"})
	}
	tracks = append(tracks, library.Track{Hash: "x1", Artist: "ArtistB", Genre: "Rock"})
	tracks = append(tracks, library.Track{Hash: "x2", Artist: "ArtistB", Genre: "Rock"})

	scored := ScoreAll(ScoreInput{
		Seed:      SeedTrack{Hash: "seed", Genres: []string{"Rock"}},
		AllTracks: tracks,
	})
	result := PickBatch(scored, 5, 0.4)

	artistCounts := make(map[string]int)
	for _, tr := range result {
		artistCounts[tr.Artist]++
	}
	maxPerArtist := 2
	for artist, cnt := range artistCounts {
		if cnt > maxPerArtist {
			t.Errorf("artist %q appears %d times (max %d)", artist, cnt, maxPerArtist)
		}
	}
}

func TestPickBatch_EmptyArtistBypassesCap(t *testing.T) {
	var scored []TrackScore
	for i := 0; i < 6; i++ {
		scored = append(scored, TrackScore{
			Track: library.Track{Hash: string(rune('a' + i)), Artist: ""},
			Score: 10,
		})
	}
	result := PickBatch(scored, 5, 0.4)
	if len(result) != 5 {
		t.Errorf("expected 5 tracks with empty artists, got %d", len(result))
	}
}

func TestPickBatch_EmptyInput(t *testing.T) {
	if got := PickBatch(nil, 5, 0.4); got != nil {
		t.Errorf("expected nil for empty scored input, got %v", got)
	}
	if got := PickBatch([]TrackScore{}, 5, 0.4); got != nil {
		t.Errorf("expected nil for empty scored input, got %v", got)
	}
}

func TestPickBatch_ZeroN(t *testing.T) {
	scored := []TrackScore{{Track: library.Track{Hash: "t1"}, Score: 10}}
	if got := PickBatch(scored, 0, 0.4); got != nil {
		t.Errorf("expected nil for n=0, got %v", got)
	}
}

func TestPickBatch_NoDuplicates(t *testing.T) {
	tracks := make([]library.Track, 20)
	for i := range tracks {
		tracks[i] = library.Track{Hash: string(rune('a' + i)), Artist: "ArtistA"}
	}
	scored := ScoreAll(ScoreInput{
		Seed:      SeedTrack{Hash: "seed"},
		AllTracks: tracks,
	})
	result := PickBatch(scored, 5, 1.0)
	seen := make(map[string]bool)
	for _, tr := range result {
		if seen[tr.Hash] {
			t.Errorf("duplicate track %s in batch", tr.Hash)
		}
		seen[tr.Hash] = true
	}
}

// Regression: when every eligible candidate has a non-positive score (e.g.,
// all penalized by recent-history / queue / dismissed), every weight collapses
// to the 0.1 floor and the weighted-random draw would degenerate into a
// uniform pick over the pool — silently discarding ScoreAll's ordering.
// PickBatch must fall back to deterministic selection (highest-scored first,
// preserved by ScoreAll's stable descending sort) in that case.
func TestPickBatch_DeterministicFallbackWhenAllNonPositive(t *testing.T) {
	// Build a pool where every score is <= 0 but ordered descending. The
	// first entry is the "least bad" (-1); subsequent entries are worse.
	scored := []TrackScore{
		{Track: library.Track{Hash: "best", Artist: ""}, Score: -1},
		{Track: library.Track{Hash: "mid1", Artist: ""}, Score: -50},
		{Track: library.Track{Hash: "mid2", Artist: ""}, Score: -50},
		{Track: library.Track{Hash: "worst1", Artist: ""}, Score: -100},
		{Track: library.Track{Hash: "worst2", Artist: ""}, Score: -100},
		{Track: library.Track{Hash: "worst3", Artist: ""}, Score: -100},
	}

	// Run multiple times — with the uniform-random bug, "best" would only
	// appear ~1/n of the time. With the deterministic fallback it must
	// always be the first pick.
	for run := 0; run < 10; run++ {
		result := PickBatch(scored, 3, 0.4)
		if len(result) == 0 {
			t.Fatalf("run %d: expected non-empty result", run)
		}
		if result[0].Hash != "best" {
			t.Fatalf("run %d: expected first pick to be 'best' (highest score), got %q",
				run, result[0].Hash)
		}
	}
}

// Companion: when at least one candidate has a positive score the regular
// weighted-random path runs and may return any of them — but the result
// must still be drawn from the pool, contain no duplicates, and respect n.
func TestPickBatch_MixedScoresStillRandomized(t *testing.T) {
	scored := []TrackScore{
		{Track: library.Track{Hash: "pos1", Artist: ""}, Score: 10},
		{Track: library.Track{Hash: "pos2", Artist: ""}, Score: 5},
		{Track: library.Track{Hash: "neg", Artist: ""}, Score: -20},
	}
	result := PickBatch(scored, 2, 1.0)
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	seen := make(map[string]bool, len(result))
	for _, tr := range result {
		if seen[tr.Hash] {
			t.Errorf("duplicate hash %q in mixed-score result", tr.Hash)
		}
		seen[tr.Hash] = true
	}
}

func TestPickBatch_FromPool(t *testing.T) {
	tracks := make([]library.Track, 10)
	for i := range tracks {
		tracks[i] = library.Track{Hash: string(rune('a' + i)), Artist: "ArtistX"}
	}
	scored := ScoreAll(ScoreInput{
		Seed:      SeedTrack{Hash: "seed"},
		AllTracks: tracks,
	})
	validHashes := make(map[string]struct{}, len(scored))
	for _, ts := range scored {
		validHashes[ts.Track.Hash] = struct{}{}
	}
	result := PickBatch(scored, 3, 0.4)
	for _, tr := range result {
		if _, ok := validHashes[tr.Hash]; !ok {
			t.Errorf("track %s not in scored pool", tr.Hash)
		}
	}
}
