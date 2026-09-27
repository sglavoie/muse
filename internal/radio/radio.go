package radio

import (
	"math"
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
		trackHash := track.Hash
		hasTrackHash := trackHash != ""
		if input.Seed.Hash != "" && trackHash == input.Seed.Hash {
			continue
		}

		score := 0.0

		// Genre matching: +10 per shared logical genre. Matched
		// case-insensitively so tag values like "Rock" and "rock" in
		// real-world libraries are treated as the same genre, and duplicate
		// case variants are only counted once.
		candidateGenres := library.SplitGenres(track.Genre)
		normalizedCandidateGenres := normalizedGenreSet(candidateGenres)
		if len(seedGenres) > 0 {
			for genre := range normalizedCandidateGenres {
				if _, ok := seedGenres[genre]; ok {
					score += 10
				}
			}
		}

		// Artist match: +5. Case-insensitive AND whitespace-trimmed to tolerate
		// inconsistent capitalization and stray leading/trailing whitespace
		// (broken taggers occasionally emit " Artist " or "Artist\t"). Without
		// trimming, the same artist with a stray space would miss the +5 boost
		// while still being treated as the same artist by the PickBatch cap
		// (which uses normalizedArtistKey). An untagged seed ("Unknown
		// Artist" or blank) says nothing about taste, so it earns no bonus
		// for every other untagged file.
		trackArtist := strings.TrimSpace(track.Artist)
		seedArtist := strings.TrimSpace(input.Seed.Artist)
		if !library.IsUnknownArtist(seedArtist) && strings.EqualFold(trackArtist, seedArtist) {
			score += 5
		}

		// Album match: +3. Case-insensitive and whitespace-trimmed for the
		// same reasons as Artist.
		trackAlbum := strings.TrimSpace(track.Album)
		seedAlbum := strings.TrimSpace(input.Seed.Album)
		if trackAlbum != "" && seedAlbum != "" && strings.EqualFold(trackAlbum, seedAlbum) {
			score += 3
		}

		// Shared tags: +8 per shared tag.
		if hasTrackHash {
			if trackTags, ok := input.TrackTags[trackHash]; ok {
				for tagID := range input.Seed.TagIDs {
					if _, has := trackTags[tagID]; has {
						score += 8
					}
				}
			}
		}

		// Favorited: +3.
		if hasTrackHash && hasHash(input.FavoriteHashes, trackHash) {
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
		// Iterate the case-folded candidate genre set so a track tagged with
		// duplicate case variants (e.g. "Rock/rock") doesn't double-count
		// the bonus — library.SplitGenres only dedupes on exact case.
		if timeGenreBonus != nil {
			for cg := range normalizedCandidateGenres {
				if bonus, ok := timeGenreBonus[cg]; ok {
					score += bonus
				}
			}
		}

		// Completion rate: +4 for high listeners, -3 for skippers. Reject
		// values outside the documented 0.0-1.0 range to avoid spurious
		// boosts/penalties from corrupted or contract-violating inputs.
		if hasTrackHash && input.CompletionRates != nil {
			if rate, ok := input.CompletionRates[trackHash]; ok && validCompletionRate(rate) {
				if rate >= 0.8 {
					score += 4
				}
				if rate <= 0.3 {
					score -= 3
				}
			}
		}

		// Session co-occurrence with seed: +6 when played together 2+ times.
		if hasTrackHash && input.CoOccurrences != nil {
			if count, ok := input.CoOccurrences[trackHash]; ok && count >= 2 {
				score += 6
			}
		}

		// Recent history penalty: -50.
		if hasTrackHash && hasHash(input.RecentHashes, trackHash) {
			score -= 50
		}

		// Queue penalty: -100.
		if hasTrackHash && hasHash(input.QueueHashes, trackHash) {
			score -= 100
		}

		// Dismissed penalty: -100.
		if hasTrackHash && hasHash(input.DismissedHashes, trackHash) {
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

func hasHash(hashes map[string]struct{}, hash string) bool {
	_, ok := hashes[hash]
	return ok
}

// PickBatch selects up to n tracks from the top min(3*n, len(scored)) scored
// candidates using weighted random selection without replacement.
// maxArtistFraction (e.g. 0.4) caps the proportion of tracks from any single artist.
// If fewer than n candidates are available, all eligible tracks are returned.
//
// It draws from crypto/rand. PickBatchWith takes an injected RandSource, and
// the planner uses a single picker across both candidate pools so the artist
// cap applies to the whole batch.
func PickBatch(scored []TrackScore, n int, maxArtistFraction float64) []library.Track {
	return PickBatchWith(scored, n, maxArtistFraction, CryptoRand())
}

// PickBatchWith is PickBatch with an injected randomness source.
func PickBatchWith(scored []TrackScore, n int, maxArtistFraction float64, rand RandSource) []library.Track {
	resultCap := n
	if resultCap > len(scored) {
		resultCap = len(scored)
	}
	picked := newPicker(resultCap, maxArtistFraction, rand).pick(scored, n)
	if len(picked) == 0 {
		return nil
	}
	tracks := make([]library.Track, 0, len(picked))
	for _, ts := range picked {
		tracks = append(tracks, ts.Track)
	}
	return tracks
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
		if math.IsNaN(bonus) || math.IsInf(bonus, 0) {
			continue
		}
		if existing, ok := normalized[key]; !ok || bonus > existing {
			normalized[key] = bonus
		}
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func normalizedGenreKey(genre string) string {
	return strings.ToLower(strings.TrimSpace(genre))
}

func validCompletionRate(rate float64) bool {
	return !math.IsNaN(rate) && !math.IsInf(rate, 0) && rate >= 0 && rate <= 1
}
