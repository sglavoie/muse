package radio

import (
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

func TestPlanBatch_ArtistDiversityAppliesAcrossBothPools(t *testing.T) {
	// One artist supplies the entire library, split across both pools. A cap
	// applied per pool would let the artist take 2 + 2 of a five-track batch;
	// the batch-wide cap of ceil(5 * 0.4) = 2 must hold.
	candidates := make([]library.Track, 0, 12)
	for i := range 6 {
		candidates = append(candidates, neverPlayed("d"+string(rune('a'+i)), "Solo Artist"))
		candidates = append(candidates, recentlyPlayed("f"+string(rune('a'+i)), "Solo Artist"))
	}

	result := PlanBatch(planInput(t, candidates))
	if len(result.Tracks) != 2 {
		t.Fatalf("selected %d tracks, want the 2 the artist cap allows: %v", len(result.Tracks), planHashes(result))
	}
	if !result.Shortages.Has(ShortageBatch) {
		t.Fatalf("expected a batch shortage from the artist cap, got %+v", result.Shortages)
	}
	assertNoDuplicateTracks(t, result)
}

func TestPlanBatch_DiversityHonoursCustomFraction(t *testing.T) {
	candidates := make([]library.Track, 0, 20)
	for i := range 10 {
		candidates = append(candidates, neverPlayed("d"+string(rune('a'+i)), "Solo Artist"))
		candidates = append(candidates, recentlyPlayed("f"+string(rune('a'+i)), "Solo Artist"))
	}
	in := planInput(t, candidates)
	in.MaxArtistFraction = 1

	if got := len(PlanBatch(in).Tracks); got != 5 {
		t.Fatalf("selected %d tracks with the cap lifted, want 5", got)
	}
}

func TestPlanBatch_ExpandsBeyondTheInitialPool(t *testing.T) {
	// The picker starts on the top 3*n = 15 of the pool. Sixteen candidates
	// outrank the rest and all share one artist, so the whole initial window
	// is capped out after two picks and the batch can only be filled by
	// expanding into the tail.
	candidates := make([]library.Track, 0, 28)
	for i := range 16 {
		candidates = append(candidates, neverPlayed("d"+string(rune('a'+i)), "Crowd"))
	}
	for i := range 12 {
		// These sort last: they share no genre with the seed, so nothing but
		// pool expansion can reach them.
		tr := neverPlayed("t"+string(rune('a'+i)), "Tail "+string(rune('a'+i)))
		tr.Genre = "Ambient"
		candidates = append(candidates, tr)
	}

	in := withDiscoveryPercent(planInput(t, candidates), 100)
	result := PlanBatch(in)
	if len(result.Tracks) != 5 {
		t.Fatalf("selected %d tracks, want 5 after expanding the pool: %v", len(result.Tracks), planHashes(result))
	}
	crowd := 0
	for _, pt := range result.Tracks {
		if pt.Track.Artist == "Crowd" {
			crowd++
		}
	}
	if crowd != 2 {
		t.Fatalf("capped artist supplied %d tracks, want 2", crowd)
	}
	assertNoDuplicateTracks(t, result)
}

func TestPlanBatch_DiscoveryIsSpreadThroughTheBatch(t *testing.T) {
	candidates := append(
		planTracks("d", 10, neverPlayed),
		planTracks("f", 10, recentlyPlayed)...,
	)
	result := PlanBatch(planInput(t, candidates))

	classes := make([]bool, 0, len(result.Tracks))
	for _, pt := range result.Tracks {
		classes = append(classes, pt.Discovery.IsDiscovery())
	}
	// Balanced over five tracks is two discovery tracks; front-loading would
	// give [true true false false false].
	want := []bool{false, true, false, true, false}
	for i, got := range classes {
		if got != want[i] {
			t.Fatalf("discovery layout = %v, want %v", classes, want)
		}
	}
}

func TestPlanBatch_ExplanationsAreFactual(t *testing.T) {
	jazz := neverPlayed("jazz", "Chosen Artist")
	jazz.Genre = "Jazz"
	in := planInput(t, []library.Track{jazz})
	in.Request.BatchSize = 1
	in.Request.Settings.Include = FilterSet{Genres: []string{"Jazz"}, Artists: []string{"Chosen Artist"}}

	result := PlanBatch(in)
	if len(result.Tracks) != 1 {
		t.Fatalf("selected %d tracks, want 1", len(result.Tracks))
	}
	reasons := result.Tracks[0].Reasons
	for _, code := range []ReasonCode{ReasonIncludeGenre, ReasonIncludeArtist, ReasonDiscoveryNever} {
		if !reasons.Has(code) {
			t.Fatalf("reasons = %q, want %q", reasons.Summary(), code)
		}
	}
	summary := reasons.Summary()
	if !strings.Contains(summary, "Jazz") || !strings.Contains(summary, "Chosen Artist") {
		t.Fatalf("reasons %q do not name the filter values that matched", summary)
	}
}

func TestPlanBatch_RejectionsAreOrderedByCount(t *testing.T) {
	candidates := make([]library.Track, 0, 6)
	for i := range 4 {
		tr := recentlyPlayed("g"+string(rune('a'+i)), "Artist "+string(rune('a'+i)))
		tr.Genre = "Metal"
		candidates = append(candidates, tr)
	}
	for i := range 2 {
		candidates = append(candidates, recentlyPlayed("b"+string(rune('a'+i)), "Blocked Artist"))
	}

	in := planInput(t, candidates)
	in.Request.Settings.Exclude = FilterSet{Genres: []string{"Metal"}, Artists: []string{"Blocked Artist"}}

	result := PlanBatch(in)
	if len(result.Rejections) != 2 {
		t.Fatalf("rejections = %+v, want two entries", result.Rejections)
	}
	if result.Rejections[0].Code != ReasonExcludeGenre || result.Rejections[0].Count != 4 {
		t.Fatalf("first rejection = %+v, want exclude_genre x4", result.Rejections[0])
	}
	if result.Rejections[1].Code != ReasonExcludeArtist || result.Rejections[1].Count != 2 {
		t.Fatalf("second rejection = %+v, want exclude_artist x2", result.Rejections[1])
	}
	if got := result.Rejections.Total(); got != 6 {
		t.Fatalf("Total() = %d, want 6", got)
	}
}

func TestPlanBatch_SeedIsNeverSelectedOrCountedAsRejected(t *testing.T) {
	seed := neverPlayed("seed", "Seed Artist")
	candidates := append([]library.Track{seed}, planTracks("f", 4, recentlyPlayed)...)

	result := PlanBatch(planInput(t, candidates))
	if containsHash(result, "seed") {
		t.Fatalf("the seed track was selected: %v", planHashes(result))
	}
	if result.Rejections.Total() != 0 {
		t.Fatalf("rejections = %+v, want the seed skipped silently", result.Rejections)
	}
}

func TestPlanBatch_IsDeterministicForAGivenRandSource(t *testing.T) {
	candidates := append(
		planTracks("d", 12, neverPlayed),
		planTracks("f", 12, recentlyPlayed)...,
	)
	in := planInput(t, candidates)
	in.Rand = SequenceRand(0.05, 0.6, 0.95, 0.3, 0.5)

	first := planHashes(PlanBatch(in))
	for i := range 4 {
		in.Rand = SequenceRand(0.05, 0.6, 0.95, 0.3, 0.5)
		got := planHashes(PlanBatch(in))
		if strings.Join(got, ",") != strings.Join(first, ",") {
			t.Fatalf("run %d selected %v, want %v", i, got, first)
		}
	}
}

func TestPlanBatch_DifferentDrawsSelectDifferentTracks(t *testing.T) {
	candidates := planTracks("d", 12, neverPlayed)
	in := withDiscoveryPercent(planInput(t, candidates), 100)
	in.Request.BatchSize = 1

	in.Rand = FixedRand(0)
	low := planHashes(PlanBatch(in))
	in.Rand = FixedRand(0.99)
	high := planHashes(PlanBatch(in))

	if len(low) != 1 || len(high) != 1 {
		t.Fatalf("expected one track per draw, got %v and %v", low, high)
	}
	if low[0] == high[0] {
		t.Fatalf("both ends of the distribution picked %q; the draw is not weighted", low[0])
	}
}

func TestPlanBatch_OutOfRangeDrawsStayInsideThePool(t *testing.T) {
	candidates := planTracks("d", 9, neverPlayed)
	in := withDiscoveryPercent(planInput(t, candidates), 100)

	for _, draw := range []float64{-1, 1, 42} {
		in.Rand = FixedRand(draw)
		result := PlanBatch(in)
		if len(result.Tracks) != 5 {
			t.Fatalf("draw %v selected %d tracks, want 5", draw, len(result.Tracks))
		}
		assertNoDuplicateTracks(t, result)
	}
}

func TestHydrationTargets_CoverBothPools(t *testing.T) {
	candidates := append(
		planTracks("d", 20, neverPlayed),
		planTracks("f", 20, recentlyPlayed)...,
	)
	in := planInput(t, candidates)

	hashes := HydrationTargets(in, 3)
	if len(hashes) != 6 {
		t.Fatalf("hydration targets = %v, want 3 from each pool", hashes)
	}
	discovery, familiar := 0, 0
	for _, h := range hashes {
		switch {
		case strings.HasPrefix(h, "d"):
			discovery++
		case strings.HasPrefix(h, "f"):
			familiar++
		}
	}
	if discovery != 3 || familiar != 3 {
		t.Fatalf("hydration targets %v cover %d discovery and %d familiar, want 3 each", hashes, discovery, familiar)
	}
}

func TestHydrationTargets_FavourTheHighestScoredOfEachPool(t *testing.T) {
	// One candidate per pool shares the seed's artist and album, so it
	// outranks its pool-mates on metadata alone.
	candidates := append(
		planTracks("d", 6, neverPlayed),
		planTracks("f", 6, recentlyPlayed)...,
	)
	candidates[0].Artist = "Seed Artist"
	candidates[0].Album = "Seed Album"
	candidates[6].Artist = "Seed Artist"
	candidates[6].Album = "Seed Album"

	hashes := HydrationTargets(planInput(t, candidates), 1)
	if len(hashes) != 2 {
		t.Fatalf("hydration targets = %v, want one per pool", hashes)
	}
	if hashes[0] != candidates[0].Hash || hashes[1] != candidates[6].Hash {
		t.Fatalf("hydration targets = %v, want the top-scored of each pool (%q, %q)",
			hashes, candidates[0].Hash, candidates[6].Hash)
	}
}

func TestHydrationTargets_EmptyWhenNothingToHydrate(t *testing.T) {
	in := planInput(t, planTracks("d", 4, neverPlayed))
	if got := HydrationTargets(in, 0); got != nil {
		t.Fatalf("HydrationTargets(0) = %v, want nil", got)
	}

	in.Request.Settings.DiscoveryPercent = 35
	if got := HydrationTargets(in, 3); got != nil {
		t.Fatalf("blocked settings produced hydration targets %v, want nil", got)
	}
}

func TestPlanFrom_UsesBehavioralScoresInBothPools(t *testing.T) {
	// Two candidates per pool. The lower-ranked one on metadata alone carries
	// a high completion rate, which must be able to move it ahead. Without
	// behavioral scoring reaching the discovery pool, only the familiar
	// preference would flip.
	candidates := []library.Track{
		neverPlayed("d1", "Artist d1"),
		neverPlayed("d2", "Artist d2"),
		recentlyPlayed("f1", "Artist f1"),
		recentlyPlayed("f2", "Artist f2"),
	}
	in := planInput(t, candidates)
	in.Request.BatchSize = 2
	in.CompletionRates = map[string]float64{"d2": 1, "f2": 1, "d1": 0.1, "f1": 0.1}

	pools := Partition(in)
	result := PlanFrom(in, pools)
	if len(result.Tracks) != 2 {
		t.Fatalf("selected %d tracks, want 2", len(result.Tracks))
	}
	got := planHashes(result)
	for _, want := range []string{"d2", "f2"} {
		if !containsHash(result, want) {
			t.Fatalf("selected %v, want the behaviorally preferred %q", got, want)
		}
	}
}

func TestPartition_SplitsByFamiliarity(t *testing.T) {
	candidates := []library.Track{
		neverPlayed("never", "A"),
		stalePlayed("stale", "B"),
		recentlyPlayed("recent", "C"),
	}
	pools := Partition(planInput(t, candidates))

	if len(pools.Discovery) != 2 {
		t.Fatalf("discovery pool = %d, want 2", len(pools.Discovery))
	}
	if len(pools.Familiar) != 1 {
		t.Fatalf("familiar pool = %d, want 1", len(pools.Familiar))
	}
	if pools.Len() != 3 {
		t.Fatalf("Len() = %d, want 3", pools.Len())
	}
	if pools.Discovery[0].Discovery != DiscoveryNever || pools.Discovery[1].Discovery != DiscoveryStale {
		t.Fatalf("discovery pool classifications = %q, %q", pools.Discovery[0].Discovery, pools.Discovery[1].Discovery)
	}
}

func TestPartition_ClassifiesAgainstTheRequestTimestamp(t *testing.T) {
	// The same track is discovery or familiar depending on when the request
	// was made, and the request timestamp wins over the injected clock.
	tr := recentlyPlayed("t", "A")
	in := planInput(t, []library.Track{tr})
	in.Clock = FixedClock(testNow.AddDate(0, 0, 400))

	if pools := Partition(in); len(pools.Familiar) != 1 {
		t.Fatalf("familiar pool = %d, want the request timestamp to win", len(pools.Familiar))
	}

	in.Request.RequestedAt = testNow.AddDate(0, 0, 200)
	if pools := Partition(in); len(pools.Discovery) != 1 {
		t.Fatalf("discovery pool = %d, want the track to have gone stale", len(pools.Discovery))
	}
}

func TestInterleave_SpreadsTheSmallerGroup(t *testing.T) {
	tests := []struct {
		name      string
		discovery int
		familiar  int
		want      string
	}{
		{"balanced", 2, 3, "FDFDF"},
		{"explore", 4, 1, "DDFDD"},
		{"all discovery", 3, 0, "DDD"},
		{"all familiar", 0, 3, "FFF"},
		{"empty", 0, 0, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			discovery := make([]PlannedTrack, tc.discovery)
			for i := range discovery {
				discovery[i] = PlannedTrack{Discovery: DiscoveryNever}
			}
			familiar := make([]PlannedTrack, tc.familiar)
			for i := range familiar {
				familiar[i] = PlannedTrack{Discovery: DiscoveryFamiliar}
			}

			var got strings.Builder
			for _, pt := range interleave(discovery, familiar) {
				if pt.Discovery.IsDiscovery() {
					got.WriteByte('D')
					continue
				}
				got.WriteByte('F')
			}
			if got.String() != tc.want {
				t.Fatalf("interleave = %q, want %q", got.String(), tc.want)
			}
		})
	}
}
