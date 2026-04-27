package radio

import (
	"math"
	"math/rand/v2"
	"sort"

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
	Seed           SeedTrack
	AllTracks      []library.Track
	TrackTags      map[string]map[int64]struct{}
	FavoriteHashes map[string]struct{}
	RecentHashes   map[string]struct{}
	QueueHashes    map[string]struct{}
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

	for _, track := range input.AllTracks {
		if track.Hash == input.Seed.Hash {
			continue
		}

		score := 0.0

		// Genre matching: +10 per shared genre.
		candidateGenres := library.SplitGenres(track.Genre)
		for _, sg := range input.Seed.Genres {
			for _, cg := range candidateGenres {
				if sg == cg {
					score += 10
				}
			}
		}

		// Artist match: +5.
		if track.Artist != "" && track.Artist == input.Seed.Artist {
			score += 5
		}

		// Album match: +3.
		if track.Album != "" && track.Album == input.Seed.Album {
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

		// Recent history penalty: -50.
		if _, ok := input.RecentHashes[track.Hash]; ok {
			score -= 50
		}

		// Queue penalty: -100.
		if _, ok := input.QueueHashes[track.Hash]; ok {
			score -= 100
		}

		results = append(results, TrackScore{Track: track, Score: score})
	}

	sort.Slice(results, func(i, j int) bool {
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

	maxPerArtist := int(math.Ceil(float64(n) * maxArtistFraction))
	if maxPerArtist < 1 {
		maxPerArtist = 1
	}

	artistCount := make(map[string]int)
	result := make([]library.Track, 0, n)

	for len(result) < n && len(pool) > 0 {
		// Collect eligible indices (not at artist cap) and total weight.
		eligible := make([]int, 0, len(pool))
		totalWeight := 0.0
		for i, ts := range pool {
			if artistCount[ts.Track.Artist] < maxPerArtist {
				eligible = append(eligible, i)
				totalWeight += math.Max(ts.Score, 0.1)
			}
		}

		if len(eligible) == 0 {
			break
		}

		// Weighted random pick.
		r := rand.Float64() * totalWeight
		pickedIdx := eligible[len(eligible)-1] // fallback to last eligible
		cumulative := 0.0
		for _, i := range eligible {
			cumulative += math.Max(pool[i].Score, 0.1)
			if r < cumulative {
				pickedIdx = i
				break
			}
		}

		picked := pool[pickedIdx]
		result = append(result, picked.Track)
		artistCount[picked.Track.Artist]++

		// Remove picked from pool.
		pool = append(pool[:pickedIdx], pool[pickedIdx+1:]...)
	}

	return result
}
