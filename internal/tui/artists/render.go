package artists

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/sglavoie/muse/internal/tui/layout"
	"github.com/sglavoie/muse/internal/tui/strip"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

func (v View) renderArtists() string {
	items := v.EffectiveArtists()
	if len(v.Artists) == 0 {
		return v.Styles.Dim.Render("No artists yet — add music to your library")
	}
	stripLine := v.renderStateStrip(len(v.Artists), len(items))
	if len(items) == 0 {
		return strip.JoinStrip(stripLine, "(no matches)")
	}

	maxRows := v.visibleRows()
	start, end := layout.VisibleWindow(v.ScrollOffset, len(items), maxRows)

	lines := make([]string, end-start)
	for i, name := range items[start:end] {
		absIdx := start + i
		_, isFav := v.FavoriteArtists[name]
		isOdd := absIdx%2 == 1
		labelW, countStr := text.RowLabelWidthAndCount(v.Width, v.ArtistCounts, name)
		label := text.FavPrefix(name, labelW, isFav)
		switch {
		case absIdx == v.Cursor && isFav:
			a := v.Styles.FavArtistAccent
			lines[i] = a.Render(label) + a.Render(countStr)
		case absIdx == v.Cursor:
			a := v.Styles.Accent
			lines[i] = a.Render(label) + a.Render(countStr)
		case isFav:
			labelStyled := styles.WithZebraBg(v.Styles.FavArtist, v.Styles.ZebraBg, isOdd).Render(label)
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

func (v View) renderAlbums() string {
	items := v.EffectiveAlbums()
	if len(v.Albums) == 0 {
		return v.Styles.Dim.Render("No albums found for this artist")
	}
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
		display := text.FavPrefix(name, v.Width, isFav)
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
	return strip.JoinStrip(stripLine, strings.Join(lines, "\n"))
}

func (v View) renderTracks() string {
	items := v.EffectiveTracks()
	if len(v.Tracks) == 0 {
		return v.Styles.Dim.Render("No tracks found for this album")
	}
	stripLine := v.renderStateStrip(len(v.Tracks), len(items))
	if len(items) == 0 {
		return strip.JoinStrip(stripLine, "(no matches)")
	}

	cols := ComputeTrack(v.Width)
	header := strings.Repeat(" ", cols.SelectionMarker) + v.Styles.ColHead.Render(
		fmt.Sprintf("%-*s %-*s %*s",
			cols.Position, "#", cols.Title, "Title", cols.Duration, "Time"))

	maxRows := v.visibleRows()
	start, end := layout.VisibleWindow(v.ScrollOffset, len(items), maxRows)

	showDisc := false
	for _, track := range items[start:end] {
		if track.DiscNumber > 1 {
			showDisc = true
			break
		}
	}

	lines := make([]string, end-start)
	for i, track := range items[start:end] {
		absIdx := start + i
		isFocused := absIdx == v.Cursor
		isNowPlaying := v.NowPlayingHash != "" && track.Hash == v.NowPlayingHash
		_, isSelected := v.SelectedHashes[track.Hash]

		markerStr := text.RowMarker(isSelected, isFocused, isNowPlaying, v.NowPlayingIcon)

		var rawPos string
		if showDisc {
			rawPos = text.FormatTrackPos(track.TrackNumber, track.DiscNumber, absIdx+1)
		} else {
			rawPos = text.FormatTrackPosSimple(track.TrackNumber, absIdx+1)
		}
		pos := fmt.Sprintf("%-*s", cols.Position, rawPos)
		_, isTrackFav := v.FavoriteHashes[track.Hash]
		titleStr := text.FavPrefix(track.Title, cols.Title, isTrackFav)
		dur := fmt.Sprintf("%*s", cols.Duration, text.FormatDurationSecs(track.Duration))
		switch {
		case isFocused:
			accent := v.Styles.Accent
			sep := accent.Render(" ")
			titleStr = styles.AccentFavRender(accent, v.Styles.FavoriteAccent, isTrackFav, titleStr)
			lines[i] = accent.Render(markerStr+pos) + sep + titleStr + sep + accent.Render(dur)
		case isNowPlaying:
			np := v.Styles.NowPlayingRow
			sep := np.Render(" ")
			titleStr = v.Styles.NowPlayingTitle.Render(titleStr)
			lines[i] = np.Render(markerStr+pos) + sep + titleStr + sep + np.Render(dur)
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
			if useBg {
				zb := lipgloss.NewStyle().Background(rowBg)
				if !isTrackFav {
					titleStr = styles.WithZebraBg(v.Styles.TextPrimary, rowBg, true).Render(titleStr)
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
				lines[i] = markerPart + posPart + sep + titleStr + sep + durPart
			} else {
				if !isTrackFav {
					titleStr = v.Styles.TextPrimary.Render(titleStr)
				}
				lines[i] = markerStr + v.Styles.TextTertiary.Render(pos) + " " + titleStr + " " + v.Styles.TextTertiary.Render(dur)
			}
		}
	}
	body := strings.Join(lines, "\n")
	rest := header + "\n" + body
	return strip.JoinStrip(stripLine, rest)
}

// renderStateStrip builds the unified single-line state strip carrying the
// breadcrumb, sort, filter, counts, and an optional "esc ←" hint at
// drill-down levels. total is the full item count before filtering;
// filtered is the count after filtering (equal to total when no filter is
// active).
func (v View) renderStateStrip(total, filtered int) string {
	root := v.StripViewLabel
	if root == "" {
		root = "Artists"
	}
	breadcrumb := []string{root}
	switch v.Level {
	case AlbumLevel:
		if v.SelectedArtist != "" {
			breadcrumb = append(breadcrumb, v.SelectedArtist)
		}
	case TrackLevel:
		if v.SelectedArtist != "" {
			breadcrumb = append(breadcrumb, v.SelectedArtist)
		}
		if v.SelectedAlbum != "" {
			breadcrumb = append(breadcrumb, v.SelectedAlbum)
		}
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

// joinStrip joins a top-level strip line with the rendered body, skipping the
// strip entirely when it is empty (zero-width fallback).
func joinStrip(stripLine, body string) string {
	if stripLine == "" {
		return body
	}
	return stripLine + "\n" + body
}
