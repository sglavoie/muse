package albums

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/layout"
	"github.com/sglavoie/muse/internal/tui/strip"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// Render returns the string representation of the albums panel.
func (v View) Render() string {
	switch v.Level {
	case TrackLevel:
		return v.renderTracks()
	default:
		return v.renderAlbums()
	}
}

func (v View) renderAlbums() string {
	labels := library.AlbumDisplayLabels(v.Albums)
	items := v.EffectiveAlbums()
	if len(v.Albums) == 0 {
		return v.renderEmpty("No albums yet — add music to your library")
	}
	width := contentWidth(v.Width)
	stripLine := v.renderStateStrip(len(v.Albums), len(items))
	if len(items) == 0 {
		return strip.JoinStrip(stripLine, "(no matches)")
	}

	start, end := layout.VisibleWindow(v.ScrollOffset, len(items), v.visibleRows())

	lines := make([]string, end-start)
	for i, name := range items[start:end] {
		lines[i] = v.renderAlbumRow(name, labels[name], start+i, width)
	}
	return strip.JoinStrip(stripLine, strings.Join(lines, "\n"))
}

// renderEmpty renders a placeholder message for an empty panel, truncated to
// the panel width once one is known.
func (v View) renderEmpty(msg string) string {
	if v.Width > 0 {
		msg = text.Truncate(msg, v.Width)
	}
	return v.Styles.Dim.Render(msg)
}

// renderAlbumRow renders one album name followed by its right-aligned track
// count zone.
func (v View) renderAlbumRow(name, displayLabel string, absIdx, width int) string {
	_, isFav := v.FavoriteAlbums[name]
	isOdd := absIdx%2 == 1
	labelW, countStr := text.RowLabelWidthAndCount(width-2, v.AlbumCounts, name)
	label := text.FocusMarker(absIdx == v.Cursor) + text.FavPrefix(displayLabel, labelW, isFav)
	switch {
	case absIdx == v.Cursor:
		a := v.Styles.Accent
		if isFav {
			a = v.Styles.FavAlbumAccent
		}
		return a.Render(label) + a.Render(countStr)
	case isFav:
		labelStyled := styles.WithZebraBg(v.Styles.FavAlbum, v.Styles.ZebraBg, isOdd).Render(label)
		return labelStyled + styles.RenderCountZone(v.Styles, countStr, isOdd)
	case isOdd:
		labelStyled := lipgloss.NewStyle().Background(v.Styles.ZebraBg).Render(label)
		return labelStyled + styles.RenderCountZone(v.Styles, countStr, true)
	default:
		return label + styles.RenderCountZone(v.Styles, countStr, false)
	}
}

func (v View) renderTracks() string {
	items := v.EffectiveTracks()
	if len(v.Tracks) == 0 {
		return v.renderEmpty("No tracks found for this album")
	}
	width := contentWidth(v.Width)
	stripLine := v.renderStateStrip(len(v.Tracks), len(items))
	if len(items) == 0 {
		return strip.JoinStrip(stripLine, "(no matches)")
	}

	cols := ComputeTrack(width)
	header := ansi.Truncate(v.renderTrackHeader(cols), width, "")

	start, end := layout.VisibleWindow(v.ScrollOffset, len(items), v.visibleRows())
	window := items[start:end]
	showDisc := anyMultiDisc(window)

	lines := make([]string, len(window))
	for i, t := range window {
		lines[i] = ansi.Truncate(v.renderTrackRow(t, cols, start+i, showDisc), width, "")
	}
	rest := header + "\n" + strings.Join(lines, "\n")
	return strip.JoinStrip(stripLine, rest)
}

func (v View) renderTrackHeader(cols TrackColumns) string {
	return strings.Repeat(" ", cols.SelectionMarker) + v.Styles.ColHead.Render(
		text.Pad("#", cols.Position)+" "+
			text.Pad("Title", cols.Title)+" "+
			text.Pad("Artist", cols.Artist)+" "+
			text.PadLeft("Time", cols.Duration))
}

// anyMultiDisc reports whether the visible window spans more than one disc, in
// which case positions are rendered as "disc-track" rather than bare tracks.
func anyMultiDisc(window []library.Track) bool {
	for _, t := range window {
		if t.DiscNumber > 1 {
			return true
		}
	}
	return false
}

// trackRow holds one row's pre-padded cells plus the flags deciding how they
// are styled.
type trackRow struct {
	marker, pos, title, artist, dur string
	trackFav, artistFav, selected   bool
}

