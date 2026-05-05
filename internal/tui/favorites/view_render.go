package favorites

import (
	"fmt"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// Render returns the string representation of the favorites panel.
func (v View) Render() string {
	switch v.Level {
	case ListLevel:
		return v.renderList()
	case TrackLevel:
		return v.renderTracks()
	default:
		return v.renderCategories()
	}
}

func (v View) renderCategories() string {
	categories := []string{
		fmt.Sprintf("Artists (%d)", v.ArtistCount),
		fmt.Sprintf("Albums (%d)", v.AlbumCount),
		fmt.Sprintf("Tracks (%d)", v.TrackCount),
	}

	lines := make([]string, len(categories))
	for i, name := range categories {
		display := text.Pad(name, v.Width)
		if i == v.Cursor {
			display = v.Styles.Accent.Render(display)
		}
		lines[i] = display
	}
	return strings.Join(lines, "\n")
}

func (v View) renderList() string {
	if v.Category == CategoryTracks {
		return v.renderTrackList()
	}

	var categoryName string
	switch v.Category {
	case CategoryArtists:
		categoryName = "Favorite Artists"
	case CategoryAlbums:
		categoryName = "Favorite Albums"
	}
	heading := v.Styles.Heading.Render(categoryName)

	items := v.EffectiveItems()
	var allItems []string
	switch v.Category {
	case CategoryArtists:
		allItems = v.Artists
	case CategoryAlbums:
		allItems = v.Albums
	}
	if len(allItems) == 0 {
		return heading + "\n\n(empty)"
	}
	if len(items) == 0 {
		return heading + v.filterSeparator() + "(no matches)"
	}

	maxRows := v.visibleRows()
	start := min(v.ScrollOffset, len(items))
	end := min(len(items), start+maxRows)

	// Determine the favorite style for this category.
	var favStyle, favAccentStyle lipgloss.Style
	var favMap map[string]struct{}
	switch v.Category {
	case CategoryArtists:
		favStyle = v.Styles.FavArtist
		favAccentStyle = v.Styles.FavArtistAccent
		favMap = v.FavoriteArtists
	case CategoryAlbums:
		favStyle = v.Styles.FavAlbum
		favAccentStyle = v.Styles.FavAlbumAccent
		favMap = v.FavoriteAlbums
	}

	lines := make([]string, end-start)
	for i, name := range items[start:end] {
		absIdx := start + i
		isFocused := absIdx == v.Cursor
		_, isFav := favMap[name]

		var marker string
		_, selected := v.SelectedItems[absIdx]
		switch {
		case selected && isFocused:
			marker = "● "
		case selected:
			marker = v.Styles.AccentMark.Render("●") + " "
		default:
			marker = "  "
		}

		display := text.Pad(name, v.Width-2)
		line := marker + display
		switch {
		case isFocused && isFav:
			line = favAccentStyle.Render(line)
		case isFocused:
			line = v.Styles.Accent.Render(line)
		case isFav:
			line = favStyle.Render(line)
		}
		lines[i] = line
	}
	return heading + v.filterSeparator() + strings.Join(lines, "\n")
}

func (v View) renderTrackList() string {
	heading := v.Styles.Heading.Render("Favorite Tracks")
	items := v.EffectiveTracks()
	if len(v.Tracks) == 0 {
		return heading + "\n\n(empty)"
	}
	if len(items) == 0 {
		return heading + v.filterSeparator() + "(no matches)"
	}

	return heading + v.filterSeparator() + v.renderTrackRows(items)
}

func (v View) renderTracks() string {
	heading := v.Styles.Heading.Render(v.DrilldownName)

	items := v.EffectiveTracks()
	if len(v.DrilldownTracks) == 0 {
		return heading + "\n\n(no tracks)"
	}
	if len(items) == 0 {
		return heading + v.filterSeparator() + "(no matches)"
	}

	return heading + v.filterSeparator() + v.renderTrackRows(items)
}

func (v View) renderTrackRows(items []library.Track) string {
	cols := ComputeTrack(v.Width)

	// Column header row.
	header := v.Styles.ColHead.Render(
		fmt.Sprintf("  %-*s %-*s %-*s %-*s %*s %*s",
			cols.Position, "#", cols.Title, "Title", cols.Artist, "Artist", cols.Album, "Album", cols.Duration, "Time", cols.PlayCount, "Plays"))

	maxRows := v.visibleRows()
	start := min(v.ScrollOffset, len(items))
	end := min(len(items), start+maxRows)

	lines := make([]string, end-start)
	for i, t := range items[start:end] {
		absIdx := start + i
		isFocused := absIdx == v.Cursor

		var marker string
		_, selected := v.SelectedItems[absIdx]
		switch {
		case selected && isFocused:
			marker = "● "
		case selected:
			marker = v.Styles.AccentMark.Render("●") + " "
		default:
			marker = "  "
		}

		pos := fmt.Sprintf("%-*s", cols.Position, text.FormatTrackPos(t.TrackNumber, t.DiscNumber, absIdx+1))
		titleStr := text.Pad(t.Title, cols.Title)
		artistStr := text.Pad(t.Artist, cols.Artist)
		albumStr := text.Pad(t.Album, cols.Album)
		dur := text.FormatDurationSecs(t.Duration)
		dur = fmt.Sprintf("%*s", cols.Duration, dur)
		plays := fmt.Sprintf("%*d", cols.PlayCount, t.PlayCount)

		_, isTrackFav := v.FavoriteHashes[t.Hash]
		_, isArtistFav := v.FavoriteArtists[t.Artist]
		_, isAlbumFav := v.FavoriteAlbums[t.Album]
		if isFocused {
			a := v.Styles.Accent
			sep := a.Render(" ")
			titleStr = styles.AccentFavRender(a, v.Styles.FavoriteAccent, isTrackFav, titleStr)
			artistStr = styles.AccentFavRender(a, v.Styles.FavArtistAccent, isArtistFav, artistStr)
			albumStr = styles.AccentFavRender(a, v.Styles.FavAlbumAccent, isAlbumFav, albumStr)
			lines[i] = a.Render(marker+pos) + sep + titleStr + sep + artistStr + sep + albumStr + sep + a.Render(dur) + sep + a.Render(plays)
		} else {
			if isTrackFav {
				titleStr = v.Styles.Favorite.Render(titleStr)
			}
			if isArtistFav {
				artistStr = v.Styles.FavArtist.Render(artistStr)
			}
			if isAlbumFav {
				albumStr = v.Styles.FavAlbum.Render(albumStr)
			}
			lines[i] = marker + pos + " " + titleStr + " " + artistStr + " " + albumStr + " " + dur + " " + plays
		}
	}
	return header + "\n" + strings.Join(lines, "\n")
}

// filterSeparator returns the line between the heading and the item list.
func (v View) filterSeparator() string {
	var sortLine string
	if v.SortIndicator != "" {
		sortLine = "\n" + v.SortIndicator
	}
	if v.FilterActive {
		fv := v.FilterView
		if fv == "" {
			fv = v.FilterText
		}
		return sortLine + "\nFilter: " + fv + "\n"
	}
	if v.FilterText != "" {
		ft := v.FilterText
		maxLen := v.Width - len("Filter: ") - 1
		if maxLen > 0 && ansi.StringWidth(ft) > maxLen {
			ft = ansi.Truncate(ft, maxLen-1, "…")
		}
		return sortLine + "\nFilter: " + ft + "\n"
	}
	return sortLine + "\n\n"
}
