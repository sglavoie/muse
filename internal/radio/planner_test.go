package radio

import (
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

func TestPlanBatch_PresetsHitTheirDiscoveryShare(t *testing.T) {
	candidates := append(
		planTracks("d", 10, neverPlayed),
		planTracks("f", 10, recentlyPlayed)...,
	)

	tests := []struct {
		preset        Preset
		wantDiscovery int
		wantShare     int
	}{
		{PresetFamiliar, 1, 20},
		{PresetBalanced, 2, 40},
		{PresetExplore, 4, 80},
	}

	for _, tc := range tests {
		t.Run(string(tc.preset), func(t *testing.T) {
			settings, err := DefaultSettings().WithPreset(tc.preset)
			if err != nil {
				t.Fatalf("WithPreset: %v", err)
			}
			in := planInput(t, candidates)
			in.Request.Settings = settings

			result := PlanBatch(in)
			if result.Outcome != PlanOutcomeOK {
				t.Fatalf("outcome = %q with reasons %q, want ok", result.Outcome, result.Reasons.Summary())
			}
			if len(result.Tracks) != 5 {
				t.Fatalf("selected %d tracks, want 5", len(result.Tracks))
			}
			if result.DiscoveryTarget != tc.wantDiscovery {
				t.Fatalf("DiscoveryTarget = %d, want %d", result.DiscoveryTarget, tc.wantDiscovery)
			}
			if got := result.DiscoveryCount(); got != tc.wantDiscovery {
				t.Fatalf("delivered %d discovery tracks, want %d (%v)", got, tc.wantDiscovery, planHashes(result))
			}
			if got := result.DiscoveryShare(); got != tc.wantShare {
				t.Fatalf("DiscoveryShare() = %d, want %d", got, tc.wantShare)
			}
			if len(result.Shortages) != 0 {
				t.Fatalf("unexpected shortages: %+v", result.Shortages)
			}
			assertNoDuplicateTracks(t, result)
		})
	}
}

func TestPlanBatch_StalePlaysCountAsDiscovery(t *testing.T) {
	// Only stale tracks can satisfy the discovery target here, so the plan is
	// only "ok" if the 90-day rule is applied.
	candidates := append(
		planTracks("s", 6, stalePlayed),
		planTracks("f", 6, recentlyPlayed)...,
	)
	result := PlanBatch(planInput(t, candidates))

	if result.Outcome != PlanOutcomeOK {
		t.Fatalf("outcome = %q (%s), want ok", result.Outcome, result.Reasons.Summary())
	}
	if got := result.DiscoveryCount(); got != 2 {
		t.Fatalf("delivered %d discovery tracks, want 2", got)
	}
	for _, pt := range result.Tracks {
		if !pt.Discovery.IsDiscovery() {
			continue
		}
		if pt.Discovery != DiscoveryStale {
			t.Fatalf("track %q classified %q, want stale", pt.Track.Hash, pt.Discovery)
		}
		if !pt.Reasons.Has(ReasonDiscoveryStale) {
			t.Fatalf("track %q reasons = %q, want a discovery_stale reason", pt.Track.Hash, pt.Reasons.Summary())
		}
	}
}

func TestPlanBatch_FractionalTargetAlternatesAcrossAcceptedBatches(t *testing.T) {
	candidates := append(
		planTracks("d", 20, neverPlayed),
		planTracks("f", 20, recentlyPlayed)...,
	)
	in := withDiscoveryPercent(planInput(t, candidates), 30)

	// Each accepted batch adopts the returned allocation and protects what it
	// selected, exactly as continuous radio does.
	protected := map[string]struct{}{}
	in.ProtectedHashes = protected
	delivered := make([]int, 0, 4)
	for range 4 {
		result := PlanBatch(in)
		if len(result.Tracks) != 5 {
			t.Fatalf("selected %d tracks, want 5", len(result.Tracks))
		}
		delivered = append(delivered, result.DiscoveryCount())
		for _, pt := range result.Tracks {
			protected[pt.Track.Hash] = struct{}{}
		}
		in.Allocation = result.NextAllocation
	}

	want := []int{2, 1, 2, 1}
	total := 0
	for i, got := range delivered {
		if got != want[i] {
			t.Fatalf("batch %d delivered %d discovery tracks, want %d (all: %v)", i, got, want[i], delivered)
		}
		total += got
	}
	if total != 6 {
		t.Fatalf("delivered %d discovery tracks over 20 selections, want 6 (30%%)", total)
	}
}

func TestPlanBatch_PreviewConsumesNoAllocation(t *testing.T) {
	candidates := append(
		planTracks("d", 20, neverPlayed),
		planTracks("f", 20, recentlyPlayed)...,
	)
	in := withDiscoveryPercent(planInput(t, candidates), 30)

	// Five previews in a row: the caller never adopts NextAllocation.
	for i := range 5 {
		preview := PlanBatch(in)
		if got := preview.DiscoveryCount(); got != 2 {
			t.Fatalf("preview %d delivered %d discovery tracks, want 2", i, got)
		}
		if preview.NextAllocation.IsZero() {
			t.Fatalf("preview %d proposed no allocation change, want the rounding remainder", i)
		}
	}

	// The live batch that follows is unaffected by all that previewing.
	live := PlanBatch(in)
	if got := live.DiscoveryCount(); got != 2 {
		t.Fatalf("live batch delivered %d discovery tracks, want 2", got)
	}
	in.Allocation = live.NextAllocation
	if got := PlanBatch(in).DiscoveryCount(); got != 1 {
		t.Fatalf("second accepted batch delivered %d discovery tracks, want 1", got)
	}
}

func TestPlanBatch_SparseDiscoveryRelaxesTargetWithoutDebt(t *testing.T) {
	// One discovery track, plenty of familiar ones, at the Explore target.
	candidates := append(
		planTracks("d", 1, neverPlayed),
		planTracks("f", 20, recentlyPlayed)...,
	)
	in := withDiscoveryPercent(planInput(t, candidates), 80)

	result := PlanBatch(in)
	if len(result.Tracks) != 5 {
		t.Fatalf("selected %d tracks, want a full batch of 5", len(result.Tracks))
	}
	if result.Outcome != PlanOutcomeRelaxed {
		t.Fatalf("outcome = %q, want relaxed", result.Outcome)
	}
	if got := result.DiscoveryCount(); got != 1 {
		t.Fatalf("delivered %d discovery tracks, want the single one available", got)
	}
	shortage, ok := result.Shortages.Get(ShortageDiscovery)
	if !ok {
		t.Fatalf("expected a discovery shortage, got %+v", result.Shortages)
	}
	if shortage.Want != 4 || shortage.Got != 1 || shortage.Missing() != 3 {
		t.Fatalf("shortage = %+v, want want=4 got=1", shortage)
	}
	if !result.Reasons.Has(ReasonDiscoveryRelaxed) {
		t.Fatalf("reasons = %q, want a discovery_relaxed reason", result.Reasons.Summary())
	}
	// The three missing discovery tracks must not become a debt the next batch
	// tries to repay: the carry only ever holds the rounding remainder.
	if !result.NextAllocation.IsZero() {
		t.Fatalf("NextAllocation = %v tracks, want no debt from a shortage", result.NextAllocation.Carry())
	}
}

func TestPlanBatch_RepeatedShortagesNeverAccumulateDebt(t *testing.T) {
	candidates := append(
		planTracks("d", 1, neverPlayed),
		planTracks("f", 60, recentlyPlayed)...,
	)
	in := withDiscoveryPercent(planInput(t, candidates), 70)
	protected := map[string]struct{}{}
	in.ProtectedHashes = protected

	for i := range 8 {
		result := PlanBatch(in)
		if result.NextAllocation.CarryUnits > maxAllocationCarry ||
			result.NextAllocation.CarryUnits < -maxAllocationCarry {
			t.Fatalf("batch %d carried %d units, outside the bound", i, result.NextAllocation.CarryUnits)
		}
		if result.DiscoveryTarget > in.Request.BatchSize {
			t.Fatalf("batch %d asked for %d discovery tracks in a batch of %d", i, result.DiscoveryTarget, in.Request.BatchSize)
		}
		for _, pt := range result.Tracks {
			protected[pt.Track.Hash] = struct{}{}
		}
		in.Allocation = result.NextAllocation
	}
}

func TestPlanBatch_SparseFamiliarBackfillsWithDiscovery(t *testing.T) {
	candidates := append(
		planTracks("d", 20, neverPlayed),
		planTracks("f", 1, recentlyPlayed)...,
	)
	in := withDiscoveryPercent(planInput(t, candidates), 20)

	result := PlanBatch(in)
	if len(result.Tracks) != 5 {
		t.Fatalf("selected %d tracks, want 5", len(result.Tracks))
	}
	if got := result.DiscoveryCount(); got != 4 {
		t.Fatalf("delivered %d discovery tracks, want 4 after backfilling the familiar quota", got)
	}
	if !result.Shortages.Has(ShortageFamiliar) {
		t.Fatalf("expected a familiar shortage, got %+v", result.Shortages)
	}
	if !result.Reasons.Has(ReasonFamiliarRelaxed) {
		t.Fatalf("reasons = %q, want a familiar_relaxed reason", result.Reasons.Summary())
	}
	if result.Outcome != PlanOutcomeRelaxed {
		t.Fatalf("outcome = %q, want relaxed", result.Outcome)
	}
}

func TestPlanBatch_SparseLibraryReturnsShortBatch(t *testing.T) {
	candidates := append(
		planTracks("d", 1, neverPlayed),
		planTracks("f", 2, recentlyPlayed)...,
	)
	result := PlanBatch(planInput(t, candidates))

	if len(result.Tracks) != 3 {
		t.Fatalf("selected %d tracks, want all 3 available", len(result.Tracks))
	}
	shortage, ok := result.Shortages.Get(ShortageBatch)
	if !ok {
		t.Fatalf("expected a batch shortage, got %+v", result.Shortages)
	}
	if shortage.Want != 5 || shortage.Got != 3 {
		t.Fatalf("shortage = %+v, want want=5 got=3", shortage)
	}
	if !result.CanContinue() {
		t.Fatalf("a short but non-empty batch must still be enqueueable")
	}
	assertNoDuplicateTracks(t, result)
}

func TestPlanBatch_NoCandidatesPausesWithAFactualReason(t *testing.T) {
	candidates := planTracks("f", 8, recentlyPlayed)
	in := planInput(t, candidates)
	in.Request.Settings.Exclude = FilterSet{Genres: []string{"Rock"}}

	result := PlanBatch(in)
	if result.Outcome != PlanOutcomeEmpty {
		t.Fatalf("outcome = %q, want empty", result.Outcome)
	}
	if result.CanContinue() {
		t.Fatalf("an empty plan must pause additions")
	}
	if !result.Reasons.Has(ReasonNoCandidates) {
		t.Fatalf("reasons = %q, want no_candidates", result.Reasons.Summary())
	}
	if got := result.Rejections.Count(ReasonExcludeGenre); got != 8 {
		t.Fatalf("excluded %d tracks by genre, want 8 (%+v)", got, result.Rejections)
	}
	if summary := result.Reasons.Summary(); !strings.Contains(summary, string(ReasonExcludeGenre)) {
		t.Fatalf("reason %q does not name the filter that removed the candidates", summary)
	}
	if !result.NextAllocation.IsZero() {
		t.Fatalf("an empty plan consumed allocation: %+v", result.NextAllocation)
	}
}

func TestPlanBatch_InvalidSettingsBlockWithoutTouchingAllocation(t *testing.T) {
	in := planInput(t, planTracks("d", 5, neverPlayed))
	in.Request.Settings.DiscoveryPercent = 35
	in.Allocation = Allocation{CarryUnits: -50}

	result := PlanBatch(in)
	if result.Outcome != PlanOutcomeBlocked {
		t.Fatalf("outcome = %q, want blocked", result.Outcome)
	}
	if !result.Reasons.Has(ReasonInvalidSettings) {
		t.Fatalf("reasons = %q, want invalid_settings", result.Reasons.Summary())
	}
	if result.NextAllocation.CarryUnits != -50 {
		t.Fatalf("NextAllocation = %+v, want the carry left untouched", result.NextAllocation)
	}
}

func TestPlanBatch_UnresolvedTagReferenceBlocks(t *testing.T) {
	in := planInput(t, planTracks("d", 5, neverPlayed))
	in.Request.Settings.Include = FilterSet{TagIDs: []int64{7}}

	// Without a catalog the planner cannot tell a live tag from a deleted
	// one, so it proceeds rather than blocking every tag filter.
	if got := PlanBatch(in).Outcome; got == PlanOutcomeBlocked {
		t.Fatalf("outcome = %q with no catalog, want the check skipped", got)
	}

	in.TagCatalog = TagIDSet{7: {}}
	if got := PlanBatch(in).Outcome; got == PlanOutcomeBlocked {
		t.Fatalf("outcome = %q with a resolving catalog, want the plan to proceed", got)
	}

	in.TagCatalog = TagIDSet{9: {}}
	result := PlanBatch(in)
	if result.Outcome != PlanOutcomeBlocked {
		t.Fatalf("outcome = %q for a deleted tag, want blocked", result.Outcome)
	}
	if !result.Reasons.Has(ReasonUnresolvedTag) {
		t.Fatalf("reasons = %q, want unresolved_tag", result.Reasons.Summary())
	}
	if result.CanContinue() {
		t.Fatalf("a blocked plan must pause additions")
	}
}

func TestPlanBatch_FiltersApplyBeforePartitioning(t *testing.T) {
	// Every discovery track is excluded by artist. Partitioning after
	// filtering means the discovery pool is genuinely empty and the target is
	// relaxed; partitioning before filtering would have reported a discovery
	// pool it could not draw from.
	candidates := append(
		planTracks("d", 6, neverPlayed),
		planTracks("f", 6, recentlyPlayed)...,
	)
	in := planInput(t, candidates)
	excluded := make([]string, 0, 6)
	for _, tr := range candidates[:6] {
		excluded = append(excluded, tr.Artist)
	}
	in.Request.Settings.Exclude = FilterSet{Artists: excluded}

	pools := Partition(in)
	if len(pools.Discovery) != 0 {
		t.Fatalf("discovery pool = %d, want 0 after exclusions", len(pools.Discovery))
	}
	if len(pools.Familiar) != 6 {
		t.Fatalf("familiar pool = %d, want 6", len(pools.Familiar))
	}

	result := PlanFrom(in, pools)
	if got := result.DiscoveryCount(); got != 0 {
		t.Fatalf("delivered %d discovery tracks from an empty pool", got)
	}
	if !result.Shortages.Has(ShortageDiscovery) {
		t.Fatalf("expected a discovery shortage, got %+v", result.Shortages)
	}
	for _, pt := range result.Tracks {
		if !pt.Reasons.Has(ReasonFamiliar) {
			t.Fatalf("track %q reasons = %q, want a familiar reason", pt.Track.Hash, pt.Reasons.Summary())
		}
	}
}

func TestPlanBatch_IncludedFiltersAreNeverRelaxed(t *testing.T) {
	jazz := neverPlayed("jazz", "Jazz Artist")
	jazz.Genre = "Jazz"
	candidates := append(planTracks("f", 10, recentlyPlayed), jazz)

	in := withDiscoveryPercent(planInput(t, candidates), 80)
	in.Request.Settings.Include = FilterSet{Genres: []string{"Jazz"}}

	result := PlanBatch(in)
	if len(result.Tracks) != 1 {
		t.Fatalf("selected %d tracks, want only the one that matched the filter", len(result.Tracks))
	}
	if result.Tracks[0].Track.Hash != "jazz" {
		t.Fatalf("selected %q, want the jazz track", result.Tracks[0].Track.Hash)
	}
	if !result.Tracks[0].Reasons.Has(ReasonIncludeGenre) {
		t.Fatalf("reasons = %q, want an include_genre match", result.Tracks[0].Reasons.Summary())
	}
	if got := result.Rejections.Count(ReasonMissingGenre); got != 10 {
		t.Fatalf("rejected %d tracks for a missing genre, want 10", got)
	}
}

func TestPlanBatch_ProtectedTracksAreNeverSelectedTwice(t *testing.T) {
	candidates := append(
		planTracks("d", 4, neverPlayed),
		planTracks("f", 4, recentlyPlayed)...,
	)
	in := planInput(t, candidates)
	in.ProtectedHashes = hashSet("da", "fa")

	result := PlanBatch(in)
	for _, pt := range result.Tracks {
		if pt.Track.Hash == "da" || pt.Track.Hash == "fa" {
			t.Fatalf("selected protected track %q", pt.Track.Hash)
		}
	}
	if got := result.Rejections.Count(ReasonAlreadyQueued); got != 2 {
		t.Fatalf("rejected %d already-queued tracks, want 2 (%+v)", got, result.Rejections)
	}
}

func TestPlanBatch_PlayedQueueRowsStayReachable(t *testing.T) {
	// A track that was played earlier in this session is in the persisted
	// queue but is not upcoming. Continuous radio must still be able to reach
	// it once it ages out of the bounded recent ring.
	played := recentlyPlayed("played", "Played Artist")
	candidates := []library.Track{played}

	in := planInput(t, candidates)
	in.ProtectedHashes = hashSet("nowplaying", "upcoming")
	in.RecentHashes = hashSet("nowplaying", "upcoming")

	result := PlanBatch(in)
	if !containsHash(result, "played") {
		t.Fatalf("a played-but-not-upcoming queue row was unreachable: %+v", result.Rejections)
	}
}

func TestPlanBatch_RecentHistoryIsAvoidedButNotExcluded(t *testing.T) {
	fresh := recentlyPlayed("fresh", "Fresh Artist")
	stale := recentlyPlayed("stale", "Stale Artist")

	in := withDiscoveryPercent(planInput(t, []library.Track{stale, fresh}), 0)
	in.Request.BatchSize = 1
	in.RecentHashes = hashSet("stale")

	// The recency penalty demotes the recently heard track...
	result := PlanBatch(in)
	assertPlanHashes(t, result, "fresh")

	// ...but never removes it: with nothing else available it is still picked.
	in.Candidates = []library.Track{stale}
	if got := PlanBatch(in); !containsHash(got, "stale") {
		t.Fatalf("recently played track was excluded rather than penalized: %+v", got.Rejections)
	}
}

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
