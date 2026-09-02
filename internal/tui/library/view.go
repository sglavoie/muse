package library

import (
	musiclib "github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// QueueAppender appends a track to the persisted play queue. Implementations
// must be safe to call from a tea.Cmd goroutine (i.e., they must not hold the
// main-loop lock). The DB single-writer goroutine satisfies this contract.
// AppendTrack returns the new queue row ID assigned by the database.
// sourcePlaylist identifies the playlist the track was enqueued from; pass ""
// for tracks enqueued individually.
type QueueAppender interface {
	AppendTrack(track musiclib.Track, sourcePlaylist string) (int64, error)
}

// TrackEnqueuedMsg is returned by the tea.Cmd created by EnqueueFocused when
// the DB write completes. The root model handles it by appending the entry to
// the queue view and, when WasEmpty is true and playback is idle, triggering
// immediate playback from index 0.
type TrackEnqueuedMsg struct {
	Track          musiclib.Track
	QueueID        int64 // DB-assigned row ID; needed by the player for PlayQueueEntry
	Err            error
	WasEmpty       bool   // true when the queue was empty before this enqueue
	SourcePlaylist string // playlist name this track was enqueued from; empty for library enqueues
	InsertAfter    int    // -1 = append (default enqueue), >= 0 = insert at this index ("play next")
}

// Panel renders the music library panel with proportional columnar layout.
// Width and Height must be set from the most recent tea.WindowSizeMsg before
// calling Render; a zero Width falls back to a simple "artist – title" format.
// Cursor holds the zero-based index of the focused track row.
// Filter holds the active/text state of the filter bar.
// FilteredMatches is the pre-computed result of the last fuzzy search; callers
// should populate this whenever Filter.Text changes so that navigation and
// rendering reuse the cached slice without re-running the search on every frame.
// Fuzzy is the fuzzy search engine used when the filter bar is active;
// inject musiclib.FakeFuzzySearcher in tests and musiclib.SahilmFuzzySearcher in production.
// SelectedHashes holds the set of track hashes that are bulk-selected.
//
// Compile-time assertion: Styles must remain a value type on Panel.
// This line fails to compile if Panel.Styles is ever changed to *styles.Styles.
var _ styles.Styles = Panel{}.Styles

type Panel struct {
	Index           musiclib.LibraryIndex
	Width           int
	Height          int
	Cursor          int
	ScrollOffset    int // first visible track index
	Filter          filter.State
	FilteredMatches []musiclib.Match // pre-computed; nil means not yet cached
	Fuzzy           musiclib.FuzzySearcher
	SelectedHashes  map[string]struct{}
	FavoriteHashes  map[string]struct{}
	FavoriteArtists map[string]struct{}
	FavoriteAlbums  map[string]struct{}
	Styles          styles.Styles
	OverrideTracks  []musiclib.Track // sorted tracks override; nil uses Index
	SortField       int              // active sort field (0 = default/none)
	SortDir         int              // 0 = asc, 1 = desc
	ShowDateAdded   bool
	ShowPlayedAt    bool
	NowPlayingHash  string // hash of the currently playing track; empty when nothing is playing
	NowPlayingIcon  string // marker glyph (e.g. "▶", "⏸", "■"); empty falls back to "♪"
	// BucketLabel returns a chunk label for a given track based on the active
	// sort. When nil or always returning "", no dividers/bucket headers are
	// emitted. Used to chunk long lists into Today/This Week/A/B/… groups.
	BucketLabel func(musiclib.Track) string
	// StripViewLabel is the left-most label in the unified state strip
	// (e.g. "Library", "Recent", "History"). The outer wrapper sets this
	// based on the current LibraryMode.
	StripViewLabel string
	// StripSortLabel and StripSortIndicator carry the human-readable sort
	// field name (e.g. "Artist") and direction arrow ("▲" / "▼"). Both are
	// empty when no sort is applied.
	StripSortLabel     string
	StripSortIndicator string
}

// tracks returns the override tracks if set, otherwise the index tracks.
func (v Panel) tracks() []musiclib.Track {
	if v.OverrideTracks != nil {
		return v.OverrideTracks
	}
	return v.Index.Tracks()
}

// filteredMatches returns the current fuzzy-match slice and whether a filter
// searcher is available. The bool is true when FilteredMatches is populated or
// Fuzzy is non-nil (i.e. a filter can be applied). When the bool is false the
// caller should fall back to the full unfiltered track list.
func (v Panel) filteredMatches(allTracks []musiclib.Track) ([]musiclib.Match, bool) {
	if v.FilteredMatches != nil {
		return validMatches(v.FilteredMatches, len(allTracks)), true
	}
	if v.Fuzzy != nil {
		return validMatches(v.Fuzzy.Search(v.Filter.Text, allTracks), len(allTracks)), true
	}
	return nil, false
}

func validMatches(matches []musiclib.Match, trackCount int) []musiclib.Match {
	valid := make([]musiclib.Match, 0, len(matches))
	for _, match := range matches {
		if match.TrackIndex < 0 || match.TrackIndex >= trackCount {
			continue
		}
		valid = append(valid, match)
	}
	return valid
}

// effectiveTrackCount returns the number of tracks currently displayed,
// accounting for any active fuzzy filter.
func (v Panel) effectiveTrackCount() int {
	if v.Filter.Text != "" {
		if m, ok := v.filteredMatches(v.tracks()); ok {
			return len(m)
		}
	}
	return len(v.tracks())
}

// VisibleTracks returns every row the view is showing, in display order, with
// the active sort and fuzzy filter applied. It walks matches exactly as
// FocusedTrack does so the cursor and the full list can never disagree about
// what row N is.
func (v Panel) VisibleTracks() []musiclib.Track {
	allTracks := v.tracks()
	if v.Filter.Text == "" {
		return allTracks
	}
	matches, ok := v.filteredMatches(allTracks)
	if !ok {
		return allTracks
	}
	out := make([]musiclib.Track, 0, len(matches))
	for _, match := range matches {
		out = append(out, allTracks[match.TrackIndex])
	}
	return out
}

// FocusedTrack returns the track currently under the cursor, accounting for
// any active fuzzy filter. The second return value is false when the library
// is empty or the cursor is out of range.
func (v Panel) FocusedTrack() (musiclib.Track, bool) {
	allTracks := v.tracks()
	if v.Filter.Text != "" {
		if matches, ok := v.filteredMatches(allTracks); ok {
			if v.Cursor < 0 || v.Cursor >= len(matches) {
				return musiclib.Track{}, false
			}
			return allTracks[matches[v.Cursor].TrackIndex], true
		}
	}
	if v.Cursor < 0 || v.Cursor >= len(allTracks) {
		return musiclib.Track{}, false
	}
	return allTracks[v.Cursor], true
}
