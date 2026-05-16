package artists

import (
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// Level identifies the active navigation level inside the artists panel.
type Level int

const (
	ArtistLevel Level = iota // showing the flat list of artists
	AlbumLevel               // showing albums for a selected artist
	TrackLevel               // showing tracks for a selected album
)

const (
	UnknownArtist = "(unknown artist)"
	UnknownAlbum  = "(unknown album)"
)

// RawName converts display sentinels back to an empty string for matching
// against raw track metadata. All other values pass through.
func RawName(s string) string {
	if s == UnknownArtist || s == UnknownAlbum {
		return ""
	}
	return s
}

// Compile-time assertion: Styles must remain a value type on View.
// This line fails to compile if View.Styles is ever changed to *styles.Styles.
var _ styles.Styles = View{}.Styles

// View is the pure render struct for the artists panel with 3-level drill-down:
// Artists -> Albums -> Tracks. All fields are set by the caller before each
// Render call.
type View struct {
	Level        Level
	Cursor       int
	ScrollOffset int

	Artists []string
	Albums  []string
	Tracks  []library.Track

	SelectedArtist string
	SelectedAlbum  string

	// Saved cursors/scroll for each level (restored on Back).
	ArtistCursor       int
	ArtistScrollOffset int
	AlbumCursor        int
	AlbumScrollOffset  int

	Width  int
	Height int
	Styles styles.Styles

	FilterActive bool
	FilterText   string
	FilterView   string // pre-rendered textinput view for active filter display
	FilterHint   string // shown instead of cursor when filter is active and empty

	SortIndicator   string // e.g. "Sort: Name ▲" — shown when non-empty
	FavoriteHashes  map[string]struct{}
	FavoriteArtists map[string]struct{}
	FavoriteAlbums  map[string]struct{}
	SelectedHashes  map[string]struct{}
	NowPlayingHash  string // hash of the currently playing track; empty when nothing is playing
	NowPlayingIcon  string // marker glyph (e.g. "▶", "⏸", "■"); empty falls back to "♪"
}

// Render returns the string representation of the artists panel.
func (v View) Render() string {
	switch v.Level {
	case AlbumLevel:
		return v.renderAlbums()
	case TrackLevel:
		return v.renderTracks()
	default:
		return v.renderArtists()
	}
}
