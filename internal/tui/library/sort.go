package library

import (
	"slices"
	"strings"

	musiclib "github.com/sglavoie/muse/internal/library"
)

// SortTracks returns a new slice sorted by artist then title, case-insensitive.
// Tracks with an empty Artist field are treated as "Unknown Artist" and sorted
// to the bottom of the list.
func SortTracks(tracks []musiclib.Track) []musiclib.Track {
	sorted := make([]musiclib.Track, len(tracks))
	copy(sorted, tracks)

	slices.SortStableFunc(sorted, func(a, b musiclib.Track) int {
		aUnknown := a.Artist == ""
		bUnknown := b.Artist == ""

		if aUnknown != bUnknown {
			if aUnknown {
				return 1
			}
			return -1
		}

		artistA := strings.ToLower(a.Artist)
		artistB := strings.ToLower(b.Artist)
		if c := strings.Compare(artistA, artistB); c != 0 {
			return c
		}

		return strings.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title))
	})

	return sorted
}
