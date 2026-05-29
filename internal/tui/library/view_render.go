package library

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	musiclib "github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/buckets"
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
		return v.Styles.RenderEmptyCard(v.Width, v.visibleRows(), "Library is empty", []styles.EmptyAction{
			{Key: "s", Label: "scan folders"},
			{Key: "i", Label: "import"},
			{Key: "?", Label: "help"},
		})
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
	dataRows := maxRows
	if v.BucketLabel != nil && maxRows > 1 && len(display) > 0 {
		labelIdx := v.Cursor
		if labelIdx < 0 || labelIdx >= len(display) {
			labelIdx = 0
		}
		if v.BucketLabel(display[labelIdx].track) != "" {
			dataRows = maxRows - 1
		}
	}
	// Clamp ScrollOffset against (len-dataRows), not len: an offset past the
	// last visible window slice would render an empty body even though items
	// (and possibly the cursor) still exist after a delete/filter shrunk the
	// list.
	maxStart := 0
	if dataRows > 0 && len(display) > dataRows {
		maxStart = len(display) - dataRows
	}
	start := min(max(0, v.ScrollOffset), maxStart)
	if v.Cursor >= 0 && v.Cursor < len(display) {
		if v.Cursor < start {
			start = v.Cursor
		} else if dataRows > 0 && v.Cursor >= start+dataRows {
			start = v.Cursor - dataRows + 1
		}
		if start < 0 {
			start = 0
		}
	}
	end := min(len(display), start+dataRows)

	if v.Width == 0 {
		// Zero-width fallback: keep the legacy "Filter: ..." line so the
		// user can still see their query before the terminal size arrives.
		separator := ""
		if v.Filter.Active {
			if v.Filter.Text == "" {
				hint := v.Styles.Dim.Render("a: artist  t: title  l: album  g: genre")
				separator = "Filter: " + hint + "\n"
			} else {
				separator = "Filter: " + v.Filter.View() + "\n"
			}
		} else if v.Filter.Text != "" {
			separator = "Filter: " + v.Filter.Text + "\n"
		}
		if len(display) == 0 {
			return separator + "No matches — press Esc to clear"
		}
		if start >= end {
			return strings.TrimSuffix(separator, "\n")
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

	dateColLabel := "Added"
	if v.ShowPlayedAt {
		dateColLabel = "Played"
	}
	header := strings.Repeat(" ", cols.SelectionMarker) + v.Styles.ColHead.Render(renderRow("#", "Artist", "Title", "Album", "Time", dateColLabel, cols))
	stripLine := v.renderStateStrip(len(allTracks), len(display))

	if len(display) == 0 {
		noMatches := "No matches"
		leftPad := max(0, (v.Width-len(noMatches))/2)
		return stripLine + "\n" + header + "\n" + strings.Repeat(" ", leftPad) + noMatches
	}
	if start >= end {
		return stripLine + "\n" + header
	}

	showDisc := false
	for _, dt := range display[start:end] {
		if dt.track.DiscNumber > 1 {
			showDisc = true
			break
		}
	}

	lines := make([]string, end-start)
	dimRunsDisabled := v.Filter.Active || v.Filter.Text != ""
	var prevArtist, prevAlbum string
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
		markerStr := text.RowMarker(isSelected, isFocused, isNowPlaying, v.NowPlayingIcon)

		_, isTrackFav := v.FavoriteHashes[t.Hash]
		_, isArtistFav := v.FavoriteArtists[t.Artist]
		_, isAlbumFav := v.FavoriteAlbums[t.Album]

		var artistStr, titleStr, albumStr string
		hasFieldPos := len(dt.artistPositions) > 0 || len(dt.titlePositions) > 0 || len(dt.albumPositions) > 0
		sameArtist := !dimRunsDisabled && t.Artist == prevArtist && prevArtist != ""
		sameAlbum := !dimRunsDisabled && t.Album == prevAlbum && prevAlbum != ""

		artistFavStyle := favStyleFor(isArtistFav, v.Styles.FavArtist)
		titleFavStyle := favStyleFor(isTrackFav, v.Styles.Favorite)
		albumFavStyle := favStyleFor(isAlbumFav, v.Styles.FavAlbum)
		switch {
		case isFocused:
			artistFavStyle = accentStyleFor(isArtistFav, v.Styles.FavArtistAccent, v.Styles.Accent)
			titleFavStyle = accentStyleFor(isTrackFav, v.Styles.FavoriteAccent, v.Styles.Accent)
			albumFavStyle = accentStyleFor(isAlbumFav, v.Styles.FavAlbumAccent, v.Styles.Accent)
		case isNowPlaying:
			// Compose NowPlayingBg into every cell so search-match highlighting
			// (hasFieldPos / matchedIndexes branches below) doesn't leave the
			// artist/title/album cells without the amber background while the
			// markers and separators carry it.
			artistFavStyle = nowPlayingStyleFor(isArtistFav, v.Styles.NowPlayingRowFavArtist, v.Styles.TextPrimary.Background(v.Styles.NowPlayingBg))
			titleFavStyle = nowPlayingStyleFor(isTrackFav, v.Styles.NowPlayingRowFavorite, v.Styles.NowPlayingTitle)
			albumFavStyle = nowPlayingStyleFor(isAlbumFav, v.Styles.NowPlayingRowFavAlbum, v.Styles.TextSecondary.Background(v.Styles.NowPlayingBg))
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
				if sameArtist {
					artistStr = styles.WithZebraBg(v.Styles.TextTertiary, rowBg, useBg).
						Render(text.DimRepeat(t.Artist, prevArtist, cols.Artist))
				}
				if sameAlbum {
					albumStr = styles.WithZebraBg(v.Styles.TextTertiary, rowBg, useBg).
						Render(text.DimRepeat(t.Album, prevAlbum, cols.Album))
				}
			}
		}

		var row string
		if cols.IsTrackNumOnly() { //nolint:gocritic // ifElseChain: branches have fundamentally different rendering logic
			markerPart := text.Pad(markerStr, cols.SelectionMarker)
			numPart := text.Pad(num, cols.TrackNum)
			if isFocused {
				row = v.Styles.Accent.Render(markerPart + numPart)
			} else {
				row = markerPart + numPart
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
				if !isAlbumFav && !sameAlbum {
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
				if !isAlbumFav && !sameAlbum {
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
		prevAlbum = t.Album
		lines[i] = row
	}

	out := lines
	if v.BucketLabel != nil {
		labels := make([]string, len(lines))
		for i := range lines {
			labels[i] = v.BucketLabel(display[start+i].track)
		}
		sticky := ""
		if len(labels) > 0 {
			sticky = labels[0]
		}
		out = buckets.Chunk(lines, labels, buckets.ChunkOptions{
			StickyBucket:     sticky,
			MaxRows:          maxRows,
			DividerEvery:     buckets.DividerEvery,
			MakeDivider:      v.renderDivider,
			MakeBucketHeader: v.renderBucketHeader,
		})
	}

	return stripLine + "\n" + header + "\n" + strings.Join(out, "\n")
}
