package radio

import (
	"errors"
	"math"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

func TestPickBatch_NoDuplicates(t *testing.T) {
	result := PickBatch(
		scoreAllForPickBatch(SeedTrack{Hash: "seed"}, trackSeq(20, "ArtistA", "")...),
		5,
		1.0,
	)

	assertUniqueNonEmptyHashes(t, result)
}

func TestPickBatch_NoDuplicateNonEmptyHashes(t *testing.T) {
	scored := []TrackScore{
		scoredTrack("same", "ArtistA", -1),
		scoredTrack("same", "ArtistB", -2),
		scoredTrack("unique", "ArtistC", -3),
	}

	result := PickBatch(scored, 3, 1.0)
	if len(result) != 2 {
		t.Fatalf("expected duplicate hash to be skipped and 2 unique picks returned, got %d: %v", len(result), result)
	}

	assertUniqueNonEmptyHashes(t, result)
	seen := make(map[string]struct{}, len(result))
	for _, tr := range result {
		seen[tr.Hash] = struct{}{}
	}
	if _, ok := seen["same"]; !ok {
		t.Fatalf("expected one duplicate-hash representative plus unique track, got %v", result)
	}
	if _, ok := seen["unique"]; !ok {
		t.Fatalf("expected one duplicate-hash representative plus unique track, got %v", result)
	}
}

func TestPickBatch_NoDuplicateEmptyHashesWhenPathMatches(t *testing.T) {
	scored := []TrackScore{
		{Track: library.Track{Path: "/music/same.mp3", Artist: "ArtistA"}, Score: -1},
		{Track: library.Track{Path: "/music/same.mp3", Artist: "ArtistB"}, Score: -2},
		{Track: library.Track{Path: "/music/unique.mp3", Artist: "ArtistC"}, Score: -3},
	}

	result := PickBatch(scored, 3, 1.0)
	if len(result) != 2 {
		t.Fatalf("expected duplicate empty-hash path to be skipped and 2 unique picks returned, got %d: %v", len(result), result)
	}

	pathCounts := make(map[string]int)
	for _, tr := range result {
		pathCounts[tr.Path]++
	}
	if pathCounts["/music/same.mp3"] != 1 {
		t.Fatalf("expected exactly one pick for duplicate path, got counts=%v", pathCounts)
	}
	if pathCounts["/music/unique.mp3"] != 1 {
		t.Fatalf("expected unique path to be picked once, got counts=%v", pathCounts)
	}
}

func TestPickBatch_AllowsEmptyHashesWhenPathsDiffer(t *testing.T) {
	scored := []TrackScore{
		{Track: library.Track{Path: "/music/a.mp3", Artist: "ArtistA"}, Score: -1},
		{Track: library.Track{Path: "/music/b.mp3", Artist: "ArtistB"}, Score: -2},
	}

	result := PickBatch(scored, 2, 1.0)
	if len(result) != 2 {
		t.Fatalf("expected both distinct paths to be picked, got %d: %v", len(result), result)
	}
}

func TestPickBatch_DeterministicFallbackWhenAllNonPositive(t *testing.T) {
	scored := []TrackScore{
		scoredTrack("best", "", -1),
		scoredTrack("mid1", "", -50),
		scoredTrack("mid2", "", -50),
		scoredTrack("worst1", "", -100),
		scoredTrack("worst2", "", -100),
		scoredTrack("worst3", "", -100),
	}

	for run := 0; run < 10; run++ {
		result := PickBatch(scored, 3, 0.4)
		if len(result) == 0 {
			t.Fatalf("run %d: expected non-empty result", run)
		}
		if result[0].Hash != "best" {
			t.Fatalf("run %d: expected first pick to be 'best' (highest score), got %q", run, result[0].Hash)
		}
	}
}

func TestPickBatch_MixedScoresStillRandomized(t *testing.T) {
	scored := []TrackScore{
		scoredTrack("pos1", "", 10),
		scoredTrack("pos2", "", 5),
		scoredTrack("neg", "", -20),
	}

	result := PickBatch(scored, 2, 1.0)
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}

	assertUniqueNonEmptyHashes(t, result)
}

func TestPickBatch_SmallPositiveScoreUsesWeightedPath(t *testing.T) {
	useHighestCryptoRand(t)

	scored := []TrackScore{
		scoredTrack("tiny-positive", "", 0.05),
		scoredTrack("zero", "", 0),
	}

	assertTrackHashes(t, PickBatch(scored, 1, 1.0), "zero")
}

func TestPickBatch_FallsBackWhenCryptoRandFails(t *testing.T) {
	withCryptoRandReader(t, readerFunc(func([]byte) (int, error) {
		return 0, errors.New("forced crypto/rand failure")
	}))

	scored := []TrackScore{
		scoredTrack("a", "ArtistA", 30),
		scoredTrack("b", "ArtistB", 20),
		scoredTrack("c", "ArtistC", 10),
		scoredTrack("d", "ArtistD", 5),
	}

	assertTrackHashes(t, PickBatch(scored, 3, 1.0), "a", "b", "c")
}

func TestPickBatch_SanitizesNaNAndInfScores(t *testing.T) {
	scored := []TrackScore{
		scoredTrack("nan", "A", math.NaN()),
		scoredTrack("posinf", "B", math.Inf(1)),
		scoredTrack("neginf", "C", math.Inf(-1)),
		scoredTrack("ok1", "D", 10),
		scoredTrack("ok2", "E", 5),
		scoredTrack("ok3", "F", 1),
	}

	for run := 0; run < 10; run++ {
		result := PickBatch(scored, 4, 1.0)
		if len(result) != 4 {
			t.Fatalf("run %d: expected 4 picks, got %d", run, len(result))
		}

		assertUniqueNonEmptyHashes(t, result)
	}
}

func TestPickBatch_FallsBackWhenTotalWeightOverflows(t *testing.T) {
	scored := []TrackScore{
		scoredTrack("best", "A", math.MaxFloat64),
		scoredTrack("runner-up", "B", math.MaxFloat64),
		scoredTrack("last", "C", 1),
	}

	assertTrackHashes(t, PickBatch(scored, 1, 1.0), "best")
}
