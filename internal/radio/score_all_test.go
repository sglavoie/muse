package radio

import (
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

func TestScoreAll_SeedExcluded(t *testing.T) {
	seed := track("s1", "ArtistA", "AlbumA", "Rock", 180)
	other := track("t1", "ArtistB", "AlbumB", "Jazz", 200)
	input := ScoreInput{
		Seed:      seedFrom(seed, []string{"Rock"}, nil),
		AllTracks: []library.Track{seed, other},
	}
	got := ScoreAll(input)
	for _, ts := range got {
		if ts.Track.Hash == seed.Hash {
			t.Errorf("seed track must not appear in ScoreAll output")
		}
	}
	if len(got) != 1 {
		t.Errorf("expected 1 result, got %d", len(got))
	}
}

func TestScoreAll_GenreMatch(t *testing.T) {
	seed := track("s1", "", "", "Rock", 0)
	match := track("t1", "", "", "Rock", 0)
	noMatch := track("t2", "", "", "Jazz", 0)
	input := ScoreInput{
		Seed:      seedFrom(seed, library.SplitGenres("Rock"), nil),
		AllTracks: []library.Track{noMatch, match},
	}
	got := ScoreAll(input)
	if got[0].Track.Hash != "t1" {
		t.Errorf("genre match should score higher: got %s first", got[0].Track.Hash)
	}
	if got[0].Score != 10 {
		t.Errorf("expected genre score +10, got %f", got[0].Score)
	}
	if got[1].Score != 0 {
		t.Errorf("expected no-match score 0, got %f", got[1].Score)
	}
}

func TestScoreAll_MultiGenre(t *testing.T) {
	seed := track("s1", "", "", "Rock/Jazz", 0)
	twoMatch := track("t1", "", "", "Rock;Jazz", 0)
	oneMatch := track("t2", "", "", "Rock", 0)
	input := ScoreInput{
		Seed:      seedFrom(seed, library.SplitGenres("Rock/Jazz"), nil),
		AllTracks: []library.Track{oneMatch, twoMatch},
	}
	got := ScoreAll(input)
	if got[0].Track.Hash != "t1" {
		t.Errorf("two genre matches should rank first")
	}
	if got[0].Score != 20 {
		t.Errorf("expected 2 genre matches = 20, got %f", got[0].Score)
	}
	if got[1].Score != 10 {
		t.Errorf("expected 1 genre match = 10, got %f", got[1].Score)
	}
}

func TestScoreAll_ArtistMatch(t *testing.T) {
	seed := track("s1", "ArtistA", "", "", 0)
	match := track("t1", "ArtistA", "", "", 0)
	noMatch := track("t2", "ArtistB", "", "", 0)
	input := ScoreInput{
		Seed:      seedFrom(seed, nil, nil),
		AllTracks: []library.Track{noMatch, match},
	}
	got := ScoreAll(input)
	if got[0].Track.Hash != "t1" {
		t.Errorf("artist match should rank first")
	}
	if got[0].Score != 5 {
		t.Errorf("expected artist score +5, got %f", got[0].Score)
	}
}

func TestScoreAll_AlbumMatch(t *testing.T) {
	seed := track("s1", "", "AlbumA", "", 0)
	match := track("t1", "", "AlbumA", "", 0)
	noMatch := track("t2", "", "AlbumB", "", 0)
	input := ScoreInput{
		Seed:      seedFrom(seed, nil, nil),
		AllTracks: []library.Track{noMatch, match},
	}
	got := ScoreAll(input)
	if got[0].Track.Hash != "t1" {
		t.Errorf("album match should rank first")
	}
	if got[0].Score != 3 {
		t.Errorf("expected album score +3, got %f", got[0].Score)
	}
}

func TestScoreAll_TagMatch(t *testing.T) {
	seed := track("s1", "", "", "", 0)
	match := track("t1", "", "", "", 0)
	noMatch := track("t2", "", "", "", 0)
	input := ScoreInput{
		Seed: SeedTrack{
			Hash:   seed.Hash,
			TagIDs: map[int64]struct{}{1: {}, 2: {}},
		},
		AllTracks: []library.Track{noMatch, match},
		TrackTags: map[string]map[int64]struct{}{
			"t1": {1: {}, 2: {}},
		},
	}
	got := ScoreAll(input)
	if got[0].Track.Hash != "t1" {
		t.Errorf("tag match should rank first")
	}
	if got[0].Score != 16 {
		t.Errorf("expected 2 tag matches = 16, got %f", got[0].Score)
	}
}

func TestScoreAll_FavoriteBoost(t *testing.T) {
	seed := track("s1", "", "", "", 0)
	fav := track("t1", "", "", "", 0)
	notFav := track("t2", "", "", "", 0)
	input := ScoreInput{
		Seed:           seedFrom(seed, nil, nil),
		AllTracks:      []library.Track{notFav, fav},
		FavoriteHashes: map[string]struct{}{"t1": {}},
	}
	got := ScoreAll(input)
	if got[0].Track.Hash != "t1" {
		t.Errorf("favorited track should rank first")
	}
	if got[0].Score != 3 {
		t.Errorf("expected favorite score +3, got %f", got[0].Score)
	}
}

func TestScoreAll_DurationBoost(t *testing.T) {
	seed := track("s1", "", "", "", 180)
	within := track("t1", "", "", "", 200)
	outside := track("t2", "", "", "", 300)
	input := ScoreInput{
		Seed:      seedFrom(seed, nil, nil),
		AllTracks: []library.Track{outside, within},
	}
	got := ScoreAll(input)
	if got[0].Track.Hash != "t1" {
		t.Errorf("duration-within track should rank first")
	}
	if got[0].Score != 1 {
		t.Errorf("expected duration score +1, got %f", got[0].Score)
	}
}

func TestScoreAll_RecentHistoryPenalty(t *testing.T) {
	seed := track("s1", "A", "", "Rock", 180)
	recent := track("t1", "A", "", "Rock", 180)
	fresh := track("t2", "A", "", "Rock", 180)
	input := ScoreInput{
		Seed:         seedFrom(seed, []string{"Rock"}, nil),
		AllTracks:    []library.Track{recent, fresh},
		RecentHashes: map[string]struct{}{"t1": {}},
	}
	got := ScoreAll(input)
	if got[0].Track.Hash != "t2" {
		t.Errorf("fresh track should rank above recent-history track")
	}
	var recentScore float64
	for _, ts := range got {
		if ts.Track.Hash == "t1" {
			recentScore = ts.Score
		}
	}
	if recentScore != -34 {
		t.Errorf("expected recent penalty score -34, got %f", recentScore)
	}
}

func TestScoreAll_QueuePenalty(t *testing.T) {
	seed := track("s1", "", "", "", 0)
	inQueue := track("t1", "", "", "", 0)
	notInQueue := track("t2", "", "", "", 0)
	input := ScoreInput{
		Seed:        seedFrom(seed, nil, nil),
		AllTracks:   []library.Track{inQueue, notInQueue},
		QueueHashes: map[string]struct{}{"t1": {}},
	}
	got := ScoreAll(input)
	if got[0].Track.Hash != "t2" {
		t.Errorf("non-queue track should rank above queued track")
	}
	if got[1].Score != -100 {
		t.Errorf("expected queue penalty -100, got %f", got[1].Score)
	}
}

func TestScoreAll_SortedDescending(t *testing.T) {
	seed := track("s1", "ArtistA", "AlbumA", "Rock", 180)
	t1 := track("t1", "ArtistA", "AlbumA", "Rock", 180)
	t2 := track("t2", "ArtistA", "", "Rock", 180)
	t3 := track("t3", "", "", "Rock", 0)
	t4 := track("t4", "", "", "Jazz", 0)
	input := ScoreInput{
		Seed:      seedFrom(seed, []string{"Rock"}, nil),
		AllTracks: []library.Track{t4, t2, t4, t3, t1},
	}
	got := ScoreAll(input)
	for i := 1; i < len(got); i++ {
		if got[i].Score > got[i-1].Score {
			t.Errorf("results not sorted descending at index %d: %f > %f", i, got[i].Score, got[i-1].Score)
		}
	}
}

func TestScoreAll_EmptyLibrary(t *testing.T) {
	seed := track("s1", "A", "", "Rock", 180)
	input := ScoreInput{
		Seed:      seedFrom(seed, []string{"Rock"}, nil),
		AllTracks: nil,
	}
	got := ScoreAll(input)
	if len(got) != 0 {
		t.Errorf("expected empty result for empty library, got %d", len(got))
	}
}

func TestScoreAll_NoMetadata(t *testing.T) {
	seed := track("s1", "", "", "", 0)
	t1 := track("t1", "", "", "", 0)
	t2 := track("t2", "", "", "", 0)
	input := ScoreInput{
		Seed:      seedFrom(seed, nil, nil),
		AllTracks: []library.Track{t1, t2},
	}
	got := ScoreAll(input)
	if len(got) != 2 {
		t.Errorf("expected 2 results, got %d", len(got))
	}
	if got[0].Score != 0 || got[1].Score != 0 {
		t.Errorf("expected zero scores for no-metadata tracks, got %f and %f", got[0].Score, got[1].Score)
	}
}

func TestScoreAll_CompletionRateHighBoost(t *testing.T) {
	seed := track("s1", "", "", "", 0)
	high := track("t1", "", "", "", 0)
	low := track("t2", "", "", "", 0)
	input := ScoreInput{
		Seed:      seedFrom(seed, nil, nil),
		AllTracks: []library.Track{low, high},
		CompletionRates: map[string]float64{
			"t1": 0.9,
			"t2": 0.2,
		},
	}
	got := ScoreAll(input)
	if got[0].Track.Hash != "t1" {
		t.Errorf("high completion rate track should rank first, got %s", got[0].Track.Hash)
	}
	if got[0].Score != 4 {
		t.Errorf("expected high completion score +4, got %f", got[0].Score)
	}
	if got[1].Score != -3 {
		t.Errorf("expected low completion score -3, got %f", got[1].Score)
	}
}

func TestScoreAll_CompletionRateNilSafe(t *testing.T) {
	seed := track("s1", "", "", "", 0)
	t1 := track("t1", "", "", "", 0)
	input := ScoreInput{
		Seed:            seedFrom(seed, nil, nil),
		AllTracks:       []library.Track{t1},
		CompletionRates: nil,
	}
	got := ScoreAll(input)
	if len(got) != 1 || got[0].Score != 0 {
		t.Errorf("nil CompletionRates should not affect score, got %v", got)
	}
}

func TestScoreAll_CoOccurrenceBoost(t *testing.T) {
	seed := track("s1", "", "", "", 0)
	coPlayed := track("t1", "", "", "", 0)
	rare := track("t2", "", "", "", 0)
	none := track("t3", "", "", "", 0)
	input := ScoreInput{
		Seed:          seedFrom(seed, nil, nil),
		AllTracks:     []library.Track{none, rare, coPlayed},
		CoOccurrences: map[string]int{"t1": 3, "t2": 1},
	}
	got := ScoreAll(input)
	if got[0].Track.Hash != "t1" {
		t.Errorf("co-occurrence track should rank first, got %s", got[0].Track.Hash)
	}
	if got[0].Score != 6 {
		t.Errorf("expected co-occurrence score +6, got %f", got[0].Score)
	}
	for _, ts := range got[1:] {
		if ts.Score != 0 {
			t.Errorf("track %s with count<2 should score 0, got %f", ts.Track.Hash, ts.Score)
		}
	}
}

func TestScoreAll_CoOccurrenceNilSafe(t *testing.T) {
	seed := track("s1", "", "", "", 0)
	t1 := track("t1", "", "", "", 0)
	input := ScoreInput{
		Seed:          seedFrom(seed, nil, nil),
		AllTracks:     []library.Track{t1},
		CoOccurrences: nil,
	}
	got := ScoreAll(input)
	if len(got) != 1 || got[0].Score != 0 {
		t.Errorf("nil CoOccurrences should not affect score, got %v", got)
	}
}

func TestScoreAll_TimeGenreBonusBoost(t *testing.T) {
	seed := track("s1", "", "", "", 0)
	t1 := track("t1", "", "", "Rock", 0)
	t2 := track("t2", "", "", "Jazz", 0)
	t3 := track("t3", "", "", "Pop", 0)
	input := ScoreInput{
		Seed:           seedFrom(seed, nil, nil),
		AllTracks:      []library.Track{t3, t2, t1},
		TimeGenreBonus: map[string]float64{"Rock": 3, "Jazz": 1},
	}
	got := ScoreAll(input)
	if got[0].Track.Hash != "t1" {
		t.Errorf("expected t1 (Rock +3) to rank first, got %s", got[0].Track.Hash)
	}
	if got[0].Score != 3 {
		t.Errorf("expected score 3 for top genre bonus, got %f", got[0].Score)
	}
	if got[1].Track.Hash != "t2" {
		t.Errorf("expected t2 (Jazz +1) to rank second, got %s", got[1].Track.Hash)
	}
	if got[1].Score != 1 {
		t.Errorf("expected score 1 for runner-up genre bonus, got %f", got[1].Score)
	}
	if got[2].Score != 0 {
		t.Errorf("expected score 0 for Pop (no bonus), got %f", got[2].Score)
	}
}

func TestScoreAll_TimeGenreBonusNilSafe(t *testing.T) {
	seed := track("s1", "", "", "", 0)
	t1 := track("t1", "", "", "Rock", 0)
	input := ScoreInput{
		Seed:           seedFrom(seed, nil, nil),
		AllTracks:      []library.Track{t1},
		TimeGenreBonus: nil,
	}
	got := ScoreAll(input)
	if len(got) != 1 || got[0].Score != 0 {
		t.Errorf("nil TimeGenreBonus should not affect score, got %v", got)
	}
}

// Regression: when SeedTrack.Hash is empty (no playing seed available),
// ScoreAll must not silently drop every candidate that also has an empty
// Hash. Previously the `track.Hash == input.Seed.Hash` guard treated
// "" == "" as a seed match and excluded those candidates.
func TestScoreAll_EmptySeedHashDoesNotDropEmptyHashCandidates(t *testing.T) {
	seed := SeedTrack{Hash: ""}
	t1 := library.Track{Hash: "", Artist: "A"}
	t2 := library.Track{Hash: "h2", Artist: "B"}
	t3 := library.Track{Hash: "", Artist: "C"}
	got := ScoreAll(ScoreInput{
		Seed:      seed,
		AllTracks: []library.Track{t1, t2, t3},
	})
	if len(got) != 3 {
		t.Fatalf("expected all 3 candidates retained, got %d", len(got))
	}
}

// Regression: ScoreAll must keep candidates with identical scores in their
// input order so downstream consumers (PickBatch pool, UI lists) see
// reproducible tie-breaking. Previously sort.Slice (unstable) could reorder
// equal-score entries arbitrarily.
func TestScoreAll_StableTieBreaking(t *testing.T) {
	seed := track("s1", "", "", "", 0)
	tracks := make([]library.Track, 32)
	for i := range tracks {
		tracks[i] = library.Track{Hash: string(rune('a' + i)), Artist: "X"}
	}
	input := ScoreInput{Seed: seedFrom(seed, nil, nil), AllTracks: tracks}
	first := ScoreAll(input)
	for run := 0; run < 5; run++ {
		got := ScoreAll(input)
		if len(got) != len(first) {
			t.Fatalf("length drift: %d vs %d", len(got), len(first))
		}
		for i := range got {
			if got[i].Track.Hash != first[i].Track.Hash {
				t.Fatalf("unstable tie-break at idx %d: %q vs %q",
					i, got[i].Track.Hash, first[i].Track.Hash)
			}
		}
	}
	// And the relative order must match input order for the equal-score run.
	for i := range first {
		if first[i].Track.Hash != tracks[i].Hash {
			t.Errorf("idx %d: got %q, want input order %q",
				i, first[i].Track.Hash, tracks[i].Hash)
		}
	}
}
