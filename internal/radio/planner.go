package radio

import (
	"time"

	"github.com/sglavoie/muse/internal/library"
)

// DefaultMaxArtistFraction is the artist-diversity cap radio has always used:
// no single artist may supply more than 40% of a batch.
const DefaultMaxArtistFraction = 0.4

// PlanInput is everything one planning attempt needs. It is plain data with no
// storage or UI dependency, so the Preview tab and live selection run the exact
// same planner over the exact same inputs — a preview is a plan whose result
// was not accepted, nothing more.
type PlanInput struct {
	// Request carries the settings, the seed, and the batch size.
	Request PlanRequest
	// Allocation is the rounding remainder carried from the last accepted
	// batch. Leave it zero for a preview off a fresh session.
	Allocation Allocation

	// Candidates is the library snapshot to plan from.
	Candidates []library.Track
	// TrackTags maps a track hash onto its tag ids, for both filtering and
	// the shared-tag score.
	TrackTags map[string]map[int64]struct{}

	FavoriteHashes map[string]struct{}
	// RecentHashes is the bounded recently-played ring. Membership is a
	// scoring penalty, which is how radio avoids repeating itself without
	// ever making a track unreachable.
	RecentHashes map[string]struct{}
	// ProtectedHashes is the set that must not be selected again: the track
	// playing now plus everything still upcoming. These are removed from the
	// candidate pools outright, which is the only hard duplicate guard.
	//
	// Queue rows that have already been played deliberately do not belong
	// here. Penalizing or excluding the whole persisted queue would shrink
	// continuous radio's reachable library with every batch it enqueues.
	ProtectedHashes map[string]struct{}
	// DismissedHashes are tracks the user dismissed from recommendation
	// mixes; membership is a heavy scoring penalty.
	DismissedHashes map[string]struct{}

	// Behavioral inputs, loaded for the hashes HydrationTargets reports.
	CompletionRates map[string]float64
	CoOccurrences   map[string]int
	TimeGenreBonus  map[string]float64

	// TagCatalog resolves the tag ids the filters reference. When it is
	// supplied, a filter pointing at a tag that no longer exists blocks the
	// plan rather than silently narrowing an inclusion or widening an
	// exclusion.
	//
	// Nil means "no catalog available" and skips the check, because
	// FilterSet.ResolveTags fails closed and would otherwise block every
	// tag filter for callers that cannot reach storage yet. The integration
	// session must supply one.
	TagCatalog TagCatalog

	// MaxArtistFraction caps how much of a batch one artist may supply.
	// Non-positive values fall back to DefaultMaxArtistFraction.
	MaxArtistFraction float64
	// Clock supplies "now" for discovery classification when the request
	// carries no timestamp. Nil falls back to the system clock.
	Clock Clock
	// Rand drives weighted selection. Nil falls back to CryptoRand.
	Rand RandSource
}

// now returns the instant discovery classification is evaluated against.
func (in PlanInput) now() time.Time {
	if !in.Request.RequestedAt.IsZero() {
		return in.Request.RequestedAt
	}
	return nowFrom(in.Clock)
}

// artistFraction returns the diversity cap to apply.
func (in PlanInput) artistFraction() float64 {
	if in.MaxArtistFraction <= 0 {
		return DefaultMaxArtistFraction
	}
	return in.MaxArtistFraction
}

// blockingReason reports why these settings cannot be evaluated at all. The
// second result is false when the plan may proceed.
func (in PlanInput) blockingReason() (Reason, bool) {
	if err := in.Request.Validate(); err != nil {
		return Reason{Code: ReasonInvalidSettings, Detail: err.Error()}, true
	}
	if in.TagCatalog == nil {
		return Reason{}, false
	}
	if err := in.Request.Settings.ResolveTags(in.TagCatalog); err != nil {
		return Reason{Code: ReasonUnresolvedTag, Detail: err.Error()}, true
	}
	return Reason{}, false
}

// Candidate is one track that survived the filters, with the evidence behind
// that verdict and its discovery classification.
type Candidate struct {
	Track     library.Track
	Discovery DiscoveryClass
	Reasons   Reasons
}