func (v View) renderTrackRow(t library.Track, cols TrackColumns, absIdx int, showDisc bool) string {
	isFocused := absIdx == v.Cursor
	isNowPlaying := v.NowPlayingHash != "" && t.Hash == v.NowPlayingHash
	_, isSelected := v.SelectedHashes[t.Hash]
	_, isTrackFav := v.FavoriteHashes[t.Hash]
	_, isArtistFav := v.FavoriteArtists[t.Artist]

	rawPos := text.FormatTrackPosSimple(t.TrackNumber, absIdx+1)
	if showDisc {
		rawPos = text.FormatTrackPos(t.TrackNumber, t.DiscNumber, absIdx+1)
	}
	r := trackRow{
		marker:    text.RowMarker(isSelected, isFocused, isNowPlaying, v.NowPlayingIcon),
		pos:       text.Pad(rawPos, cols.Position),
		title:     text.FavPrefix(t.Title, cols.Title, isTrackFav),
		artist:    text.FavPrefix(t.Artist, cols.Artist, isArtistFav),
		dur:       text.PadLeft(text.FormatDurationSecs(t.Duration), cols.Duration),
		trackFav:  isTrackFav,
		artistFav: isArtistFav,
		selected:  isSelected,
	}
	switch {
	case isFocused:
		return v.renderFocusedTrackRow(r)
	case isNowPlaying:
		return v.renderNowPlayingTrackRow(r)
	case isSelected:
		return v.renderFilledTrackRow(r, v.Styles.SelectionBg)
	case absIdx%2 == 1:
		return v.renderFilledTrackRow(r, v.Styles.ZebraBg)
	default:
		return v.renderPlainTrackRow(r)
	}
}

func (v View) renderFocusedTrackRow(r trackRow) string {
	a := v.Styles.Accent
	sep := a.Render(" ")
	title := styles.AccentFavRender(a, v.Styles.FavoriteAccent, r.trackFav, r.title)
	artist := styles.AccentFavRender(a, v.Styles.FavArtistAccent, r.artistFav, r.artist)
	return a.Render(r.marker+r.pos) + sep + title + sep + artist + sep + a.Render(r.dur)
}

func (v View) renderNowPlayingTrackRow(r trackRow) string {
	np := v.Styles.NowPlayingRow
	sep := np.Render(" ")
	title := v.Styles.NowPlayingTitle.Render(r.title)
	artist := v.Styles.TextPrimary.Background(v.Styles.NowPlayingBg).Render(r.artist)
	if r.artistFav {
		artist = v.Styles.NowPlayingRowFavArtist.Render(r.artist)
	}
	return np.Render(r.marker+r.pos) + sep + title + sep + artist + sep + np.Render(r.dur)
}

// renderFilledTrackRow renders a zebra or bulk-selected row. Every cell,
// separator included, carries rowBg so no unpainted gap leaks through.
func (v View) renderFilledTrackRow(r trackRow, rowBg color.Color) string {
	zb := lipgloss.NewStyle().Background(rowBg)
	title := styles.WithZebraBg(v.Styles.TextPrimary, rowBg, true).Render(r.title)
	if r.trackFav {
		title = styles.WithZebraBg(v.Styles.Favorite, rowBg, true).Render(r.title)
	}
	artist := styles.WithZebraBg(v.Styles.TextPrimary, rowBg, true).Render(r.artist)
	if r.artistFav {
		artist = styles.WithZebraBg(v.Styles.FavArtist, rowBg, true).Render(r.artist)
	}
	marker := zb.Render(r.marker)
	if r.selected {
		marker = v.Styles.AccentMark.Background(rowBg).Render("●") + zb.Render(" ")
	}
	pos := styles.WithZebraBg(v.Styles.TextTertiary, rowBg, true).Render(r.pos)
	dur := styles.WithZebraBg(v.Styles.TextTertiary, rowBg, true).Render(r.dur)
	sep := zb.Render(" ")
	return marker + pos + sep + title + sep + artist + sep + dur
}

func (v View) renderPlainTrackRow(r trackRow) string {
	// A favorite cell keeps its own colour and no background: WithZebraBg
	// returns the style unchanged when the row is not filled.
	title := v.Styles.TextPrimary.Render(r.title)
	if r.trackFav {
		title = v.Styles.Favorite.Render(r.title)
	}
	artist := v.Styles.TextPrimary.Render(r.artist)
	if r.artistFav {
		artist = v.Styles.FavArtist.Render(r.artist)
	}
	return r.marker + v.Styles.TextTertiary.Render(r.pos) + " " + title + " " + artist + " " + v.Styles.TextTertiary.Render(r.dur)
}

// renderStateStrip builds the unified single-line state strip carrying the
// breadcrumb, sort, filter, counts, and an "esc ←" hint at the track
// level. total is the full item count before filtering; filtered is the
// count after filtering (equal to total when no filter is active).
func (v View) renderStateStrip(total, filtered int) string {
	root := v.StripViewLabel
	if root == "" {
		root = "Albums"
	}
	breadcrumb := []string{root}
	if v.Level == TrackLevel && v.SelectedAlbum != "" {
		breadcrumb = append(breadcrumb, library.AlbumLabel(v.SelectedAlbum))
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
		Unit:      []string{"albums", "tracks"}[v.Level],
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

func contentWidth(width int) int {
	if width > 0 {
		return width
	}
	return 80
}
