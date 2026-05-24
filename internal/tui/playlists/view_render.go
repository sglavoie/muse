package playlists

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// Render returns the string representation of the playlists panel.
func (v View) Render() string {
	switch v.Level {
	case ListLevel:
		return v.renderList()
	default:
		return v.renderTracks()
	}
}

func (v View) renderList() string {
	if len(v.Playlists) == 0 && !v.Creating {
		card := v.Styles.RenderEmptyCard(v.Width, v.visibleRows(), "No playlists yet", []styles.EmptyAction{
			{Key: "n", Label: "new"},
			{Key: "i", Label: "import m3u"},
			{Key: "?", Label: "help"},
		})
		if v.SortField == "" {
			return card
		}
		return v.renderListSortIndicator() + "\n" + card
	}

	cols := Compute(v.Width)

	maxRows := v.visibleRows()
	start := visibleWindowStart(v.ScrollOffset, v.Cursor, len(v.Playlists), maxRows)
	end := min(len(v.Playlists), start+maxRows)

	var lines []string
	for _, p := range v.Playlists[start:end] {
		absIdx := start + len(lines) // track absolute index before appending
		if v.Renaming && absIdx == v.Cursor {
			lines = append(lines, v.renderRenameInputRow(cols))
			continue
		}
		var name string
		var count string
		if p.IsSmart {
			// Smart playlists show a ⚡ glyph and either the evaluated track count
			// (once known) or "(smart)" as a placeholder. TrackCount < 0 means the
			// background evaluation has not yet returned.
			name = text.Pad("⚡ "+p.Name, cols.Name)
			if p.TrackCount >= 0 {
				count = text.PadLeft(fmt.Sprintf("(%d)", p.TrackCount), cols.Count)
			} else {
				count = text.PadLeft("(smart)", cols.Count)
			}
		} else {
			name = text.Pad(p.Name, cols.Name)
			count = text.PadLeft(fmt.Sprintf("(%d)", p.TrackCount), cols.Count)
		}
		line := name + "  " + count
		if absIdx == v.Cursor {
			line = v.Styles.Accent.Render(line)
		}
		lines = append(lines, line)
		if v.Creating && absIdx == v.Cursor {
			lines = append(lines, v.renderCreateInputRow(cols))
		}
	}
	// When the list is empty the input is still shown at the top.
	if v.Creating && len(v.Playlists) == 0 {
		lines = append(lines, v.renderCreateInputRow(cols))
	}
	out := strings.Join(lines, "\n")
	if v.SortField != "" {
		out = v.renderListSortIndicator() + "\n" + out
	}
	return out
}

func (v View) renderListSortIndicator() string {
	return v.Styles.Dim.Render(fmt.Sprintf("Sort: %s %s", v.SortField, v.SortDir))
}

// renderCreateInputRow renders the inline text input as a highlighted row
// fitted to the same column widths as a normal playlist row.
func (v View) renderCreateInputRow(cols Columns) string {
	rowWidth := cols.Name + nameSep + cols.Count
	display := text.Pad(v.CreateInput.View(), rowWidth)
	return v.Styles.Accent.Render(display)
}

// renderRenameInputRow renders the inline rename input as a highlighted row
// fitted to the same column widths as a normal playlist row.
func (v View) renderRenameInputRow(cols Columns) string {
	rowWidth := cols.Name + nameSep + cols.Count
	display := text.Pad(v.RenameInput.View(), rowWidth)
	return v.Styles.Accent.Render(display)
}

