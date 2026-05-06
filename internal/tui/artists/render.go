package artists

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

func (v View) renderArtists() string {
	items := v.EffectiveArtists()
	if len(v.Artists) == 0 {
		return "(no artists)"
	}
	if len(items) == 0 {
		return v.topFilterSeparator() + "(no matches)"
	}

	maxRows := v.visibleRows()
	start := min(v.ScrollOffset, len(items))
	end := min(len(items), start+maxRows)

	lines := make([]string, end-start)
	for i, name := range items[start:end] {
		absIdx := start + i
		_, isFav := v.FavoriteArtists[name]
		display := text.Pad(name, v.Width)
		isOdd := absIdx%2 == 1
		switch {
		case absIdx == v.Cursor && isFav:
			display = v.Styles.FavArtistAccent.Render(display)
		case absIdx == v.Cursor:
			display = v.Styles.Accent.Render(display)
		case isFav:
			display = styles.WithZebraBg(v.Styles.FavArtist, v.Styles.ZebraBg, isOdd).Render(display)
		case isOdd:
			display = lipgloss.NewStyle().Background(v.Styles.ZebraBg).Render(display)
		}
		lines[i] = display
	}
	return v.topFilterSeparator() + strings.Join(lines, "\n")
}

func (v View) renderAlbums() string {
	heading := v.Styles.Heading.Render(v.SelectedArtist)
	items := v.EffectiveAlbums()
	if len(v.Albums) == 0 {
		return heading + "\n\n(no albums)"
	}
	if len(items) == 0 {
		return heading + v.filterSeparator() + "(no matches)"
	}

	maxRows := v.visibleRows()
	start := min(v.ScrollOffset, len(items))
	end := min(len(items), start+maxRows)

	lines := make([]string, end-start)
	for i, name := range items[start:end] {
		absIdx := start + i
		_, isFav := v.FavoriteAlbums[name]
		display := text.Pad(name, v.Width)
		isOdd := absIdx%2 == 1
		switch {
		case absIdx == v.Cursor && isFav:
			display = v.Styles.FavAlbumAccent.Render(display)
		case absIdx == v.Cursor:
			display = v.Styles.Accent.Render(display)
		case isFav:
			display = styles.WithZebraBg(v.Styles.FavAlbum, v.Styles.ZebraBg, isOdd).Render(display)
		case isOdd:
			display = lipgloss.NewStyle().Background(v.Styles.ZebraBg).Render(display)
		}
		lines[i] = display
	}
	return heading + v.filterSeparator() + strings.Join(lines, "\n")
}

