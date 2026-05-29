package radio

import (
	"math"
	"testing"
)

func TestPickBatch_ArtistCap(t *testing.T) {
	tracks := append(trackSeq(8, "ArtistA", "Rock"),
		track("x1", "ArtistB", "", "Rock", 0),
		track("x2", "ArtistB", "", "Rock", 0),
	)

	result := PickBatch(
		scoreAllForPickBatch(SeedTrack{Hash: "seed", Genres: []string{"Rock"}}, tracks...),
		5,
		0.4,
	)

	assertMaxArtistCount(t, result, 2)
}

func TestPickBatch_HugeNArtistCapUsesAvailableCandidateCount(t *testing.T) {
	scored := []TrackScore{
		scoredTrack("a1", "ArtistA", -1),
		scoredTrack("a2", "ArtistA", -2),
		scoredTrack("a3", "ArtistA", -3),
		scoredTrack("a4", "ArtistA", -4),
		scoredTrack("a5", "ArtistA", -5),
		scoredTrack("a6", "ArtistA", -6),
		scoredTrack("b1", "ArtistB", -7),
		scoredTrack("c1", "ArtistC", -8),
	}
	hugeN := int(^uint(0) >> 1)

	result := PickBatch(scored, hugeN, 0.4)

	if len(result) == len(scored) {
		t.Fatalf("expected artist cap to limit a huge request against a small pool, got all tracks: %v", result)
	}
	assertMaxArtistCount(t, result, 4)
}

func TestPickBatch_ExtendsBeyondInitialPoolWhenCappedArtistsDominate(t *testing.T) {
	scored := make([]TrackScore, 0, 14)
	for i := 0; i < 10; i++ {
		scored = append(scored, scoredTrack(string(rune('a'+i)), "ArtistA", float64(100-i)))
	}
	scored = append(scored,
		scoredTrack("b1", "ArtistB", 50),
		scoredTrack("b2", "ArtistB", 49),
		scoredTrack("c1", "ArtistC", 10),
		scoredTrack("d1", "ArtistD", 9),
	)

	result := PickBatch(scored, 4, 0.25)
	if len(result) != 4 {
		t.Fatalf("expected 4 tracks after extending beyond the initial pool, got %d", len(result))
	}

	assertMaxArtistCount(t, result, 1)
	counts := normalizedArtistCounts(result)
	if counts[normalizedArtistKey("ArtistC")] != 1 || counts[normalizedArtistKey("ArtistD")] != 1 {
		t.Fatalf("expected later eligible artists to fill the batch, got counts=%v", counts)
	}
}

func TestPickBatch_EmptyArtistBypassesCap(t *testing.T) {
	scored := make([]TrackScore, 6)
	for i := range scored {
		scored[i] = scoredTrack(string(rune('a'+i)), "", 10)
	}

	result := PickBatch(scored, 5, 0.4)
	if len(result) != 5 {
		t.Fatalf("expected 5 tracks with empty artists, got %d", len(result))
	}
}

func TestPickBatch_ArtistCapCaseInsensitive(t *testing.T) {
	scored := []TrackScore{
		scoredTrack("a1", "ArtistA", -1),
		scoredTrack("a2", "artista", -2),
		scoredTrack("a3", " ARTISTA ", -3),
		scoredTrack("b1", "ArtistB", -4),
	}

	result := PickBatch(scored, 3, 0.34)
	if len(result) != 3 {
		t.Fatalf("expected 3 tracks, got %d", len(result))
	}

	counts := normalizedArtistCounts(result)
	artistAKey := normalizedArtistKey("ArtistA")
	if counts[artistAKey] > 2 {
		t.Fatalf("expected at most 2 case-variant picks for ArtistA, got %d (result=%v)", counts[artistAKey], result)
	}
	if counts[normalizedArtistKey("ArtistB")] != 1 {
		t.Fatalf("expected ArtistB to fill third slot after ArtistA cap, got result=%v", result)
	}
}

func TestPickBatch_MaxArtistFractionNaNStillCapsArtists(t *testing.T) {
	scored := []TrackScore{
		scoredTrack("a1", "ArtistA", 10),
		scoredTrack("a2", "ArtistA", 9),
		scoredTrack("a3", "ArtistA", 8),
		scoredTrack("a4", "ArtistA", 7),
		scoredTrack("b1", "ArtistB", 6),
		scoredTrack("c1", "ArtistC", 5),
	}

	for _, frac := range []float64{math.NaN(), -1.0, 0.0} {
		result := PickBatch(scored, 4, frac)
		if len(result) == 0 {
			t.Fatalf("frac=%v: expected non-empty result", frac)
		}

		artistACount := normalizedArtistCounts(result)[normalizedArtistKey("ArtistA")]
		if artistACount > 1 {
			t.Errorf("frac=%v: ArtistA appears %d times; expected cap of 1 when fraction is invalid", frac, artistACount)
		}
	}
}
