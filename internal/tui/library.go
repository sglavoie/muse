package tui

import (
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/filter"
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

// LibraryView renders the music library panel backed by a sorted LibraryIndex.
type LibraryView struct {
	index           library.LibraryIndex
	sourceTracks    []library.Track
	width           int
	height          int
	cursor          int // zero-based index of the focused track row
	scrollOffset    int // first visible track index
	filter          filter.State
	filteredMatches []library.Match // cached result of the last fuzzy search
	fuzzy           library.FuzzySearcher
	selectedHashes  map[string]struct{} // bulk-selected track hashes
	sortState       SortState
	sortedTracks    []library.Track // cached sorted copy; nil when SortDefault
	showDateAdded   bool
	showPlayedAt    bool
	tagFilterHashes map[string]struct{} // non-nil when a cross-view tag filter is active
	mode            LibraryMode
	modeState       [3]libraryModeSnapshot // saved cursor/scroll/filter per mode
}

func (v *LibraryView) setSize(w, h int) { v.width = w; v.height = h }

// render returns a string representation of the library panel with proportional
// columnar layout. Column widths resize with the terminal width.
func (v LibraryView) render(s styles.Styles, favHashes, favArtists, favAlbums map[string]struct{}, nowPlayingHash, nowPlayingIcon string) string {
	lv := v.view()
	lv.Styles = s
	lv.FavoriteHashes = favHashes
	lv.FavoriteArtists = favArtists
	lv.FavoriteAlbums = favAlbums
	lv.NowPlayingHash = nowPlayingHash
	lv.NowPlayingIcon = nowPlayingIcon
	return lv.Render()
}
