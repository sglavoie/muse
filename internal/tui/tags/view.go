package tags

import (
	"strings"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/strip"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// Level identifies the active navigation level inside the tags panel.
type Level int

const (
	TagLevel   Level = iota // showing the flat list of tags
	TrackLevel              // showing tracks for a selected tag
)

// Compile-time assertion: Styles must remain a value type on View.
var _ styles.Styles = View{}.Styles

// View is the pure render struct for the tags panel with 2-level drill-down:
// Tags -> Tracks. All fields are set by the caller before each Render call.
type View struct {
	Level        Level
	Cursor       int
	ScrollOffset int

	Tags            []db.TagRow
	TagCounts       map[int64]int // tag ID → track count
	Tracks          []db.PlaylistTrackRow
	SelectedTag     db.TagRow
	TagCursor       int // saved tag cursor for restoring on Back
	TagScrollOffset int // saved tag scroll offset for restoring on Back

	Width  int
	Height int
	Styles styles.Styles

	FilterActive bool
	FilterText   string
	FilterView   string // pre-rendered textinput view for active filter display

	SortIndicator  string // e.g. "Sort: Name ▲"
	FavoriteHashes map[string]struct{}
	SelectedHashes map[string]struct{}
	NowPlayingHash string // hash of the currently playing track; empty when nothing is playing
	NowPlayingIcon string // marker glyph (e.g. "▶", "⏸", "■"); empty falls back to "♪"

	// Inline create/rename input state (passed through from TagsView).
	Creating    bool
	CreateInput string // current value of the inline create field
	Renaming    bool
	RenameInput string // current value of the inline rename field
}

// EffectiveTags returns the tag list filtered by FilterText, excluding tags
// with no associated tracks.
func (v View) EffectiveTags() []db.TagRow {
	q := strings.ToLower(v.FilterText)
	var result []db.TagRow
	for _, t := range v.Tags {
		if v.TagCounts != nil && v.TagCounts[t.ID] == 0 {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(t.Name), q) {
			continue
		}
		result = append(result, t)
	}
	return result
}

// EffectiveTracks returns the track list filtered by FilterText.
func (v View) EffectiveTracks() []db.PlaylistTrackRow {
	if v.FilterText == "" {
		return v.Tracks
	}
	fq := library.ParseFilterQuery(v.FilterText)
	var result []db.PlaylistTrackRow
	for _, t := range v.Tracks {
		if fq.MatchesMetadata(t.Title, t.Artist, t.Album, t.AlbumArtist, t.Genre, t.Path) {
			result = append(result, t)
		}
	}
	return result
}

// Render returns the string representation of the tags panel.
func (v View) Render() string {
	if v.Level == TrackLevel {
		return v.renderTracks()
	}
	unfiltered := v
	unfiltered.FilterText = ""
	rows, _ := v.createRowBudget()
	start, end := v.visibleWindow(len(v.EffectiveTags()), rows)
	heading := strip.Collection(v.Width, "Tags", "tag", len(v.EffectiveTags()), len(unfiltered.EffectiveTags()), v.Styles, start, end)
	return heading + "\n" + v.renderTags()
}
