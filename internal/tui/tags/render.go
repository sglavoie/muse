package tags

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

func (v View) renderTags() string {
	items := v.EffectiveTags()
	if len(items) == 0 && !v.Creating && v.FilterText == "" && !v.FilterActive {
		title := "No tags yet"
		if len(v.Tags) > 0 {
			title = "No tagged tracks yet"
		}
		return v.Styles.RenderEmptyCard(v.Width, v.visibleRows(), title, []styles.EmptyAction{
			{Key: "c", Label: "create a tag"},
			{Key: "?", Label: "help"},
		})
	}

	// When inline create is active, show the input as an extra row at bottom.
	maxRows, createRow := v.createRowBudget()

	if len(items) == 0 && !v.Creating {
		return v.topFilterSeparator() + "(no matches)"
	}

	start, end := v.visibleWindow(len(items), maxRows)

	lines := make([]string, end-start)
	for i, tag := range items[start:end] {
		absIdx := start + i
		lines[i] = v.renderTagRow(tag, absIdx == v.Cursor, absIdx%2 == 1)
	}
	return v.topFilterSeparator() + strings.Join(lines, "\n") + createRow
}

// createRowBudget returns the row budget left for the tag list and the extra
// bottom row holding the inline-create input, which claims one row when active.
func (v View) createRowBudget() (maxRows int, createRow string) {
	maxRows = v.visibleRows()
	if !v.Creating {
		return maxRows, ""
	}
	maxRows = max(maxRows-1, 0) // reserve one row for the input
	label := text.Pad("+ "+v.CreateInput+"_", v.Width)
	return maxRows, "\n" + v.Styles.Accent.Render(label)
}

// renderTagRow styles one tag row, substituting the inline rename input when
// the focused row is being renamed.
func (v View) renderTagRow(tag db.TagRow, isFocused, isOdd bool) string {
	if isFocused && v.Renaming {
		return v.Styles.Accent.Render(text.Pad("r "+v.RenameInput+"_", v.Width))
	}
	count := 0
	if v.TagCounts != nil {
		count = v.TagCounts[tag.ID]
	}
	display := text.FocusMarker(isFocused) + renderTagLabel(styles.ColoredDot(tag.Color), tag.Name, count, v.Width-2)
	switch {
	case isFocused:
		return v.Styles.Accent.Render(display)
	case isOdd:
		return lipgloss.NewStyle().Background(v.Styles.ZebraBg).Render(display)
	default:
		return display
	}
}

func (v View) renderTracks() string {
	heading := v.Styles.Heading.Render("Tags — " + v.SelectedTag.Name)
	items := v.EffectiveTracks()
	if len(v.Tracks) == 0 {
		return heading + "\n\n(no tracks)"
	}
	if len(items) == 0 {
		return heading + v.filterSeparator() + "(no matches)"
	}

	cols := ComputeTrack(v.Width)

	maxRows := v.visibleRows()
	start, end := v.visibleWindow(len(items), maxRows)
	window := items[start:end]

	lines := make([]string, len(window))
	for i, t := range window {
		row := v.trackRowAt(t, start+i, cols)
		lines[i] = text.Pad(v.renderTrackRow(row), v.Width)
	}
	header := text.Pad(trackHeader(cols, v.Styles), v.Width)
	return heading + v.filterSeparator() + header + "\n" + strings.Join(lines, "\n")
}

// trackHeader builds the column-heading line above the track rows.
func trackHeader(cols TrackColumns, st styles.Styles) string {
	return strings.Repeat(" ", cols.SelectionMarker) + st.ColHead.Render(joinTagCells(" ", text.Pad("Title", cols.Title),
		text.Pad("Artist", cols.Artist), text.Pad("Album", cols.Album), text.PadLeft("Time", cols.Duration)))
}

