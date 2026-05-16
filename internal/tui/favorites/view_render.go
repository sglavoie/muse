package favorites

import (
	"fmt"
	"strings"

	lipgloss "charm.land/lipgloss/v2"

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
		isOdd := i%2 == 1
		if i == v.Cursor {
			display = v.Styles.Accent.Render(display)
		} else if isOdd {
			display = lipgloss.NewStyle().Background(v.Styles.ZebraBg).Render(display)
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
		return heading + "\n\n" + v.Styles.Dim.Render("No favorites yet — press F on any artist or album")
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

		_, selected := v.SelectedItems[absIdx]
		marker := text.RowMarker(selected, isFocused, false, "")

		display := text.FavPrefix(name, v.Width-2, isFav)
		line := marker + display
		isOdd := absIdx%2 == 1
		switch {
		case isFocused && isFav:
			line = favAccentStyle.Render(line)
		case isFocused:
			line = v.Styles.Accent.Render(line)
		case selected && isFav:
			sb := lipgloss.NewStyle().Background(v.Styles.SelectionBg)
			line = v.Styles.AccentMark.Background(v.Styles.SelectionBg).Render("●") + sb.Render(" ") +
				styles.WithZebraBg(favStyle, v.Styles.SelectionBg, true).Render(display)
		case selected:
			sb := lipgloss.NewStyle().Background(v.Styles.SelectionBg)
			line = v.Styles.AccentMark.Background(v.Styles.SelectionBg).Render("●") + sb.Render(" "+display)
		case isFav:
			line = styles.WithZebraBg(favStyle, v.Styles.ZebraBg, isOdd).Render(line)
		case isOdd:
			line = lipgloss.NewStyle().Background(v.Styles.ZebraBg).Render(line)
		}
		lines[i] = line
	}
	return heading + v.filterSeparator() + strings.Join(lines, "\n")
}

