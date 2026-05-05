package genres

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

func (v View) renderGenres() string {
	items := v.EffectiveGenres()
	if len(v.Genres) == 0 {
		return "(no genres)"
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
		display := text.Pad(name, v.Width)
		if absIdx == v.Cursor {
			display = v.Styles.Accent.Render(display)
		}
		lines[i] = display
	}
	return v.topFilterSeparator() + strings.Join(lines, "\n")
}

func (v View) renderArtists() string {
	heading := v.Styles.Heading.Render(v.SelectedGenre)
	items := v.EffectiveArtists()
	if len(v.Artists) == 0 {
		return heading + "\n\n(no artists)"
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
		_, isFav := v.FavoriteArtists[name]
		display := text.Pad(name, v.Width)
		switch {
		case absIdx == v.Cursor && isFav:
			display = v.Styles.FavArtistAccent.Render(display)
		case absIdx == v.Cursor:
			display = v.Styles.Accent.Render(display)
		case isFav:
			display = v.Styles.FavArtist.Render(display)
		}
		lines[i] = display
	}
	return heading + v.filterSeparator() + strings.Join(lines, "\n")
}

func (v View) renderTracks() string {
	heading := v.SelectedGenre
	if v.SelectedArtist != "" {
		heading += " — " + v.SelectedArtist
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
	for i, t := range items[start:end] {
		absIdx := start + i
		isFocused := absIdx == v.Cursor
		_, isSelected := v.SelectedHashes[t.Hash]

		var markerStr string
		switch {
		case isSelected && isFocused:
			markerStr = "● "
		case isSelected:
			markerStr = v.Styles.AccentMark.Render("●") + " "
		default:
			markerStr = "  "
		}

		pos := fmt.Sprintf("%-*s", cols.Position, text.FormatTrackPos(t.TrackNumber, t.DiscNumber, absIdx+1))
		titleStr := text.Pad(t.Title, cols.Title)
		dur := text.FormatDurationSecs(t.Duration)
		dur = fmt.Sprintf("%*s", cols.Duration, dur)

		_, isTrackFav := v.FavoriteHashes[t.Hash]
		if isFocused {
			a := v.Styles.Accent
			sep := a.Render(" ")
			titleStr = styles.AccentFavRender(a, v.Styles.FavoriteAccent, isTrackFav, titleStr)
			lines[i] = a.Render(markerStr+pos) + sep + titleStr + sep + a.Render(dur)
		} else {
			if isTrackFav {
				titleStr = v.Styles.Favorite.Render(titleStr)
			}
			lines[i] = markerStr + pos + " " + titleStr + " " + dur
		}
	}
	return styledHeading + v.filterSeparator() + header + "\n" + strings.Join(lines, "\n")
}

// filterSeparator returns the line between the heading and the item list.
func (v View) filterSeparator() string {
	return v.buildFilterSeparator(true)
}

// topFilterSeparator is like filterSeparator but without a leading newline,
// for use in top-level views where the title bar provides the spacing.
func (v View) topFilterSeparator() string {
	return v.buildFilterSeparator(false)
}

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
		fv := v.FilterView
		if fv == "" {
			fv = v.FilterText
		}
		return prefix + sep + "Filter: " + fv + "\n"
	}
	if v.FilterText != "" {
		sep := "\n"
		if !leadingNewline {
			sep = ""
		}
		ft := v.FilterText
		maxLen := v.Width - len("Filter: ") - 1
		if maxLen > 0 && ansi.StringWidth(ft) > maxLen {
			ft = ansi.Truncate(ft, maxLen-1, "…")
		}
		return prefix + sep + "Filter: " + ft + "\n"
	}
	if leadingNewline {
		return prefix + "\n\n"
	}
	return prefix
}
