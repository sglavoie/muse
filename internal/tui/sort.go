package tui

import (
	"sort"
	"strings"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
)

// SortField identifies which column/attribute to sort by.
type SortField int

const (
	SortDefault    SortField = iota // natural/original order
	SortArtist                      // artist name
	SortTitle                       // track title
	SortAlbum                       // album name
	SortDuration                    // track duration
	SortDateAdded                   // library created-at timestamp
	SortName                        // for string lists (artist names, album names)
	SortTrackCount                  // for playlist list level
	SortSource                      // for queue (source playlist)
	SortLastPlayed                  // for history tab (played_at timestamp)
)

// SortDir is the sort direction.
type SortDir int

const (
	SortAsc  SortDir = iota // ascending (A→Z, short→long)
	SortDesc                // descending (Z→A, long→short)
)

// SortState holds the current sort field and direction.
type SortState struct {
	Field SortField
	Dir   SortDir
}

// Sort field cycles per view+level.
var (
	librarySortFields       = []SortField{SortDefault, SortArtist, SortTitle, SortAlbum, SortDuration}
	recentlyAddedSortFields = []SortField{SortDateAdded}
	historySortFields       = []SortField{SortLastPlayed}
	queueSortFields         = []SortField{SortDefault, SortTitle, SortArtist, SortAlbum, SortDuration, SortSource}
	playlistListSortFields  = []SortField{SortDefault, SortName, SortTrackCount}
	playlistTrackSortFields = []SortField{SortDefault, SortTitle, SortArtist, SortAlbum, SortDuration}
	artistArtistSortFields  = []SortField{SortDefault, SortName}
	artistAlbumSortFields   = []SortField{SortDefault, SortName}
	artistTrackSortFields   = []SortField{SortDefault, SortTitle, SortDuration}
	albumAlbumSortFields    = []SortField{SortDefault, SortName}
	albumTrackSortFields    = []SortField{SortDefault, SortTitle, SortDuration}
	favListSortFields       = []SortField{SortDefault, SortName}
	favTrackSortFields      = []SortField{SortDefault, SortTitle, SortArtist, SortAlbum, SortDuration}
	genreGenreSortFields    = []SortField{SortDefault, SortName}
	genreArtistSortFields   = []SortField{SortDefault, SortName}
	genreTrackSortFields    = []SortField{SortDefault, SortTitle, SortDuration}
	tagsTrackSortFields     = []SortField{SortDefault, SortTitle, SortArtist, SortAlbum, SortDuration}
)

// CycleSortField advances to the next field in the cycle, wrapping around.
func CycleSortField(current SortField, fields []SortField) SortField {
	for i, f := range fields {
		if f == current {
			return fields[(i+1)%len(fields)]
		}
	}
	// current not in cycle — start from the beginning
	if len(fields) > 0 {
		return fields[0]
	}
	return SortDefault
}

// SortFieldLabel returns a human-readable label for the field.
func SortFieldLabel(f SortField) string {
	switch f {
	case SortDefault:
		return "default"
	case SortArtist:
		return "Artist"
	case SortTitle:
		return "Title"
	case SortAlbum:
		return "Album"
	case SortDuration:
		return "Duration"
	case SortDateAdded:
		return "Date Added"
	case SortName:
		return "Name"
	case SortTrackCount:
		return "Track Count"
	case SortSource:
		return "Source"
	case SortLastPlayed:
		return "Last Played"
	default:
		return "unknown"
	}
}

// SortDirIndicator returns an arrow character for the sort direction.
func SortDirIndicator(dir SortDir) string {
	if dir == SortAsc {
		return "▲"
	}
	return "▼"
}

// isUnknownValue reports whether s is empty or a common unknown sentinel.
func isUnknownValue(s string) bool {
	if s == "" {
		return true
	}
	lower := strings.ToLower(s)
	return lower == "unknown artist" ||
		lower == "(unknown artist)" ||
		lower == "(unknown album)" ||
		lower == "(unknown genre)"
}

