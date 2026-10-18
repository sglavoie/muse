package queue

import (
	"fmt"
	"github.com/sglavoie/muse/internal/library"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/sglavoie/muse/internal/tui/buckets"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/sorting"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// queueMarkerWidth is the width of the gutter holding the selection and
// now-playing markers.
const queueMarkerWidth = 2

// Render builds the queue panel string using a 6-column layout:
//
//	Position | Title | Artist | Album | optional Source | Duration
//
// Identity columns share the Library proportions; compact tables omit Album and Source.
//
// Proportional columns resize on WindowSizeMsg; fixed-width columns are
// constant. Missing entries are wrapped in ANSI dim escape codes so they
// appear grayed out in the terminal.
func (v View) Render(s styles.Styles, currentPlayingIndex int, playbackState playback.State, favHashes, favArtists, favAlbums map[string]struct{}) string {
	timeline := v.traversalSnapshot()
	entries := v.filteredEntries(timeline)
	if len(entries) == 0 {
		return v.renderEmpty(s)
	}

	maxRows := v.VisibleRows()
	start := min(v.ScrollOffset, len(entries))
	end := min(len(entries), start+maxRows)

	hasSource := anySourcePlaylist(entries)
	cols := computeQueueColumns(v.Width, hasSource)
	header := clampQueueLine(strings.Repeat(" ", queueMarkerWidth)+s.ColHead.Render(queueHeaderBody(cols, hasSource)), v.Width)

	r := queueRowRenderer{
		s:                   s,
		cols:                cols,
		width:               v.Width,
		cursor:              v.Cursor,
		hasSource:           hasSource,
		playbackState:       playbackState,
		currentPlayingIndex: currentPlayingIndex,
		favHashes:           favHashes,
		favArtists:          favArtists,
		favAlbums:           favAlbums,
		timeline:            newTimelineRanks(timeline, len(v.Entries)),
	}
	lines := make([]string, end-start)
	for i := range lines {
		lines[i] = clampQueueLine(r.renderRow(entries[start+i], start+i), v.Width)
	}

	bodyLines := v.chunkIntoBuckets(s, lines, entries, start, maxRows)
	if end == len(entries) {
		bodyLines = append(bodyLines, s.Dim.Render("— end of queue —"))
	}
	return v.assemblePanel(s, header, bodyLines, len(entries), v.timelineActive(timeline))
}

// renderEmpty renders the placeholder shown when no entry is visible, which
// means either an empty queue or a filter that matched nothing.
func (v View) renderEmpty(s styles.Styles) string {
	if v.Filter.Text != "" || v.TagFilterHashes != nil {
		lines := strings.Split(filter.NoMatchesMessage(v.Filter.Text != "", v.TagFilterHashes != nil), "\n")
		for i, line := range lines {
			lines[i] = clampQueueLine(line, v.Width)
		}
		if stripLine := v.renderStateStrip(s, len(v.Entries), 0, v.timelineActive(v.traversalSnapshot())); stripLine != "" {
			lines = append([]string{stripLine}, lines...)
		}
		return strings.Join(lines, "\n")
	}
	return s.RenderEmptyCard(v.Width, v.VisibleRows(), "Queue is empty", []styles.EmptyAction{
		{Key: "1", Label: "go to library"},
		{Key: "?", Label: "help"},
	})
}

// anySourcePlaylist reports whether any entry came from a playlist, which is
// what makes the Source column worth its width.
func anySourcePlaylist(entries []Entry) bool {
	for _, e := range entries {
		if e.SourcePlaylist != "" {
			return true
		}
	}
	return false
}

// queueHeaderBody renders the column headings without the leading marker
// gutter, so the dotted underline only spans the actual data columns.
func queueHeaderBody(cols queueColumns, hasSource bool) string {
	return assembleQueueRow(" ", hasSource, "", text.Pad("#", cols.Position), text.Pad("Title", cols.Title),
		text.Pad("Artist", cols.Artist), text.Pad("Album", cols.Album), text.Pad("Source", cols.Source), text.PadLeft("Time", cols.Duration))
}

// chunkIntoBuckets interleaves bucket headers and dividers into the rendered
// rows when the active sort field defines buckets.
func (v View) chunkIntoBuckets(s styles.Styles, lines []string, entries []Entry, start, maxRows int) []string {
	if !sorting.SortFieldHasBuckets(v.SortState.Field) {
		return lines
	}
	labels := make([]string, len(lines))
	for i := range lines {
		labels[i] = EntryBucket(v.SortState.Field, entries[start+i])
	}
	sticky := ""
	if len(labels) > 0 {
		sticky = labels[0]
	}
	return buckets.Chunk(lines, labels, buckets.ChunkOptions{
		StickyBucket:     sticky,
		MaxRows:          maxRows,
		DividerEvery:     buckets.DividerEvery,
		MakeDivider:      func() string { return renderQueueDivider(s, v.Width) },
		MakeBucketHeader: func(label string) string { return renderQueueBucketHeader(s, label, v.Width) },
	})
}

// assemblePanel stacks the optional state strip, the header, the body, and the
// save-as-playlist prompt into the final panel string.
func (v View) assemblePanel(s styles.Styles, header string, bodyLines []string, filtered int, timelineActive bool) string {
	out := header + "\n" + strings.Join(bodyLines, "\n")
	if stripLine := v.renderStateStrip(s, len(v.Entries), filtered, timelineActive); stripLine != "" {
		out = stripLine + "\n" + out
	}
	if v.SavingAsPlaylist {
		prompt := "Save as playlist: "
		if v.SavingFromSelection {
			prompt = "Save selection as playlist: "
		}
		out += "\n" + s.Accent.Render(prompt) + v.SaveInput.View()
	}
	return out
}

// queueRowRenderer carries the per-frame context every row needs, so each row
// helper only takes what actually varies from row to row.
type queueRowRenderer struct {
	s                   styles.Styles
	cols                queueColumns
	width               int
	cursor              int
	hasSource           bool
	playbackState       playback.State
	currentPlayingIndex int
	favHashes           map[string]struct{}
	favArtists          map[string]struct{}
	favAlbums           map[string]struct{}
	timeline            timelineRanks
}

// queueRow holds one entry's pre-padded cells plus the flags deciding how they
// are styled. Row helpers restyle the cells in place and hand the row back to
// assemble.
type queueRow struct {
	marker, pos, title, artist, album, source, dur string
	trackFav, artistFav, albumFav, selected        bool
}

// rowFill describes the background painted behind a row that is neither
// focused nor playing.
type rowFill struct {
	bg   color.Color
	used bool
}

func (r *queueRowRenderer) assemble(sep string, row queueRow) string {
	return assembleQueueRow(sep, r.hasSource, row.marker, row.pos, row.title, row.artist, row.album, row.source, row.dur)
}

// fillFor reports the background for a plain row: bulk selection wins over the
// zebra stripe, and only odd rows are striped.
func (r *queueRowRenderer) fillFor(selected bool, absIdx int) rowFill {
	if selected {
		return rowFill{bg: r.s.SelectionBg, used: true}
	}
	return rowFill{bg: r.s.ZebraBg, used: absIdx%2 == 1}
}

func (r *queueRowRenderer) renderRow(e Entry, absIdx int) string {
	isFocused := absIdx == r.cursor
	isNowPlaying := r.isNowPlaying(e)
	row := r.cells(e, absIdx, isFocused, isNowPlaying)

	switch {
	case e.Missing:
		return r.renderMissingRow(e, row)
	case isFocused && isNowPlaying:
		return r.renderFocusedNowPlayingRow(row)
	case isFocused:
		return r.renderFocusedRow(row)
	case isNowPlaying:
		return r.renderNowPlayingRow(row)
	case r.isPlayed(e, isNowPlaying):
		return r.renderPlayedRow(row, r.fillFor(e.Selected, absIdx))
	default:
		return r.renderQueuedRow(row, r.fillFor(e.Selected, absIdx))
	}
}

// isNowPlaying reports whether e is the entry the player currently sits on.
func (r *queueRowRenderer) isNowPlaying(e Entry) bool {
	return r.currentPlayingIndex >= 0 && e.OrigIdx == r.currentPlayingIndex
}

// isPlayed reports whether playback has already moved past e. With a live
// timeline that is a question about traversal rank, not about the underlying
// queue index: under shuffle a row near the top of the queue may be the last
// thing scheduled, and a row near the bottom may already be spent.
func (r *queueRowRenderer) isPlayed(e Entry, isNowPlaying bool) bool {
	if isNowPlaying {
		return false
	}
	if r.timeline.active {
		return r.timeline.played(e.OrigIdx)
	}
	return r.currentPlayingIndex >= 0 && e.OrigIdx < r.currentPlayingIndex
}

// cells lays out one entry's columns, padded to width and carrying the favorite
// prefixes, before any row styling is applied.
func (r *queueRowRenderer) cells(e Entry, absIdx int, isFocused, isNowPlaying bool) queueRow {
	npIcon, _ := NowPlayingIndicator(r.s, r.playbackState)
	_, trackFav := r.favHashes[e.Hash]
	_, artistFav := r.favArtists[e.Artist]
	_, albumFav := r.favAlbums[library.AlbumKey(library.Track{Album: e.Album,
		AlbumArtist: e.AlbumArtist, Artist: e.Artist, Path: e.Path})]
	source := e.SourcePlaylist
	if e.DJOwned {
		source = "Auto-DJ"
	}
	return queueRow{
		marker:    text.RowMarker(e.Selected, isFocused, isNowPlaying, npIcon),
		pos:       text.PadLeft(fmt.Sprintf("%d", absIdx+1), r.cols.Position),
		title:     text.FavPrefix(e.Title, r.cols.Title, trackFav),
		artist:    text.FavPrefix(e.Artist, r.cols.Artist, artistFav),
		album:     text.FavPrefix(e.Album, r.cols.Album, albumFav),
		source:    padQueue(source, r.cols.Source),
		dur:       text.PadLeft(text.FormatDurationMs(e.DurationMs), r.cols.Duration),
		trackFav:  trackFav,
		artistFav: artistFav,
		albumFav:  albumFav,
		selected:  e.Selected,
	}
}

// renderMissingRow greys out an entry whose file has disappeared. It drops the
// favorite prefixes so the "[missing]" suffix is what the row is about.
func (r *queueRowRenderer) renderMissingRow(e Entry, row queueRow) string {
	row.title = padQueue(e.Title, r.cols.Title)
	row.artist = padQueue(e.Artist, r.cols.Artist)
	row.album = padQueue(e.Album, r.cols.Album)
	return r.s.Dim.Render(appendQueueSuffix(r.assemble(" ", row), " [missing]", r.width))
}

// accentChrome paints the cells outside the text columns with the accent style
// so a focused row's highlight spans the full width.
func (r *queueRowRenderer) accentChrome(row queueRow) queueRow {
	row.marker = r.s.Accent.Render(row.marker)
	row.pos = r.s.Accent.Render(row.pos)
	row.source = r.s.Accent.Render(row.source)
	row.dur = r.s.Accent.Render(row.dur)
	return row
}

// renderFocusedNowPlayingRow renders the row that is both focused and playing:
// the now-playing foregrounds are re-backgrounded onto the accent so the
// selection colour still reads as the dominant one.
func (r *queueRowRenderer) renderFocusedNowPlayingRow(row queueRow) string {
	bg := r.s.Accent.GetBackground()
	npText, npFav, npFavArtist, npFavAlbum := nowPlayingTextStyles(r.s, r.playbackState)
	row.title = styleOr(npFav, npText, row.trackFav).Background(bg).Render(row.title)
	row.artist = styleOr(npFavArtist, npText, row.artistFav).Background(bg).Render(row.artist)
	row.album = styleOr(npFavAlbum, npText, row.albumFav).Background(bg).Render(row.album)
	return r.assemble(r.s.Accent.Render(" "), r.accentChrome(row))
}

func (r *queueRowRenderer) renderFocusedRow(row queueRow) string {
	row.title = styles.AccentFavRender(r.s.Accent, r.s.FavoriteAccent, row.trackFav, row.title)
	row.artist = styles.AccentFavRender(r.s.Accent, r.s.FavArtistAccent, row.artistFav, row.artist)
	row.album = styles.AccentFavRender(r.s.Accent, r.s.FavAlbumAccent, row.albumFav, row.album)
	return r.assemble(r.s.Accent.Render(" "), r.accentChrome(row))
}

func (r *queueRowRenderer) renderNowPlayingRow(row queueRow) string {
	npIcon, npStyle := NowPlayingIndicator(r.s, r.playbackState)
	_, npFav, npFavArtist, npFavAlbum := nowPlayingTextStyles(r.s, r.playbackState)
	bg := r.s.NowPlayingBg
	nb := lipgloss.NewStyle().Background(bg)

	row.title = styleOr(npFav.Background(bg), r.s.NowPlayingTitle, row.trackFav).Render(row.title)
	row.artist = styleOr(npFavArtist, r.s.TextPrimary, row.artistFav).Background(bg).Render(row.artist)
	row.album = styleOr(npFavAlbum, r.s.TextSecondary, row.albumFav).Background(bg).Render(row.album)
	row.marker = npStyle.Background(bg).Render(npIcon) + nb.Render(" ")
	row.pos = nb.Render(row.pos)
	row.source = nb.Render(row.source)
	row.dur = nb.Render(row.dur)
	return r.assemble(nb.Render(" "), row)
}

// renderPlayedRow dims a row the queue has already moved past. Favorite colours
// give way to the played style so the row reads as spent at a glance.
func (r *queueRowRenderer) renderPlayedRow(row queueRow, fill rowFill) string {
	played := r.s.PlayedRow
	sep := " "
	if fill.used {
		played = played.Background(fill.bg)
		sep = lipgloss.NewStyle().Background(fill.bg).Render(" ")
	}
	switch {
	case row.selected:
		row.marker = r.s.AccentMark.Background(fill.bg).Render("●") + lipgloss.NewStyle().Background(fill.bg).Render(" ")
	case fill.used:
		row.marker = played.Render("╵") + lipgloss.NewStyle().Background(fill.bg).Render(" ")
	default:
		row.marker = played.Render("╵ ")
	}
	row.title = played.Render(row.title)
	row.artist = played.Render(row.artist)
	row.album = played.Render(row.album)
	row.pos = played.Render(row.pos)
	row.source = played.Render(row.source)
	row.dur = played.Render(row.dur)
	return r.assemble(sep, row)
}

func (r *queueRowRenderer) renderQueuedRow(row queueRow, fill rowFill) string {
	if !fill.used {
		return r.renderPlainQueuedRow(row)
	}
	rb := lipgloss.NewStyle().Background(fill.bg)
	row.title = styles.WithZebraBg(styleOr(r.s.Favorite, r.s.TextPrimary, row.trackFav), fill.bg, true).Render(row.title)
	row.artist = styles.WithZebraBg(styleOr(r.s.FavArtist, r.s.TextPrimary, row.artistFav), fill.bg, true).Render(row.artist)
	row.album = styles.WithZebraBg(styleOr(r.s.FavAlbum, r.s.TextSecondary, row.albumFav), fill.bg, true).Render(row.album)
	row.marker = rb.Render(row.marker)
	if row.selected {
		row.marker = r.s.AccentMark.Background(fill.bg).Render("●") + rb.Render(" ")
	}
	row.pos = styles.WithZebraBg(r.s.TextTertiary, fill.bg, true).Render(row.pos)
	row.source = rb.Render(row.source)
	row.dur = styles.WithZebraBg(r.s.TextTertiary, fill.bg, true).Render(row.dur)
	return r.assemble(rb.Render(" "), row)
}

// renderPlainQueuedRow renders an unfilled row: every cell carries its own
// foreground and no background at all.
func (r *queueRowRenderer) renderPlainQueuedRow(row queueRow) string {
	row.title = styleOr(r.s.Favorite, r.s.TextPrimary, row.trackFav).Render(row.title)
	row.artist = styleOr(r.s.FavArtist, r.s.TextPrimary, row.artistFav).Render(row.artist)
	row.album = styleOr(r.s.FavAlbum, r.s.TextSecondary, row.albumFav).Render(row.album)
	row.pos = r.s.TextTertiary.Render(row.pos)
	row.dur = r.s.TextTertiary.Render(row.dur)
	return r.assemble(" ", row)
}

// styleOr picks the favorite style when isFav, and the plain one otherwise.
func styleOr(fav, plain lipgloss.Style, isFav bool) lipgloss.Style {
	if isFav {
		return fav
	}
	return plain
}
