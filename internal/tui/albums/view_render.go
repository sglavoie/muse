package albums

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/sglavoie/muse/internal/tui/layout"
	"github.com/sglavoie/muse/internal/tui/strip"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// Render returns the string representation of the albums panel.
func (v View) Render() string {
	switch v.Level {
	case TrackLevel:
		return v.renderTracks()
	default:
		return v.renderAlbums()
	}
}

func (v View) renderAlbums() string {
	items := v.EffectiveAlbums()
	if len(v.Albums) == 0 {
		return v.Styles.Dim.Render("No albums yet — add music to your library")
	}
	width := contentWidth(v.Width)
	stripLine := v.renderStateStrip(len(v.Albums), len(items))
	if len(items) == 0 {
		return strip.JoinStrip(stripLine, "(no matches)")
	}

	maxRows := v.visibleRows()
	start, end := layout.VisibleWindow(v.ScrollOffset, len(items), maxRows)

	lines := make([]string, end-start)
	for i, name := range items[start:end] {
		absIdx := start + i
		_, isFav := v.FavoriteAlbums[name]
		isOdd := absIdx%2 == 1
		labelW, countStr := text.RowLabelWidthAndCount(width, v.AlbumCounts, name)
		label := text.FavPrefix(name, labelW, isFav)
		switch {
		case absIdx == v.Cursor && isFav:
			a := v.Styles.FavAlbumAccent
			lines[i] = a.Render(label) + a.Render(countStr)
		case absIdx == v.Cursor:
			a := v.Styles.Accent
			lines[i] = a.Render(label) + a.Render(countStr)
		case isFav:
			labelStyled := styles.WithZebraBg(v.Styles.FavAlbum, v.Styles.ZebraBg, isOdd).Render(label)
			lines[i] = labelStyled + styles.RenderCountZone(v.Styles, countStr, isOdd)
		case isOdd:
			labelStyled := lipgloss.NewStyle().Background(v.Styles.ZebraBg).Render(label)
			lines[i] = labelStyled + styles.RenderCountZone(v.Styles, countStr, true)
		default:
			lines[i] = label + styles.RenderCountZone(v.Styles, countStr, false)
		}
	}
	return strip.JoinStrip(stripLine, strings.Join(lines, "\n"))
}

