package tui

import (
	"time"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/buckets"
)

// TrackBucket returns the bucket label for a Track under the given sort
// field. Returns the empty string when the field has no bucketing
// (Duration / Default / numeric fields).
func TrackBucket(field SortField, t library.Track, now time.Time) string {
	switch field {
	case SortArtist, SortName:
		return buckets.FirstLetter(t.Artist)
	case SortTitle:
		return buckets.FirstLetter(t.Title)
	case SortAlbum:
		return buckets.FirstLetter(t.Album)
	case SortDateAdded:
		return buckets.DateBucket(t.CreatedAt, now)
	case SortLastPlayed:
		return buckets.DateBucket(t.LastPlayedAt, now)
	default:
		return ""
	}
}

// QueueEntryBucket returns the bucket label for a QueueEntry under the given
// sort field. Returns the empty string when the field has no bucketing.
func QueueEntryBucket(field SortField, e QueueEntry) string {
	switch field {
	case SortTitle:
		return buckets.FirstLetter(e.Title)
	case SortArtist:
		return buckets.FirstLetter(e.Artist)
	case SortAlbum:
		return buckets.FirstLetter(e.Album)
	case SortSource:
		return buckets.FirstLetter(e.SourcePlaylist)
	default:
		return ""
	}
}

// SortFieldHasBuckets reports whether the given sort field produces a
// non-empty bucket label.
func SortFieldHasBuckets(field SortField) bool {
	switch field {
	case SortArtist, SortTitle, SortAlbum, SortName, SortSource,
		SortDateAdded, SortLastPlayed:
		return true
	default:
		return false
	}
}