func (v View) renderTracks() string {
	heading := v.SelectedArtist
	if v.SelectedAlbum != "" {
		heading += " — " + v.SelectedAlbum
	}
	styledHeading := v.Styles.Heading.Render(heading)
	items := v.EffectiveTracks()
	if len(v.Tracks) == 0 {
		return styledHeading + "\n\n(no tracks)"
	}
	if len(items) == 0 {
		return styledHeading + v.filterSeparator() + "(no matches)"
	}

	cols := ComputeTrack(v.Width)
	header := v.Styles.ColHead.Render(
		strings.Repeat(" ", cols.SelectionMarker) +
			fmt.Sprintf("%-*s %-*s %*s",
				cols.Position, "#", cols.Title, "Title", cols.Duration, "Time"))

	maxRows := v.visibleRows()
	start := min(v.ScrollOffset, len(items))
	end := min(len(items), start+maxRows)

	lines := make([]string, end-start)
	for i, track := range items[start:end] {
		absIdx := start + i
		isFocused := absIdx == v.Cursor
		isNowPlaying := v.NowPlayingHash != "" && track.Hash == v.NowPlayingHash
		_, isSelected := v.SelectedHashes[track.Hash]

		var markerStr string
		switch {
		case isSelected && isFocused:
			markerStr = "● "
		case isSelected:
			markerStr = v.Styles.AccentMark.Render("●") + " "
		case isFocused && isNowPlaying:
			markerStr = "♪ "
		case isFocused:
			markerStr = "▎ "
		case isNowPlaying:
			markerStr = "♪ "
		default:
			markerStr = "  "
		}

		pos := fmt.Sprintf("%-*s", cols.Position, text.FormatTrackPos(track.TrackNumber, track.DiscNumber, absIdx+1))
		titleStr := text.Pad(track.Title, cols.Title)
		dur := fmt.Sprintf("%*s", cols.Duration, text.FormatDurationSecs(track.Duration))

		_, isTrackFav := v.FavoriteHashes[track.Hash]
		if isFocused {
			accent := v.Styles.Accent
			sep := accent.Render(" ")
			titleStr = styles.AccentFavRender(accent, v.Styles.FavoriteAccent, isTrackFav, titleStr)
			lines[i] = accent.Render(markerStr+pos) + sep + titleStr + sep + accent.Render(dur)
		} else if isNowPlaying {
			np := v.Styles.NowPlayingRow
			sep := np.Render(" ")
			titleStr = v.Styles.NowPlayingTitle.Render(titleStr)
			lines[i] = np.Render(markerStr+pos) + sep + titleStr + sep + np.Render(dur)
		} else {
			isOdd := absIdx%2 == 1
			if isTrackFav {
				titleStr = styles.WithZebraBg(v.Styles.Favorite, v.Styles.ZebraBg, isOdd).Render(titleStr)
			}
			if isOdd {
				zb := lipgloss.NewStyle().Background(v.Styles.ZebraBg)
				if !isTrackFav {
					titleStr = styles.WithZebraBg(v.Styles.TextPrimary, v.Styles.ZebraBg, true).Render(titleStr)
				}
				markerPart := zb.Render(markerStr)
				posPart := styles.WithZebraBg(v.Styles.TextTertiary, v.Styles.ZebraBg, isOdd).Render(pos)
				durPart := styles.WithZebraBg(v.Styles.TextTertiary, v.Styles.ZebraBg, isOdd).Render(dur)
				sep := zb.Render(" ")
				lines[i] = markerPart + posPart + sep + titleStr + sep + durPart
			} else {
				if !isTrackFav {
					titleStr = v.Styles.TextPrimary.Render(titleStr)
				}
				lines[i] = markerStr + v.Styles.TextTertiary.Render(pos) + " " + titleStr + " " + v.Styles.TextTertiary.Render(dur)
			}
		}
	}
	return styledHeading + v.filterSeparator() + header + "\n" + strings.Join(lines, "\n")
}

// filterSeparator returns the line between the heading and the item list.
// When the filter is active or committed it shows the query; otherwise it
// returns a plain blank line.
func (v View) filterSeparator() string {
	return v.buildFilterSeparator(true)
}

// topFilterSeparator is like filterSeparator but without a leading newline,
// for use in top-level views where the title bar provides the spacing.
func (v View) topFilterSeparator() string {
	return v.buildFilterSeparator(false)
}

// buildFilterSeparator builds the filter/sort separator string. When
// leadingNewline is true, sort and filter lines are prefixed with a newline
// (used below a heading); when false the leading newline is omitted (used
// at the top level where the title bar already provides spacing).
func (v View) buildFilterSeparator(leadingNewline bool) string {
	var prefix string
	if v.SortIndicator != "" {
		if leadingNewline {
			prefix = "\n" + v.SortIndicator
		} else {
			prefix = v.SortIndicator + "\n"
		}
	}
	if v.FilterActive {
		sep := "\n"
		if !leadingNewline {
			sep = ""
		}
		filterView := v.FilterView
		if filterView == "" {
			filterView = v.FilterText
		}
		return prefix + sep + "Filter: " + filterView + "\n"
	}
	if v.FilterText != "" {
		sep := "\n"
		if !leadingNewline {
			sep = ""
		}
		filterText := v.FilterText
		maxLen := v.Width - len("Filter: ") - 1
		if maxLen > 0 && ansi.StringWidth(filterText) > maxLen {
			filterText = ansi.Truncate(filterText, maxLen-1, "…")
		}
		return prefix + sep + "Filter: " + filterText + "\n"
	}
	if leadingNewline {
		return prefix + "\n\n"
	}
	return prefix
}
