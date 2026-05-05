package favorites

import (
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// Level identifies the active navigation level inside the favorites panel.
type Level int

const (
	CategoryLevel Level = iota // showing the three category rows
	ListLevel                  // showing favorites in selected category
	TrackLevel                 // showing tracks for a favorite artist/album drill-down
)

// Category identifies which type of favorite is selected.
type Category int

const (
	CategoryArtists Category = iota
	CategoryAlbums
	CategoryTracks
)

// Compile-time assertion: Styles must remain a value type on View.
var _ styles.Styles = View{}.Styles

// View is the pure render struct for the favorites panel with 3-level drill-down:
// Categories -> List -> Tracks. All fields are set by the caller before each
// Render call.
type View struct {
	Level        Level
	Cursor       int
	ScrollOffset int

	Category Category

	// Category counts (shown at CategoryLevel).
	ArtistCount int
	AlbumCount  int
	TrackCount  int

	// List items (populated at ListLevel).
	Artists []string
	Albums  []string
	Tracks  []library.Track

	// Drill-down tracks (populated at TrackLevel from a favorite artist/album).
	DrilldownTracks []library.Track
	DrilldownName   string // heading text for TrackLevel (artist or album name)

	// Saved cursors/scroll for each level (restored on Back).
	CategoryCursor   int
	ListCursor       int
	ListScrollOffset int

	Width  int
	Height int
	Styles styles.Styles

	FilterActive bool
	FilterText   string
	FilterView   string // pre-rendered textinput view for active filter display

	SortIndicator string

	// SelectedItems tracks bulk-selected items at ListLevel/TrackLevel.
	SelectedItems   map[int]struct{}
	FavoriteHashes  map[string]struct{}
	FavoriteArtists map[string]struct{}
	FavoriteAlbums  map[string]struct{}
	NowPlayingHash  string // hash of the currently playing track; empty when nothing is playing
}
