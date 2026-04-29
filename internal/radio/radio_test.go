package radio

import (
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

func track(hash, artist, album, genre string, dur float64) library.Track {
	return library.Track{Hash: hash, Artist: artist, Album: album, Genre: genre, Duration: dur}
}

func seedFrom(t library.Track, genres []string, tagIDs map[int64]struct{}) SeedTrack {
	return SeedTrack{
		Hash:     t.Hash,
		Artist:   t.Artist,
		Album:    t.Album,
		Genres:   genres,
		Duration: t.Duration,
		TagIDs:   tagIDs,
	}
}

// --- ScoreAll tests ---

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
	within := track("t1", "", "", "", 200)  // 200/180 ≈ 1.11, within ±30%
	outside := track("t2", "", "", "", 300) // 300/180 ≈ 1.67, outside ±30%
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
	// fresh has score 15 (genre+artist), recent has 15-50 = -35
	if got[0].Track.Hash != "t2" {
		t.Errorf("fresh track should rank above recent-history track")
	}
	var recentScore float64
	for _, ts := range got {
		if ts.Track.Hash == "t1" {
			recentScore = ts.Score
		}
	}
	// genre+artist+duration(+1) = 16, minus recent penalty 50 = -34
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
	t1 := track("t1", "ArtistA", "AlbumA", "Rock", 180) // artist+album+genre+dur = 5+3+10+1 = 19
	t2 := track("t2", "ArtistA", "", "Rock", 180)       // artist+genre+dur = 5+10+1 = 16
	t3 := track("t3", "", "", "Rock", 0)                // genre only = 10
	t4 := track("t4", "", "", "Jazz", 0)                // no match = 0
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

// --- PickBatch tests ---

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
	// Fewer candidates than batch size.
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
	// 8 tracks all from ArtistA, 2 from ArtistB.
	// With n=5, maxArtistFraction=0.4 → max 2 per artist (ceil(5*0.4)=2).
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
	maxPerArtist := 2 // ceil(5 * 0.4)
	for artist, cnt := range artistCounts {
		if cnt > maxPerArtist {
			t.Errorf("artist %q appears %d times (max %d)", artist, cnt, maxPerArtist)
		}
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
	result := PickBatch(scored, 5, 1.0) // no artist cap
	seen := make(map[string]bool)
	for _, tr := range result {
		if seen[tr.Hash] {
			t.Errorf("duplicate track %s in batch", tr.Hash)
		}
		seen[tr.Hash] = true
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
			"t1": 0.9, // >= 0.8 → +4
			"t2": 0.2, // <= 0.3 → -3
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
	coPlayed := track("t1", "", "", "", 0) // co-occurred 3 times → +6
	rare := track("t2", "", "", "", 0)     // co-occurred 1 time → no boost
	none := track("t3", "", "", "", 0)     // no entry
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
	// rare (count=1) and none both score 0
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
	// t1 has genre "Rock" which gets +3 bonus; t2 has "Jazz" (+1); t3 has "Pop" (no entry).
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

func TestPickBatch_FromPool(t *testing.T) {
	// With 10 tracks and n=3, only the top min(9, 10)=9 are eligible.
	// Verify all returned tracks came from the input scored slice.
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
