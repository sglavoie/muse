package radio

import (
	"math"
	"path/filepath"
	"strings"

	"github.com/sglavoie/muse/internal/library"
)

// weightFloor is the minimum selection weight. Without it a zero or negative
// score would be unreachable in the weighted draw, so a penalized track could
// never be picked even when it is the only candidate left.
const weightFloor = 0.1

// picker draws tracks from scored pools without replacement. Its state is the
// batch-wide artist tally and the set of already-picked tracks, so the same
// picker can be run over the discovery pool and then the familiar pool while
// the artist-diversity cap and duplicate suppression still apply across the
// whole batch rather than per pool.
type picker struct {
	artistCount  map[string]int
	pickedKeys   map[string]struct{}
	maxPerArtist int
	rand         RandSource
}

// newPicker builds a picker whose artist cap is derived from the size of the
// batch it will fill, not from the size of any individual pool.
func newPicker(batchSize int, maxArtistFraction float64, rand RandSource) *picker {
	if rand == nil {
		rand = CryptoRand()
	}
	return &picker{
		artistCount:  make(map[string]int),
		pickedKeys:   make(map[string]struct{}),
		maxPerArtist: maxTracksPerArtist(batchSize, maxArtistFraction),
		rand:         rand,
	}
}

// pick selects up to n tracks from the top min(3*n, len(scored)) candidates
// using weighted random selection without replacement, expanding into the tail
// of scored whenever the artist cap or duplicate suppression exhausts the
// working pool. Candidates already taken by an earlier pick on this picker are
// never returned twice.
func (p *picker) pick(scored []TrackScore, n int) []TrackScore {
	if n <= 0 || len(scored) == 0 {
		return nil
	}

	pool, nextIdx := initialPool(scored, n)
	// Cap the allocation by the pool: n is a request, and a caller asking for
	// more tracks than exist must not size a slice from that request.
	result := make([]TrackScore, 0, min(n, len(scored)))
	for len(result) < n {
		candidates := p.collectEligible(pool)
		for len(candidates.indexes) == 0 && nextIdx < len(scored) {
			pool = append(pool, scored[nextIdx])
			nextIdx++
			candidates = p.collectEligible(pool)
		}
		if len(candidates.indexes) == 0 {
			break
		}

		idx := p.choose(candidates)
		picked := pool[idx]
		result = append(result, picked)
		p.note(picked.Track)
		pool = append(pool[:idx], pool[idx+1:]...)
	}

	return result
}

// note records a pick so later draws respect the artist cap and never return
// the same track twice.
func (p *picker) note(t library.Track) {
	if key := normalizedArtistKey(t.Artist); key != "" {
		p.artistCount[key]++
	}
	if key := trackSelectionKey(t); key != "" {
		p.pickedKeys[key] = struct{}{}
	}
}

// picked reports whether the track has already been taken by this picker.
func (p *picker) picked(t library.Track) bool {
	key := trackSelectionKey(t)
	if key == "" {
		return false
	}
	_, ok := p.pickedKeys[key]
	return ok
}

// initialPool returns the working pool and the index of the first candidate
// held in reserve. Starting at 3*n keeps the draw focused on the strongest
// candidates while leaving a tail to expand into.
func initialPool(scored []TrackScore, n int) ([]TrackScore, int) {
	poolSize := len(scored)
	if n <= len(scored)/3 {
		poolSize = 3 * n
	}
	pool := make([]TrackScore, poolSize)
	copy(pool, scored[:poolSize])
	return pool, poolSize
}

// eligibleSet is the draw-ready view of a pool: the indexes that may still be
// picked, their weights, and the bookkeeping the draw needs to decide between
// a weighted pick and the deterministic fallback.
type eligibleSet struct {
	indexes     []int
	weights     []float64
	totalWeight float64
	anyPositive bool
}

// choose returns the pool index to take.
//
// When no eligible candidate has a positive score, every weight is the floor
// and the weighted draw is uniform — defeating the purpose of scoring. In that
// case, and whenever the totals or the entropy source are unusable, fall back
// to the first eligible candidate, which is the highest-scored one because
// ScoreAll sorts descending.
func (p *picker) choose(set eligibleSet) int {
	total := set.totalWeight
	if !set.anyPositive || total <= 0 || math.IsNaN(total) || math.IsInf(total, 0) {
		return set.indexes[0]
	}
	raw, err := p.rand.Float64()
	if err != nil {
		return set.indexes[0]
	}
	r, ok := unitInterval(raw)
	if !ok {
		return set.indexes[0]
	}

	target := r * total
	picked := set.indexes[len(set.indexes)-1]
	cumulative := 0.0
	for j, i := range set.indexes {
		cumulative += set.weights[j]
		if target < cumulative {
			picked = i
			break
		}
	}
	return picked
}

// collectEligible filters the pool down to the candidates that may still be
// drawn and computes their weights.
func (p *picker) collectEligible(pool []TrackScore) eligibleSet {
	set := eligibleSet{
		indexes: make([]int, 0, len(pool)),
		weights: make([]float64, 0, len(pool)),
	}
	for i, ts := range pool {
		if p.picked(ts.Track) {
			continue
		}
		artistKey := normalizedArtistKey(ts.Track.Artist)
		if artistKey != "" && p.artistCount[artistKey] >= p.maxPerArtist {
			continue
		}
		score := sanitizedScore(ts.Score)
		weight := math.Max(score, weightFloor)
		set.indexes = append(set.indexes, i)
		set.weights = append(set.weights, weight)
		set.totalWeight += weight
		if score > 0 {
			set.anyPositive = true
		}
	}
	return set
}

// sanitizedScore neutralizes NaN and Inf scores so they can never poison the
// cumulative draw: NaN propagates through addition, making totalWeight NaN and
// every `target < cumulative` comparison false — which silently degrades to
// "always pick the last eligible". An Inf score makes every weight after it
// unreachable. Treating both as zero keeps the rest of the pool live and keeps
// them out of the positive-score check.
func sanitizedScore(score float64) float64 {
	if math.IsNaN(score) || math.IsInf(score, 0) {
		return 0
	}
	return score
}

// maxTracksPerArtist converts a diversity fraction into a whole-track cap for
// a batch of n. A missing or nonsensical fraction collapses to one track per
// artist rather than to no limit.
func maxTracksPerArtist(n int, maxArtistFraction float64) int {
	if math.IsNaN(maxArtistFraction) || math.IsInf(maxArtistFraction, 0) || maxArtistFraction <= 0 {
		return 1
	}
	if maxArtistFraction >= 1 {
		return n
	}

	maxPerArtist := int(math.Ceil(float64(n) * maxArtistFraction))
	if maxPerArtist < 1 {
		return 1
	}
	return maxPerArtist
}

func normalizedArtistKey(artist string) string {
	return strings.ToLower(strings.TrimSpace(artist))
}

// trackSelectionKey identifies a track for duplicate suppression, preferring
// the content hash and falling back to the cleaned path for library entries
// that have not been hashed yet.
func trackSelectionKey(t library.Track) string {
	if t.Hash != "" {
		return "hash:" + t.Hash
	}
	if path := strings.TrimSpace(t.Path); path != "" {
		return "path:" + filepath.Clean(path)
	}
	return ""
}