// Pools is the discovery/familiar partition of the eligible candidates.
//
// Partitioning happens before any pool is narrowed. Scoring the whole library
// first and then keeping a top-N would let a familiar-heavy library crowd every
// discovery track out of the pool before the discovery target is ever
// consulted, which is exactly how a 80% Explore target silently degrades into a
// familiar batch.
type Pools struct {
	Discovery []Candidate
	Familiar  []Candidate
	// Rejections tallies why the remaining candidates were removed.
	Rejections Rejections
	// Blocked is non-zero when the settings could not be evaluated at all.
	Blocked Reason
}

// Len returns the number of eligible candidates across both pools.
func (p Pools) Len() int { return len(p.Discovery) + len(p.Familiar) }

// Partition filters the candidates and splits the survivors by familiarity.
func Partition(in PlanInput) Pools {
	if blocked, ok := in.blockingReason(); ok {
		return Pools{Blocked: blocked}
	}

	filters := in.Request.Settings.Compile()
	clock := FixedClock(in.now())
	tally := newRejectionTally()
	pools := Pools{}
	for _, t := range in.Candidates {
		candidate, rejected, ok := in.evaluate(t, filters, clock)
		if !ok {
			tally.add(rejected)
			continue
		}
		if candidate.Discovery.IsDiscovery() {
			pools.Discovery = append(pools.Discovery, candidate)
			continue
		}
		pools.Familiar = append(pools.Familiar, candidate)
	}
	pools.Rejections = tally.result()
	return pools
}

// evaluate classifies one track. The third result is false when the track is
// not a candidate; the second then explains why, or is the zero Reason for a
// track that was never in the running (the seed itself).
func (in PlanInput) evaluate(t library.Track, filters CompiledFilters, clock Clock) (Candidate, Reason, bool) {
	if t.Hash != "" {
		if t.Hash == in.Request.Seed.TrackHash {
			return Candidate{}, Reason{}, false
		}
		if hasHash(in.ProtectedHashes, t.Hash) {
			return Candidate{}, Reason{Code: ReasonAlreadyQueued}, false
		}
	}

	verdict := filters.Match(FactsFromTrack(t, in.TrackTags[t.Hash]))
	if !verdict.Eligible {
		return Candidate{}, verdict.Blocked, false
	}

	class := ClassifyTrack(t, clock)
	reasons := make(Reasons, 0, len(verdict.Matched)+1)
	reasons = append(reasons, verdict.Matched...)
	reasons = append(reasons, DiscoveryReason(class))
	return Candidate{Track: t, Discovery: class, Reasons: reasons}, Reason{}, true
}

// HydrationTargets returns the hashes whose behavioral data is worth loading
// before the final plan: the top perGroup of each pool ranked on metadata
// alone.
//
// Taking perGroup from each pool separately is what guarantees both groups
// receive behavioral scoring. A single global top-N over the merged pools would
// hand every hydration slot to whichever group scores higher on metadata, so
// the other group would be scored without completion rates or co-occurrence and
// would lose every close contest.
func (p Pools) HydrationTargets(in PlanInput, perGroup int) []string {
	if perGroup <= 0 {
		return nil
	}
	seen := make(map[string]struct{}, 2*perGroup)
	hashes := make([]string, 0, 2*perGroup)
	for _, pool := range [][]Candidate{p.Discovery, p.Familiar} {
		scored := scoreCandidates(in, pool, false)
		for i := 0; i < len(scored) && i < perGroup; i++ {
			hash := scored[i].Track.Hash
			if hash == "" {
				continue
			}
			if _, dup := seen[hash]; dup {
				continue
			}
			seen[hash] = struct{}{}
			hashes = append(hashes, hash)
		}
	}
	if len(hashes) == 0 {
		return nil
	}
	return hashes
}

// HydrationTargets partitions the candidates and reports the hashes to load
// behavioral data for. Callers that go on to plan should keep the Pools from
// Partition and call the method instead of partitioning twice.
func HydrationTargets(in PlanInput, perGroup int) []string {
	return Partition(in).HydrationTargets(in, perGroup)
}

