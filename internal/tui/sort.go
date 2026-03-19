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
	SortName                        // for string lists (artist names, album names)
	SortTrackCount                  // for playlist list level
	SortSource                      // for queue (source playlist)
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
	queueSortFields         = []SortField{SortDefault, SortTitle, SortArtist, SortAlbum, SortDuration, SortSource}
	playlistListSortFields  = []SortField{SortDefault, SortName, SortTrackCount}
	playlistTrackSortFields = []SortField{SortDefault, SortTitle, SortArtist, SortDuration}
	artistArtistSortFields  = []SortField{SortDefault, SortName}
	artistAlbumSortFields   = []SortField{SortDefault, SortName}
	artistTrackSortFields   = []SortField{SortDefault, SortTitle, SortDuration}
	albumAlbumSortFields    = []SortField{SortDefault, SortName}
	albumTrackSortFields    = []SortField{SortDefault, SortTitle, SortDuration}
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
	case SortName:
		return "Name"
	case SortTrackCount:
		return "Track Count"
	case SortSource:
		return "Source"
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
	return s == "" || strings.EqualFold(s, "Unknown Artist")
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
			if dir == SortAsc {
				return a.Duration < b.Duration
			}
			return a.Duration > b.Duration
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
			if dir == SortAsc {
				return ea.DurationMs < eb.DurationMs
			}
			return ea.DurationMs > eb.DurationMs
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
		case SortDuration:
			if dir == SortAsc {
				return ta.DurationMs < tb.DurationMs
			}
			return ta.DurationMs > tb.DurationMs
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
