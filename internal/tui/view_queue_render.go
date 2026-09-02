package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/sglavoie/muse/internal/tui/buckets"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/sorting"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// render builds the queue panel string using a 6-column layout:
//
//	Position (~4 chars fixed) | Title (~30% of width) | Artist (~20% of width) | Album (~20% of width) | Source (~10% of width) | Duration (~10 chars fixed)
//
// Proportional columns resize on WindowSizeMsg; fixed-width columns are
// constant. Missing entries are wrapped in ANSI dim escape codes so they
// appear grayed out in the terminal.
func (v QueueView) render(s styles.Styles, currentPlayingIndex int, playbackState playback.State, favHashes, favArtists, favAlbums map[string]struct{}) string {
	entries := v.filteredEntries()
	if len(entries) == 0 {
		if v.filter.Text != "" {
			return "(no matches)"
		}
		return s.RenderEmptyCard(v.width, v.visibleRows(), "Queue is empty", []styles.EmptyAction{
			{Key: "1", Label: "go to library"},
			{Key: "?", Label: "help"},
		})
	}

	const (
		markerWidth = 2
	)

	maxRows := v.visibleRows()
	start := min(v.scrollOffset, len(entries))
	end := min(len(entries), start+maxRows)

	hasSource := false
	for _, e := range entries {
		if e.SourcePlaylist != "" {
			hasSource = true
			break
		}
	}
	cols := computeQueueColumns(v.width, hasSource)

	titleHead, artistHead, albumHead, sourceHead, timeHead := "Title", "Artist", "Album", "Source", "Time"
	// headerBody excludes the leading marker-width prefix so the dotted
	// underline only spans the actual data columns (not the marker gutter).
	var headerBody string
	if hasSource {
		headerBody = text.Pad("#", cols.Position) + " " +
			text.Pad(titleHead, cols.Title) + " " +
			text.Pad(artistHead, cols.Artist) + " " +
			text.Pad(albumHead, cols.Album) + " " +
			text.Pad(sourceHead, cols.Source) + " " +
			text.PadLeft(timeHead, cols.Duration)
	} else {
		headerBody = text.Pad("#", cols.Position) + " " +
			text.Pad(titleHead, cols.Title) + " " +
			text.Pad(artistHead, cols.Artist) + " " +
			text.Pad(albumHead, cols.Album) + " " +
			text.PadLeft(timeHead, cols.Duration)
	}
	header := clampQueueLine(strings.Repeat(" ", markerWidth)+s.ColHead.Render(headerBody), v.width)

	lines := make([]string, end-start)
	for i := range end - start {
		absIdx := start + i
		e := entries[absIdx]
		isFocused := absIdx == v.cursor
		isNowPlaying := currentPlayingIndex >= 0 && e.origIdx == currentPlayingIndex
		isPlayed := currentPlayingIndex >= 0 && !isNowPlaying && e.origIdx < currentPlayingIndex

		// Determine the now-playing icon and style based on playback state.
		npIcon, npStyle := nowPlayingIndicator(s, playbackState)

		markerStr := text.RowMarker(e.Selected, isFocused, isNowPlaying, npIcon)

		pos := text.PadLeft(fmt.Sprintf("%d", absIdx+1), cols.Position)
		source := padQueue(e.SourcePlaylist, cols.Source)
		dur := text.PadLeft(text.FormatDurationMs(e.DurationMs), cols.Duration)

		_, isTrackFav := favHashes[e.Hash]
		_, isArtistFav := favArtists[e.Artist]
		_, isAlbumFav := favAlbums[e.Album]

		titleStr := text.FavPrefix(e.Title, cols.Title, isTrackFav)
		artistStr := text.FavPrefix(e.Artist, cols.Artist, isArtistFav)
		albumStr := text.FavPrefix(e.Album, cols.Album, isAlbumFav)
		if e.Missing { //nolint:gocritic // ifElseChain: branches have fundamentally different rendering logic
			plainLine := assembleQueueRow(" ", hasSource, markerStr, pos, padQueue(e.Title, cols.Title), padQueue(e.Artist, cols.Artist), padQueue(e.Album, cols.Album), source, dur)
			lines[i] = s.Dim.Render(appendQueueSuffix(plainLine, " [missing]", v.width))
		} else if isFocused {
			bg := s.Accent.GetBackground()
			sep := s.Accent.Render(" ")
			if isNowPlaying {
				npText, npFav, npFavArtist, npFavAlbum := nowPlayingTextStyles(s, playbackState)
				titleStr = npText.Background(bg).Render(titleStr)
				artistStr = npText.Background(bg).Render(artistStr)
				albumStr = npText.Background(bg).Render(albumStr)
				if isTrackFav {
					titleStr = npFav.Background(bg).Render(text.FavPrefix(e.Title, cols.Title, true))
				}
				if isArtistFav {
					artistStr = npFavArtist.Background(bg).Render(text.FavPrefix(e.Artist, cols.Artist, true))
				}
				if isAlbumFav {
					albumStr = npFavAlbum.Background(bg).Render(text.FavPrefix(e.Album, cols.Album, true))
				}
			} else {
				if isTrackFav {
					titleStr = s.FavoriteAccent.Render(titleStr)
				} else {
					titleStr = s.Accent.Render(titleStr)
				}
				if isArtistFav {
					artistStr = s.FavArtistAccent.Render(artistStr)
				} else {
					artistStr = s.Accent.Render(artistStr)
				}
				if isAlbumFav {
					albumStr = s.FavAlbumAccent.Render(albumStr)
				} else {
					albumStr = s.Accent.Render(albumStr)
				}
			}
			markerStr = s.Accent.Render(markerStr)
			pos = s.Accent.Render(pos)
			source = s.Accent.Render(source)
			dur = s.Accent.Render(dur)
			lines[i] = assembleQueueRow(sep, hasSource, markerStr, pos, titleStr, artistStr, albumStr, source, dur)
		} else if isNowPlaying {
			_, npFav, npFavArtist, npFavAlbum := nowPlayingTextStyles(s, playbackState)
			nb := lipgloss.NewStyle().Background(s.NowPlayingBg)
			titleStr = s.NowPlayingTitle.Render(titleStr)
			artistStr = s.TextPrimary.Background(s.NowPlayingBg).Render(artistStr)
			albumStr = s.TextSecondary.Background(s.NowPlayingBg).Render(albumStr)
			if isTrackFav {
				titleStr = npFav.Background(s.NowPlayingBg).Render(text.FavPrefix(e.Title, cols.Title, true))
			}
			if isArtistFav {
				artistStr = npFavArtist.Background(s.NowPlayingBg).Render(text.FavPrefix(e.Artist, cols.Artist, true))
			}
			if isAlbumFav {
				albumStr = npFavAlbum.Background(s.NowPlayingBg).Render(text.FavPrefix(e.Album, cols.Album, true))
			}
			markerStr = npStyle.Background(s.NowPlayingBg).Render(npIcon) + nb.Render(" ")
			pos = nb.Render(pos)
			source = nb.Render(source)
			dur = nb.Render(dur)
			sep := nb.Render(" ")
			lines[i] = assembleQueueRow(sep, hasSource, markerStr, pos, titleStr, artistStr, albumStr, source, dur)
		} else if isPlayed {
			isOdd := absIdx%2 == 1
			rowBg := s.ZebraBg
			useBg := isOdd
			if e.Selected {
				rowBg = s.SelectionBg
				useBg = true
			}
			played := s.PlayedRow
			if useBg {
				played = played.Background(rowBg)
			}
			switch {
			case e.Selected:
				rb := lipgloss.NewStyle().Background(rowBg)
				markerStr = s.AccentMark.Background(rowBg).Render("●") + rb.Render(" ")
			case useBg:
				rb := lipgloss.NewStyle().Background(rowBg)
				markerStr = played.Render("╵") + rb.Render(" ")
			default:
				markerStr = played.Render("╵ ")
			}
			titleStr = played.Render(titleStr)
			artistStr = played.Render(artistStr)
			albumStr = played.Render(albumStr)
			pos = played.Render(pos)
			source = played.Render(source)
			dur = played.Render(dur)
			sep := " "
			if useBg {
				sep = lipgloss.NewStyle().Background(rowBg).Render(" ")
			}
			lines[i] = assembleQueueRow(sep, hasSource, markerStr, pos, titleStr, artistStr, albumStr, source, dur)
		} else {
			isOdd := absIdx%2 == 1
			rowBg := s.ZebraBg
			useBg := isOdd
			if e.Selected {
				rowBg = s.SelectionBg
				useBg = true
			}
			if isTrackFav {
				titleStr = styles.WithZebraBg(s.Favorite, rowBg, useBg).Render(titleStr)
			}
			if isArtistFav {
				artistStr = styles.WithZebraBg(s.FavArtist, rowBg, useBg).Render(artistStr)
			}
			if isAlbumFav {
				albumStr = styles.WithZebraBg(s.FavAlbum, rowBg, useBg).Render(albumStr)
			}
			if useBg {
				rb := lipgloss.NewStyle().Background(rowBg)
				if !isTrackFav {
					titleStr = styles.WithZebraBg(s.TextPrimary, rowBg, true).Render(titleStr)
				}
				if !isArtistFav {
					artistStr = styles.WithZebraBg(s.TextPrimary, rowBg, true).Render(artistStr)
				}
				if !isAlbumFav {
					albumStr = styles.WithZebraBg(s.TextSecondary, rowBg, true).Render(albumStr)
				}
				if e.Selected {
					markerStr = s.AccentMark.Background(rowBg).Render("●") + rb.Render(" ")
				} else {
					markerStr = rb.Render(markerStr)
				}
				pos = styles.WithZebraBg(s.TextTertiary, rowBg, true).Render(pos)
				source = rb.Render(source)
				dur = styles.WithZebraBg(s.TextTertiary, rowBg, true).Render(dur)
				sep := rb.Render(" ")
				lines[i] = assembleQueueRow(sep, hasSource, markerStr, pos, titleStr, artistStr, albumStr, source, dur)
			} else {
				if !isTrackFav {
					titleStr = s.TextPrimary.Render(titleStr)
				}
				if !isArtistFav {
					artistStr = s.TextPrimary.Render(artistStr)
				}
				if !isAlbumFav {
					albumStr = s.TextSecondary.Render(albumStr)
				}
				lines[i] = assembleQueueRow(" ", hasSource, markerStr, s.TextTertiary.Render(pos), titleStr, artistStr, albumStr, source, s.TextTertiary.Render(dur))
			}
		}
		lines[i] = clampQueueLine(lines[i], v.width)
	}

	stripLine := v.renderStateStrip(s, len(v.entries), len(entries))

	bodyLines := lines
	if sorting.SortFieldHasBuckets(v.sortState.Field) {
		labels := make([]string, len(lines))
		for i := range lines {
			labels[i] = QueueEntryBucket(v.sortState.Field, entries[start+i])
		}
		sticky := ""
		if len(labels) > 0 {
			sticky = labels[0]
		}
		bodyLines = buckets.Chunk(lines, labels, buckets.ChunkOptions{
			StickyBucket:     sticky,
			MaxRows:          maxRows,
			DividerEvery:     buckets.DividerEvery,
			MakeDivider:      func() string { return renderQueueDivider(s, v.width) },
			MakeBucketHeader: func(label string) string { return renderQueueBucketHeader(s, label, v.width) },
		})
	}

	if end == len(entries) && len(entries) > 0 {
		bodyLines = append(bodyLines, s.Dim.Render("— end of queue —"))
	}

	var out string
	if stripLine != "" {
		out = stripLine + "\n" + header + "\n" + strings.Join(bodyLines, "\n")
	} else {
		out = header + "\n" + strings.Join(bodyLines, "\n")
	}
	if v.savingAsPlaylist {
		prompt := "Save as playlist: "
		if v.savingFromSelection {
			prompt = "Save selection as playlist: "
		}
		out += "\n" + s.Accent.Render(prompt) + v.saveInput.View()
	}
	return out
}
