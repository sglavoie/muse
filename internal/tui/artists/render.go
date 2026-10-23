package artists

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/layout"
	"github.com/sglavoie/muse/internal/tui/strip"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// emptyMessage renders the dim placeholder shown when a level has no items at
// all, truncated to the panel width when one is known.
func (v View) emptyMessage(msg string) string {
	if v.Width > 0 {
		msg = text.Truncate(msg, v.Width)
	}
	return v.Styles.Dim.Render(msg)
}

func (v View) renderArtists() string {
	items := v.EffectiveArtists()
	if len(v.Artists) == 0 {
		return v.emptyMessage("No artists yet — add music to your library")
	}
	stripLine := v.renderStateStrip(len(v.Artists), len(items))
	if len(items) == 0 {
		return strip.JoinStrip(stripLine, "(no matches)")
	}

	maxRows := v.visibleRows()
	start, end := layout.VisibleWindow(v.ScrollOffset, len(items), maxRows)

	lines := make([]string, end-start)
	for i, name := range items[start:end] {
		absIdx := start + i
		_, isFav := v.FavoriteArtists[name]
		labelW, countStr := text.RowLabelWidthAndCount(v.Width-2, v.ArtistCounts, name)
		label := text.FocusMarker(absIdx == v.Cursor) + text.FavPrefix(name, labelW, isFav)
		lines[i] = v.renderArtistRow(label, countStr, absIdx == v.Cursor, isFav, absIdx%2 == 1)
	}
	return strip.JoinStrip(stripLine, strings.Join(lines, "\n"))
}

// renderArtistRow styles one artist row's label and trailing count zone
// according to its focus, favourite and zebra state.
func (v View) renderArtistRow(label, countStr string, isFocused, isFav, isOdd bool) string {
	switch {
	case isFocused && isFav:
		a := v.Styles.FavArtistAccent
		return a.Render(label) + a.Render(countStr)
	case isFocused:
		a := v.Styles.Accent
		return a.Render(label) + a.Render(countStr)
	case isFav:
		labelStyled := styles.WithZebraBg(v.Styles.FavArtist, v.Styles.ZebraBg, isOdd).Render(label)
		return labelStyled + styles.RenderCountZone(v.Styles, countStr, isOdd)
	case isOdd:
		labelStyled := lipgloss.NewStyle().Background(v.Styles.ZebraBg).Render(label)
		return labelStyled + styles.RenderCountZone(v.Styles, countStr, true)
	default:
		return label + styles.RenderCountZone(v.Styles, countStr, false)
	}
}

