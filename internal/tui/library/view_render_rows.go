package library

import (
	"image/color"

	"charm.land/lipgloss/v2"
	musiclib "github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// rowRun carries the values a row inherits from the one above it, so repeated
// artist and album names can be dimmed into runs.
type rowRun struct {
	showDisc        bool
	dimRunsDisabled bool
	prevArtist      string
	prevAlbum       string
}

// rowState holds the per-row decisions shared by the cell-styling and
// row-assembly steps.
type rowState struct {
	isSelected   bool
	isFocused    bool
	isNowPlaying bool
	isTrackFav   bool
	isArtistFav  bool
	isAlbumFav   bool
	useBg        bool
	rowBg        color.Color
	sameArtist   bool
	sameAlbum    bool
	prevArtist   string
	prevAlbum    string
}

// rowCells holds the width-fitted pieces of one rendered row.
type rowCells struct {
	marker    string
	num       string
	artist    string
	title     string
	album     string
	dur       string
	dateAdded string
}

// renderRows renders the [start, end) window, one line per track.
func (v Panel) renderRows(display []displayTrack, start, end int, cols Columns) []string {
	run := rowRun{
		showDisc:        anyMultiDisc(display[start:end]),
		dimRunsDisabled: v.Filter.Active || v.Filter.Text != "",
	}
	lines := make([]string, end-start)
	for i, dt := range display[start:end] {
		lines[i] = v.renderTrackRow(dt, cols, start+i, run)
		run.prevArtist = dt.track.Artist
		run.prevAlbum = dt.track.Album
	}
	return lines
}

// anyMultiDisc reports whether the window holds a track from a disc other than
// the first, which is when the disc number is worth showing in the # column.
func anyMultiDisc(window []displayTrack) bool {
	for _, dt := range window {
		if dt.track.DiscNumber > 1 {
			return true
		}
	}
	return false
}

func (v Panel) renderTrackRow(dt displayTrack, cols Columns, absIdx int, run rowRun) string {
	t := dt.track
	st := v.rowStateFor(dt, absIdx, run)

	num := text.FormatTrackPosSimple(t.TrackNumber, absIdx+1)
	if run.showDisc {
		num = text.FormatTrackPos(t.TrackNumber, t.DiscNumber, absIdx+1)
	}
	dateAdded := formatDateAdded(t.CreatedAt)
	if v.ShowPlayedAt {
		dateAdded = formatPlayedAt(t.LastPlayedAt)
	}

	artist, title, album := v.renderRowCells(dt, cols, st)
	return v.assembleRow(rowCells{
		// When the row is focused, omit the inner accent/reset from the marker
		// so the outer reverse-video wrap covers the full row uniformly. An
		// inner ANSI reset would reset the cursor highlight mid-row.
		marker:    text.RowMarker(st.isSelected, st.isFocused, st.isNowPlaying, v.NowPlayingIcon),
		num:       num,
		artist:    artist,
		title:     title,
		album:     album,
		dur:       text.FormatDurationSecs(t.Duration),
		dateAdded: dateAdded,
	}, cols, st)
}

// rowStateFor resolves the focus, selection, now-playing, favorite and
// zebra-background decisions for one row.
func (v Panel) rowStateFor(dt displayTrack, absIdx int, run rowRun) rowState {
	t := dt.track
	_, isSelected := v.SelectedHashes[t.Hash]
	isFocused := absIdx == v.Cursor
	isNowPlaying := v.NowPlayingHash != "" && t.Hash == v.NowPlayingHash
	st := rowState{
		isSelected:   isSelected,
		isFocused:    isFocused,
		isNowPlaying: isNowPlaying,
		useBg:        absIdx%2 == 1,
		rowBg:        v.Styles.ZebraBg,
		sameArtist:   !run.dimRunsDisabled && t.Artist == run.prevArtist && run.prevArtist != "",
		sameAlbum:    !run.dimRunsDisabled && t.Album == run.prevAlbum && run.prevAlbum != "",
		prevArtist:   run.prevArtist,
		prevAlbum:    run.prevAlbum,
	}
	if isSelected && !isFocused && !isNowPlaying {
		st.rowBg = v.Styles.SelectionBg
		st.useBg = true
	}
	_, st.isTrackFav = v.FavoriteHashes[t.Hash]
	_, st.isArtistFav = v.FavoriteArtists[t.Artist]
	_, st.isAlbumFav = v.FavoriteAlbums[musiclib.AlbumKey(t)]
	return st
}

// renderRowCells produces the artist/title/album cells padded to their column
// widths, highlighting the runes the fuzzy filter matched when it supplied
// positions.
func (v Panel) renderRowCells(dt displayTrack, cols Columns, st rowState) (artist, title, album string) {
	t := dt.track
	artistStyle, titleStyle, albumStyle := v.cellBaseStyles(st)
	switch {
	case len(dt.artistPositions) > 0 || len(dt.titlePositions) > 0 || len(dt.albumPositions) > 0:
		return padHighlight(t.Artist, cols.Artist, dt.artistPositions, artistStyle...),
			padHighlight(t.Title, cols.Title, dt.titlePositions, titleStyle...),
			padHighlight(t.Album, cols.Album, dt.albumPositions, albumStyle...)
	case len(dt.matchedIndexes) > 0:
		artistOffset, artistLen, titleOffset, titleLen, albumOffset, albumLen :=
			searchFieldOffsetsAll(t.Artist, t.Title, t.Album)
		return padHighlight(t.Artist, cols.Artist, fieldMatchPositions(dt.matchedIndexes, artistOffset, artistLen), artistStyle...),
			padHighlight(t.Title, cols.Title, fieldMatchPositions(dt.matchedIndexes, titleOffset, titleLen), titleStyle...),
			padHighlight(t.Album, cols.Album, fieldMatchPositions(dt.matchedIndexes, albumOffset, albumLen), albumStyle...)
	default:
		return v.renderUnhighlightedCells(t, cols, st)
	}
}

// cellBaseStyles picks the foreground style each highlighted cell composes
// with. A now-playing row composes NowPlayingBg into every cell so search-match
// highlighting doesn't leave the artist/title/album cells without the amber
// background while the markers and separators carry it.
func (v Panel) cellBaseStyles(st rowState) (artist, title, album []lipgloss.Style) {
	switch {
	case st.isFocused:
		return accentStyleFor(st.isArtistFav, v.Styles.FavArtistAccent, v.Styles.Accent),
			accentStyleFor(st.isTrackFav, v.Styles.FavoriteAccent, v.Styles.Accent),
			accentStyleFor(st.isAlbumFav, v.Styles.FavAlbumAccent, v.Styles.Accent)
	case st.isNowPlaying:
		return nowPlayingStyleFor(st.isArtistFav, v.Styles.NowPlayingRowFavArtist, v.Styles.TextPrimary.Background(v.Styles.NowPlayingBg)),
			nowPlayingStyleFor(st.isTrackFav, v.Styles.NowPlayingRowFavorite, v.Styles.NowPlayingTitle),
			nowPlayingStyleFor(st.isAlbumFav, v.Styles.NowPlayingRowFavAlbum, v.Styles.TextSecondary.Background(v.Styles.NowPlayingBg))
	default:
		return favStyleFor(st.isArtistFav, v.Styles.FavArtist),
			favStyleFor(st.isTrackFav, v.Styles.Favorite),
			favStyleFor(st.isAlbumFav, v.Styles.FavAlbum)
	}
}

// renderUnhighlightedCells renders the artist/title/album cells of a row with
// no fuzzy-match positions, where repeated artist and album names within a run
// collapse to a dimmed repeat glyph.
func (v Panel) renderUnhighlightedCells(t musiclib.Track, cols Columns, st rowState) (artist, title, album string) {
	artist = text.FavPrefix(t.Artist, cols.Artist, st.isArtistFav)
	title = text.FavPrefix(t.Title, cols.Title, st.isTrackFav)
	album = text.FavPrefix(t.Album, cols.Album, st.isAlbumFav)
	switch {
	case st.isFocused:
		return accentStyleFor(st.isArtistFav, v.Styles.FavArtistAccent, v.Styles.Accent)[0].Render(artist),
			accentStyleFor(st.isTrackFav, v.Styles.FavoriteAccent, v.Styles.Accent)[0].Render(title),
			accentStyleFor(st.isAlbumFav, v.Styles.FavAlbumAccent, v.Styles.Accent)[0].Render(album)
	case st.isNowPlaying:
		title = v.Styles.NowPlayingTitle.Render(title)
		if st.isArtistFav {
			artist = v.Styles.NowPlayingRowFavArtist.Render(artist)
		} else {
			artist = v.Styles.TextPrimary.Background(v.Styles.NowPlayingBg).Render(artist)
		}
		if st.isAlbumFav {
			album = v.Styles.NowPlayingRowFavAlbum.Render(album)
		} else {
			album = v.Styles.TextSecondary.Background(v.Styles.NowPlayingBg).Render(album)
		}
		return artist, title, album
	}
	if st.isTrackFav {
		title = styles.WithZebraBg(v.Styles.Favorite, st.rowBg, st.useBg).Render(title)
	}
	if st.isArtistFav {
		artist = styles.WithZebraBg(v.Styles.FavArtist, st.rowBg, st.useBg).Render(artist)
	}
	if st.isAlbumFav {
		album = styles.WithZebraBg(v.Styles.FavAlbum, st.rowBg, st.useBg).Render(album)
	}
	if st.sameArtist {
		artist = styles.WithZebraBg(v.Styles.TextTertiary, st.rowBg, st.useBg).
			Render(text.DimRepeat(t.Artist, st.prevArtist, cols.Artist))
	}
	if st.sameAlbum {
		album = styles.WithZebraBg(v.Styles.TextTertiary, st.rowBg, st.useBg).
			Render(text.DimRepeat(t.Album, st.prevAlbum, cols.Album))
	}
	return artist, title, album
}

// assembleRow joins the rendered cells into a line, in whichever of the five
// row shapes applies. Branch order matters: a track-number-only layout has no
// room for the data columns whatever the row's focus state.
func (v Panel) assembleRow(c rowCells, cols Columns, st rowState) string {
	switch {
	case cols.IsTrackNumOnly():
		return v.assembleTrackNumOnlyRow(c, cols, st)
	case st.isFocused:
		return assembleUniformRow(c, cols, v.Styles.Accent)
	case st.isNowPlaying:
		return assembleUniformRow(c, cols, v.Styles.NowPlayingRow)
	case st.useBg:
		return v.assembleZebraRow(c, cols, st)
	default:
		return v.assemblePlainRow(c, cols, st)
	}
}

func (v Panel) assembleTrackNumOnlyRow(c rowCells, cols Columns, st rowState) string {
	row := text.Pad(c.marker, cols.SelectionMarker) + text.Pad(c.num, cols.TrackNum)
	if st.isFocused {
		return v.Styles.Accent.Render(row)
	}
	return row
}

// assembleUniformRow joins a row whose marker, number, separators, duration and
// date all carry one style; the artist/title/album cells already carry their
// own background. Used for the focused and now-playing rows.
func assembleUniformRow(c rowCells, cols Columns, style lipgloss.Style) string {
	sep := style.Render("  ")
	row := style.Render(c.marker+text.Pad(c.num, cols.TrackNum)) + sep +
		c.artist + sep + c.title + sep + c.album + sep +
		style.Render(text.PadLeft(c.dur, cols.Duration))
	if cols.DateAdded > 0 {
		row += sep + style.Render(text.Pad(c.dateAdded, cols.DateAdded))
	}
	return row
}

// assembleZebraRow joins a row that carries a zebra or selection background,
// filling every cell and separator so the band spans the full width.
func (v Panel) assembleZebraRow(c rowCells, cols Columns, st rowState) string {
	rb := lipgloss.NewStyle().Background(st.rowBg)
	if !st.isTrackFav {
		c.title = styles.WithZebraBg(v.Styles.TextPrimary, st.rowBg, true).Render(c.title)
	}
	if !st.isArtistFav && !st.sameArtist {
		c.artist = styles.WithZebraBg(v.Styles.TextPrimary, st.rowBg, true).Render(c.artist)
	}
	if !st.isAlbumFav && !st.sameAlbum {
		c.album = styles.WithZebraBg(v.Styles.TextSecondary, st.rowBg, true).Render(c.album)
	}
	markerPart := rb.Render(c.marker)
	if st.isSelected {
		markerPart = v.Styles.AccentMark.Background(st.rowBg).Render("●") + rb.Render(" ")
	}
	numPart := styles.WithZebraBg(v.Styles.TextTertiary, st.rowBg, true).Render(text.Pad(c.num, cols.TrackNum))
	durPart := styles.WithZebraBg(v.Styles.TextTertiary, st.rowBg, true).Render(text.PadLeft(c.dur, cols.Duration))
	sep := rb.Render("  ")
	row := markerPart + numPart + sep + c.artist + sep + c.title + sep + c.album + sep + durPart
	if cols.DateAdded > 0 {
		row += sep + rb.Render(text.Pad(c.dateAdded, cols.DateAdded))
	}
	return row
}

// assemblePlainRow joins a row with no background fill. Favorite and dimmed-run
// cells arrive already styled, so only the remaining cells are coloured here.
func (v Panel) assemblePlainRow(c rowCells, cols Columns, st rowState) string {
	if !st.isTrackFav {
		c.title = v.Styles.TextPrimary.Render(c.title)
	}
	if !st.isArtistFav && !st.sameArtist {
		c.artist = v.Styles.TextPrimary.Render(c.artist)
	}
	if !st.isAlbumFav && !st.sameAlbum {
		c.album = v.Styles.TextSecondary.Render(c.album)
	}
	row := c.marker + v.Styles.TextTertiary.Render(text.Pad(c.num, cols.TrackNum)) + "  " +
		c.artist + "  " + c.title + "  " + c.album + "  " +
		v.Styles.TextTertiary.Render(text.PadLeft(c.dur, cols.Duration))
	if cols.DateAdded > 0 {
		row += "  " + text.Pad(c.dateAdded, cols.DateAdded)
	}
	return row
}
