package sorting

import (
	"cmp"
	"slices"
	"sort"
	"strings"
	"unicode"

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
	LibrarySortFields       = []SortField{SortDefault, SortArtist, SortTitle, SortAlbum, SortDuration}
	RecentlyAddedSortFields = []SortField{SortDateAdded}
	HistorySortFields       = []SortField{SortLastPlayed}
	QueueSortFields         = []SortField{SortDefault, SortTitle, SortArtist, SortAlbum, SortDuration, SortSource}
	PlaylistListSortFields  = []SortField{SortDefault, SortName, SortTrackCount}
	PlaylistTrackSortFields = []SortField{SortDefault, SortTitle, SortArtist, SortAlbum, SortDuration}
	ArtistArtistSortFields  = []SortField{SortDefault, SortName}
	ArtistAlbumSortFields   = []SortField{SortDefault, SortName}
	ArtistTrackSortFields   = []SortField{SortDefault, SortTitle, SortDuration}
	AlbumAlbumSortFields    = []SortField{SortDefault, SortName}
	AlbumTrackSortFields    = []SortField{SortDefault, SortTitle, SortDuration}
	FavListSortFields       = []SortField{SortDefault, SortName}
	FavTrackSortFields      = []SortField{SortDefault, SortTitle, SortArtist, SortAlbum, SortDuration}
	GenreGenreSortFields    = []SortField{SortDefault, SortName}
	GenreArtistSortFields   = []SortField{SortDefault, SortName}
	GenreTrackSortFields    = []SortField{SortDefault, SortTitle, SortDuration}
	TagsTrackSortFields     = []SortField{SortDefault, SortTitle, SortArtist, SortAlbum, SortDuration}
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

// sortFieldLabels is the display name of every sort field. A field missing
// from the table renders as "unknown".
var sortFieldLabels = map[SortField]string{
	SortDefault:    "default",
	SortArtist:     "Artist",
	SortTitle:      "Title",
	SortAlbum:      "Album",
	SortDuration:   "Duration",
	SortDateAdded:  "Date Added",
	SortName:       "Name",
	SortTrackCount: "Track Count",
	SortSource:     "Source",
	SortLastPlayed: "Last Played",
}

// SortFieldLabel returns a human-readable label for the field.
func SortFieldLabel(f SortField) string {
	if label, ok := sortFieldLabels[f]; ok {
		return label
	}
	return "unknown"
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
	return library.CompareFolded(s, "unknown artist") == 0 ||
		library.CompareFolded(s, "(unknown artist)") == 0 ||
		library.CompareFolded(s, "(unknown album)") == 0 ||
		library.CompareFolded(s, "(unknown genre)") == 0
}

// SortTracks returns a sorted copy of tracks by the given field and direction.
// Unknown/empty values sort after known values regardless of direction.
//
// Each track's key is computed once and the sort permutes those small keys
// rather than whole Track values, so a 100k-track library is not case-folded
// and unknown-checked on every one of the sort's O(n log n) comparisons.
func SortTracks(tracks []library.Track, field SortField, dir SortDir) []library.Track {
	if field == SortDefault || len(tracks) == 0 {
		return nil
	}
	keys := make([]trackSortKey, len(tracks))
	for i := range tracks {
		keys[i] = newTrackSortKey(i, &tracks[i], field)
	}
	slices.SortStableFunc(keys, func(a, b trackSortKey) int {
		return compareTrackKeys(a, b, field, dir)
	})
	sorted := make([]library.Track, len(tracks))
	for i, k := range keys {
		sorted[i] = tracks[k.index]
	}
	return sorted
}

// trackSortKey is one track's precomputed sort key. text is the case-folded
// value of a string field: comparing folded strings bytewise orders them
// exactly as library.CompareFolded does, since UTF-8 preserves rune order.
type trackSortKey struct {
	index   int
	unknown bool
	text    string
	num     float64
	stamp   int64
}

func newTrackSortKey(index int, t *library.Track, field SortField) trackSortKey {
	k := trackSortKey{index: index}
	switch field {
	case SortArtist:
		k.text, k.unknown = foldedSortText(t.Artist)
	case SortTitle:
		k.text, k.unknown = foldedSortText(t.Title)
	case SortAlbum:
		k.text, k.unknown = foldedSortText(t.Album)
	case SortDuration:
		k.num = t.Duration
		k.unknown = t.Duration <= 0
	case SortDateAdded:
		k.stamp = t.CreatedAt
	case SortLastPlayed:
		k.stamp = t.LastPlayedAt
	}
	return k
}

// foldedSortText returns s folded the way library.CompareFolded compares it,
// and whether it is an unknown value (see isUnknownValue).
func foldedSortText(s string) (string, bool) {
	folded := strings.Map(unicode.ToLower, s)
	switch folded {
	case "", "unknown artist", "(unknown artist)", "(unknown album)", "(unknown genre)":
		return folded, true
	}
	return folded, false
}

// compareOrdered is cmp.Compare without its NaN ordering, so a NaN duration
// compares equal to everything exactly as it did under CompareNumericZeroLast.
func compareOrdered(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// compareTrackKeys orders two keys by field, matching CompareStringsUnknownLast
// and CompareNumericZeroLast: unknowns last in either direction and equal among
// themselves, so the stable sort keeps their order. Timestamps have no unknown
// rule: an unset timestamp is zero and sorts with the other low values.
func compareTrackKeys(a, b trackSortKey, field SortField, dir SortDir) int {
	if a.unknown != b.unknown {
		if a.unknown {
			return 1
		}
		return -1
	}
	if a.unknown {
		return 0
	}
	var c int
	switch field {
	case SortArtist, SortTitle, SortAlbum:
		c = strings.Compare(a.text, b.text)
	case SortDuration:
		c = compareOrdered(a.num, b.num)
	case SortDateAdded, SortLastPlayed:
		c = cmp.Compare(a.stamp, b.stamp)
	}
	if dir == SortDesc {
		return -c
	}
	return c
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
			return CompareStringsUnknownLast(a.Name, b.Name, dir)
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
			return CompareStringsUnknownLast(ta.Title, tb.Title, dir)
		case SortArtist:
			return CompareStringsUnknownLast(ta.Artist, tb.Artist, dir)
		case SortAlbum:
			return CompareStringsUnknownLast(ta.Album, tb.Album, dir)
		case SortDuration:
			return CompareNumericZeroLast(ta.DurationMs, tb.DurationMs, dir)
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
		a, b := albumSortLabel(sorted[i]), albumSortLabel(sorted[j])
		return CompareStringsUnknownLast(a, b, dir)
	})
	return sorted
}

