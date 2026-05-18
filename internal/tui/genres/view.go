package genres

import (
	"github.com/sglavoie/muse/internal/library"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// Level identifies the active navigation level inside the genres panel.
type Level int

const (
	GenreLevel  Level = iota // showing the flat list of genres
	ArtistLevel              // showing artists for a selected genre
	TrackLevel               // showing tracks for a selected artist
)

const (
	UnknownGenre = "(unknown genre)"
)

// Compile-time assertion: Styles must remain a value type on View.
var _ styles.Styles = View{}.Styles

// View is the pure render struct for the genres panel with 3-level drill-down:
// Genres -> Artists -> Tracks. All fields are set by the caller before each
// Render call.
type View struct {
	Level        Level
	Cursor       int
	ScrollOffset int

	Genres  []string
	Artists []string
	Tracks  []library.Track

	// GenreCounts maps each genre name to its track count for the right-aligned
	// count column at GenreLevel. Optional — when nil the count column is omitted.
	GenreCounts map[string]int

	SelectedGenre  string
	SelectedArtist string

	// Saved cursors/scroll for each level (restored on Back).
	GenreCursor        int
	GenreScrollOffset  int
	ArtistCursor       int
	ArtistScrollOffset int

	Width  int
	Height int
	Styles styles.Styles

	FilterActive bool
	FilterText   string
	FilterView   string // pre-rendered textinput view for active filter display
	FilterHint   string // shown instead of cursor when filter is active and empty

	StripViewLabel     string // label rendered in the state strip; defaults to "Genres"
	StripSortLabel     string // sort field label for the strip; empty when sort inactive
	StripSortIndicator string // sort direction arrow for the strip; "▲" or "▼"

	FavoriteHashes  map[string]struct{}
	FavoriteArtists map[string]struct{}
	FavoriteAlbums  map[string]struct{}
	SelectedHashes  map[string]struct{}
	NowPlayingHash  string // hash of the currently playing track; empty when nothing is playing
	NowPlayingIcon  string // marker glyph (e.g. "▶", "⏸", "■"); empty falls back to "♪"
}

// RawGenre converts the unknown sentinel back to an empty string for matching
// against raw track metadata. All other values pass through.
func RawGenre(s string) string {
	if s == UnknownGenre {
		return ""
	}
	return s
}

// EffectiveGenres returns the genre list filtered by FilterText.
func (v View) EffectiveGenres() []string { return arttui.FilterStrings(v.Genres, v.FilterText) }

// EffectiveArtists returns the artist list filtered by FilterText.
func (v View) EffectiveArtists() []string { return arttui.FilterStrings(v.Artists, v.FilterText) }

// EffectiveTracks returns the track list filtered by FilterText.
func (v View) EffectiveTracks() []library.Track { return arttui.FilterTracks(v.Tracks, v.FilterText) }

// ---------- Rendering ----------

// Render returns the string representation of the genres panel.
func (v View) Render() string {
	switch v.Level {
	case ArtistLevel:
		return v.renderArtists()
	case TrackLevel:
		return v.renderTracks()
	default:
		return v.renderGenres()
	}
}
