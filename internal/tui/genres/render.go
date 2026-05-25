package genres

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/sglavoie/muse/internal/tui/strip"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

func (v View) renderGenres() string {
	items := v.EffectiveGenres()
	if len(v.Genres) == 0 {
		return v.Styles.Dim.Render("No genres yet — add music to your library")
	}
	stripLine := v.renderStateStrip(len(v.Genres), len(items))
	if len(items) == 0 {
		return strip.JoinStrip(stripLine, "(no matches)")
	}

	start, end, cursor := v.renderWindow(len(items))

	lines := make([]string, end-start)
	for i, name := range items[start:end] {
		absIdx := start + i
		isOdd := absIdx%2 == 1
		labelW, countStr := text.RowLabelWidthAndCount(v.Width, v.GenreCounts, name)
		label := text.Pad(name, labelW)
		switch {
		case absIdx == cursor:
			a := v.Styles.Accent
			lines[i] = a.Render(label) + a.Render(countStr)
		case isOdd:
			labelStyled := lipgloss.NewStyle().Background(v.Styles.ZebraBg).Render(label)
			lines[i] = labelStyled + styles.RenderCountZone(v.Styles, countStr, true)
		default:
			lines[i] = label + styles.RenderCountZone(v.Styles, countStr, false)
		}
	}
	return strip.JoinStrip(stripLine, strings.Join(lines, "\n"))
}

func (v View) renderArtists() string {
	items := v.EffectiveArtists()
	if len(v.Artists) == 0 {
		return v.Styles.Dim.Render("No artists found for this genre")
	}
	stripLine := v.renderStateStrip(len(v.Artists), len(items))
	if len(items) == 0 {
		return strip.JoinStrip(stripLine, "(no matches)")
	}

	start, end, cursor := v.renderWindow(len(items))

	lines := make([]string, end-start)
	for i, name := range items[start:end] {
		absIdx := start + i
		_, isFav := v.FavoriteArtists[name]
		display := text.FavPrefix(name, v.Width, isFav)
		isOdd := absIdx%2 == 1
		switch {
		case absIdx == cursor && isFav:
			display = v.Styles.FavArtistAccent.Render(display)
		case absIdx == cursor:
			display = v.Styles.Accent.Render(display)
		case isFav:
			display = styles.WithZebraBg(v.Styles.FavArtist, v.Styles.ZebraBg, isOdd).Render(display)
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
		return v.Styles.Dim.Render("No tracks found for this artist")
	}
	stripLine := v.renderStateStrip(len(v.Tracks), len(items))
	if len(items) == 0 {
		return strip.JoinStrip(stripLine, "(no matches)")
	}

	cols := ComputeTrack(v.Width)
	header := strings.Repeat(" ", cols.SelectionMarker) + v.Styles.ColHead.Render(
		fmt.Sprintf("%-*s %-*s %*s",
			cols.Position, "#", cols.Title, "Title", cols.Duration, "Time"))

	start, end, cursor := v.renderWindow(len(items))

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
		isFocused := absIdx == cursor
		isNowPlaying := v.NowPlayingHash != "" && t.Hash == v.NowPlayingHash
		_, isSelected := v.SelectedHashes[t.Hash]

		markerStr := text.RowMarker(isSelected, isFocused, isNowPlaying, v.NowPlayingIcon)

		var rawPos string
		if showDisc {
			rawPos = text.FormatTrackPos(t.TrackNumber, t.DiscNumber, absIdx+1)
		} else {
			rawPos = text.FormatTrackPosSimple(t.TrackNumber, absIdx+1)
		}
		pos := fmt.Sprintf("%-*s", cols.Position, rawPos)
		_, isTrackFav := v.FavoriteHashes[t.Hash]
		titleStr := text.FavPrefix(t.Title, cols.Title, isTrackFav)
		dur := text.FormatDurationSecs(t.Duration)
		dur = fmt.Sprintf("%*s", cols.Duration, dur)
		switch {
		case isFocused:
			a := v.Styles.Accent
			sep := a.Render(" ")
			titleStr = styles.AccentFavRender(a, v.Styles.FavoriteAccent, isTrackFav, titleStr)
			lines[i] = a.Render(markerStr+pos) + sep + titleStr + sep + a.Render(dur)
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
	rest := header + "\n" + strings.Join(lines, "\n")
	return strip.JoinStrip(stripLine, rest)
}

// renderStateStrip builds the unified single-line state strip carrying the
// breadcrumb, sort, filter, counts, and an "esc ←" hint at drill-down
// levels. total is the full item count before filtering; filtered is the
// count after filtering (equal to total when no filter is active).
func (v View) renderStateStrip(total, filtered int) string {
	root := v.StripViewLabel
	if root == "" {
		root = "Genres"
	}
	breadcrumb := []string{root}
	switch v.Level {
	case ArtistLevel:
		if v.SelectedGenre != "" {
			breadcrumb = append(breadcrumb, v.SelectedGenre)
		}
	case TrackLevel:
		if v.SelectedGenre != "" {
			breadcrumb = append(breadcrumb, v.SelectedGenre)
		}
		if v.SelectedArtist != "" {
			breadcrumb = append(breadcrumb, v.SelectedArtist)
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

// renderWindow clamps stale cursor and scroll state to the currently rendered
// list. This keeps filtered/shrunk lists visible even when the caller has not
// yet normalized navigation state.
func (v View) renderWindow(itemCount int) (start, end, cursor int) {
	if itemCount <= 0 {
		return 0, 0, 0
	}
	maxRows := v.visibleRows()
	cursor = max(0, min(v.Cursor, itemCount-1))
	start = max(0, v.ScrollOffset)
	if start > cursor {
		start = cursor
	}
	if cursor >= start+maxRows {
		start = cursor - maxRows + 1
	}
	maxStart := max(0, itemCount-maxRows)
	if start > maxStart {
		start = maxStart
	}
	end = min(itemCount, start+maxRows)
	return start, end, cursor
}
