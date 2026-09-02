package queue

import (
	"charm.land/lipgloss/v2"

	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/sorting"
	"github.com/sglavoie/muse/internal/tui/strip"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// renderStateStrip builds the unified single-line state strip carrying
// the view label, sort, filter, and counts for the queue. total is the
// full entry count before filtering; filtered is the count after filter
// is applied (equal to total when no filter is active).
func (v View) renderStateStrip(s styles.Styles, total, filtered int) string {
	var sortChunk strip.SortChunk
	if v.SortState.Field != sorting.SortDefault {
		sortChunk = strip.SortChunk{
			Label:     sorting.SortFieldLabel(v.SortState.Field),
			Indicator: sorting.SortDirIndicator(v.SortState.Dir),
		}
	}
	f := strip.FilterChunk{Active: v.Filter.Active, Text: v.Filter.Text}
	if v.Filter.Active {
		if v.Filter.Text == "" {
			f.Hint = s.TextSecondary.Render("a: artist  t: title  l: album")
		} else {
			f.InputView = v.Filter.View()
		}
	}
	counts := strip.CountsChunk{
		Filtered:  filtered,
		Total:     total,
		Filtering: v.Filter.Text != "",
	}
	var sel *strip.SelectionChunk
	if n := len(v.SelectedHashes()); n > 0 {
		sel = &strip.SelectionChunk{Count: n}
	}
	return strip.Render(v.Width, []string{"Queue"}, sortChunk, f, counts, sel, "", s)
}

// NowPlayingIndicator returns the icon string and its style for the
// now-playing marker based on the current playback state. The returned icon
// is unstyled so callers can apply additional styling (e.g. Accent background).
func NowPlayingIndicator(s styles.Styles, state playback.State) (icon string, style lipgloss.Style) {
	switch state {
	case playback.StatePaused:
		return "⏸", s.Warning
	case playback.StateStopped:
		return "■", s.Error
	default:
		return "▶", s.Success
	}
}

// nowPlayingTextStyles returns the text styles for the now-playing row based
// on playback state. Returns (base, favorite, favArtist, favAlbum) styles.
// When playing, these are the standard NowPlaying* styles (green). When
// paused or stopped, the base style shifts to Warning (gold) or Error (red)
// while favorite styles are preserved since they already carry distinct colors.
func nowPlayingTextStyles(s styles.Styles, state playback.State) (base, fav, favArtist, favAlbum lipgloss.Style) {
	switch state {
	case playback.StatePaused:
		base = s.Warning.Bold(true)
		return base, s.NowPlayingFavorite, s.NowPlayingFavArtist, s.NowPlayingFavAlbum
	case playback.StateStopped:
		base = s.Error.Bold(true)
		return base, s.NowPlayingFavorite, s.NowPlayingFavArtist, s.NowPlayingFavAlbum
	default:
		return s.NowPlaying, s.NowPlayingFavorite, s.NowPlayingFavArtist, s.NowPlayingFavAlbum
	}
}
