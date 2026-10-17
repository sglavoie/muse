package playlists

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/library"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/layout"
	"github.com/sglavoie/muse/internal/tui/strip"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// Render returns the string representation of the playlists panel.
func (v View) Render() string {
	switch v.Level {
	case ListLevel:
		return v.listHeading() + "\n" + v.renderList()
	default:
		return v.renderTracks()
	}
}

func (v View) renderList() string {
	if len(v.Playlists) == 0 && !v.Creating {
		return v.renderEmptyList()
	}

	cols := Compute(v.Width)
	lines := v.renderPlaylistRows(cols)
	// When the list is empty the input is still shown at the top.
	if v.Creating && len(v.Playlists) == 0 {
		lines = append(lines, v.renderCreateInputRow(cols))
	}
	out := strings.Join(lines, "\n")
	if v.SortField != "" {
		out = v.renderListSortIndicator() + "\n" + out
	}
	return out
}

// renderPlaylistRows renders the visible window of playlist rows, splicing the
// rename input in place of the focused row and the create input just below it.
func (v View) renderPlaylistRows(cols Columns) []string {
	start, end := visibleWindow(v.ScrollOffset, v.Cursor, len(v.Playlists), v.visibleRows())
	focusCursor := -1
	if len(v.Playlists) > 0 {
		focusCursor = clamp(v.Cursor, 0, len(v.Playlists)-1)
	}

	var lines []string
	for _, p := range v.Playlists[start:end] {
		absIdx := start + len(lines) // track absolute index before appending
		if v.Renaming && absIdx == focusCursor {
			lines = append(lines, v.renderRenameInputRow(cols))
			continue
		}
		lines = append(lines, v.renderPlaylistRow(p, cols, absIdx == focusCursor))
		if v.Creating && absIdx == focusCursor {
			lines = append(lines, v.renderCreateInputRow(cols))
		}
	}
	return lines
}

// renderEmptyList renders the "no playlists yet" card, keeping the sort
// indicator above it when a sort is active.
func (v View) renderEmptyList() string {
	card := v.Styles.RenderEmptyCard(v.Width, v.visibleRows(), "No playlists yet", []styles.EmptyAction{
		{Key: "c", Label: "create a playlist"},
		{Key: "?", Label: "help"},
	})
	if v.SortField == "" {
		return card
	}
	return v.renderListSortIndicator() + "\n" + card
}

// renderPlaylistRow renders one name+count row. Smart playlists show a ⚡ glyph
// and either the evaluated track count (once known) or "calculating…" as a
// placeholder — TrackCount < 0 means the background evaluation has not yet
// returned.
func (v View) renderPlaylistRow(p db.PlaylistRow, cols Columns, isFocused bool) string {
	name := text.Pad(p.Name, cols.Name-2)
	count := text.PadLeft(text.FormatCount(p.TrackCount)+" "+text.Pluralize(p.TrackCount, "track", "tracks"), cols.Count)
	if p.IsSmart {
		name = text.Pad("⚡ "+p.Name, cols.Name-2)
		if p.TrackCount < 0 {
			count = text.PadLeft("calculating…", cols.Count)
		}
	}
	line := text.FocusMarker(isFocused) + name + "  " + count
	if isFocused {
		return v.Styles.Accent.Render(line)
	}
	return line
}

func (v View) renderListSortIndicator() string {
	return v.Styles.Dim.Render(fmt.Sprintf("Sort: %s %s", v.SortField, v.SortDir))
}

// renderCreateInputRow renders the inline text input as a highlighted row
// fitted to the same column widths as a normal playlist row.
func (v View) renderCreateInputRow(cols Columns) string {
	rowWidth := cols.Name + nameSep + cols.Count
	display := text.Pad(v.CreateInput.View(), rowWidth)
	return v.Styles.Accent.Render(display)
}

// renderRenameInputRow renders the inline rename input as a highlighted row
// fitted to the same column widths as a normal playlist row.
func (v View) renderRenameInputRow(cols Columns) string {
	rowWidth := cols.Name + nameSep + cols.Count
	display := text.Pad(v.RenameInput.View(), rowWidth)
	return v.Styles.Accent.Render(display)
}