// PlanBatch is the pure planner shared by preview and live selection. It
// partitions, scores, and selects one batch, and reports what it could not do.
func PlanBatch(in PlanInput) PlanResult {
	return PlanFrom(in, Partition(in))
}

// PlanFrom plans from an already-computed partition, so a caller that hydrated
// behavioral data from Pools.HydrationTargets does not pay for a second
// partition pass.
func PlanFrom(in PlanInput, pools Pools) PlanResult {
	carried := in.Allocation.Normalized()
	if !pools.Blocked.IsZero() {
		result := BlockedPlan(pools.Blocked)
		result.Rejections = pools.Rejections
		result.NextAllocation = carried
		return result
	}

	target, next := carried.TargetFor(in.Request.BatchSize, in.Request.Settings.DiscoveryPercent)
	if pools.Len() == 0 {
		result := EmptyPlan(target, Reason{Code: ReasonNoCandidates, Detail: noCandidatesDetail(pools)})
		result.Rejections = pools.Rejections
		// Nothing was selected, so nothing consumed the allocation.
		result.NextAllocation = carried
		return result
	}

	tracks := selectBatch(in, pools, target)
	result := summarize(tracks, target, in.Request.BatchSize)
	result.Rejections = pools.Rejections
	result.NextAllocation = next
	return result
}

// selectBatch fills the discovery quota, then the familiar quota, then relaxes
// the discovery target to backfill whatever is left.
//
// Both quotas are drawn by one picker, so the artist-diversity cap and
// duplicate suppression apply across the whole batch instead of once per pool.
// Filters are never relaxed: both pools already contain only tracks that
// satisfied them.
func selectBatch(in PlanInput, pools Pools, target int) []PlannedTrack {
	batchSize := in.Request.BatchSize
	discoveryScored := scoreCandidates(in, pools.Discovery, true)
	familiarScored := scoreCandidates(in, pools.Familiar, true)
	byKey := candidatesByKey(pools)
	p := newPicker(batchSize, in.artistFraction(), in.Rand)

	discovery := p.pick(discoveryScored, target)
	familiar := p.pick(familiarScored, batchSize-target)
	if missing := batchSize - len(discovery) - len(familiar); missing > 0 {
		discovery = append(discovery, p.pick(discoveryScored, missing)...)
	}
	if missing := batchSize - len(discovery) - len(familiar); missing > 0 {
		familiar = append(familiar, p.pick(familiarScored, missing)...)
	}

	clock := FixedClock(in.now())
	return interleave(
		plannedFrom(discovery, byKey, clock),
		plannedFrom(familiar, byKey, clock),
	)
}

// summarize turns a selection into a PlanResult, recording every quota the
// candidates could not fill.
func summarize(tracks []PlannedTrack, target, batchSize int) PlanResult {
	result := PlanResult{Tracks: tracks, Outcome: PlanOutcomeOK, DiscoveryTarget: target}
	if len(tracks) == 0 {
		result.Outcome = PlanOutcomeEmpty
		result.Reasons = Reasons{{Code: ReasonNoCandidates, Detail: "no candidate survived selection"}}
		return result
	}

	discoveryGot := result.DiscoveryCount()
	familiarTarget := batchSize - target
	familiarGot := len(tracks) - discoveryGot
	if discoveryGot < target {
		result.Shortages = append(result.Shortages, Shortage{Kind: ShortageDiscovery, Want: target, Got: discoveryGot})
		result.Reasons = append(result.Reasons, Reason{
			Code:   ReasonDiscoveryRelaxed,
			Detail: "not enough discovery tracks matched the filters",
		})
	}
	if familiarGot < familiarTarget {
		result.Shortages = append(result.Shortages, Shortage{Kind: ShortageFamiliar, Want: familiarTarget, Got: familiarGot})
		result.Reasons = append(result.Reasons, Reason{
			Code:   ReasonFamiliarRelaxed,
			Detail: "not enough familiar tracks matched the filters",
		})
	}
	if len(tracks) < batchSize {
		result.Shortages = append(result.Shortages, Shortage{Kind: ShortageBatch, Want: batchSize, Got: len(tracks)})
		result.Reasons = append(result.Reasons, Reason{
			Code:   ReasonBatchShort,
			Detail: "the eligible candidates could not fill the batch",
		})
	}
	if len(result.Shortages) > 0 {
		result.Outcome = PlanOutcomeRelaxed
	}
	return result
}

