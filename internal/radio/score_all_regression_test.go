package radio

import (
	"math"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

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

// Regression: hash-keyed scoring inputs must not apply empty-string entries to
// every unhashed candidate. An empty Hash means the track is unidentified, so a
// stale "" key in favorites, queue, dismissed, tags, completion, or
// co-occurrence data should not score it as though all empty-hash tracks were
// the same track.
func TestScoreAll_EmptyCandidateHashIgnoresHashKeyedSignals(t *testing.T) {
	got := ScoreAll(ScoreInput{
		Seed: SeedTrack{
			Hash:   "seed",
			TagIDs: map[int64]struct{}{1: {}},
		},
		AllTracks: []library.Track{{Hash: "", Artist: "Unhashed"}},
		TrackTags: map[string]map[int64]struct{}{
			"": {1: {}},
		},
		FavoriteHashes:  map[string]struct{}{"": {}},
		RecentHashes:    map[string]struct{}{"": {}},
		QueueHashes:     map[string]struct{}{"": {}},
		DismissedHashes: map[string]struct{}{"": {}},
		CompletionRates: map[string]float64{"": 0.9},
		CoOccurrences:   map[string]int{"": 3},
	})
	if len(got) != 1 {
		t.Fatalf("expected one unhashed candidate, got %d", len(got))
	}
	if got[0].Score != 0 {
		t.Fatalf("empty-hash candidate should ignore hash-keyed signals, got score %f", got[0].Score)
	}
}

// Regression: Artist matching should be case-insensitive so real-world
// libraries with inconsistent capitalization between tracks of the same
// artist still earn the +5 boost.
func TestScoreAll_ArtistMatchCaseInsensitive(t *testing.T) {
	seed := track("s1", "The Beatles", "", "", 0)
	mixed := track("t1", "the beatles", "", "", 0)
	other := track("t2", "ArtistB", "", "", 0)
	got := ScoreAll(ScoreInput{
		Seed:      seedFrom(seed, nil, nil),
		AllTracks: []library.Track{other, mixed},
	})
	if got[0].Track.Hash != "t1" {
		t.Fatalf("case-insensitive artist match should rank first, got %s", got[0].Track.Hash)
	}
	if got[0].Score != 5 {
		t.Errorf("expected artist score +5 for case-insensitive match, got %f", got[0].Score)
	}
}

// Regression: Album matching should be case-insensitive for the same reason
// as Artist.
func TestScoreAll_AlbumMatchCaseInsensitive(t *testing.T) {
	seed := track("s1", "", "Abbey Road", "", 0)
	mixed := track("t1", "", "abbey road", "", 0)
	other := track("t2", "", "Other", "", 0)
	got := ScoreAll(ScoreInput{
		Seed:      seedFrom(seed, nil, nil),
		AllTracks: []library.Track{other, mixed},
	})
	if got[0].Track.Hash != "t1" {
		t.Fatalf("case-insensitive album match should rank first, got %s", got[0].Track.Hash)
	}
	if got[0].Score != 3 {
		t.Errorf("expected album score +3 for case-insensitive match, got %f", got[0].Score)
	}
}

// Regression: Genre matching should be case-insensitive — many real
// libraries mix "Rock" / "rock" / "ROCK" across tracks.
func TestScoreAll_GenreMatchCaseInsensitive(t *testing.T) {
	seed := track("s1", "", "", "Rock", 0)
	mixed := track("t1", "", "", "rock", 0)
	other := track("t2", "", "", "Jazz", 0)
	got := ScoreAll(ScoreInput{
		Seed:      SeedTrack{Hash: seed.Hash, Genres: []string{"Rock"}},
		AllTracks: []library.Track{other, mixed},
	})
	if got[0].Track.Hash != "t1" {
		t.Fatalf("case-insensitive genre match should rank first, got %s", got[0].Track.Hash)
	}
	if got[0].Score != 10 {
		t.Errorf("expected genre score +10 for case-insensitive match, got %f", got[0].Score)
	}
}

func TestScoreAll_GenreMatchDedupesCaseVariants(t *testing.T) {
	candidate := track("t1", "", "", "rock/ROCK/jazz/JAZZ", 0)
	other := track("t2", "", "", "Pop", 0)
	got := ScoreAll(ScoreInput{
		Seed: SeedTrack{
			Hash:   "s1",
			Genres: []string{"Rock", "rock", "Jazz", "JAZZ"},
		},
		AllTracks: []library.Track{other, candidate},
	})
	if got[0].Track.Hash != "t1" {
		t.Fatalf("deduped case-variant genre match should rank first, got %s", got[0].Track.Hash)
	}
	if got[0].Score != 20 {
		t.Errorf("expected 2 shared logical genres = 20, got %f", got[0].Score)
	}
	if got[1].Score != 0 {
		t.Errorf("expected no genre overlap for Pop, got %f", got[1].Score)
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

// Regression: Artist matching must tolerate leading/trailing whitespace in
// addition to case differences. PickBatch's per-artist cap normalises with
// strings.TrimSpace + lower-casing — without symmetrical trimming in ScoreAll,
// a tagger that emits " The Beatles" would be treated as the same artist for
// the cap but miss the +5 affinity boost.
func TestScoreAll_ArtistMatchTrimsWhitespace(t *testing.T) {
	seed := track("s1", "The Beatles", "", "", 0)
	padded := track("t1", " The Beatles ", "", "", 0)
	tabbed := track("t2", "\tthe beatles\n", "", "", 0)
	other := track("t3", "Stones", "", "", 0)
	got := ScoreAll(ScoreInput{
		Seed:      seedFrom(seed, nil, nil),
		AllTracks: []library.Track{other, padded, tabbed},
	})

	scoreByHash := scoresByHash(got)
	if scoreByHash["t1"] != 5 {
		t.Errorf("whitespace-padded artist should still earn +5, got %f", scoreByHash["t1"])
	}
	if scoreByHash["t2"] != 5 {
		t.Errorf("tab/newline-padded artist should still earn +5, got %f", scoreByHash["t2"])
	}
	if scoreByHash["t3"] != 0 {
		t.Errorf("unrelated artist must not earn the boost, got %f", scoreByHash["t3"])
	}
}

// Regression: Album matching mirrors Artist — whitespace-padded album names
// should still earn the +3 affinity boost.
func TestScoreAll_AlbumMatchTrimsWhitespace(t *testing.T) {
	seed := track("s1", "", "Abbey Road", "", 0)
	padded := track("t1", "", " Abbey Road ", "", 0)
	tabbed := track("t2", "", "\tabbey road", "", 0)
	other := track("t3", "", "Other", "", 0)
	got := ScoreAll(ScoreInput{
		Seed:      seedFrom(seed, nil, nil),
		AllTracks: []library.Track{other, padded, tabbed},
	})

	scoreByHash := scoresByHash(got)
	if scoreByHash["t1"] != 3 {
		t.Errorf("whitespace-padded album should still earn +3, got %f", scoreByHash["t1"])
	}
	if scoreByHash["t2"] != 3 {
		t.Errorf("tab-padded + case-different album should still earn +3, got %f", scoreByHash["t2"])
	}
	if scoreByHash["t3"] != 0 {
		t.Errorf("unrelated album must not earn the boost, got %f", scoreByHash["t3"])
	}
}

// Regression: NaN or Inf entries in TimeGenreBonus must not propagate into
// candidate scores. A single NaN score breaks sort.Slice ordering (every
// comparison with NaN is false), producing non-deterministic results and
// invalidating the descending-score invariant downstream consumers rely on.
func TestScoreAll_TimeGenreBonusRejectsNaNAndInf(t *testing.T) {
	seed := track("s1", "", "", "", 0)
	rockTrack := track("t1", "", "", "Rock", 0)
	jazzTrack := track("t2", "", "", "Jazz", 0)
	popTrack := track("t3", "", "", "Pop", 0)
	got := ScoreAll(ScoreInput{
		Seed:      seedFrom(seed, nil, nil),
		AllTracks: []library.Track{rockTrack, jazzTrack, popTrack},
		TimeGenreBonus: map[string]float64{
			"Rock": math.NaN(),
			"Jazz": math.Inf(1),
			"Pop":  2,
		},
	})
	for _, ts := range got {
		if math.IsNaN(ts.Score) || math.IsInf(ts.Score, 0) {
			t.Errorf("track %s scored NaN/Inf via poisoned time-genre bonus: %f",
				ts.Track.Hash, ts.Score)
		}
	}
	if got[0].Track.Hash != "t3" || got[0].Score != 2 {
		t.Errorf("expected Pop (+2) to remain top after NaN/Inf filtering, got %s score=%f",
			got[0].Track.Hash, got[0].Score)
	}
}

// Regression: A track tagged with duplicate case-variant genres (e.g.
// "Rock/rock") would harvest the time-of-day genre bonus once per variant
// because library.SplitGenres only dedupes on exact case. The bonus must
// be applied once per distinct logical genre, mirroring the genre-match
// behaviour.
func TestScoreAll_TimeGenreBonusDedupesCaseVariants(t *testing.T) {
	seed := track("s1", "", "", "", 0)
	duped := track("t1", "", "", "Rock/rock/ROCK", 0)
	other := track("t2", "", "", "Jazz", 0)
	got := ScoreAll(ScoreInput{
		Seed:           seedFrom(seed, nil, nil),
		AllTracks:      []library.Track{other, duped},
		TimeGenreBonus: map[string]float64{"Rock": 3},
	})
	scoreByHash := scoresByHash(got)
	if scoreByHash["t1"] != 3 {
		t.Errorf("expected single +3 bonus for duped case variants, got %f", scoreByHash["t1"])
	}
	if scoreByHash["t2"] != 0 {
		t.Errorf("expected 0 for non-matching genre, got %f", scoreByHash["t2"])
	}
}

// Regression: TimeGenreBonus map entries that differ only by case or
// surrounding whitespace still represent one logical genre. Normalization
// must not stack those variants into an inflated bonus.
func TestScoreAll_TimeGenreBonusInputVariantsDoNotStack(t *testing.T) {
	seed := track("s1", "", "", "", 0)
	rockTrack := track("t1", "", "", "Rock", 0)
	other := track("t2", "", "", "Jazz", 0)
	got := ScoreAll(ScoreInput{
		Seed:      seedFrom(seed, nil, nil),
		AllTracks: []library.Track{other, rockTrack},
		TimeGenreBonus: map[string]float64{
			"Rock":  3,
			" rock": 2,
			"ROCK ": 1,
		},
	})
	scoreByHash := scoresByHash(got)
	if scoreByHash["t1"] != 3 {
		t.Errorf("expected normalized Rock variants to apply one +3 bonus, got %f", scoreByHash["t1"])
	}
	if scoreByHash["t2"] != 0 {
		t.Errorf("expected unrelated genre to remain 0, got %f", scoreByHash["t2"])
	}
}

// Regression: A seed with a whitespace-only Artist or Album must not match
// candidates whose Artist/Album is also whitespace-only or empty. Without the
// "trimmed != \"\"" guard, EqualFold("", "") would return true and every empty
// candidate would harvest spurious +5/+3 bonuses.
func TestScoreAll_WhitespaceSeedDoesNotSpuriouslyMatch(t *testing.T) {
	seed := SeedTrack{Hash: "s1", Artist: "   ", Album: "\t"}
	emptyBoth := track("t1", "", "", "", 0)
	whitespaceArtist := track("t2", "  ", "", "", 0)
	got := ScoreAll(ScoreInput{
		Seed:      seed,
		AllTracks: []library.Track{emptyBoth, whitespaceArtist},
	})
	for _, ts := range got {
		if ts.Score != 0 {
			t.Errorf("track %s should score 0 against a whitespace seed, got %f",
				ts.Track.Hash, ts.Score)
		}
	}
}
