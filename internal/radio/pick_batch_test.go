package radio

import (
	cryptorand "crypto/rand"
	"errors"
	"math"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) {
	return f(p)
}

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

func TestPickBatch_ExtendsBeyondInitialPoolWhenCappedArtistsDominate(t *testing.T) {
	scored := make([]TrackScore, 0, 14)
	for i := 0; i < 10; i++ {
		scored = append(scored, TrackScore{
			Track: library.Track{Hash: string(rune('a' + i)), Artist: "ArtistA"},
			Score: float64(100 - i),
		})
	}
	scored = append(scored,
		TrackScore{Track: library.Track{Hash: "b1", Artist: "ArtistB"}, Score: 50},
		TrackScore{Track: library.Track{Hash: "b2", Artist: "ArtistB"}, Score: 49},
		TrackScore{Track: library.Track{Hash: "c1", Artist: "ArtistC"}, Score: 10},
		TrackScore{Track: library.Track{Hash: "d1", Artist: "ArtistD"}, Score: 9},
	)

	result := PickBatch(scored, 4, 0.25)
	if len(result) != 4 {
		t.Fatalf("expected 4 tracks after extending beyond the initial pool, got %d", len(result))
	}

	artistCounts := make(map[string]int)
	for _, track := range result {
		artistCounts[track.Artist]++
	}
	for artist, count := range artistCounts {
		if count > 1 {
			t.Fatalf("artist %q appears %d times, want at most 1", artist, count)
		}
	}
	if artistCounts["ArtistC"] != 1 || artistCounts["ArtistD"] != 1 {
		t.Fatalf("expected later eligible artists to fill the batch, got counts=%v", artistCounts)
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

func TestPickBatch_HugeNDoesNotOverflowPoolSize(t *testing.T) {
	scored := []TrackScore{{Track: library.Track{Hash: "t1", Artist: "ArtistA"}, Score: 10}}
	hugeN := int(^uint(0)>>1)/3 + 1

	got := PickBatch(scored, hugeN, 1.0)
	if len(got) != 1 {
		t.Fatalf("expected one available track for huge n, got %d", len(got))
	}
	if got[0].Hash != "t1" {
		t.Fatalf("expected t1, got %q", got[0].Hash)
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

func TestPickBatch_NoDuplicateNonEmptyHashes(t *testing.T) {
	scored := []TrackScore{
		{Track: library.Track{Hash: "same", Artist: "ArtistA"}, Score: -1},
		{Track: library.Track{Hash: "same", Artist: "ArtistB"}, Score: -2},
		{Track: library.Track{Hash: "unique", Artist: "ArtistC"}, Score: -3},
	}

	result := PickBatch(scored, 3, 1.0)
	if len(result) != 2 {
		t.Fatalf("expected duplicate hash to be skipped and 2 unique picks returned, got %d: %v", len(result), result)
	}

	seen := make(map[string]bool, len(result))
	for _, tr := range result {
		if tr.Hash == "" {
			t.Fatalf("test fixture unexpectedly returned empty hash: %v", result)
		}
		if seen[tr.Hash] {
			t.Fatalf("duplicate non-empty hash %q in result: %v", tr.Hash, result)
		}
		seen[tr.Hash] = true
	}
	if !seen["same"] || !seen["unique"] {
		t.Fatalf("expected one duplicate-hash representative plus unique track, got %v", result)
	}
}

func TestPickBatch_ArtistCapCaseInsensitive(t *testing.T) {
	scored := []TrackScore{
		{Track: library.Track{Hash: "a1", Artist: "ArtistA"}, Score: -1},
		{Track: library.Track{Hash: "a2", Artist: "artista"}, Score: -2},
		{Track: library.Track{Hash: "a3", Artist: " ARTISTA "}, Score: -3},
		{Track: library.Track{Hash: "b1", Artist: "ArtistB"}, Score: -4},
	}

	result := PickBatch(scored, 3, 0.34) // ceil(3*0.34)=2 per artist
	if len(result) != 3 {
		t.Fatalf("expected 3 tracks, got %d", len(result))
	}

	artistCounts := make(map[string]int)
	for _, tr := range result {
		artistCounts[tr.Artist]++
	}

	artistAVariants := artistCounts["ArtistA"] + artistCounts["artista"] + artistCounts[" ARTISTA "]
	if artistAVariants > 2 {
		t.Fatalf("expected at most 2 case-variant picks for ArtistA, got %d (result=%v)", artistAVariants, result)
	}
	if artistCounts["ArtistB"] != 1 {
		t.Fatalf("expected ArtistB to fill third slot after ArtistA cap, got result=%v", result)
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

func TestPickBatch_SmallPositiveScoreUsesWeightedPath(t *testing.T) {
	origReader := cryptorand.Reader
	cryptorand.Reader = readerFunc(func(p []byte) (int, error) {
		for i := range p {
			p[i] = 0xff
		}
		if len(p) > 0 {
			p[0] = 0x1f // 2^53 - 1 for PickBatch's 2^53 random range.
		}
		return len(p), nil
	})
	t.Cleanup(func() {
		cryptorand.Reader = origReader
	})

	scored := []TrackScore{
		{Track: library.Track{Hash: "tiny-positive", Artist: ""}, Score: 0.05},
		{Track: library.Track{Hash: "zero", Artist: ""}, Score: 0},
	}

	result := PickBatch(scored, 1, 1.0)
	if len(result) != 1 {
		t.Fatalf("expected one result, got %d", len(result))
	}
	if result[0].Hash != "zero" {
		t.Fatalf("expected high random draw to use weighted path and pick zero, got %q", result[0].Hash)
	}
}

func TestPickBatch_FallsBackWhenCryptoRandFails(t *testing.T) {
	origReader := cryptorand.Reader
	cryptorand.Reader = readerFunc(func([]byte) (int, error) {
		return 0, errors.New("forced crypto/rand failure")
	})
	t.Cleanup(func() {
		cryptorand.Reader = origReader
	})

	scored := []TrackScore{
		{Track: library.Track{Hash: "a", Artist: "ArtistA"}, Score: 30},
		{Track: library.Track{Hash: "b", Artist: "ArtistB"}, Score: 20},
		{Track: library.Track{Hash: "c", Artist: "ArtistC"}, Score: 10},
		{Track: library.Track{Hash: "d", Artist: "ArtistD"}, Score: 5},
	}

	result := PickBatch(scored, 3, 1.0)
	if len(result) != 3 {
		t.Fatalf("expected full batch despite crypto/rand failure, got %d", len(result))
	}
	for i, want := range []string{"a", "b", "c"} {
		if result[i].Hash != want {
			t.Fatalf("idx %d: expected deterministic fallback pick %q, got %q", i, want, result[i].Hash)
		}
	}
}

// Regression: maxArtistFraction must tolerate NaN and negative values. Without
// sanitization, int(math.Ceil(NaN)) is undefined (typically a large negative
// on amd64), silently disabling the per-artist cap and letting one artist
// monopolize the batch.
func TestPickBatch_MaxArtistFractionNaNStillCapsArtists(t *testing.T) {
	scored := []TrackScore{
		{Track: library.Track{Hash: "a1", Artist: "ArtistA"}, Score: 10},
		{Track: library.Track{Hash: "a2", Artist: "ArtistA"}, Score: 9},
		{Track: library.Track{Hash: "a3", Artist: "ArtistA"}, Score: 8},
		{Track: library.Track{Hash: "a4", Artist: "ArtistA"}, Score: 7},
		{Track: library.Track{Hash: "b1", Artist: "ArtistB"}, Score: 6},
		{Track: library.Track{Hash: "c1", Artist: "ArtistC"}, Score: 5},
	}
	for _, frac := range []float64{math.NaN(), -1.0, 0.0} {
		result := PickBatch(scored, 4, frac)
		if len(result) == 0 {
			t.Fatalf("frac=%v: expected non-empty result", frac)
		}
		counts := make(map[string]int)
		for _, tr := range result {
			counts[tr.Artist]++
		}
		if counts["ArtistA"] > 1 {
			t.Errorf("frac=%v: ArtistA appears %d times; expected cap of 1 when fraction is invalid",
				frac, counts["ArtistA"])
		}
	}
}

// Regression: TrackScore.Score values of NaN or Inf must not poison the
// cumulative weighted-random draw. NaN propagates through math.Max and the
// running cumulative sum, making every `r < cumulative` comparison false and
// silently degrading the picker to "always return the last eligible". Inf
// would similarly dominate the cumulative sum and starve later candidates.
// Sanitizing the per-track score in collectEligible keeps the rest of the
// pool live.
func TestPickBatch_SanitizesNaNAndInfScores(t *testing.T) {
	scored := []TrackScore{
		{Track: library.Track{Hash: "nan", Artist: "A"}, Score: math.NaN()},
		{Track: library.Track{Hash: "posinf", Artist: "B"}, Score: math.Inf(1)},
		{Track: library.Track{Hash: "neginf", Artist: "C"}, Score: math.Inf(-1)},
		{Track: library.Track{Hash: "ok1", Artist: "D"}, Score: 10},
		{Track: library.Track{Hash: "ok2", Artist: "E"}, Score: 5},
		{Track: library.Track{Hash: "ok3", Artist: "F"}, Score: 1},
	}
	// Run a handful of times — with the bug, NaN/Inf would cause picks to
	// repeatedly land on the same degenerate index and produce inconsistent
	// counts. We mainly need to assert the picker returns the requested
	// number of distinct tracks without panicking or returning duplicates.
	for run := 0; run < 10; run++ {
		result := PickBatch(scored, 4, 1.0)
		if len(result) != 4 {
			t.Fatalf("run %d: expected 4 picks, got %d", run, len(result))
		}
		seen := make(map[string]bool, len(result))
		for _, tr := range result {
			if seen[tr.Hash] {
				t.Fatalf("run %d: duplicate hash %q", run, tr.Hash)
			}
			seen[tr.Hash] = true
		}
	}
}

// Regression: very large finite scores can overflow totalWeight to +Inf.
// Multiplying a random fraction by +Inf then yields +Inf (or NaN for zero),
// every cumulative comparison fails, and the picker silently falls through to
// the last eligible candidate. It should instead fall back to the highest
// scored eligible track.
func TestPickBatch_FallsBackWhenTotalWeightOverflows(t *testing.T) {
	scored := []TrackScore{
		{Track: library.Track{Hash: "best", Artist: "A"}, Score: math.MaxFloat64},
		{Track: library.Track{Hash: "runner-up", Artist: "B"}, Score: math.MaxFloat64},
		{Track: library.Track{Hash: "last", Artist: "C"}, Score: 1},
	}

	result := PickBatch(scored, 1, 1.0)
	if len(result) != 1 {
		t.Fatalf("expected one pick, got %d", len(result))
	}
	if result[0].Hash != "best" {
		t.Fatalf("expected overflow fallback to pick highest-scored track, got %q", result[0].Hash)
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