// scoreCandidates runs the existing scorer over one pool. Behavioral inputs are
// withheld for the metadata-only pass that chooses what to hydrate.
//
// ScoreInput.QueueHashes is deliberately left unset: duplicate suppression is
// done by removing ProtectedHashes from the pools, and applying the queue
// penalty to the whole persisted queue would push every already-played row out
// of reach of continuous radio.
func scoreCandidates(in PlanInput, pool []Candidate, behavioral bool) []TrackScore {
	if len(pool) == 0 {
		return nil
	}
	tracks := make([]library.Track, 0, len(pool))
	for _, c := range pool {
		tracks = append(tracks, c.Track)
	}
	input := ScoreInput{
		Seed:            in.Request.Seed.SeedTrack(),
		AllTracks:       tracks,
		TrackTags:       in.TrackTags,
		FavoriteHashes:  in.FavoriteHashes,
		RecentHashes:    in.RecentHashes,
		DismissedHashes: in.DismissedHashes,
	}
	if behavioral {
		input.CompletionRates = in.CompletionRates
		input.CoOccurrences = in.CoOccurrences
		input.TimeGenreBonus = in.TimeGenreBonus
	}
	return ScoreAll(input)
}

// candidatesByKey indexes both pools so a pick can be paired back with the
// reasons collected when it was classified.
func candidatesByKey(pools Pools) map[string]Candidate {
	byKey := make(map[string]Candidate, pools.Len())
	for _, pool := range [][]Candidate{pools.Discovery, pools.Familiar} {
		for _, c := range pool {
			if key := trackSelectionKey(c.Track); key != "" {
				byKey[key] = c
			}
		}
	}
	return byKey
}

// plannedFrom pairs each pick with its recorded evidence. A track with neither
// a hash nor a path cannot be indexed, so it is reclassified on the spot rather
// than reported with an empty classification.
func plannedFrom(picked []TrackScore, byKey map[string]Candidate, clock Clock) []PlannedTrack {
	if len(picked) == 0 {
		return nil
	}
	planned := make([]PlannedTrack, 0, len(picked))
	for _, ts := range picked {
		candidate, ok := byKey[trackSelectionKey(ts.Track)]
		if !ok {
			class := ClassifyTrack(ts.Track, clock)
			candidate = Candidate{Track: ts.Track, Discovery: class, Reasons: Reasons{DiscoveryReason(class)}}
		}
		planned = append(planned, PlannedTrack{
			Track:     ts.Track,
			Score:     ts.Score,
			Discovery: candidate.Discovery,
			Reasons:   candidate.Reasons,
		})
	}
	return planned
}

// interleave spreads the discovery picks evenly through the batch instead of
// front-loading them, so an Explore batch does not play four unfamiliar tracks
// back to back before its one familiar track. The distribution is exact and
// deterministic: slot i takes a discovery track whenever the running count
// falls behind its proportional share.
func interleave(discovery, familiar []PlannedTrack) []PlannedTrack {
	total := len(discovery) + len(familiar)
	if total == 0 {
		return nil
	}
	out := make([]PlannedTrack, 0, total)
	di, fi := 0, 0
	for i := 0; i < total; i++ {
		want := ((i+1)*len(discovery)*2 + total) / (2 * total)
		if di < len(discovery) && (want > di || fi >= len(familiar)) {
			out = append(out, discovery[di])
			di++
			continue
		}
		out = append(out, familiar[fi])
		fi++
	}
	return out
}

// noCandidatesDetail explains an empty partition in terms of what removed the
// candidates, so the paused-additions message is factual rather than generic.
func noCandidatesDetail(pools Pools) string {
	if top, ok := pools.Rejections.Top(); ok {
		return "no track matched the filters; most were removed by " + top.Reason().String()
	}
	return "no track was available to plan from"
}