// ClampCursorToCount constrains cursor and scrollOffset to a list of count
// items. When count is 0 both values reset to 0. Otherwise cursor is clamped
// to [0, count-1] and scrollOffset is pulled down to cursor when it exceeds it.
func ClampCursorToCount(cursor, scrollOffset, count int) (int, int) {
	if count == 0 {
		return 0, 0
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= count {
		cursor = count - 1
	}
	if scrollOffset < 0 {
		scrollOffset = 0
	}
	if scrollOffset > cursor {
		scrollOffset = cursor
	}
	return cursor, scrollOffset
}

// AdjustScroll adjusts scrollOffset so that cursor is within the visible window.
func AdjustScroll(cursor, scrollOffset, maxRows int) int {
	if cursor < scrollOffset {
		return cursor
	}
	if maxRows > 0 && cursor >= scrollOffset+maxRows {
		return cursor - maxRows + 1
	}
	return scrollOffset
}

// FindStringPosition returns the index of name in items, or 0 if not found.
// An empty name returns 0 immediately so that an out-of-bounds cursor (which
// leaves the captured name as "") never accidentally matches an empty string
// inside items (e.g., an "unknown artist" entry rendered as "").
func FindStringPosition(items []string, name string) int {
	if name == "" {
		return 0
	}
	for i, s := range items {
		if s == name {
			return i
		}
	}
	return 0
}

// FindTrackHashPosition returns the index of the track with the given hash, or 0 if not found.
func FindTrackHashPosition(tracks []library.Track, hash string) int {
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

// FindPlaylistPosition returns the index of the playlist with the given ID, or 0 if not found.
func FindPlaylistPosition(playlists []db.PlaylistRow, id int64) int {
	for i, p := range playlists {
		if p.ID == id {
			return i
		}
	}
	return 0
}

// CompareNumericZeroLast compares two ordered numeric values (int64 or
// float64), pushing zero-or-negative (unknown/unset) values to the end
// regardless of sort direction. This matches the behaviour of
// CompareStringsUnknownLast for string fields.
func CompareNumericZeroLast[T int64 | float64](a, b T, dir SortDir) bool {
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

// CompareStringsUnknownLast compares two strings, pushing unknown/empty values
// to the end regardless of sort direction.
func CompareStringsUnknownLast(a, b string, dir SortDir) bool {
	aUnknown := isUnknownValue(a)
	bUnknown := isUnknownValue(b)
	if aUnknown != bUnknown {
		return bUnknown // known before unknown, always
	}
	if aUnknown && bUnknown {
		return false // stable
	}
	comparison := library.CompareFolded(a, b)
	if dir == SortAsc {
		return comparison < 0
	}
	return comparison > 0
}

func albumSortLabel(value string) string {
	if library.AlbumTitle(value) == "" {
		return ""
	}
	return library.AlbumLabel(value)
}
