package tui

import (
	"github.com/sglavoie/muse/internal/tui/buckets"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

// QueueEntryBucket returns the bucket label for a QueueEntry under the given
// sort field. Returns the empty string when the field has no bucketing.
func QueueEntryBucket(field sorting.SortField, e QueueEntry) string {
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