// SortTracks returns a sorted copy of tracks by the given field and direction.
// Unknown/empty values sort after known values regardless of direction.
func SortTracks(tracks []library.Track, field SortField, dir SortDir) []library.Track {
	if field == SortDefault || len(tracks) == 0 {
		return nil
	}
	sorted := make([]library.Track, len(tracks))
	copy(sorted, tracks)

	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		switch field {
		case SortArtist:
			return compareStringsUnknownLast(a.Artist, b.Artist, dir)
		case SortTitle:
			return compareStringsUnknownLast(a.Title, b.Title, dir)
		case SortAlbum:
			return compareStringsUnknownLast(a.Album, b.Album, dir)
		case SortDuration:
			return compareDurationsZeroLast(a.Duration, b.Duration, dir)
		case SortDateAdded:
			if dir == SortAsc {
				return a.CreatedAt < b.CreatedAt
			}
			return a.CreatedAt > b.CreatedAt
		case SortLastPlayed:
			if dir == SortAsc {
				return a.LastPlayedAt < b.LastPlayedAt
			}
			return a.LastPlayedAt > b.LastPlayedAt
		default:
			return false
		}
	})
	return sorted
}

// SortQueueIndices returns a permutation array where displayOrder[i] is the
// index into entries for display row i. Returns nil when field is SortDefault.
func SortQueueIndices(entries []QueueEntry, field SortField, dir SortDir) []int {
	if field == SortDefault || len(entries) == 0 {
		return nil
	}
	indices := make([]int, len(entries))
	for i := range indices {
		indices[i] = i
	}
	sort.SliceStable(indices, func(a, b int) bool {
		ea, eb := entries[indices[a]], entries[indices[b]]
		switch field {
		case SortTitle:
			return compareStringsUnknownLast(ea.Title, eb.Title, dir)
		case SortArtist:
			return compareStringsUnknownLast(ea.Artist, eb.Artist, dir)
		case SortAlbum:
			return compareStringsUnknownLast(ea.Album, eb.Album, dir)
		case SortDuration:
			return compareDurationMsZeroLast(ea.DurationMs, eb.DurationMs, dir)
		case SortSource:
			return compareStringsUnknownLast(ea.SourcePlaylist, eb.SourcePlaylist, dir)
		default:
			return false
		}
	})
	return indices
}

// SortPlaylists returns a sorted copy of playlists by the given field and direction.
func SortPlaylists(playlists []db.PlaylistRow, field SortField, dir SortDir) []db.PlaylistRow {
	if field == SortDefault || len(playlists) == 0 {
		return nil
	}
	sorted := make([]db.PlaylistRow, len(playlists))
	copy(sorted, playlists)
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		switch field {
		case SortName:
			return compareStringsUnknownLast(a.Name, b.Name, dir)
		case SortTrackCount:
			if dir == SortAsc {
				return a.TrackCount < b.TrackCount
			}
			return a.TrackCount > b.TrackCount
		default:
			return false
		}
	})
	return sorted
}

// SortPlaylistTrackIndices returns a permutation array for playlist tracks,
// where result[i] is the index into tracks for display row i. Returns nil
// when field is SortDefault.
func SortPlaylistTrackIndices(tracks []db.PlaylistTrackRow, field SortField, dir SortDir) []int {
	if field == SortDefault || len(tracks) == 0 {
		return nil
	}
	indices := make([]int, len(tracks))
	for i := range indices {
		indices[i] = i
	}
	sort.SliceStable(indices, func(a, b int) bool {
		ta, tb := tracks[indices[a]], tracks[indices[b]]
		switch field {
		case SortTitle:
			return compareStringsUnknownLast(ta.Title, tb.Title, dir)
		case SortArtist:
			return compareStringsUnknownLast(ta.Artist, tb.Artist, dir)
		case SortAlbum:
			return compareStringsUnknownLast(ta.Album, tb.Album, dir)
		case SortDuration:
			return compareDurationMsZeroLast(ta.DurationMs, tb.DurationMs, dir)
		default:
			return false
		}
	})
	return indices
}

