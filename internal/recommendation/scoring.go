package recommendation

import "github.com/sglavoie/muse/internal/library"

// FilterSeen removes tracks whose hash appears in seen from the input slice.
// Returns the original slice unmodified when seen is empty.
func FilterSeen(tracks []library.Track, seen map[string]struct{}) []library.Track {
	if len(tracks) == 0 {
		return tracks
	}
	if len(seen) == 0 {
		return tracks
	}
	result := make([]library.Track, 0, len(tracks))
	for _, t := range tracks {
		if _, ok := seen[t.Hash]; !ok {
			result = append(result, t)
		}
	}
	return result
}