func (v View) renderTracks() string {
	heading := v.Styles.Heading.Render("Tracks")
	if v.IsSmartPlaylist {
		heading += "  " + v.Styles.Dim.Render("(smart playlist — read-only)")
	}
	if len(v.Tracks) == 0 {
		return heading + "\n\n(no tracks)"
	}

	cols := ComputeTrack(v.Width)
	titleHead, artistHead, albumHead, timeHead := "Title", "Artist", "Album", "Time"
	if v.SortField != "" {
		switch v.SortField {
		case "Title":
			titleHead += v.SortDir
		case "Artist":
			artistHead += v.SortDir
		case "Album":
			albumHead += v.SortDir
		case "Duration":
			timeHead += v.SortDir
		}
	}

	header := strings.Repeat(" ", cols.Marker) + v.Styles.ColHead.Render(
		text.Pad("#", cols.Position)+" "+
			text.Pad(titleHead, cols.Title)+" "+
			text.Pad(artistHead, cols.Artist)+" "+
			text.Pad(albumHead, cols.Album)+" "+
			text.PadLeft(timeHead, cols.Duration))

	maxRows := v.visibleRows()
	start := visibleWindowStart(v.ScrollOffset, v.Cursor, len(v.Tracks), maxRows)
	end := min(len(v.Tracks), start+maxRows)

	lines := make([]string, end-start)
	for i, t := range v.Tracks[start:end] {
		absIdx := start + i
		_, isSelected := v.SelectedTrackIDs[t.ID]
		isFocused := absIdx == v.Cursor
		isNowPlaying := v.NowPlayingHash != "" && t.Hash == v.NowPlayingHash

		// When the row is focused, omit the inner accent/reset from the marker
		// so the outer background highlight covers the full row uniformly. An
		// inner ANSI reset would reset the cursor highlight mid-row.
		markerStr := text.RowMarker(isSelected, isFocused, isNowPlaying, v.NowPlayingIcon)

		pos := fmt.Sprintf("%-*s", cols.Position, text.FormatTrackPos(t.TrackNumber, t.DiscNumber, absIdx+1))
		dur := text.FormatDurationMs(t.DurationMs)
		dur = fmt.Sprintf("%*s", cols.Duration, dur)

		// Per-cell favorite lookups.
		_, isTrackFav := v.FavoriteHashes[t.Hash]
		_, isArtistFav := v.FavoriteArtists[t.Artist]
		_, isAlbumFav := v.FavoriteAlbums[t.Album]

		titleStr := text.FavPrefix(t.Title, cols.Title, isTrackFav)
		artistStr := text.FavPrefix(t.Artist, cols.Artist, isArtistFav)
		albumStr := text.FavPrefix(t.Album, cols.Album, isAlbumFav)
		switch {
		case isFocused:
			a := v.Styles.Accent
			sep := a.Render(" ")
			titleStr = styles.AccentFavRender(a, v.Styles.FavoriteAccent, isTrackFav, titleStr)
			artistStr = styles.AccentFavRender(a, v.Styles.FavArtistAccent, isArtistFav, artistStr)
			albumStr = styles.AccentFavRender(a, v.Styles.FavAlbumAccent, isAlbumFav, albumStr)
			lines[i] = a.Render(markerStr+pos) + sep + titleStr + sep + artistStr + sep + albumStr + sep + a.Render(dur)
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
			lines[i] = np.Render(markerStr+pos) + sep + titleStr + sep + artistStr + sep + albumStr + sep + np.Render(dur)
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
			if isAlbumFav {
				albumStr = styles.WithZebraBg(v.Styles.FavAlbum, rowBg, useBg).Render(albumStr)
			}
			if useBg {
				zb := lipgloss.NewStyle().Background(rowBg)
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
				if isSelected {
					markerPart = v.Styles.AccentMark.Background(rowBg).Render("●") + zb.Render(" ")
				} else {
					markerPart = zb.Render(markerStr)
				}
				posPart := styles.WithZebraBg(v.Styles.TextTertiary, rowBg, true).Render(pos)
				durPart := styles.WithZebraBg(v.Styles.TextTertiary, rowBg, true).Render(dur)
				sep := zb.Render(" ")
				lines[i] = markerPart + posPart + sep + titleStr + sep + artistStr + sep + albumStr + sep + durPart
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
				lines[i] = markerStr + v.Styles.TextTertiary.Render(pos) + " " + titleStr + " " + artistStr + " " + albumStr + " " + v.Styles.TextTertiary.Render(dur)
			}
		}
	}
	return heading + "\n\n" + header + "\n" + strings.Join(lines, "\n")
}

func visibleWindowStart(scrollOffset, cursor, itemCount, maxRows int) int {
	if itemCount <= 0 || maxRows <= 0 {
		return 0
	}

	maxStart := max(0, itemCount-maxRows)
	start := min(max(scrollOffset, 0), maxStart)

	if cursor < 0 {
		return start
	}
	cursor = min(cursor, itemCount-1)
	if cursor < start {
		start = cursor
	} else if cursor >= start+maxRows {
		start = cursor - maxRows + 1
	}

	return min(max(start, 0), maxStart)
}