func (v View) renderTracks() string {
	heading := v.renderTrackHeading() + "\n" + v.renderTrackSummary()
	if len(v.Tracks) == 0 {
		return heading + "\n(no tracks)"
	}

	cols := ComputeTrack(v.Width)
	header := v.renderTrackHeader(cols)

	start, end := visibleWindow(v.ScrollOffset, v.Cursor, len(v.Tracks), v.visibleRows())
	focusCursor := clamp(v.Cursor, 0, len(v.Tracks)-1)

	lines := make([]string, end-start)
	for i, t := range v.Tracks[start:end] {
		absIdx := start + i
		lines[i] = v.renderTrackRow(t, cols, absIdx, absIdx == focusCursor)
	}
	return heading + "\n" + header + "\n" + strings.Join(lines, "\n")
}

// renderTrackHeader builds the column header row, appending the sort direction
// glyph to whichever column the current sort is keyed on.
func (v View) renderTrackHeader(cols TrackColumns) string {
	titleHead, artistHead, albumHead, timeHead := "Title", "Artist", "Album", "Time"
	switch v.SortField {
	case "Title":
		titleHead += v.SortDir
	case "Artist":
		artistHead += v.SortDir
	case "Album":
		albumHead += v.SortDir
	case "Duration":
		timeHead += v.SortDir
	}
	return strings.Repeat(" ", cols.Marker) + v.Styles.ColHead.Render(joinPlaylistCells(" ", text.Pad("#", cols.Position),
		text.Pad(titleHead, cols.Title), text.Pad(artistHead, cols.Artist), text.Pad(albumHead, cols.Album), text.PadLeft(timeHead, cols.Duration)))
}

// trackRow holds the pre-padded cells of one track row together with the
// favorite flags that decide how each cell is coloured. The cells are already
// width-fitted, so the per-state renderers below only add styling.
type trackRow struct {
	marker    string
	pos       string
	dur       string
	title     string
	artist    string
	album     string
	trackFav  bool
	artistFav bool
	albumFav  bool
	selected  bool
}

// renderTrackRow renders a single track line in whichever of the four row
// states applies: focused, now-playing, zebra/selection background, or plain.
func (v View) renderTrackRow(t db.PlaylistTrackRow, cols TrackColumns, absIdx int, isFocused bool) string {
	_, isSelected := v.SelectedTrackIDs[t.ID]
	isNowPlaying := v.NowPlayingHash != "" && t.Hash == v.NowPlayingHash

	// Per-cell favorite lookups.
	_, isTrackFav := v.FavoriteHashes[t.Hash]
	_, isArtistFav := v.FavoriteArtists[t.Artist]
	_, isAlbumFav := v.FavoriteAlbums[library.AlbumKey(library.Track{Album: t.Album,
		AlbumArtist: t.AlbumArtist, Artist: t.Artist, Path: t.Path})]

	r := trackRow{
		// When the row is focused, omit the inner accent/reset from the marker
		// so the outer background highlight covers the full row uniformly. An
		// inner ANSI reset would reset the cursor highlight mid-row.
		marker:    text.RowMarker(isSelected, isFocused, isNowPlaying, v.NowPlayingIcon),
		pos:       text.Pad(text.FormatTrackPos(t.TrackNumber, t.DiscNumber, absIdx+1), cols.Position),
		dur:       text.PadLeft(text.FormatDurationMs(t.DurationMs), cols.Duration),
		title:     text.FavPrefix(t.Title, cols.Title, isTrackFav),
		artist:    text.FavPrefix(t.Artist, cols.Artist, isArtistFav),
		album:     text.FavPrefix(t.Album, cols.Album, isAlbumFav),
		trackFav:  isTrackFav,
		artistFav: isArtistFav,
		albumFav:  isAlbumFav,
		selected:  isSelected,
	}
	switch {
	case isFocused:
		return v.renderFocusedTrackRow(r)
	case isNowPlaying:
		return v.renderNowPlayingTrackRow(r)
	default:
		return v.renderZebraTrackRow(r, absIdx%2 == 1)
	}
}

func (v View) renderFocusedTrackRow(r trackRow) string {
	a := v.Styles.Accent
	sep := a.Render(" ")
	title := styles.AccentFavRender(a, v.Styles.FavoriteAccent, r.trackFav, r.title)
	artist := styles.AccentFavRender(a, v.Styles.FavArtistAccent, r.artistFav, r.artist)
	album := styles.AccentFavRender(a, v.Styles.FavAlbumAccent, r.albumFav, r.album)
	return joinPlaylistCells(sep, a.Render(r.marker+r.pos), title, artist, album, a.Render(r.dur))
}