func (v View) renderTracks() string {
	items := v.EffectiveTracks()
	if len(v.Tracks) == 0 {
		return v.Styles.Dim.Render("No tracks found for this album")
	}
	width := contentWidth(v.Width)
	stripLine := v.renderStateStrip(len(v.Tracks), len(items))
	if len(items) == 0 {
		return strip.JoinStrip(stripLine, "(no matches)")
	}

	cols := ComputeTrack(width)

	header := strings.Repeat(" ", cols.SelectionMarker) + v.Styles.ColHead.Render(
		text.Pad("#", cols.Position)+" "+
			text.Pad("Title", cols.Title)+" "+
			text.Pad("Artist", cols.Artist)+" "+
			text.PadLeft("Time", cols.Duration))

	maxRows := v.visibleRows()
	start, end := layout.VisibleWindow(v.ScrollOffset, len(items), maxRows)

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
		_, isSelected := v.SelectedHashes[t.Hash]

		markerStr := text.RowMarker(isSelected, isFocused, isNowPlaying, v.NowPlayingIcon)

		var rawPos string
		if showDisc {
			rawPos = text.FormatTrackPos(t.TrackNumber, t.DiscNumber, absIdx+1)
		} else {
			rawPos = text.FormatTrackPosSimple(t.TrackNumber, absIdx+1)
		}
		pos := text.Pad(rawPos, cols.Position)
		_, isTrackFav := v.FavoriteHashes[t.Hash]
		_, isArtistFav := v.FavoriteArtists[t.Artist]
		titleStr := text.FavPrefix(t.Title, cols.Title, isTrackFav)
		artistStr := text.FavPrefix(t.Artist, cols.Artist, isArtistFav)
		dur := text.PadLeft(text.FormatDurationSecs(t.Duration), cols.Duration)
		switch {
		case isFocused:
			a := v.Styles.Accent
			sep := a.Render(" ")
			titleStr = styles.AccentFavRender(a, v.Styles.FavoriteAccent, isTrackFav, titleStr)
			artistStr = styles.AccentFavRender(a, v.Styles.FavArtistAccent, isArtistFav, artistStr)
			lines[i] = a.Render(markerStr+pos) + sep + titleStr + sep + artistStr + sep + a.Render(dur)
		case isNowPlaying:
			np := v.Styles.NowPlayingRow
			sep := np.Render(" ")
			titleStr = v.Styles.NowPlayingTitle.Render(titleStr)
			if isArtistFav {
				artistStr = v.Styles.NowPlayingRowFavArtist.Render(artistStr)
			} else {
				artistStr = v.Styles.TextPrimary.Background(v.Styles.NowPlayingBg).Render(artistStr)
			}
			lines[i] = np.Render(markerStr+pos) + sep + titleStr + sep + artistStr + sep + np.Render(dur)
		default:
			isOdd := absIdx%2 == 1
			rowBg := v.Styles.ZebraBg
			useBg := isOdd
			if isSelected {
				rowBg = v.Styles.SelectionBg
				useBg = true
			}
			if isTrackFav {
				titleStr = styles.WithZebraBg(v.Styles.Favorite, rowBg, useBg).Render(titleStr)
			}
			if isArtistFav {
				artistStr = styles.WithZebraBg(v.Styles.FavArtist, rowBg, useBg).Render(artistStr)
			}
			if useBg {
				zb := lipgloss.NewStyle().Background(rowBg)
				if !isTrackFav {
					titleStr = styles.WithZebraBg(v.Styles.TextPrimary, rowBg, true).Render(titleStr)
				}
				if !isArtistFav {
					artistStr = styles.WithZebraBg(v.Styles.TextPrimary, rowBg, true).Render(artistStr)
				}
				var markerPart string
				if isSelected {
					markerPart = v.Styles.AccentMark.Background(rowBg).Render("●") + zb.Render(" ")
				} else {
					markerPart = zb.Render(markerStr)
				}
				posPart := styles.WithZebraBg(v.Styles.TextTertiary, rowBg, true).Render(pos)
				durPart := styles.WithZebraBg(v.Styles.TextTertiary, rowBg, true).Render(dur)
				sep := zb.Render(" ")
				lines[i] = markerPart + posPart + sep + titleStr + sep + artistStr + sep + durPart
			} else {
				if !isTrackFav {
					titleStr = v.Styles.TextPrimary.Render(titleStr)
				}
				if !isArtistFav {
					artistStr = v.Styles.TextPrimary.Render(artistStr)
				}
				lines[i] = markerStr + v.Styles.TextTertiary.Render(pos) + " " + titleStr + " " + artistStr + " " + v.Styles.TextTertiary.Render(dur)
			}
		}
	}
	rest := header + "\n" + strings.Join(lines, "\n")
	return strip.JoinStrip(stripLine, rest)
}

// renderStateStrip builds the unified single-line state strip carrying the
// breadcrumb, sort, filter, counts, and an "esc ←" hint at the track
// level. total is the full item count before filtering; filtered is the
// count after filtering (equal to total when no filter is active).
func (v View) renderStateStrip(total, filtered int) string {
	root := v.StripViewLabel
	if root == "" {
		root = "Albums"
	}
	breadcrumb := []string{root}
	if v.Level == TrackLevel && v.SelectedAlbum != "" {
		breadcrumb = append(breadcrumb, v.SelectedAlbum)
	}
	hint := ""
	if len(breadcrumb) > 1 {
		hint = "esc ←"
	}
	sortChunk := strip.SortChunk{Label: v.StripSortLabel, Indicator: v.StripSortIndicator}
	f := strip.FilterChunk{Active: v.FilterActive, Text: v.FilterText}
	if v.FilterActive {
		if v.FilterText == "" && v.FilterHint != "" {
			f.Hint = v.Styles.TextSecondary.Render(v.FilterHint)
		} else {
			f.InputView = v.FilterView
		}
	}
	counts := strip.CountsChunk{
		Filtered:  filtered,
		Total:     total,
		Filtering: v.FilterText != "",
	}
	var sel *strip.SelectionChunk
	if n := len(v.SelectedHashes); n > 0 {
		sel = &strip.SelectionChunk{Count: n}
	}
	return strip.Render(v.Width, breadcrumb, sortChunk, f, counts, sel, hint, v.Styles)
}

func contentWidth(width int) int {
	if width > 0 {
		return width
	}
	return 80
}
