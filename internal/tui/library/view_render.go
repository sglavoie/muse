package library

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	musiclib "github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// displayTrack pairs a track with its fuzzy match positions for highlight rendering.
type displayTrack struct {
	track           musiclib.Track
	matchedIndexes  []int
	artistPositions []int
	titlePositions  []int
	albumPositions  []int
}

// Render returns the complete library panel string. When Width is zero (before
// the first terminal size message) it uses a simple single-line format.
func (v View) Render() string {
	allTracks := v.tracks()
	n := len(allTracks)
	if n == 0 {
		return v.Styles.Dim.Render("No music found — check library path in settings (ctrl+,)")
	}

	// Resolve the set of tracks to display, with optional match positions.
	var display []displayTrack
	if v.Filter.Text != "" {
		if matches, ok := v.filteredMatches(allTracks); ok {
			display = make([]displayTrack, len(matches))
			for i, m := range matches {
				display[i] = displayTrack{
					track:           allTracks[m.TrackIndex],
					matchedIndexes:  m.MatchedIndexes,
					artistPositions: m.ArtistPositions,
					titlePositions:  m.TitlePositions,
					albumPositions:  m.AlbumPositions,
				}
			}
		}
	}
	if display == nil {
		display = make([]displayTrack, len(allTracks))
		for i, t := range allTracks {
			display[i] = displayTrack{track: t}
		}
	}

	maxRows := v.visibleRows()
	start := min(v.ScrollOffset, len(display))
	end := min(len(display), start+maxRows)

	separator := ""
	if v.Filter.Active {
		if v.Filter.Text == "" {
			hint := v.Styles.Dim.Render("a: artist  t: title  l: album  g: genre")
			separator = "Filter: " + hint + "\n"
		} else {
			separator = "Filter: " + v.Filter.View() + "\n"
		}
	} else if v.Filter.Text != "" {
		ft := v.Filter.Text
		maxLen := v.Width - len("Filter: ") - 1
		if maxLen > 0 && ansi.StringWidth(ft) > maxLen {
			ft = ansi.Truncate(ft, maxLen-1, "…")
		}
		separator = "Filter: " + ft + "\n"
	}

	if v.Width == 0 {
		if start >= end {
			return separator + "No matches — press Esc to clear"
		}
		lines := make([]string, end-start)
		for i, dt := range display[start:end] {
			absIdx := start + i
			row := fmt.Sprintf("%s – %s", dt.track.Artist, dt.track.Title)
			if absIdx == v.Cursor {
				row = v.Styles.Accent.Render(row)
			}
			lines[i] = row
		}
		return separator + strings.Join(lines, "\n")
	}

	cols := Compute(v.Width)
	switch {
	case v.ShowPlayedAt:
		cols = ComputeWithPlayedAt(v.Width)
	case v.ShowDateAdded:
		cols = ComputeWithDateAdded(v.Width)
	}

	// Append sort indicator to the active column header.
	dateColLabel := "Added"
	if v.ShowPlayedAt {
		dateColLabel = "Played"
	}
	artistH, titleH, albumH, timeH, dateAddedH := "Artist", "Title", "Album", "Time", dateColLabel
	if v.SortField != 0 {
		ind := "▲"
		if v.SortDir == 1 {
			ind = "▼"
		}
		switch v.SortField {
		case 1: // SortArtist
			artistH += ind
		case 2: // SortTitle
			titleH += ind
		case 3: // SortAlbum
			albumH += ind
		case 4: // SortDuration
			timeH += ind
		case 5: // SortDateAdded
			dateAddedH += ind
		case 9: // SortLastPlayed
			dateAddedH += ind
		}
	}
	header := v.Styles.ColHead.Render(strings.Repeat(" ", cols.SelectionMarker) + renderRow("#", artistH, titleH, albumH, timeH, dateAddedH, cols))

	if start >= end {
		noMatches := "No matches"
		leftPad := max(0, (v.Width-len(noMatches))/2)
		return separator + header + "\n" + strings.Repeat(" ", leftPad) + noMatches
	}

	showDisc := false
	for _, dt := range display[start:end] {
		if dt.track.DiscNumber > 1 {
			showDisc = true
			break
		}
	}

	lines := make([]string, end-start)
	var prevArtist string
	for i, dt := range display[start:end] {
		absIdx := start + i
		t := dt.track
		var num string
		if showDisc {
			num = text.FormatTrackPos(t.TrackNumber, t.DiscNumber, absIdx+1)
		} else {
			num = text.FormatTrackPosSimple(t.TrackNumber, absIdx+1)
		}
		dur := text.FormatDurationSecs(t.Duration)
		var dateAdded string
		if v.ShowPlayedAt {
			dateAdded = formatPlayedAt(t.LastPlayedAt)
		} else {
			dateAdded = formatDateAdded(t.CreatedAt)
		}

		_, isSelected := v.SelectedHashes[t.Hash]
		isFocused := absIdx == v.Cursor
		isOdd := absIdx%2 == 1
		isNowPlaying := v.NowPlayingHash != "" && t.Hash == v.NowPlayingHash
		rowBg := v.Styles.ZebraBg
		useBg := isOdd
		if isSelected && !isFocused && !isNowPlaying {
			rowBg = v.Styles.SelectionBg
			useBg = true
		}

		// When the row is focused, omit the inner accent/reset from the marker
		// so the outer reverse-video wrap covers the full row uniformly. An
		// inner ANSI reset would reset the cursor highlight mid-row.
		var markerStr string
		switch {
		case isSelected && isFocused:
			markerStr = "● "
		case isSelected:
			markerStr = "● "
		case isFocused && isNowPlaying:
			markerStr = "♪ "
		case isFocused:
			markerStr = "▎ "
		case isNowPlaying:
			markerStr = "♪ "
		default:
			markerStr = "  "
		}

		_, isTrackFav := v.FavoriteHashes[t.Hash]
		_, isArtistFav := v.FavoriteArtists[t.Artist]
		_, isAlbumFav := v.FavoriteAlbums[t.Album]

		var artistStr, titleStr, albumStr string
		hasFieldPos := len(dt.artistPositions) > 0 || len(dt.titlePositions) > 0 || len(dt.albumPositions) > 0
		sameArtist := t.Artist == prevArtist && prevArtist != ""

		artistFavStyle := favStyleFor(isArtistFav, v.Styles.FavArtist)
		titleFavStyle := favStyleFor(isTrackFav, v.Styles.Favorite)
		albumFavStyle := favStyleFor(isAlbumFav, v.Styles.FavAlbum)
		if isFocused {
			artistFavStyle = accentStyleFor(isArtistFav, v.Styles.FavArtistAccent, v.Styles.Accent)
			titleFavStyle = accentStyleFor(isTrackFav, v.Styles.FavoriteAccent, v.Styles.Accent)
			albumFavStyle = accentStyleFor(isAlbumFav, v.Styles.FavAlbumAccent, v.Styles.Accent)
		}

		switch {
		case hasFieldPos:
			artistStr = padHighlight(t.Artist, cols.Artist, dt.artistPositions, artistFavStyle...)
			titleStr = padHighlight(t.Title, cols.Title, dt.titlePositions, titleFavStyle...)
			albumStr = padHighlight(t.Album, cols.Album, dt.albumPositions, albumFavStyle...)
		case len(dt.matchedIndexes) > 0:
			artistOffset, artistLen, titleOffset, titleLen, albumOffset, albumLen :=
				searchFieldOffsetsAll(t.Artist, t.Title, t.Album)
			artistPos := fieldMatchPositions(dt.matchedIndexes, artistOffset, artistLen)
			titlePos := fieldMatchPositions(dt.matchedIndexes, titleOffset, titleLen)
			albumPos := fieldMatchPositions(dt.matchedIndexes, albumOffset, albumLen)
			artistStr = padHighlight(t.Artist, cols.Artist, artistPos, artistFavStyle...)
			titleStr = padHighlight(t.Title, cols.Title, titlePos, titleFavStyle...)
			albumStr = padHighlight(t.Album, cols.Album, albumPos, albumFavStyle...)
		default:
			artistStr = text.FavPrefix(t.Artist, cols.Artist, isArtistFav)
			titleStr = text.FavPrefix(t.Title, cols.Title, isTrackFav)
			albumStr = text.FavPrefix(t.Album, cols.Album, isAlbumFav)
			switch {
			case isFocused:
				artistStr = accentStyleFor(isArtistFav, v.Styles.FavArtistAccent, v.Styles.Accent)[0].Render(artistStr)
				titleStr = accentStyleFor(isTrackFav, v.Styles.FavoriteAccent, v.Styles.Accent)[0].Render(titleStr)
				albumStr = accentStyleFor(isAlbumFav, v.Styles.FavAlbumAccent, v.Styles.Accent)[0].Render(albumStr)
			case isNowPlaying:
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
			default:
				if isTrackFav {
					titleStr = styles.WithZebraBg(v.Styles.Favorite, rowBg, useBg).Render(titleStr)
				}
				if isArtistFav {
					artistStr = styles.WithZebraBg(v.Styles.FavArtist, rowBg, useBg).Render(artistStr)
				}
				if isAlbumFav {
					albumStr = styles.WithZebraBg(v.Styles.FavAlbum, rowBg, useBg).Render(albumStr)
				}
				if sameArtist && !isArtistFav {
					artistStr = styles.WithZebraBg(v.Styles.TextTertiary, rowBg, useBg).Render(artistStr)
				}
			}
		}

		var row string
		if cols.Artist == 0 { //nolint:gocritic // ifElseChain: branches have fundamentally different rendering logic
			if isFocused {
				row = v.Styles.Accent.Render(markerStr + text.Pad(num, cols.TrackNum))
			} else {
				row = markerStr + text.Pad(num, cols.TrackNum)
			}
		} else if isFocused {
			sep := v.Styles.Accent.Render("  ")
			numStr := v.Styles.Accent.Render(markerStr + text.Pad(num, cols.TrackNum))
			durStr := v.Styles.Accent.Render(text.PadLeft(dur, cols.Duration))
			row = numStr + sep + artistStr + sep + titleStr + sep + albumStr + sep + durStr
			if cols.DateAdded > 0 {
				row += sep + v.Styles.Accent.Render(text.Pad(dateAdded, cols.DateAdded))
			}
		} else if isNowPlaying {
			np := v.Styles.NowPlayingRow
			sep := np.Render("  ")
			numStr := np.Render(markerStr + text.Pad(num, cols.TrackNum))
			durStr := np.Render(text.PadLeft(dur, cols.Duration))
			row = numStr + sep + artistStr + sep + titleStr + sep + albumStr + sep + durStr
			if cols.DateAdded > 0 {
				row += sep + np.Render(text.Pad(dateAdded, cols.DateAdded))
			}
		} else {
			if useBg {
				rb := lipgloss.NewStyle().Background(rowBg)
				if !isTrackFav {
					titleStr = styles.WithZebraBg(v.Styles.TextPrimary, rowBg, true).Render(titleStr)
				}
				if !isArtistFav && !sameArtist {
					artistStr = styles.WithZebraBg(v.Styles.TextPrimary, rowBg, true).Render(artistStr)
				}
				if !isAlbumFav {
					albumStr = styles.WithZebraBg(v.Styles.TextSecondary, rowBg, true).Render(albumStr)
				}
				var markerPart string
				if isSelected {
					markerPart = v.Styles.AccentMark.Background(rowBg).Render("●") + rb.Render(" ")
				} else {
					markerPart = rb.Render(markerStr)
				}
				numPart := styles.WithZebraBg(v.Styles.TextTertiary, rowBg, true).Render(text.Pad(num, cols.TrackNum))
				durPart := styles.WithZebraBg(v.Styles.TextTertiary, rowBg, true).Render(text.PadLeft(dur, cols.Duration))
				sep := rb.Render("  ")
				row = markerPart + numPart + sep + artistStr + sep + titleStr + sep + albumStr + sep + durPart
				if cols.DateAdded > 0 {
					row += sep + rb.Render(text.Pad(dateAdded, cols.DateAdded))
				}
			} else {
				if !isTrackFav {
					titleStr = v.Styles.TextPrimary.Render(titleStr)
				}
				if !isArtistFav && !sameArtist {
					artistStr = v.Styles.TextPrimary.Render(artistStr)
				}
				if !isAlbumFav {
					albumStr = v.Styles.TextSecondary.Render(albumStr)
				}
				row = markerStr + v.Styles.TextTertiary.Render(text.Pad(num, cols.TrackNum)) + "  " +
					artistStr + "  " + titleStr + "  " + albumStr + "  " +
					v.Styles.TextTertiary.Render(text.PadLeft(dur, cols.Duration))
				if cols.DateAdded > 0 {
					row += "  " + text.Pad(dateAdded, cols.DateAdded)
				}
			}
		}

		prevArtist = t.Artist
		lines[i] = row
	}

	return separator + header + "\n" + strings.Join(lines, "\n")
}

// renderRow formats a single row with each field padded or truncated to its
// column width. When cols.Artist is zero the layout degenerates to just the
// track number field.
func renderRow(num, artist, title, album, duration, dateAdded string, cols Columns) string {
	if cols.Artist == 0 {
		return text.Pad(num, cols.TrackNum)
	}
	row := text.Pad(num, cols.TrackNum) + "  " +
		text.Pad(artist, cols.Artist) + "  " +
		text.Pad(title, cols.Title) + "  " +
		text.Pad(album, cols.Album) + "  " +
		text.PadLeft(duration, cols.Duration)
	if cols.DateAdded > 0 {
		row += "  " + text.Pad(dateAdded, cols.DateAdded)
	}
	return row
}

func formatDateAdded(createdAt int64) string {
	if createdAt <= 0 {
		return "--"
	}
	return time.Unix(createdAt, 0).Local().Format("2006-01-02")
}

func formatPlayedAt(ts int64) string {
	if ts <= 0 {
		return "--"
	}
	return time.Unix(ts, 0).Local().Format("2006-01-02 15:04")
}
