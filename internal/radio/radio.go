package radio

import (
	cryptorand "crypto/rand"
	"math"
	"math/big"
	"sort"
	"strings"

	"github.com/sglavoie/muse/internal/library"
)

// SeedTrack holds the metadata for the currently playing track used to score candidates.
type SeedTrack struct {
	Hash     string
	Artist   string
	Album    string
	Genres   []string // pre-split via library.SplitGenres
	Duration float64
	TagIDs   map[int64]struct{}
}

// ScoreInput bundles all data needed to score a library against a seed.
type ScoreInput struct {
	Seed            SeedTrack
	AllTracks       []library.Track
	TrackTags       map[string]map[int64]struct{}
	FavoriteHashes  map[string]struct{}
	RecentHashes    map[string]struct{}
	QueueHashes     map[string]struct{}
	DismissedHashes map[string]struct{} // tracks dismissed from recommendation mixes
	CompletionRates map[string]float64  // hash -> avg completion 0.0-1.0; nil = not loaded
	CoOccurrences   map[string]int      // hash -> co-occurrence count with seed; nil = not loaded
	TimeGenreBonus  map[string]float64  // genre -> bonus for current hour; nil = disabled
}

// TrackScore pairs a track with its computed relevance score.
type TrackScore struct {
	Track library.Track
	Score float64
}

// ScoreAll scores every candidate in input.AllTracks against the seed and
// returns the results sorted descending by score. The seed track itself is
// excluded from the output.
func ScoreAll(input ScoreInput) []TrackScore {
	results := make([]TrackScore, 0, len(input.AllTracks))
	seedGenres := normalizedGenreSet(input.Seed.Genres)
	timeGenreBonus := normalizedGenreBonusMap(input.TimeGenreBonus)

	for _, track := range input.AllTracks {
		// Only skip seed by hash when seed.Hash is non-empty; otherwise an
		// empty seed Hash would accidentally drop every candidate whose Hash
		// is also empty (e.g., not-yet-hashed library entries).
		if input.Seed.Hash != "" && track.Hash == input.Seed.Hash {
			continue
		}

		score := 0.0

		// Genre matching: +10 per shared logical genre. Matched
		// case-insensitively so tag values like "Rock" and "rock" in
		// real-world libraries are treated as the same genre, and duplicate
		// case variants are only counted once.
		candidateGenres := library.SplitGenres(track.Genre)
		if len(seedGenres) > 0 {
			for genre := range normalizedGenreSet(candidateGenres) {
				if _, ok := seedGenres[genre]; ok {
					score += 10
				}
			}
		}

		// Artist match: +5. Case-insensitive to tolerate inconsistent
		// capitalization between releases of the same artist.
		if track.Artist != "" && strings.EqualFold(track.Artist, input.Seed.Artist) {
			score += 5
		}

		// Album match: +3. Case-insensitive for the same reason as Artist.
		if track.Album != "" && strings.EqualFold(track.Album, input.Seed.Album) {
			score += 3
		}

		// Shared tags: +8 per shared tag.
		if trackTags, ok := input.TrackTags[track.Hash]; ok {
			for tagID := range input.Seed.TagIDs {
				if _, has := trackTags[tagID]; has {
					score += 8
				}
			}
		}

		// Favorited: +3.
		if _, ok := input.FavoriteHashes[track.Hash]; ok {
			score += 3
		}

		// Duration within ±30% of seed: +1.
		if input.Seed.Duration > 0 && track.Duration > 0 {
			ratio := track.Duration / input.Seed.Duration
			if ratio >= 0.7 && ratio <= 1.3 {
				score += 1
			}
		}

		// Time-of-day genre affinity: variable bonus per matching genre.
		if timeGenreBonus != nil {
			for _, cg := range candidateGenres {
				if bonus, ok := timeGenreBonus[normalizedGenreKey(cg)]; ok {
					score += bonus
				}
			}
		}

		// Completion rate: +4 for high listeners, -3 for skippers.
		if input.CompletionRates != nil {
			if rate, ok := input.CompletionRates[track.Hash]; ok {
				if rate >= 0.8 {
					score += 4
				}
				if rate <= 0.3 {
					score -= 3
				}
			}
		}

		// Session co-occurrence with seed: +6 when played together 2+ times.
		if input.CoOccurrences != nil {
			if count, ok := input.CoOccurrences[track.Hash]; ok && count >= 2 {
				score += 6
			}
		}

		// Recent history penalty: -50.
		if _, ok := input.RecentHashes[track.Hash]; ok {
			score -= 50
		}

		// Queue penalty: -100.
		if _, ok := input.QueueHashes[track.Hash]; ok {
			score -= 100
		}

		// Dismissed penalty: -100.
		if _, ok := input.DismissedHashes[track.Hash]; ok {
			score -= 100
		}

		results = append(results, TrackScore{Track: track, Score: score})
	}

	// Stable sort so candidates with identical scores keep input order — this
	// makes the top-K pool (and any downstream PickBatch sampling) reproducible
	// for a given AllTracks ordering and avoids subtle non-determinism between
	// runs from Go's unstable quicksort tie-breaking.
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results
}

