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

// View renders the music library panel with proportional columnar layout.
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
// Compile-time assertion: Styles must remain a value type on View.
// This line fails to compile if View.Styles is ever changed to *styles.Styles.
var _ styles.Styles = View{}.Styles

type View struct {
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
}

// tracks returns the override tracks if set, otherwise the index tracks.
func (v View) tracks() []musiclib.Track {
	if v.OverrideTracks != nil {
		return v.OverrideTracks
	}
	return v.Index.Tracks()
}

// filteredMatches returns the current fuzzy-match slice and whether a filter
// searcher is available. The bool is true when FilteredMatches is populated or
// Fuzzy is non-nil (i.e. a filter can be applied). When the bool is false the
// caller should fall back to the full unfiltered track list.
func (v View) filteredMatches(allTracks []musiclib.Track) ([]musiclib.Match, bool) {
	if v.FilteredMatches != nil {
		return v.FilteredMatches, true
	}
	if v.Fuzzy != nil {
		return v.Fuzzy.Search(v.Filter.Text, allTracks), true
	}
	return nil, false
}

// effectiveTrackCount returns the number of tracks currently displayed,
// accounting for any active fuzzy filter.
func (v View) effectiveTrackCount() int {
	if v.Filter.Text != "" {
		if m, ok := v.filteredMatches(v.tracks()); ok {
			return len(m)
		}
	}
	return len(v.tracks())
}

// FocusedTrack returns the track currently under the cursor, accounting for
// any active fuzzy filter. The second return value is false when the library
// is empty or the cursor is out of range.
func (v View) FocusedTrack() (musiclib.Track, bool) {
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