// trackRow carries one track row's pre-padded cells and visual state so each
// per-state renderer below takes a single argument.
type trackRow struct {
	marker   string
	title    string
	artist   string
	album    string
	dur      string
	focused  bool
	playing  bool
	selected bool
	fav      bool
	odd      bool
}

// trackRowAt derives the row descriptor for the track at absolute index absIdx.
func (v View) trackRowAt(t db.PlaylistTrackRow, absIdx int, cols TrackColumns) trackRow {
	isFocused := absIdx == v.Cursor
	isNowPlaying := v.NowPlayingHash != "" && t.Hash == v.NowPlayingHash
	_, isSelected := v.SelectedHashes[t.Hash]
	_, isTrackFav := v.FavoriteHashes[t.Hash]

	return trackRow{
		marker:   text.RowMarker(isSelected, isFocused, isNowPlaying, v.NowPlayingIcon),
		title:    text.FavPrefix(t.Title, cols.Title, isTrackFav),
		artist:   text.Pad(t.Artist, cols.Artist),
		album:    text.Pad(t.Album, cols.Album),
		dur:      text.PadLeft(text.FormatDurationSecs(float64(t.DurationMs)/1000), cols.Duration),
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
	return a.Render(r.marker) + joinTagCells(sep, title, a.Render(r.artist), a.Render(r.album), a.Render(r.dur))
}

func (v View) renderNowPlayingTrackRow(r trackRow) string {
	np := v.Styles.NowPlayingRow
	sep := np.Render(" ")
	title := v.Styles.NowPlayingTitle.Render(r.title)
	artist := v.Styles.TextPrimary.Background(v.Styles.NowPlayingBg).Render(r.artist)
	album := v.Styles.TextSecondary.Background(v.Styles.NowPlayingBg).Render(r.album)
	return np.Render(r.marker) + joinTagCells(sep, title, artist, album, np.Render(r.dur))
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
	artistPart := styles.WithZebraBg(v.Styles.TextPrimary, rowBg, true).Render(r.artist)
	albumPart := styles.WithZebraBg(v.Styles.TextSecondary, rowBg, true).Render(r.album)
	durPart := styles.WithZebraBg(v.Styles.TextTertiary, rowBg, true).Render(r.dur)
	sep := zb.Render(" ")
	return markerPart + joinTagCells(sep, title, artistPart, albumPart, durPart)
}

// renderUnfilledTrackRow renders a resting row with no row background.
func (v View) renderUnfilledTrackRow(r trackRow, title string) string {
	if !r.fav {
		title = v.Styles.TextPrimary.Render(title)
	}
	return r.marker + joinTagCells(" ", title, v.Styles.TextPrimary.Render(r.artist), v.Styles.TextSecondary.Render(r.album), v.Styles.TextTertiary.Render(r.dur))
}

func renderTagLabel(dot, name string, count, width int) string {
	prefix := dot + " "
	suffix := " " + text.FormatCount(count) + " " + text.Pluralize(count, "track", "tracks")
	fixedWidth := ansi.StringWidth(prefix) + ansi.StringWidth(suffix)
	if fixedWidth <= width {
		return prefix + text.Pad(name, width-fixedWidth) + suffix
	}
	return text.Pad(prefix+name+suffix, width)
}

// filterSeparator returns the line between the heading and the item list.
func (v View) filterSeparator() string {
	return text.BuildFilterSeparator(v.SortIndicator, v.FilterActive, v.FilterText, v.FilterView, "", v.Width, true)
}

// topFilterSeparator is like filterSeparator but without a leading newline.
func (v View) topFilterSeparator() string {
	return text.BuildFilterSeparator(v.SortIndicator, v.FilterActive, v.FilterText, v.FilterView, "", v.Width, false)
}

func joinTagCells(sep, title, artist, album, duration string) string {
	if ansi.StringWidth(duration) == 7 && ansi.StringWidth(album) == 0 {
		return strings.Join([]string{title, artist, duration}, sep)
	}
	return strings.Join([]string{title, artist, album, duration}, sep)
}
