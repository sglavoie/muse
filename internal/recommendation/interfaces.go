package recommendation

import "github.com/sglavoie/muse/internal/library"

// RecommendationStore is the subset of *db.DB methods used by mix loading.
// The compile-time check in internal/db ensures *db.DB satisfies this interface.
type RecommendationStore interface {
	ReadRediscoverMix(minPlays, ageDays, limit int) ([]library.Track, error)
	ReadHiddenGemsMix(maxPlays, topArtists, topGenres, limit int) ([]library.Track, error)
	ReadOnRepeatMix(minPlays, windowDays, limit int) ([]library.Track, error)
	ReadDeepCutsMix(minPlays, maxPlays int, minCompletion float64, limit int) ([]library.Track, error)
	ReadNeverPlayedMix(limit int) ([]library.Track, error)
	ReadCachedMix(mixID, limit int) ([]library.Track, error)
}

// CacheWriter pre-computes mix memberships into recommendation_cache.
type CacheWriter interface {
	RebuildRecommendationCache(entries []CacheEntry) error
}
