package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/tui/buckets"
	"github.com/sglavoie/muse/internal/tui/strip"
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
func (v QueueView) render(s styles.Styles, currentPlayingIndex int, playbackState PlaybackState, favHashes, favArtists, favAlbums map[string]struct{}) string {
	entries := v.filteredEntries()
	if len(entries) == 0 {
		if v.filter.Text != "" {
			return "(no matches)"
		}
		return s.RenderEmptyCard(v.width, v.visibleRows(), "Queue is empty", []styles.EmptyAction{
			{Key: "1", Label: "go to library"},
			{Key: "r", Label: "random album"},
			{Key: "?", Label: "help"},
		})
	}

	const (
		posWidth    = 4
		durWidth    = 10
		markerWidth = 2
		sepCount    = 5
	)

	titleWidth := 20
	artistWidth := 15
	albumWidth := 15
	sourceWidth := 10
	if v.width > 0 {
		fixed := posWidth + durWidth + markerWidth + sepCount
		flex := v.width - fixed
		flex = max(flex, 4)
		titleWidth = flex * 30 / 80
		artistWidth = flex * 20 / 80
		albumWidth = flex * 20 / 80
		sourceWidth = flex - titleWidth - artistWidth - albumWidth
	}
	titleWidth = max(titleWidth, 1)
	artistWidth = max(artistWidth, 1)
	albumWidth = max(albumWidth, 1)
	sourceWidth = max(sourceWidth, 1)

	maxRows := v.visibleRows()
	start := min(v.scrollOffset, len(entries))
	end := min(len(entries), start+maxRows)

	showDisc := false
	for _, e := range entries[start:end] {
		if e.DiscNumber > 1 {
			showDisc = true
			break
		}
	}

	hasSource := false
	for _, e := range entries[start:end] {
		if e.SourcePlaylist != "" {
			hasSource = true
			break
		}
	}
	if !hasSource {
		titleWidth += sourceWidth
		sourceWidth = 0
	}

	titleHead, artistHead, albumHead, sourceHead, timeHead := "Title", "Artist", "Album", "Source", "Time"
	// headerBody excludes the leading marker-width prefix so the dotted
	// underline only spans the actual data columns (not the marker gutter).
	var headerBody string
	if hasSource {
		headerBody = fmt.Sprintf("%-*s %-*s %-*s %-*s %-*s %*s",
			posWidth, "#", titleWidth, titleHead, artistWidth, artistHead, albumWidth, albumHead, sourceWidth, sourceHead, durWidth, timeHead)
	} else {
		headerBody = fmt.Sprintf("%-*s %-*s %-*s %-*s %*s",
			posWidth, "#", titleWidth, titleHead, artistWidth, artistHead, albumWidth, albumHead, durWidth, timeHead)
	}
	header := strings.Repeat(" ", markerWidth) + s.ColHead.Render(headerBody)

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

		var rawPos string
		if showDisc {
			rawPos = text.FormatTrackPos(e.TrackNumber, e.DiscNumber, absIdx+1)
		} else {
			rawPos = text.FormatTrackPosSimple(e.TrackNumber, absIdx+1)
		}
		pos := fmt.Sprintf("%4s", rawPos)
		source := padQueue(e.SourcePlaylist, sourceWidth)
		dur := fmt.Sprintf("%*s", durWidth, formatQueueDuration(e.DurationMs))

		_, isTrackFav := favHashes[e.Hash]
		_, isArtistFav := favArtists[e.Artist]
		_, isAlbumFav := favAlbums[e.Album]

		titleStr := text.FavPrefix(e.Title, titleWidth, isTrackFav)
		artistStr := text.FavPrefix(e.Artist, artistWidth, isArtistFav)
		albumStr := text.FavPrefix(e.Album, albumWidth, isAlbumFav)
		if e.Missing { //nolint:gocritic // ifElseChain: branches have fundamentally different rendering logic
			plainLine := assembleQueueRow(" ", hasSource, markerStr, pos, padQueue(e.Title, titleWidth), padQueue(e.Artist, artistWidth), padQueue(e.Album, albumWidth), source, dur)
			lines[i] = s.Dim.Render(plainLine + " [missing]")
		} else if isFocused {
			bg := s.Accent.GetBackground()
			sep := s.Accent.Render(" ")
			if isNowPlaying {
				npText, npFav, npFavArtist, npFavAlbum := nowPlayingTextStyles(s, playbackState)
				titleStr = npText.Background(bg).Render(titleStr)
				artistStr = npText.Background(bg).Render(artistStr)
				albumStr = npText.Background(bg).Render(albumStr)
				if isTrackFav {
					titleStr = npFav.Background(bg).Render(text.FavPrefix(e.Title, titleWidth, true))
				}
				if isArtistFav {
					artistStr = npFavArtist.Background(bg).Render(text.FavPrefix(e.Artist, artistWidth, true))
				}
				if isAlbumFav {
					albumStr = npFavAlbum.Background(bg).Render(text.FavPrefix(e.Album, albumWidth, true))
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
				titleStr = npFav.Background(s.NowPlayingBg).Render(text.FavPrefix(e.Title, titleWidth, true))
			}
			if isArtistFav {
				artistStr = npFavArtist.Background(s.NowPlayingBg).Render(text.FavPrefix(e.Artist, artistWidth, true))
			}
			if isAlbumFav {
				albumStr = npFavAlbum.Background(s.NowPlayingBg).Render(text.FavPrefix(e.Album, albumWidth, true))
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
			if e.Selected {
				if useBg {
					rb := lipgloss.NewStyle().Background(rowBg)
					markerStr = s.AccentMark.Background(rowBg).Render("●") + rb.Render(" ")
				} else {
					markerStr = s.AccentMark.Render("●") + " "
				}
			} else if useBg {
				rb := lipgloss.NewStyle().Background(rowBg)
				markerStr = played.Render("╵") + rb.Render(" ")
			} else {
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
	}

	stripLine := v.renderStateStrip(s, len(v.entries), len(entries))

	bodyLines := lines
	if SortFieldHasBuckets(v.sortState.Field) {
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

// renderStateStrip builds the unified single-line state strip carrying
// the view label, sort, filter, and counts for the queue. total is the
// full entry count before filtering; filtered is the count after filter
// is applied (equal to total when no filter is active).
func (v QueueView) renderStateStrip(s styles.Styles, total, filtered int) string {
	var sortChunk strip.SortChunk
	if v.sortState.Field != SortDefault {
		sortChunk = strip.SortChunk{
			Label:     SortFieldLabel(v.sortState.Field),
			Indicator: SortDirIndicator(v.sortState.Dir),
		}
	}
	f := strip.FilterChunk{Active: v.filter.Active, Text: v.filter.Text}
	if v.filter.Active {
		if v.filter.Text == "" {
			f.Hint = s.Dim.Render("a: artist  t: title  l: album")
		} else {
			f.InputView = v.filter.View()
		}
	}
	counts := strip.CountsChunk{
		Filtered:  filtered,
		Total:     total,
		Filtering: v.filter.Text != "",
	}
	var sel *strip.SelectionChunk
	if n := len(v.selectedHashes()); n > 0 {
		sel = &strip.SelectionChunk{Count: n}
	}
	return strip.Render(v.width, []string{"Queue"}, sortChunk, f, counts, sel, "", s)
}

// nowPlayingIndicator returns the icon string and its style for the
// now-playing marker based on the current playback state. The returned icon
// is unstyled so callers can apply additional styling (e.g. Accent background).
func nowPlayingIndicator(s styles.Styles, state PlaybackState) (icon string, style lipgloss.Style) {
	switch state {
	case PlaybackPaused:
		return "⏸", s.Warning
	case PlaybackStopped:
		return "■", s.Error
	default:
		return "▶", s.Success
	}
}

// nowPlayingTextStyles returns the text styles for the now-playing row based
// on playback state. Returns (base, favorite, favArtist, favAlbum) styles.
// When playing, these are the standard NowPlaying* styles (green). When
// paused or stopped, the base style shifts to Warning (gold) or Error (red)
// while favorite styles are preserved since they already carry distinct colors.
func nowPlayingTextStyles(s styles.Styles, state PlaybackState) (base, fav, favArtist, favAlbum lipgloss.Style) {
	switch state {
	case PlaybackPaused:
		base = s.Warning.Bold(true)
		return base, s.NowPlayingFavorite, s.NowPlayingFavArtist, s.NowPlayingFavAlbum
	case PlaybackStopped:
		base = s.Error.Bold(true)
		return base, s.NowPlayingFavorite, s.NowPlayingFavArtist, s.NowPlayingFavAlbum
	default:
		return s.NowPlaying, s.NowPlayingFavorite, s.NowPlayingFavArtist, s.NowPlayingFavAlbum
	}
}

// assembleQueueRow joins pre-styled queue row columns using sep as the column
// separator. When hasSource is false the source column is omitted entirely.
func assembleQueueRow(sep string, hasSource bool, marker, pos, title, artist, album, source, dur string) string {
	if hasSource {
		return marker + pos + sep + title + sep + artist + sep + album + sep + source + sep + dur
	}
	return marker + pos + sep + title + sep + artist + sep + album + sep + dur
}

// formatQueueDuration converts a duration in milliseconds to a "m:ss" or
// "h:mm:ss" string. Returns "--:--" when ms is zero or negative.
func formatQueueDuration(ms int64) string {
	return text.FormatDurationMs(ms)
}

// renderQueueDivider builds a dim horizontal divider line spanning the
// queue panel width.
func renderQueueDivider(s styles.Styles, width int) string {
	if width <= 0 {
		width = 1
	}
	return s.Divider.Render(strings.Repeat("─", width))
}

// renderQueueBucketHeader builds a bucket-header line padded to the queue
// panel width so neighbouring rows align.
func renderQueueBucketHeader(s styles.Styles, label string, width int) string {
	if label == "" {
		return ""
	}
	padded := label
	if width > 0 {
		if pw := ansi.StringWidth(label); pw < width {
			padded = label + strings.Repeat(" ", width-pw)
		}
	}
	return s.BucketHeader.Render(padded)
}

// padQueue truncates or right-pads s with spaces to exactly width display columns.
func padQueue(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = truncateTitle(s, width)
	n := ansi.StringWidth(s)
	if n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return s
}
