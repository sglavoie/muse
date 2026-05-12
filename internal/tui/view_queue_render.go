package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

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
		return s.Dim.Render("Press Enter in Library to add tracks  ·  ? for help")
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
	if v.sortState.Field != SortDefault {
		ind := SortDirIndicator(v.sortState.Dir)
		switch v.sortState.Field {
		case SortTitle:
			titleHead += ind
		case SortArtist:
			artistHead += ind
		case SortAlbum:
			albumHead += ind
		case SortSource:
			sourceHead += ind
		case SortDuration:
			timeHead += ind
		}
	}
	var header string
	if hasSource {
		header = fmt.Sprintf("  %-*s %-*s %-*s %-*s %-*s %*s",
			posWidth, "#", titleWidth, titleHead, artistWidth, artistHead, albumWidth, albumHead, sourceWidth, sourceHead, durWidth, timeHead)
	} else {
		header = fmt.Sprintf("  %-*s %-*s %-*s %-*s %*s",
			posWidth, "#", titleWidth, titleHead, artistWidth, artistHead, albumWidth, albumHead, durWidth, timeHead)
	}

	lines := make([]string, end-start)
	for i := range end - start {
		absIdx := start + i
		e := entries[absIdx]
		isFocused := absIdx == v.cursor
		isNowPlaying := currentPlayingIndex >= 0 && e.origIdx == currentPlayingIndex

		// Determine the now-playing icon and style based on playback state.
		npIcon, npStyle := nowPlayingIndicator(s, playbackState)

		var markerStr string
		switch {
		case e.Selected && isFocused:
			markerStr = "● "
		case e.Selected:
			markerStr = s.AccentMark.Render("●") + " "
		case isFocused && isNowPlaying:
			markerStr = npIcon + " "
		case isFocused:
			markerStr = s.AccentMark.Render("▎") + " "
		case isNowPlaying:
			markerStr = npIcon + " "
		default:
			markerStr = "  "
		}

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
			var plainLine string
			if hasSource {
				plainLine = markerStr + pos + " " + padQueue(e.Title, titleWidth) + " " + padQueue(e.Artist, artistWidth) + " " + padQueue(e.Album, albumWidth) + " " + source + " " + dur
			} else {
				plainLine = markerStr + pos + " " + padQueue(e.Title, titleWidth) + " " + padQueue(e.Artist, artistWidth) + " " + padQueue(e.Album, albumWidth) + " " + dur
			}
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
			if hasSource {
				lines[i] = markerStr + pos + sep + titleStr + sep + artistStr + sep + albumStr + sep + source + sep + dur
			} else {
				lines[i] = markerStr + pos + sep + titleStr + sep + artistStr + sep + albumStr + sep + dur
			}
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
			if hasSource {
				lines[i] = markerStr + pos + sep + titleStr + sep + artistStr + sep + albumStr + sep + source + sep + dur
			} else {
				lines[i] = markerStr + pos + sep + titleStr + sep + artistStr + sep + albumStr + sep + dur
			}
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
				markerStr = rb.Render(markerStr)
				pos = styles.WithZebraBg(s.TextTertiary, rowBg, true).Render(pos)
				source = rb.Render(source)
				dur = styles.WithZebraBg(s.TextTertiary, rowBg, true).Render(dur)
				sep := rb.Render(" ")
				if hasSource {
					lines[i] = markerStr + pos + sep + titleStr + sep + artistStr + sep + albumStr + sep + source + sep + dur
				} else {
					lines[i] = markerStr + pos + sep + titleStr + sep + artistStr + sep + albumStr + sep + dur
				}
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
				if hasSource {
					lines[i] = markerStr + s.TextTertiary.Render(pos) + " " + titleStr + " " + artistStr + " " + albumStr + " " + source + " " + s.TextTertiary.Render(dur)
				} else {
					lines[i] = markerStr + s.TextTertiary.Render(pos) + " " + titleStr + " " + artistStr + " " + albumStr + " " + s.TextTertiary.Render(dur)
				}
			}
		}
	}

	var filterLine string
	if v.filter.Active {
		filterLine = "Filter: " + v.filter.View() + "\n"
	} else if v.filter.Text != "" {
		ft := v.filter.Text
		maxLen := v.width - len("Filter: ") - 1
		if maxLen > 0 && ansi.StringWidth(ft) > maxLen {
			ft = ansi.Truncate(ft, maxLen-1, "…")
		}
		filterLine = "Filter: " + ft + "\n"
	}

	out := filterLine + s.ColHead.Render(header) + "\n" + strings.Join(lines, "\n")
	if v.savingAsPlaylist {
		prompt := "Save as playlist: "
		if v.savingFromSelection {
			prompt = "Save selection as playlist: "
		}
		out += "\n" + s.Accent.Render(prompt) + v.saveInput.View()
	}
	return out
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

// formatQueueDuration converts a duration in milliseconds to a "m:ss" or
// "h:mm:ss" string. Returns "--:--" when ms is zero or negative.
func formatQueueDuration(ms int64) string {
	return text.FormatDurationMs(ms)
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