func (v View) renderNowPlayingTrackRow(r trackRow) string {
	np := v.Styles.NowPlayingRow
	sep := np.Render(" ")
	title := v.Styles.NowPlayingTitle.Render(r.title)
	artist := v.Styles.TextPrimary.Background(v.Styles.NowPlayingBg).Render(r.artist)
	if r.artistFav {
		artist = v.Styles.NowPlayingRowFavArtist.Render(r.artist)
	}
	album := v.Styles.TextSecondary.Background(v.Styles.NowPlayingBg).Render(r.album)
	if r.albumFav {
		album = v.Styles.NowPlayingRowFavAlbum.Render(r.album)
	}
	return joinPlaylistCells(sep, np.Render(r.marker+r.pos), title, artist, album, np.Render(r.dur))
}

// renderZebraTrackRow renders an unfocused, not-now-playing row. Selected rows
// take the selection background regardless of parity; otherwise odd rows take
// the zebra background and even rows are left unstyled.
func (v View) renderZebraTrackRow(r trackRow, isOdd bool) string {
	rowBg := v.Styles.ZebraBg
	useBg := isOdd
	if r.selected {
		rowBg = v.Styles.SelectionBg
		useBg = true
	}
	if r.trackFav {
		r.title = styles.WithZebraBg(v.Styles.Favorite, rowBg, useBg).Render(r.title)
	}
	if r.artistFav {
		r.artist = styles.WithZebraBg(v.Styles.FavArtist, rowBg, useBg).Render(r.artist)
	}
	if r.albumFav {
		r.album = styles.WithZebraBg(v.Styles.FavAlbum, rowBg, useBg).Render(r.album)
	}
	if !useBg {
		return v.renderPlainTrackRow(r)
	}

	zb := lipgloss.NewStyle().Background(rowBg)
	if !r.trackFav {
		r.title = styles.WithZebraBg(v.Styles.TextPrimary, rowBg, true).Render(r.title)
	}
	if !r.artistFav {
		r.artist = styles.WithZebraBg(v.Styles.TextPrimary, rowBg, true).Render(r.artist)
	}
	if !r.albumFav {
		r.album = styles.WithZebraBg(v.Styles.TextSecondary, rowBg, true).Render(r.album)
	}
	markerPart := zb.Render(r.marker)
	if r.selected {
		markerPart = v.Styles.AccentMark.Background(rowBg).Render("●") + zb.Render(" ")
	}
	posPart := styles.WithZebraBg(v.Styles.TextTertiary, rowBg, true).Render(r.pos)
	durPart := styles.WithZebraBg(v.Styles.TextTertiary, rowBg, true).Render(r.dur)
	sep := zb.Render(" ")
	return joinPlaylistCells(sep, markerPart+posPart, r.title, r.artist, r.album, durPart)
}

// renderPlainTrackRow renders a row with no background fill. Favorite cells
// arrive already styled, so only the remaining cells are coloured here.
func (v View) renderPlainTrackRow(r trackRow) string {
	if !r.trackFav {
		r.title = v.Styles.TextPrimary.Render(r.title)
	}
	if !r.artistFav {
		r.artist = v.Styles.TextPrimary.Render(r.artist)
	}
	if !r.albumFav {
		r.album = v.Styles.TextSecondary.Render(r.album)
	}
	return joinPlaylistCells(" ", r.marker+v.Styles.TextTertiary.Render(r.pos), r.title, r.artist, r.album, v.Styles.TextTertiary.Render(r.dur))
}

// visibleWindow returns the [start, end) row window for the playlists panel.
// Playlists pulls a stale cursor into range before windowing, so the window
// still follows the nearest valid row; the shared helper deliberately ignores
// an out-of-range cursor.
func visibleWindow(scrollOffset, cursor, itemCount, maxRows int) (int, int) {
	if itemCount <= 0 {
		return 0, 0
	}
	return layout.VisibleWindowForCursor(clamp(cursor, 0, itemCount-1), scrollOffset, itemCount, maxRows, nil)
}

func joinPlaylistCells(sep, pos, title, artist, album, duration string) string {
	cells := []string{pos, title, artist}
	if ansi.StringWidth(album) > 0 || ansi.StringWidth(duration) == trackDurationFixed {
		cells = append(cells, album)
	}
	return strings.Join(append(cells, duration), sep)
}

func (v View) listHeading() string {
	return strip.Collection(v.Width, "Playlists", "playlist", len(v.Playlists), max(len(v.Playlists), v.totalPlaylists), v.Styles)
}
