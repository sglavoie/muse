package tui

import (
	"charm.land/lipgloss/v2"

	"github.com/sglavoie/muse/internal/tui/strip"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// renderStateStrip builds the unified single-line state strip carrying
// the view label, sort, filter, and counts for the queue. total is the
// full entry count before filtering; filtered is the count after filter
// is applied (equal to total when no filter is active).
func (v QueueView) renderStateStrip(s styles.Styles, total, filtered int) string {
	var sortChunk strip.SortChunk
	if v.sortState.Field != SortDefault {
		sortChunk = strip.SortChunk{
			Label:     SortFieldLabel(v.sortState.Field),
			Indicator: SortDirIndicator(v.sortState.Dir),
		}
	}
	f := strip.FilterChunk{Active: v.filter.Active, Text: v.filter.Text}
	if v.filter.Active {
		if v.filter.Text == "" {
			f.Hint = s.TextSecondary.Render("a: artist  t: title  l: album")
		} else {
			f.InputView = v.filter.View()
		}
	}
	counts := strip.CountsChunk{
		Filtered:  filtered,
		Total:     total,
		Filtering: v.filter.Text != "",
	}
	var sel *strip.SelectionChunk
	if n := len(v.selectedHashes()); n > 0 {
		sel = &strip.SelectionChunk{Count: n}
	}
	return strip.Render(v.width, []string{"Queue"}, sortChunk, f, counts, sel, "", s)
}

// nowPlayingIndicator returns the icon string and its style for the
// now-playing marker based on the current playback state. The returned icon
// is unstyled so callers can apply additional styling (e.g. Accent background).
func nowPlayingIndicator(s styles.Styles, state PlaybackState) (icon string, style lipgloss.Style) {
	switch state {
	case PlaybackPaused:
		return "⏸", s.Warning
	case PlaybackStopped:
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
func nowPlayingTextStyles(s styles.Styles, state PlaybackState) (base, fav, favArtist, favAlbum lipgloss.Style) {
	switch state {
	case PlaybackPaused:
		base = s.Warning.Bold(true)
		return base, s.NowPlayingFavorite, s.NowPlayingFavArtist, s.NowPlayingFavAlbum
	case PlaybackStopped:
		base = s.Error.Bold(true)
		return base, s.NowPlayingFavorite, s.NowPlayingFavArtist, s.NowPlayingFavAlbum
	default:
		return s.NowPlaying, s.NowPlayingFavorite, s.NowPlayingFavArtist, s.NowPlayingFavAlbum
	}
}
