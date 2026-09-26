package library

import (
	musiclib "github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/sorting"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// LibraryMode identifies the sub-mode of the Library view.
type LibraryMode uint8

const (
	LibraryModeAll     LibraryMode = iota // standard Library (all tracks, default sort)
	LibraryModeRecent                     // Recently Added (sorted by date added)
	LibraryModeHistory                    // History (sorted by last played)
)

// libraryModeSnapshot saves per-mode cursor/scroll/filter state for restoration
// when the user cycles back to a previously visited mode.
type libraryModeSnapshot struct {
	cursor int
	scroll int
	filter filter.State
}

// View renders the music library panel backed by a sorted LibraryIndex.
type View struct {
	Index           musiclib.LibraryIndex
	SourceTracks    []musiclib.Track
	Width           int
	Height          int
	Cursor          int // zero-based index of the focused track row
	ScrollOffset    int // first visible track index
	Filter          filter.State
	FilteredMatches []musiclib.Match // cached result of the last fuzzy search
	Fuzzy           musiclib.FuzzySearcher
	SelectedHashes  map[string]struct{} // bulk-selected track hashes
	SortState       sorting.SortState
	SortedTracks    []musiclib.Track // cached sorted copy; nil when SortDefault
	ShowDateAdded   bool
	ShowPlayedAt    bool
	TagFilterHashes map[string]struct{} // non-nil when a cross-view tag filter is active
	Mode            LibraryMode
	Label           string                 // overrides the strip's view label when non-empty (an embedded list's own name)
	modeState       [3]libraryModeSnapshot // saved cursor/scroll/filter per mode
}

func (v *View) SetSize(w, h int) { v.Width = w; v.Height = h }

// VisibleReserve returns the number of rows the renderer reserves above the
// track list (state strip, filter bar), which callers subtract from the view
// height to get the number of visible track rows.
func VisibleReserve(v View) int {
	if v.Width == 0 {
		if v.Filter.Active || v.Filter.Text != "" {
			return 1
		}
		return 0
	}
	return 2
}

// Render returns a string representation of the library panel with proportional
// columnar layout. Column widths resize with the terminal width.
func (v View) Render(s styles.Styles, favHashes, favArtists, favAlbums map[string]struct{}, nowPlayingHash, nowPlayingIcon string) string {
	lv := v.Panel()
	lv.Styles = s
	lv.FavoriteHashes = favHashes
	lv.FavoriteArtists = favArtists
	lv.FavoriteAlbums = favAlbums
	lv.NowPlayingHash = nowPlayingHash
	lv.NowPlayingIcon = nowPlayingIcon
	return lv.Render()
}