func (v View) renderAlbums() string {
	labels := library.AlbumDisplayLabels(v.Albums)
	items := v.EffectiveAlbums()
	if len(v.Albums) == 0 {
		return v.emptyMessage("No albums found for this artist")
	}
	stripLine := v.renderStateStrip(len(v.Albums), len(items))
	if len(items) == 0 {
		return strip.JoinStrip(stripLine, "(no matches)")
	}

	maxRows := v.visibleRows()
	start, end := layout.VisibleWindow(v.ScrollOffset, len(items), maxRows)

	lines := make([]string, end-start)
	for i, name := range items[start:end] {
		absIdx := start + i
		_, isFav := v.FavoriteAlbums[name]
		display := text.FocusMarker(absIdx == v.Cursor) + text.FavPrefix(labels[name], v.Width-2, isFav)
		isOdd := absIdx%2 == 1
		switch {
		case absIdx == v.Cursor && isFav:
			display = v.Styles.FavAlbumAccent.Render(display)
		case absIdx == v.Cursor:
			display = v.Styles.Accent.Render(display)
		case isFav:
			display = styles.WithZebraBg(v.Styles.FavAlbum, v.Styles.ZebraBg, isOdd).Render(display)
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
		return v.emptyMessage("No tracks found for this album")
	}
	stripLine := v.renderStateStrip(len(v.Tracks), len(items))
	if len(items) == 0 {
		return strip.JoinStrip(stripLine, "(no matches)")
	}

	cols := ComputeTrack(v.Width)

	maxRows := v.visibleRows()
	start, end := layout.VisibleWindow(v.ScrollOffset, len(items), maxRows)
	window := items[start:end]
	showDisc := anyMultiDisc(window)

	lines := make([]string, len(window))
	for i, track := range window {
		lines[i] = v.renderTrackRow(v.trackRowAt(track, start+i, cols, showDisc))
	}
	rest := trackHeader(cols, v.Styles) + "\n" + strings.Join(lines, "\n")
	return strip.JoinStrip(stripLine, rest)
}

// anyMultiDisc reports whether the window contains a track from a disc past
// the first, which is what promotes positions to the "disc-track" form.
func anyMultiDisc(tracks []library.Track) bool {
	for _, track := range tracks {
		if track.DiscNumber > 1 {
			return true
		}
	}
	return false
}

// trackHeader builds the column-heading line above the track rows.
func trackHeader(cols TrackColumns, st styles.Styles) string {
	return strings.Repeat(" ", cols.SelectionMarker) + st.ColHead.Render(
		fmt.Sprintf("%-*s %-*s %*s",
			cols.Position, "#", cols.Title, "Title", cols.Duration, "Time"))
}

// trackRow carries one track row's pre-padded cells and visual state so each
// per-state renderer below takes a single argument.
type trackRow struct {
	marker   string
	pos      string
	title    string
	dur      string
	focused  bool
	playing  bool
	selected bool
	fav      bool
	odd      bool
}

// trackRowAt derives the row descriptor for the track at absolute index absIdx.
func (v View) trackRowAt(track library.Track, absIdx int, cols TrackColumns, showDisc bool) trackRow {
	isFocused := absIdx == v.Cursor
	isNowPlaying := v.NowPlayingHash != "" && track.Hash == v.NowPlayingHash
	_, isSelected := v.SelectedHashes[track.Hash]
	_, isTrackFav := v.FavoriteHashes[track.Hash]

	rawPos := text.FormatTrackPosSimple(track.TrackNumber, absIdx+1)
	if showDisc {
		rawPos = text.FormatTrackPos(track.TrackNumber, track.DiscNumber, absIdx+1)
	}

	return trackRow{
		marker:   text.RowMarker(isSelected, isFocused, isNowPlaying, v.NowPlayingIcon),
		pos:      fmt.Sprintf("%-*s", cols.Position, rawPos),
		title:    text.FavPrefix(track.Title, cols.Title, isTrackFav),
		dur:      fmt.Sprintf("%*s", cols.Duration, text.FormatDurationSecs(track.Duration)),
		focused:  isFocused,
		playing:  isNowPlaying,
		selected: isSelected,
		fav:      isTrackFav,
		odd:      absIdx%2 == 1,
	}
}

func (v View) renderTrackRow(r trackRow) string {
	switch {
	case r.focused:
		return v.renderFocusedTrackRow(r)
	case r.playing:
		return v.renderNowPlayingTrackRow(r)
	default:
		return v.renderRestingTrackRow(r)
	}
}

func (v View) renderFocusedTrackRow(r trackRow) string {
	accent := v.Styles.Accent
	sep := accent.Render(" ")
	title := styles.AccentFavRender(accent, v.Styles.FavoriteAccent, r.fav, r.title)
	return accent.Render(r.marker+r.pos) + sep + title + sep + accent.Render(r.dur)
}

func (v View) renderNowPlayingTrackRow(r trackRow) string {
	np := v.Styles.NowPlayingRow
	sep := np.Render(" ")
	title := v.Styles.NowPlayingTitle.Render(r.title)
	return np.Render(r.marker+r.pos) + sep + title + sep + np.Render(r.dur)
}

// renderRestingTrackRow renders a row that is neither focused nor playing,
// resolving which background — zebra, bulk-selection, or none — the row wears.
func (v View) renderRestingTrackRow(r trackRow) string {
	rowBg := v.Styles.ZebraBg
	useBg := r.odd
	if r.selected {
		rowBg = v.Styles.SelectionBg
		useBg = true
	}
	title := r.title
	if r.fav {
		title = styles.WithZebraBg(v.Styles.Favorite, rowBg, useBg).Render(title)
	}
	if !useBg {
		return v.renderUnfilledTrackRow(r, title)
	}
	return v.renderFilledTrackRow(r, title, rowBg)
}

// renderFilledTrackRow renders a resting row whose every cell carries rowBg,
// so separators and gaps have to be painted too.
func (v View) renderFilledTrackRow(r trackRow, title string, rowBg color.Color) string {
	zb := lipgloss.NewStyle().Background(rowBg)
	if !r.fav {
		title = styles.WithZebraBg(v.Styles.TextPrimary, rowBg, true).Render(title)
	}
	markerPart := zb.Render(r.marker)
	if r.selected {
		markerPart = v.Styles.AccentMark.Background(rowBg).Render("●") + zb.Render(" ")
	}
	posPart := styles.WithZebraBg(v.Styles.TextTertiary, rowBg, true).Render(r.pos)
	durPart := styles.WithZebraBg(v.Styles.TextTertiary, rowBg, true).Render(r.dur)
	sep := zb.Render(" ")
	return markerPart + posPart + sep + title + sep + durPart
}

// renderUnfilledTrackRow renders a resting row with no row background.
func (v View) renderUnfilledTrackRow(r trackRow, title string) string {
	if !r.fav {
		title = v.Styles.TextPrimary.Render(title)
	}
	return r.marker + v.Styles.TextTertiary.Render(r.pos) + " " + title + " " + v.Styles.TextTertiary.Render(r.dur)
}

// renderStateStrip builds the unified single-line state strip carrying the
// breadcrumb, sort, filter, counts, and an optional "esc ←" hint at
// drill-down levels. total is the full item count before filtering;
// filtered is the count after filtering (equal to total when no filter is
// active).
func (v View) renderStateStrip(total, filtered int) string {
	breadcrumb := v.breadcrumb()
	hint := ""
	if len(breadcrumb) > 1 {
		hint = "esc ←"
	}
	sortChunk := strip.SortChunk{Label: v.StripSortLabel, Indicator: v.StripSortIndicator}
	start, end := layout.VisibleWindow(v.ScrollOffset, filtered, v.visibleRows())
	counts := strip.CountsChunk{
		WindowStart: start, WindowEnd: end,
		Unit:      []string{"artists", "albums", "tracks"}[v.Level],
		Filtered:  filtered,
		Total:     total,
		Filtering: v.FilterText != "",
	}
	var sel *strip.SelectionChunk
	if n := len(v.SelectedHashes); n > 0 {
		sel = &strip.SelectionChunk{Count: n}
	}
	return strip.Render(v.Width, breadcrumb, sortChunk, v.filterChunk(), counts, sel, hint, v.Styles)
}

// breadcrumb returns the strip's trail from the root label down to the
// currently opened artist and album.
func (v View) breadcrumb() []string {
	root := v.StripViewLabel
	if root == "" {
		root = "Artists"
	}
	breadcrumb := []string{root}
	switch v.Level {
	case AlbumLevel:
		if v.SelectedArtist != "" {
			breadcrumb = append(breadcrumb, v.SelectedArtist)
		}
	case TrackLevel:
		if v.SelectedArtist != "" {
			breadcrumb = append(breadcrumb, v.SelectedArtist)
		}
		if v.SelectedAlbum != "" {
			breadcrumb = append(breadcrumb, library.AlbumDisplayLabel(v.SelectedAlbum, v.Albums))
		}
	}
	return breadcrumb
}

// filterChunk describes the filter zone of the strip: the live input view
// while typing, or the placeholder hint when the query is still empty.
func (v View) filterChunk() strip.FilterChunk {
	f := strip.FilterChunk{Active: v.FilterActive, Text: v.FilterText}
	if !v.FilterActive {
		return f
	}
	if v.FilterText == "" && v.FilterHint != "" {
		f.Hint = v.Styles.TextSecondary.Render(v.FilterHint)
		return f
	}
	f.InputView = v.FilterView
	return f
}
