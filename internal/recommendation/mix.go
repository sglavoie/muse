package recommendation

// MixID identifies a recommendation mix type.
type MixID int

const (
	MixRediscover  MixID = iota // play_count >= minPlays, not heard in ageDays days
	MixHiddenGems               // play_count <= maxPlays, artist/genre affinity
	MixOnRepeat                 // minPlays+ plays in last windowDays days
	MixDeepCuts                 // play_count minPlays–maxPlays, high completion rate
	MixNeverPlayed              // never played, ranked by affinity
)

// MixInfo describes a recommendation mix shown at the list level of MixesView.
type MixInfo struct {
	ID          MixID
	Name        string
	Description string
	TrackCount  int
	Progress    string // shown instead of generic message when TrackCount == 0
}

// CacheEntry is a single pre-computed mix membership row for
// recommendation_cache.
type CacheEntry struct {
	MixID     int
	TrackHash string
	Score     float64
}

// Registry returns the fixed set of recommendation mixes in display order.
func Registry() []MixInfo {
	return []MixInfo{
		{ID: MixRediscover, Name: "Rediscover", Description: "Tracks you loved but haven't played in 90+ days"},
		{ID: MixHiddenGems, Name: "Hidden Gems", Description: "Low-played tracks by your favorite artists"},
		{ID: MixOnRepeat, Name: "On Repeat", Description: "Your most played tracks lately"},
		{ID: MixDeepCuts, Name: "Deep Cuts", Description: "Tracks you always listen to completion"},
		{ID: MixNeverPlayed, Name: "Never Played", Description: "Tracks you might like but haven't tried"},
	}
}
