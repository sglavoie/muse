package genres

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/library"
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

func (v View) renderGenres() string {
	items := v.EffectiveGenres()
	if len(v.Genres) == 0 {
		return v.emptyMessage("No genres yet — add music to your library")
	}
	stripLine := v.renderStateStrip(len(v.Genres), len(items))
	if len(items) == 0 {
		return strip.JoinStrip(stripLine, "(no matches)")
	}

	start, end, cursor := v.renderWindow(len(items))

	lines := make([]string, end-start)
	for i, name := range items[start:end] {
		absIdx := start + i
		labelW, countStr := text.RowLabelWidthAndCount(v.Width-2, v.GenreCounts, name)
		label := text.FocusMarker(absIdx == cursor) + text.Pad(name, labelW)
		lines[i] = v.renderGenreRow(label, countStr, absIdx == cursor, absIdx%2 == 1)
	}
	return strip.JoinStrip(stripLine, strings.Join(lines, "\n"))
}

// renderGenreRow styles one genre row's label and trailing count zone
// according to its focus and zebra state.
func (v View) renderGenreRow(label, countStr string, isFocused, isOdd bool) string {
	switch {
	case isFocused:
		a := v.Styles.Accent
		return a.Render(label) + a.Render(countStr)
	case isOdd:
		labelStyled := lipgloss.NewStyle().Background(v.Styles.ZebraBg).Render(label)
		return labelStyled + styles.RenderCountZone(v.Styles, countStr, true)
	default:
		return label + styles.RenderCountZone(v.Styles, countStr, false)
	}
}

func (v View) renderArtists() string {
	items := v.EffectiveArtists()
	if len(v.Artists) == 0 {
		return v.emptyMessage("No artists found for this genre")
	}
	stripLine := v.renderStateStrip(len(v.Artists), len(items))
	if len(items) == 0 {
		return strip.JoinStrip(stripLine, "(no matches)")
	}

	start, end, cursor := v.renderWindow(len(items))

	lines := make([]string, end-start)
	for i, name := range items[start:end] {
		absIdx := start + i
		_, isFav := v.FavoriteArtists[name]
		display := text.FocusMarker(absIdx == cursor) + text.FavPrefix(name, v.Width-2, isFav)
		isOdd := absIdx%2 == 1
		switch {
		case absIdx == cursor && isFav:
			display = v.Styles.FavArtistAccent.Render(display)
		case absIdx == cursor:
			display = v.Styles.Accent.Render(display)
		case isFav:
			display = styles.WithZebraBg(v.Styles.FavArtist, v.Styles.ZebraBg, isOdd).Render(display)
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
		return v.Styles.Dim.Render("No tracks found for this artist")
	}
	width := contentWidth(v.Width)
	stripLine := v.renderStateStrip(len(v.Tracks), len(items))
	if len(items) == 0 {
		return strip.JoinStrip(stripLine, "(no matches)")
	}

	cols := ComputeTrack(width)

	start, end, cursor := v.renderWindow(len(items))
	window := items[start:end]
	showDisc := anyMultiDisc(window)

	lines := make([]string, len(window))
	for i, t := range window {
		row := v.trackRowAt(t, start+i, cursor, cols, showDisc)
		lines[i] = ansi.Truncate(v.renderTrackRow(row), width, "")
	}
	rest := trackHeader(cols, width, v.Styles) + "\n" + strings.Join(lines, "\n")
	return strip.JoinStrip(stripLine, rest)
}

// anyMultiDisc reports whether the window contains a track from a disc past
// the first, which is what promotes positions to the "disc-track" form.
func anyMultiDisc(tracks []library.Track) bool {
	for _, t := range tracks {
		if t.DiscNumber > 1 {
			return true
		}
	}
	return false
}

// trackHeader builds the column-heading line above the track rows, clamped to
// the content width.
func trackHeader(cols TrackColumns, width int, st styles.Styles) string {
	header := strings.Repeat(" ", cols.SelectionMarker) + st.ColHead.Render(
		text.Pad("#", cols.Position)+" "+
			text.Pad("Title", cols.Title)+" "+
			text.PadLeft("Time", cols.Duration))
	return ansi.Truncate(header, width, "")
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
func (v View) trackRowAt(t library.Track, absIdx, cursor int, cols TrackColumns, showDisc bool) trackRow {
	isFocused := absIdx == cursor
	isNowPlaying := v.NowPlayingHash != "" && t.Hash == v.NowPlayingHash
	_, isSelected := v.SelectedHashes[t.Hash]
	_, isTrackFav := v.FavoriteHashes[t.Hash]

	rawPos := text.FormatTrackPosSimple(t.TrackNumber, absIdx+1)
	if showDisc {
		rawPos = text.FormatTrackPos(t.TrackNumber, t.DiscNumber, absIdx+1)
	}

	return trackRow{
		marker:   text.RowMarker(isSelected, isFocused, isNowPlaying, v.NowPlayingIcon),
		pos:      text.Pad(rawPos, cols.Position),
		title:    text.FavPrefix(t.Title, cols.Title, isTrackFav),
		dur:      text.PadLeft(text.FormatDurationSecs(t.Duration), cols.Duration),
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
	a := v.Styles.Accent
	sep := a.Render(" ")
	title := styles.AccentFavRender(a, v.Styles.FavoriteAccent, r.fav, r.title)
	return a.Render(r.marker+r.pos) + sep + title + sep + a.Render(r.dur)
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
// breadcrumb, sort, filter, counts, and an "esc ←" hint at drill-down
// levels. total is the full item count before filtering; filtered is the
// count after filtering (equal to total when no filter is active).
func (v View) renderStateStrip(total, filtered int) string {
	breadcrumb := v.breadcrumb()
	hint := ""
	if len(breadcrumb) > 1 {
		hint = "esc ←"
	}
	sortChunk := strip.SortChunk{Label: v.StripSortLabel, Indicator: v.StripSortIndicator}
	start, end, _ := v.renderWindow(filtered)
	counts := strip.CountsChunk{
		WindowStart: start, WindowEnd: end,
		Unit:      []string{"genres", "artists", "tracks"}[v.Level],
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
// currently opened genre and artist.
func (v View) breadcrumb() []string {
	root := v.StripViewLabel
	if root == "" {
		root = "Genres"
	}
	breadcrumb := []string{root}
	switch v.Level {
	case ArtistLevel:
		if v.SelectedGenre != "" {
			breadcrumb = append(breadcrumb, v.SelectedGenre)
		}
	case TrackLevel:
		if v.SelectedGenre != "" {
			breadcrumb = append(breadcrumb, v.SelectedGenre)
		}
		if v.SelectedArtist != "" {
			breadcrumb = append(breadcrumb, v.SelectedArtist)
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

func contentWidth(width int) int {
	if width > 0 {
		return width
	}
	return 80
}

// renderWindow clamps stale cursor and scroll state to the currently rendered
// list. This keeps filtered/shrunk lists visible even when the caller has not
// yet normalized navigation state.
func (v View) renderWindow(itemCount int) (start, end, cursor int) {
	if itemCount <= 0 {
		return 0, 0, 0
	}
	maxRows := v.visibleRows()
	cursor = max(0, min(v.Cursor, itemCount-1))
	start = max(0, v.ScrollOffset)
	if start > cursor {
		start = cursor
	}
	if cursor >= start+maxRows {
		start = cursor - maxRows + 1
	}
	maxStart := max(0, itemCount-maxRows)
	if start > maxStart {
		start = maxStart
	}
	end = min(itemCount, start+maxRows)
	return start, end, cursor
}
