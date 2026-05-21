package recommendation

import "github.com/sglavoie/muse/internal/library"

// FilterSeen removes tracks whose hash appears in seen from the input slice.
// Returns the original slice unmodified when seen is empty.
//
// Tracks with an empty Hash are always preserved: an unhashed library entry
// is not "seen" — it is unidentified — and must not be dropped just because
// the seen map happens to contain an empty-string key (which can occur when a
// dismissed entry was written before its hash was populated).
func FilterSeen(tracks []library.Track, seen map[string]struct{}) []library.Track {
	if tracks == nil {
		return nil
	}
	if len(tracks) == 0 {
		return tracks
	}
	if len(seen) == 0 {
		return tracks
	}
	result := make([]library.Track, 0, len(tracks))
	for _, t := range tracks {
		if t.Hash == "" {
			result = append(result, t)
			continue
		}
		if _, ok := seen[t.Hash]; !ok {
			result = append(result, t)
		}
	}
	return result
}
