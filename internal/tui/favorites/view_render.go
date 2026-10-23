package favorites

import (
	"fmt"
	"strconv"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/library"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/strip"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// Render returns the string representation of the favorites panel.
func (v View) Render() string {
	switch v.Level {
	case ListLevel:
		return v.renderList()
	case TrackLevel:
		return v.renderTracks()
	default:
		return strip.Collection(v.Width, "Favorites", "category", 3, 3, v.Styles) + "\n" + v.renderCategories()
	}
}

func (v View) renderCategories() string {
	categories := []string{
		"Artists",
		"Albums",
		"Tracks",
	}

	counts := []int{v.ArtistCount, v.AlbumCount, v.TrackCount}
	units := []string{"artist", "album", "track"}
	lines := make([]string, len(categories))
	for i, name := range categories {
		count := text.FormatCount(counts[i]) + " " + text.Pluralize(counts[i], units[i], units[i]+"s")
		display := text.FocusMarker(i == v.Cursor) + text.Pad(name, max(0, v.Width-3-ansi.StringWidth(count))) + " " + count
		isOdd := i%2 == 1
		if i == v.Cursor {
			display = v.Styles.Accent.Render(display)
		} else if isOdd {
			display = lipgloss.NewStyle().Background(v.Styles.ZebraBg).Render(display)
		}
		lines[i] = display
	}
	return strings.Join(lines, "\n")
}

func (v View) renderList() string {
	if v.Category == CategoryTracks {
		return v.renderTrackList()
	}

	heading := v.selectionHeading(v.categoryHeading())
	items := v.EffectiveItems()
	if len(v.categoryItems()) == 0 {
		card := v.Styles.RenderEmptyCard(v.Width, v.visibleRows(), "No favorites yet", []styles.EmptyAction{
			{Key: "1", Label: "go to library"},
			{Key: "F", Label: "favorite an artist or album"},
			{Key: "?", Label: "help"},
		})
		return heading + "\n\n" + card
	}
	if len(items) == 0 {
		return heading + v.filterSeparator() + "(no matches)"
	}

	start, end := v.visibleWindow(len(items))
	heading = v.headingWithWindow(heading, start, end, len(items))
	st := v.categoryStyling()
	labels := library.AlbumDisplayLabels(v.Albums)
	lines := make([]string, end-start)
	for i, name := range items[start:end] {
		label := name
		if v.Category == CategoryAlbums {
			label = labels[name]
		}
		lines[i] = v.renderItemRow(name, label, start+i, st)
	}
	return heading + v.filterSeparator() + strings.Join(lines, "\n")
}

// categoryHeading returns the list heading for the current category, or the
// empty string outside the artist and album lists.
func (v View) categoryHeading() string {
	switch v.Category {
	case CategoryArtists:
		return "Favorite Artists"
	case CategoryAlbums:
		return "Favorite Albums"
	default:
		return ""
	}
}

// categoryItems returns the unfiltered item slice backing the current category.
func (v View) categoryItems() []string {
	switch v.Category {
	case CategoryArtists:
		return v.Artists
	case CategoryAlbums:
		return v.Albums
	default:
		return nil
	}
}

// listStyling carries the per-category favorite colours and lookup map used to
// paint the artist or album list rows.
type listStyling struct {
	fav       lipgloss.Style
	favAccent lipgloss.Style
	favMap    map[string]struct{}
}

func (v View) categoryStyling() listStyling {
	switch v.Category {
	case CategoryArtists:
		return listStyling{fav: v.Styles.FavArtist, favAccent: v.Styles.FavArtistAccent, favMap: v.FavoriteArtists}
	case CategoryAlbums:
		return listStyling{fav: v.Styles.FavAlbum, favAccent: v.Styles.FavAlbumAccent, favMap: v.FavoriteAlbums}
	default:
		return listStyling{}
	}
}

// renderItemRow renders one artist or album row. Focus wins over bulk
// selection, which in turn wins over the favorite colour and zebra banding.
func (v View) renderItemRow(name, displayLabel string, absIdx int, st listStyling) string {
	isFocused := absIdx == v.Cursor
	_, isFav := st.favMap[name]
	_, selected := v.SelectedItems[name]
	isOdd := absIdx%2 == 1

	display := text.FavPrefix(displayLabel, v.Width-2, isFav)
	line := text.RowMarker(selected, isFocused, false, "") + display
	switch {
	case isFocused && isFav:
		return st.favAccent.Render(line)
	case isFocused:
		return v.Styles.Accent.Render(line)
	case selected && isFav:
		sb := lipgloss.NewStyle().Background(v.Styles.SelectionBg)
		return v.Styles.AccentMark.Background(v.Styles.SelectionBg).Render("●") + sb.Render(" ") +
			styles.WithZebraBg(st.fav, v.Styles.SelectionBg, true).Render(display)
	case selected:
		sb := lipgloss.NewStyle().Background(v.Styles.SelectionBg)
		return v.Styles.AccentMark.Background(v.Styles.SelectionBg).Render("●") + sb.Render(" "+display)
	case isFav:
		return styles.WithZebraBg(st.fav, v.Styles.ZebraBg, isOdd).Render(line)
	case isOdd:
		return lipgloss.NewStyle().Background(v.Styles.ZebraBg).Render(line)
	default:
		return line
	}
}

func (v View) renderTrackList() string {
	heading := v.selectionHeading("Favorite Tracks")
	items := v.EffectiveTracks()
	if len(v.Tracks) == 0 {
		card := v.Styles.RenderEmptyCard(v.Width, v.visibleRows(), "No favorites yet", []styles.EmptyAction{
			{Key: "1", Label: "go to library"},
			{Key: "F", Label: "favorite a track"},
			{Key: "?", Label: "help"},
		})
		return heading + "\n\n" + card
	}
	if len(items) == 0 {
		return heading + v.filterSeparator() + "(no matches)"
	}

	start, end := v.visibleWindow(len(items))
	heading = v.headingWithWindow(heading, start, end, len(items))
	return heading + v.filterSeparator() + v.renderTrackRows(items)
}

func (v View) renderTracks() string {
	heading := v.selectionHeading(library.AlbumDisplayLabel(v.DrilldownName, v.Albums))

	items := v.EffectiveTracks()
	if len(v.DrilldownTracks) == 0 {
		return heading + "\n\n" + v.Styles.Dim.Render("No tracks found for this selection")
	}
	if len(items) == 0 {
		return heading + v.filterSeparator() + "(no matches)"
	}

	start, end := v.visibleWindow(len(items))
	heading = v.headingWithWindow(heading, start, end, len(items))
	return heading + v.filterSeparator() + v.renderTrackRows(items)
}

func (v View) renderTrackRows(items []library.Track) string {
	cols := ComputeTrack(v.Width)
	start, end := v.visibleWindow(len(items))
	showDisc := anyMultiDisc(items[start:end])

	lines := make([]string, end-start)
	for i, t := range items[start:end] {
		lines[i] = v.renderTrackRow(t, cols, start+i, showDisc)
	}
	return v.renderTrackHeader(cols) + "\n" + strings.Join(lines, "\n")
}

// renderTrackHeader builds the column header row. The 2-space marker gutter is
// excluded from the styled body so the dotted underline starts at the first
// data column.
func (v View) renderTrackHeader(cols TrackColumns) string {
	return "  " + v.Styles.ColHead.Render(joinFavoriteCells(" ", text.Pad("#", cols.Position),
		text.Pad("Title", cols.Title), text.Pad("Artist", cols.Artist), text.Pad("Album", cols.Album),
		text.PadLeft("Time", cols.Duration), text.PadLeft("Plays", cols.PlayCount)))
}

// anyMultiDisc reports whether the window holds a track from a disc other than
// the first, which is when the disc number is worth showing in the # column.
func anyMultiDisc(window []library.Track) bool {
	for _, t := range window {
		if t.DiscNumber > 1 {
			return true
		}
	}
	return false
}

// trackRow holds the pre-padded cells of one track row together with the
// favorite flags that decide how each cell is coloured. The cells are already
// width-fitted, so the per-state renderers below only add styling.
type trackRow struct {
	marker    string
	pos       string
	title     string
	artist    string
	album     string
	dur       string
	plays     string
	trackFav  bool
	artistFav bool
	albumFav  bool
	selected  bool
}

// renderTrackRow renders a single track line in whichever of the four row
// states applies: focused, now-playing, zebra/selection background, or plain.
func (v View) renderTrackRow(t library.Track, cols TrackColumns, absIdx int, showDisc bool) string {
	isFocused := absIdx == v.Cursor
	isNowPlaying := v.NowPlayingHash != "" && t.Hash == v.NowPlayingHash
	_, selected := v.SelectedItems[trackSelectionKey(t)]

	rawPos := text.FormatTrackPosSimple(t.TrackNumber, absIdx+1)
	if showDisc {
		rawPos = text.FormatTrackPos(t.TrackNumber, t.DiscNumber, absIdx+1)
	}
	artist := arttui.DisplayArtistName(t.Artist)
	album := t.Album
	if album == "" {
		album = arttui.UnknownAlbum
	}
	_, isTrackFav := v.FavoriteHashes[t.Hash]
	_, isArtistFav := v.FavoriteArtists[artist]
	_, isAlbumFav := v.FavoriteAlbums[library.AlbumKey(t)]

	r := trackRow{
		marker:    text.RowMarker(selected, isFocused, isNowPlaying, v.NowPlayingIcon),
		pos:       text.Pad(rawPos, cols.Position),
		title:     text.FavPrefix(t.Title, cols.Title, isTrackFav),
		artist:    text.FavPrefix(artist, cols.Artist, isArtistFav),
		album:     text.FavPrefix(album, cols.Album, isAlbumFav),
		dur:       text.PadLeft(text.FormatDurationSecs(t.Duration), cols.Duration),
		plays:     text.PadLeft(strconv.Itoa(t.PlayCount), cols.PlayCount),
		trackFav:  isTrackFav,
		artistFav: isArtistFav,
		albumFav:  isAlbumFav,
		selected:  selected,
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
	return joinFavoriteCells(sep, a.Render(r.marker+r.pos), title, artist, album, a.Render(r.dur), a.Render(r.plays))
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
	return joinFavoriteCells(sep, np.Render(r.marker+r.pos), title, artist, album, np.Render(r.dur), np.Render(r.plays))
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

	rb := lipgloss.NewStyle().Background(rowBg)
	if !r.trackFav {
		r.title = styles.WithZebraBg(v.Styles.TextPrimary, rowBg, true).Render(r.title)
	}
	if !r.artistFav {
		r.artist = styles.WithZebraBg(v.Styles.TextPrimary, rowBg, true).Render(r.artist)
	}
	if !r.albumFav {
		r.album = styles.WithZebraBg(v.Styles.TextSecondary, rowBg, true).Render(r.album)
	}
	markerPart := rb.Render(r.marker)
	if r.selected {
		markerPart = v.Styles.AccentMark.Background(rowBg).Render("●") + rb.Render(" ")
	}
	posPart := styles.WithZebraBg(v.Styles.TextTertiary, rowBg, true).Render(r.pos)
	durPart := styles.WithZebraBg(v.Styles.TextTertiary, rowBg, true).Render(r.dur)
	sep := rb.Render(" ")
	return joinFavoriteCells(sep, markerPart+posPart, r.title, r.artist, r.album, durPart, rb.Render(r.plays))
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
	return joinFavoriteCells(" ", r.marker+v.Styles.TextTertiary.Render(r.pos), r.title, r.artist, r.album, v.Styles.TextTertiary.Render(r.dur), r.plays)
}

// filterSeparator returns the line between the heading and the item list.
func (v View) filterSeparator() string {
	return text.BuildFilterSeparator(v.SortIndicator, v.FilterActive, v.FilterText, v.FilterView, v.Styles.Dim.Render(v.FilterHint), v.Width, true)
}

// Keep selection feedback on the existing heading line, including when every
// selected row is hidden. Scrolling does not change the hidden count.
func (v View) selectionHeading(title string) string {
	suffix := ""
	if len(v.SelectedItems) > 0 {
		visible := FavoritesView{Level: v.Level, Category: v.Category, Artists: v.Artists,
			Albums: v.Albums, Tracks: v.Tracks, DrilldownTracks: v.DrilldownTracks}
		visible.Filter.Text = v.FilterText
		keys := visible.SelectionKeys()
		shown := make(map[string]struct{})
		for _, key := range keys {
			if _, ok := v.SelectedItems[key]; ok {
				shown[key] = struct{}{}
			}
		}
		suffix = fmt.Sprintf(" · %d selected", len(v.SelectedItems))
		if hidden := len(v.SelectedItems) - len(shown); hidden > 0 {
			suffix += fmt.Sprintf(" · %d hidden", hidden)
		}
	}
	return v.Styles.Heading.Render(text.Truncate(title, max(0, v.Width-ansi.StringWidth(suffix))) + text.Truncate(suffix, v.Width))
}

func joinFavoriteCells(sep, pos, title, artist, album, duration, plays string) string {
	cells := []string{pos, title, artist}
	if ansi.StringWidth(album) > 0 {
		cells = append(cells, album)
	}
	cells = append(cells, duration)
	if ansi.StringWidth(plays) > 0 {
		cells = append(cells, plays)
	}
	return strings.Join(cells, sep)
}

func (v View) headingWithWindow(heading string, start, end, total int) string {
	label := strip.WindowLabel(start, end, total)
	if len(v.SelectedItems) > 0 && ansi.StringWidth(heading)+len(label)+2 > v.Width {
		return heading
	}
	return strip.WithWindow(heading, v.Width, start, end, total, v.Styles)
}
