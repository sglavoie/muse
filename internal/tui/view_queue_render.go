package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/tui/styles"
)

// render builds the queue panel string using a 6-column layout:
//
//	Position (~4 chars fixed) | Title (~30% of width) | Artist (~20% of width) | Album (~20% of width) | Source (~10% of width) | Duration (~10 chars fixed)
//
// Proportional columns resize on WindowSizeMsg; fixed-width columns are
// constant. Missing entries are wrapped in ANSI dim escape codes so they
// appear grayed out in the terminal.
func (v QueueView) render(s styles.Styles, currentPlayingIndex int, favHashes, favArtists, favAlbums map[string]struct{}) string {
	entries := v.filteredEntries()
	if len(entries) == 0 {
		if v.filter.Text != "" {
			return "(no matches)"
		}
		return "(empty)"
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
	header := fmt.Sprintf("  %-*s %-*s %-*s %-*s %-*s %*s",
		posWidth, "#", titleWidth, titleHead, artistWidth, artistHead, albumWidth, albumHead, sourceWidth, sourceHead, durWidth, timeHead)

	lines := make([]string, end-start)
	for i := range end - start {
		absIdx := start + i
		e := entries[absIdx]
		isFocused := absIdx == v.cursor
		isNowPlaying := currentPlayingIndex >= 0 && e.origIdx == currentPlayingIndex

		var markerStr string
		switch {
		case e.Selected && isFocused:
			markerStr = "● "
		case e.Selected:
			markerStr = s.AccentMark.Render("●") + " "
		case isNowPlaying && isFocused:
			markerStr = "▶ "
		case isNowPlaying:
			markerStr = "▶ "
		default:
			markerStr = "  "
		}

		pos := fmt.Sprintf("%4d", absIdx+1)
		source := padQueue(e.SourcePlaylist, sourceWidth)
		dur := fmt.Sprintf("%*s", durWidth, formatQueueDuration(e.DurationMs))

		_, isTrackFav := favHashes[e.Hash]
		_, isArtistFav := favArtists[e.Artist]
		_, isAlbumFav := favAlbums[e.Album]

		titleStr := padQueue(e.Title, titleWidth)
		artistStr := padQueue(e.Artist, artistWidth)
		albumStr := padQueue(e.Album, albumWidth)
		if e.Missing { //nolint:gocritic // ifElseChain: branches have fundamentally different rendering logic
			plainLine := markerStr + pos + " " + padQueue(e.Title, titleWidth) + " " + padQueue(e.Artist, artistWidth) + " " + padQueue(e.Album, albumWidth) + " " + source + " " + dur
			lines[i] = s.Dim.Render(plainLine + " [missing]")
		} else if isFocused {
			bg := s.Accent.GetBackground()
			sep := s.Accent.Render(" ")
			if isNowPlaying {
				titleStr = s.NowPlaying.Background(bg).Render(titleStr)
				artistStr = s.NowPlaying.Background(bg).Render(artistStr)
				albumStr = s.NowPlaying.Background(bg).Render(albumStr)
				if isTrackFav {
					titleStr = s.NowPlayingFavorite.Background(bg).Render(padQueue(e.Title, titleWidth))
				}
				if isArtistFav {
					artistStr = s.NowPlayingFavArtist.Background(bg).Render(padQueue(e.Artist, artistWidth))
				}
				if isAlbumFav {
					albumStr = s.NowPlayingFavAlbum.Background(bg).Render(padQueue(e.Album, albumWidth))
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
			lines[i] = markerStr + pos + sep + titleStr + sep + artistStr + sep + albumStr + sep + source + sep + dur
		} else if isNowPlaying {
			titleStr = s.NowPlaying.Render(titleStr)
			artistStr = s.NowPlaying.Render(artistStr)
			albumStr = s.NowPlaying.Render(albumStr)
			if isTrackFav {
				titleStr = s.NowPlayingFavorite.Render(padQueue(e.Title, titleWidth))
			}
			if isArtistFav {
				artistStr = s.NowPlayingFavArtist.Render(padQueue(e.Artist, artistWidth))
			}
			if isAlbumFav {
				albumStr = s.NowPlayingFavAlbum.Render(padQueue(e.Album, albumWidth))
			}
			lines[i] = markerStr + pos + " " + titleStr + " " + artistStr + " " + albumStr + " " + source + " " + dur
		} else {
			if isTrackFav {
				titleStr = s.Favorite.Render(titleStr)
			}
			if isArtistFav {
				artistStr = s.FavArtist.Render(artistStr)
			}
			if isAlbumFav {
				albumStr = s.FavAlbum.Render(albumStr)
			}
			lines[i] = markerStr + pos + " " + titleStr + " " + artistStr + " " + albumStr + " " + source + " " + dur
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

// formatQueueDuration converts a duration in milliseconds to a "m:ss" or
// "h:mm:ss" string. Returns "--:--" when ms is zero or negative.
func formatQueueDuration(ms int64) string {
	return FormatDurationMs(ms)
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