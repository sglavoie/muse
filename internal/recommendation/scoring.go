package recommendation

import (
	"strings"

	"github.com/sglavoie/muse/internal/library"
)

// FilterSeen removes tracks whose hash appears in seen from the input slice.
// Returns the original slice unmodified when seen is empty.
//
// Hash comparisons are trimmed and case-normalized because track hashes are
// hex fingerprints, but persisted dismissals may come from older or manual
// data. Tracks with an empty Hash are always preserved: an unhashed library
// entry is not "seen" — it is unidentified — and must not be dropped just
// because the seen map happens to contain an empty-string key (which can occur
// when a dismissed entry was written before its hash was populated).
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
	normalizedSeen := make(map[string]struct{}, len(seen))
	for hash := range seen {
		key := normalizeHash(hash)
		if key == "" {
			continue
		}
		normalizedSeen[key] = struct{}{}
	}
	if len(normalizedSeen) == 0 {
		return tracks
	}
	var result []library.Track
	for i, t := range tracks {
		hash := normalizeHash(t.Hash)
		if hash == "" {
			if result != nil {
				result = append(result, t)
			}
			continue
		}
		if _, ok := normalizedSeen[hash]; ok {
			if result == nil {
				result = make([]library.Track, 0, len(tracks)-1)
				result = append(result, tracks[:i]...)
			}
			continue
		}
		if result != nil {
			result = append(result, t)
		}
	}
	if result == nil {
		return tracks
	}
	return result
}

func normalizeHash(hash string) string {
	return strings.ToLower(strings.TrimSpace(hash))
}
