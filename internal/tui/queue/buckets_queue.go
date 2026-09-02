package queue

import (
	"github.com/sglavoie/muse/internal/tui/buckets"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

// EntryBucket returns the bucket label for an Entry under the given
// sort field. Returns the empty string when the field has no bucketing.
func EntryBucket(field sorting.SortField, e Entry) string {
	switch field {
	case sorting.SortTitle:
		return buckets.FirstLetter(e.Title)
	case sorting.SortArtist:
		return buckets.FirstLetter(e.Artist)
	case sorting.SortAlbum:
		return buckets.FirstLetter(e.Album)
	case sorting.SortSource:
		return buckets.FirstLetter(e.SourcePlaylist)
	default:
		return ""
	}
}