// PickBatch selects up to n tracks from the top min(3*n, len(scored)) scored
// candidates using weighted random selection without replacement.
// maxArtistFraction (e.g. 0.4) caps the proportion of tracks from any single artist.
// If fewer than n candidates are available, all available tracks are returned.
func PickBatch(scored []TrackScore, n int, maxArtistFraction float64) []library.Track {
	if n <= 0 || len(scored) == 0 {
		return nil
	}

	poolSize := min(3*n, len(scored))
	pool := make([]TrackScore, poolSize)
	copy(pool, scored[:poolSize])
	nextIdx := poolSize

	maxPerArtist := int(math.Ceil(float64(n) * maxArtistFraction))
	if maxPerArtist < 1 {
		maxPerArtist = 1
	}

	artistCount := make(map[string]int)
	result := make([]library.Track, 0, n)

	for len(result) < n {
		eligible, weights, totalWeight, anyPositive := collectEligible(pool, artistCount, maxPerArtist)
		for len(eligible) == 0 && nextIdx < len(scored) {
			pool = append(pool, scored[nextIdx])
			nextIdx++
			eligible, weights, totalWeight, anyPositive = collectEligible(pool, artistCount, maxPerArtist)
		}
		if len(eligible) == 0 {
			break
		}

		// Deterministic fallback: when no eligible candidate has a positive
		// score, every weight is the 0.1 floor and the weighted-random draw
		// is uniform — defeating the purpose of scoring. Pick the first
		// eligible (highest-scored due to ScoreAll's descending sort).
		var pickedIdx int
		if !anyPositive || totalWeight <= 0 {
			pickedIdx = eligible[0]
		} else {
			maxWeight := new(big.Int).SetUint64(1 << 53)
			randomWeight, err := cryptorand.Int(cryptorand.Reader, maxWeight)
			if err != nil {
				pickedIdx = eligible[0]
			} else {
				r := (float64(randomWeight.Uint64()) / float64(1<<53)) * totalWeight
				pickedIdx = eligible[len(eligible)-1] // fallback to last eligible
				cumulative := 0.0
				for j, i := range eligible {
					cumulative += weights[j]
					if r < cumulative {
						pickedIdx = i
						break
					}
				}
			}
		}

		picked := pool[pickedIdx]
		result = append(result, picked.Track)
		if key := normalizedArtistKey(picked.Track.Artist); key != "" {
			artistCount[key]++
		}

		// Remove picked from pool.
		pool = append(pool[:pickedIdx], pool[pickedIdx+1:]...)
	}

	return result
}

func normalizedGenreSet(genres []string) map[string]struct{} {
	if len(genres) == 0 {
		return nil
	}

	normalized := make(map[string]struct{}, len(genres))
	for _, genre := range genres {
		key := normalizedGenreKey(genre)
		if key == "" {
			continue
		}
		normalized[key] = struct{}{}
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func normalizedGenreBonusMap(bonuses map[string]float64) map[string]float64 {
	if len(bonuses) == 0 {
		return nil
	}

	normalized := make(map[string]float64, len(bonuses))
	for genre, bonus := range bonuses {
		key := normalizedGenreKey(genre)
		if key == "" {
			continue
		}
		normalized[key] += bonus
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func normalizedGenreKey(genre string) string {
	return strings.ToLower(strings.TrimSpace(genre))
}

func collectEligible(pool []TrackScore, artistCount map[string]int, maxPerArtist int) ([]int, []float64, float64, bool) {
	eligible := make([]int, 0, len(pool))
	weights := make([]float64, 0, len(pool))
	totalWeight := 0.0
	anyPositive := false
	for i, ts := range pool {
		artistKey := normalizedArtistKey(ts.Track.Artist)
		if artistKey != "" && artistCount[artistKey] >= maxPerArtist {
			continue
		}
		weight := math.Max(ts.Score, 0.1)
		eligible = append(eligible, i)
		weights = append(weights, weight)
		totalWeight += weight
		if ts.Score > 0.1 {
			anyPositive = true
		}
	}
	return eligible, weights, totalWeight, anyPositive
}

func normalizedArtistKey(artist string) string {
	return strings.ToLower(strings.TrimSpace(artist))
}
