package tags

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

func (v View) renderTags() string {
	items := v.EffectiveTags()
	if len(items) == 0 && !v.Creating && v.FilterText == "" && !v.FilterActive {
		title := "No tags yet"
		if len(v.Tags) > 0 {
			title = "No tagged tracks yet"
		}
		return v.Styles.RenderEmptyCard(v.Width, v.visibleRows(), title, []styles.EmptyAction{
			{Key: "t", Label: "tag a track"},
			{Key: "?", Label: "help"},
		})
	}

	maxRows := v.visibleRows()

	// When inline create is active, show the input as an extra row at bottom.
	var createRow string
	if v.Creating {
		maxRows-- // reserve one row for the input
		if maxRows < 0 {
			maxRows = 0
		}
		label := text.Pad("+ "+v.CreateInput+"_", v.Width)
		createRow = "\n" + v.Styles.Accent.Render(label)
	}

	if len(items) == 0 && !v.Creating {
		return v.topFilterSeparator() + "(no matches)"
	}

	start, end := v.visibleWindow(len(items), maxRows)

	lines := make([]string, end-start)
	for i, tag := range items[start:end] {
		absIdx := start + i
		count := 0
		if v.TagCounts != nil {
			count = v.TagCounts[tag.ID]
		}
		dot := styles.ColoredDot(tag.Color)
		name := fmt.Sprintf("%s %s (%d)", dot, tag.Name, count)
		display := text.Pad(name, v.Width)
		isOdd := absIdx%2 == 1
		if absIdx == v.Cursor {
			if v.Renaming {
				// Show inline rename for the focused row.
				renamed := text.Pad("r "+v.RenameInput+"_", v.Width)
				display = v.Styles.Accent.Render(renamed)
			} else {
				display = v.Styles.Accent.Render(display)
			}
		} else if isOdd {
			display = lipgloss.NewStyle().Background(v.Styles.ZebraBg).Render(display)
		}
		lines[i] = display
	}
	return v.topFilterSeparator() + strings.Join(lines, "\n") + createRow
}

func (v View) renderTracks() string {
	heading := v.Styles.Heading.Render("Tags — " + v.SelectedTag.Name)
	items := v.EffectiveTracks()
	if len(v.Tracks) == 0 {
		return heading + "\n\n(no tracks)"
	}
	if len(items) == 0 {
		return heading + v.filterSeparator() + "(no matches)"
	}

	cols := ComputeTrack(v.Width)

	header := strings.Repeat(" ", cols.SelectionMarker) + v.Styles.ColHead.Render(
		fmt.Sprintf("%-*s %-*s %-*s %*s",
			cols.Title, "Title",
			cols.Artist, "Artist",
			cols.Album, "Album",
			cols.Duration, "Time"))

	maxRows := v.visibleRows()
	start, end := v.visibleWindow(len(items), maxRows)

	lines := make([]string, end-start)
	for i, t := range items[start:end] {
		absIdx := start + i
		isFocused := absIdx == v.Cursor
		isNowPlaying := v.NowPlayingHash != "" && t.Hash == v.NowPlayingHash
		_, isSelected := v.SelectedHashes[t.Hash]
		_, isTrackFav := v.FavoriteHashes[t.Hash]

		markerStr := text.RowMarker(isSelected, isFocused, isNowPlaying, v.NowPlayingIcon)

		titleStr := text.FavPrefix(t.Title, cols.Title, isTrackFav)
		artistStr := text.Pad(t.Artist, cols.Artist)
		albumStr := text.Pad(t.Album, cols.Album)
		dur := text.FormatDurationSecs(float64(t.DurationMs) / 1000)
		durStr := fmt.Sprintf("%*s", cols.Duration, dur)

		switch {
		case isFocused:
			a := v.Styles.Accent
			sep := a.Render(" ")
			titleStr = styles.AccentFavRender(a, v.Styles.FavoriteAccent, isTrackFav, titleStr)
			lines[i] = a.Render(markerStr) + titleStr + sep + a.Render(artistStr) + sep + a.Render(albumStr) + sep + a.Render(durStr)
		case isNowPlaying:
			np := v.Styles.NowPlayingRow
			sep := np.Render(" ")
			titleStr = v.Styles.NowPlayingTitle.Render(titleStr)
			artistStr = v.Styles.TextPrimary.Background(v.Styles.NowPlayingBg).Render(artistStr)
			albumStr = v.Styles.TextSecondary.Background(v.Styles.NowPlayingBg).Render(albumStr)
			lines[i] = np.Render(markerStr) + titleStr + sep + artistStr + sep + albumStr + sep + np.Render(durStr)
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
				artistPart := styles.WithZebraBg(v.Styles.TextPrimary, rowBg, true).Render(artistStr)
				albumPart := styles.WithZebraBg(v.Styles.TextSecondary, rowBg, true).Render(albumStr)
				durPart := styles.WithZebraBg(v.Styles.TextTertiary, rowBg, true).Render(durStr)
				sep := zb.Render(" ")
				lines[i] = markerPart + titleStr + sep + artistPart + sep + albumPart + sep + durPart
			} else {
				if !isTrackFav {
					titleStr = v.Styles.TextPrimary.Render(titleStr)
				}
				lines[i] = markerStr + titleStr + " " + v.Styles.TextPrimary.Render(artistStr) + " " + v.Styles.TextSecondary.Render(albumStr) + " " + v.Styles.TextTertiary.Render(durStr)
			}
		}
	}
	return heading + v.filterSeparator() + header + "\n" + strings.Join(lines, "\n")
}

// filterSeparator returns the line between the heading and the item list.
func (v View) filterSeparator() string {
	return text.BuildFilterSeparator(v.SortIndicator, v.FilterActive, v.FilterText, v.FilterView, "", v.Width, true)
}

// topFilterSeparator is like filterSeparator but without a leading newline.
func (v View) topFilterSeparator() string {
	return text.BuildFilterSeparator(v.SortIndicator, v.FilterActive, v.FilterText, v.FilterView, "", v.Width, false)
}