func (v View) renderTrackList() string {
	heading := v.Styles.Heading.Render("Favorite Tracks")
	items := v.EffectiveTracks()
	if len(v.Tracks) == 0 {
		return heading + "\n\n" + v.Styles.Dim.Render("No favorites yet — press F on any track")
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
		return heading + "\n\n" + v.Styles.Dim.Render("No tracks found for this selection")
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

	showDisc := false
	for _, t := range items[start:end] {
		if t.DiscNumber > 1 {
			showDisc = true
			break
		}
	}

	lines := make([]string, end-start)
	for i, t := range items[start:end] {
		absIdx := start + i
		isFocused := absIdx == v.Cursor
		isNowPlaying := v.NowPlayingHash != "" && t.Hash == v.NowPlayingHash

		_, selected := v.SelectedItems[absIdx]
		marker := text.RowMarker(selected, isFocused, isNowPlaying, "♪")

		var rawPos string
		if showDisc {
			rawPos = text.FormatTrackPos(t.TrackNumber, t.DiscNumber, absIdx+1)
		} else {
			rawPos = text.FormatTrackPosSimple(t.TrackNumber, absIdx+1)
		}
		pos := fmt.Sprintf("%-*s", cols.Position, rawPos)
		_, isTrackFav := v.FavoriteHashes[t.Hash]
		_, isArtistFav := v.FavoriteArtists[t.Artist]
		_, isAlbumFav := v.FavoriteAlbums[t.Album]
		titleStr := text.FavPrefix(t.Title, cols.Title, isTrackFav)
		artistStr := text.FavPrefix(t.Artist, cols.Artist, isArtistFav)
		albumStr := text.FavPrefix(t.Album, cols.Album, isAlbumFav)
		dur := text.FormatDurationSecs(t.Duration)
		dur = fmt.Sprintf("%*s", cols.Duration, dur)
		plays := fmt.Sprintf("%*d", cols.PlayCount, t.PlayCount)
		switch {
		case isFocused:
			a := v.Styles.Accent
			sep := a.Render(" ")
			titleStr = styles.AccentFavRender(a, v.Styles.FavoriteAccent, isTrackFav, titleStr)
			artistStr = styles.AccentFavRender(a, v.Styles.FavArtistAccent, isArtistFav, artistStr)
			albumStr = styles.AccentFavRender(a, v.Styles.FavAlbumAccent, isAlbumFav, albumStr)
			lines[i] = a.Render(marker+pos) + sep + titleStr + sep + artistStr + sep + albumStr + sep + a.Render(dur) + sep + a.Render(plays)
		case isNowPlaying:
			np := v.Styles.NowPlayingRow
			sep := np.Render(" ")
			titleStr = v.Styles.NowPlayingTitle.Render(titleStr)
			if isArtistFav {
				artistStr = v.Styles.NowPlayingRowFavArtist.Render(artistStr)
			} else {
				artistStr = v.Styles.TextPrimary.Background(v.Styles.NowPlayingBg).Render(artistStr)
			}
			if isAlbumFav {
				albumStr = v.Styles.NowPlayingRowFavAlbum.Render(albumStr)
			} else {
				albumStr = v.Styles.TextSecondary.Background(v.Styles.NowPlayingBg).Render(albumStr)
			}
			lines[i] = np.Render(marker+pos) + sep + titleStr + sep + artistStr + sep + albumStr + sep + np.Render(dur) + sep + np.Render(plays)
		default:
			isOdd := absIdx%2 == 1
			rowBg := v.Styles.ZebraBg
			useBg := isOdd
			if selected {
				rowBg = v.Styles.SelectionBg
				useBg = true
			}
			if isTrackFav {
				titleStr = styles.WithZebraBg(v.Styles.Favorite, rowBg, useBg).Render(titleStr)
			}
			if isArtistFav {
				artistStr = styles.WithZebraBg(v.Styles.FavArtist, rowBg, useBg).Render(artistStr)
			}
			if isAlbumFav {
				albumStr = styles.WithZebraBg(v.Styles.FavAlbum, rowBg, useBg).Render(albumStr)
			}
			if useBg {
				rb := lipgloss.NewStyle().Background(rowBg)
				if !isTrackFav {
					titleStr = styles.WithZebraBg(v.Styles.TextPrimary, rowBg, true).Render(titleStr)
				}
				if !isArtistFav {
					artistStr = styles.WithZebraBg(v.Styles.TextPrimary, rowBg, true).Render(artistStr)
				}
				if !isAlbumFav {
					albumStr = styles.WithZebraBg(v.Styles.TextSecondary, rowBg, true).Render(albumStr)
				}
				var markerPart string
				if selected {
					markerPart = v.Styles.AccentMark.Background(rowBg).Render("●") + rb.Render(" ")
				} else {
					markerPart = rb.Render(marker)
				}
				posPart := styles.WithZebraBg(v.Styles.TextTertiary, rowBg, true).Render(pos)
				durPart := styles.WithZebraBg(v.Styles.TextTertiary, rowBg, true).Render(dur)
				playsPart := rb.Render(plays)
				sep := rb.Render(" ")
				lines[i] = markerPart + posPart + sep + titleStr + sep + artistStr + sep + albumStr + sep + durPart + sep + playsPart
			} else {
				if !isTrackFav {
					titleStr = v.Styles.TextPrimary.Render(titleStr)
				}
				if !isArtistFav {
					artistStr = v.Styles.TextPrimary.Render(artistStr)
				}
				if !isAlbumFav {
					albumStr = v.Styles.TextSecondary.Render(albumStr)
				}
				lines[i] = marker + v.Styles.TextTertiary.Render(pos) + " " + titleStr + " " + artistStr + " " + albumStr + " " + v.Styles.TextTertiary.Render(dur) + " " + plays
			}
		}
	}
	return header + "\n" + strings.Join(lines, "\n")
}

// filterSeparator returns the line between the heading and the item list.
func (v View) filterSeparator() string {
	return text.BuildFilterSeparator(v.SortIndicator, v.FilterActive, v.FilterText, v.FilterView, v.Styles.Dim.Render(v.FilterHint), v.Width, true)
}