// SortStrings returns a sorted copy of items by the given direction.
// Unknown/empty values sort after known values regardless of direction.
func SortStrings(items []string, dir SortDir) []string {
	if len(items) == 0 {
		return nil
	}
	sorted := make([]string, len(items))
	copy(sorted, items)
	sort.SliceStable(sorted, func(i, j int) bool {
		return compareStringsUnknownLast(sorted[i], sorted[j], dir)
	})
	return sorted
}

// clampCursorToCount constrains cursor and scrollOffset to a list of count
// items. When count is 0 both values reset to 0. Otherwise cursor is clamped
// to [0, count-1] and scrollOffset is pulled down to cursor when it exceeds it.
func clampCursorToCount(cursor, scrollOffset, count int) (int, int) {
	if count == 0 {
		return 0, 0
	}
	if cursor >= count {
		cursor = count - 1
	}
	if scrollOffset > cursor {
		scrollOffset = cursor
	}
	return cursor, scrollOffset
}

// adjustScroll adjusts scrollOffset so that cursor is within the visible window.
func adjustScroll(cursor, scrollOffset, maxRows int) int {
	if cursor < scrollOffset {
		return cursor
	}
	if maxRows > 0 && cursor >= scrollOffset+maxRows {
		return cursor - maxRows + 1
	}
	return scrollOffset
}

// findStringPosition returns the index of name in items, or 0 if not found.
func findStringPosition(items []string, name string) int {
	for i, s := range items {
		if s == name {
			return i
		}
	}
	return 0
}

// findTrackHashPosition returns the index of the track with the given hash, or 0 if not found.
func findTrackHashPosition(tracks []library.Track, hash string) int {
	if hash == "" {
		return 0
	}
	for i, t := range tracks {
		if t.Hash == hash {
			return i
		}
	}
	return 0
}

// findPlaylistPosition returns the index of the playlist with the given ID, or 0 if not found.
func findPlaylistPosition(playlists []db.PlaylistRow, id int64) int {
	for i, p := range playlists {
		if p.ID == id {
			return i
		}
	}
	return 0
}

// compareDurationsZeroLast compares two float64 durations, pushing zero
// (unknown) values to the end regardless of sort direction. This matches
// the behaviour of compareStringsUnknownLast for string fields.
func compareDurationsZeroLast(a, b float64, dir SortDir) bool {
	aZero := a <= 0
	bZero := b <= 0
	if aZero != bZero {
		return bZero // known before unknown, always
	}
	if aZero && bZero {
		return false // stable
	}
	if dir == SortAsc {
		return a < b
	}
	return a > b
}

// compareDurationMsZeroLast compares two int64 durations in milliseconds,
// pushing zero (unknown) values to the end regardless of sort direction.
func compareDurationMsZeroLast(a, b int64, dir SortDir) bool {
	aZero := a <= 0
	bZero := b <= 0
	if aZero != bZero {
		return bZero // known before unknown, always
	}
	if aZero && bZero {
		return false // stable
	}
	if dir == SortAsc {
		return a < b
	}
	return a > b
}

// compareStringsUnknownLast compares two strings, pushing unknown/empty values
// to the end regardless of sort direction.
func compareStringsUnknownLast(a, b string, dir SortDir) bool {
	aUnknown := isUnknownValue(a)
	bUnknown := isUnknownValue(b)
	if aUnknown != bUnknown {
		return bUnknown // known before unknown, always
	}
	if aUnknown && bUnknown {
		return false // stable
	}
	aLower := strings.ToLower(a)
	bLower := strings.ToLower(b)
	if dir == SortAsc {
		return aLower < bLower
	}
	return aLower > bLower
}
