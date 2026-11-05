package albums

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/strip"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// gridFocusBar underlines the focused tile on the line below its artist, a
// line every tile reserves, so focus never moves anything.
const gridFocusBar = "▔"

// renderGrid draws the visible tile rows only, so a library of thousands of
// albums costs no more per frame than one screenful. The output is padded
// to the panel height: the grid owns the whole panel.
func (v View) renderGrid(g Grid, items []string) string {
	n := len(items)
	top := v.gridTop(g, n)
	start := top * g.Cols
	end := min(n, start+g.Rows*g.Cols)
	stripLine := v.renderStateStripWindow(len(v.Albums), n, start, end)

	lines := make([]string, 0, v.Height)
	for row := start; row < end; row += g.Cols {
		lines = append(lines, v.renderTileRow(g, items[row:min(row+g.Cols, end)], row)...)
	}
	for len(lines) < v.Height-1 {
		lines = append(lines, "")
	}
	return strip.JoinStrip(stripLine, strings.Join(lines, "\n"))
}

// renderTileRow draws one row of tiles, line by line: the covers side by
// side, then the titles, artists and the focus bar.
func (v View) renderTileRow(g Grid, albums []string, first int) []string {
	gap := strings.Repeat(" ", g.Gap)
	covers := make([][]string, len(albums))
	for i, album := range albums {
		covers[i] = v.tileCover(album, g)
	}
	out := make([]string, 0, g.TileHeight())
	for y := range g.ArtRows {
		parts := make([]string, len(albums))
		for i := range albums {
			parts[i] = covers[i][y]
		}
		out = append(out, strings.Join(parts, gap))
	}
	for line := range gridTextRows {
		parts := make([]string, len(albums))
		for i, album := range albums {
			parts[i] = v.tileText(album, line, first+i == v.Cursor, g.ArtCols, v.tileTextWidth(g, i))
		}
		out = append(out, strings.TrimRight(strings.Join(parts, " "), " "))
	}
	return out
}

// tileTextWidth lets a tile's title and artist run into the gutter beside
// it, keeping one blank column before the next tile, so names lose fewer
// letters than the cover width alone would allow.
func (v View) tileTextWidth(g Grid, col int) int {
	w := g.ArtCols + max(g.Gap-1, 0)
	x := col * (g.ArtCols + g.Gap)
	return max(min(w, contentWidth(v.Width)-x), g.ArtCols)
}

// tileCover returns the cover's lines, or a placeholder while the cover
// loads or when the album has none.
func (v View) tileCover(album string, g Grid) []string {
	if v.Covers != nil {
		if art := v.Covers(album, g.ArtCols, g.ArtRows); art != "" {
			if lines := strings.Split(art, "\n"); len(lines) == g.ArtRows {
				return lines
			}
		}
	}
	return tilePlaceholder(v.Styles, initials(tileTitle(album)), g.ArtCols, g.ArtRows)
}

// tilePlaceholder is a quiet tile carrying the album's initials, the same
// size as a cover so nothing moves when the cover arrives.
func tilePlaceholder(s styles.Styles, label string, cols, rows int) []string {
	tile := s.Dim
	if s.Background != nil && s.Primary != nil {
		tile = tile.Background(styles.Mix(s.Background, s.Primary, 0.10))
	}
	lines := make([]string, rows)
	for i := range lines {
		content := strings.Repeat(" ", cols)
		if i == (rows-1)/2 {
			pad := max((cols-len([]rune(label)))/2, 0)
			content = text.Pad(strings.Repeat(" ", pad)+label, cols)
		}
		lines[i] = tile.Render(content)
	}
	return lines
}

// tileText renders one of a tile's text lines, width columns wide: 0 the
// title, 1 the artist, 2 the focus bar, which spans the cover's artCols.
func (v View) tileText(album string, line int, focused bool, artCols, width int) string {
	switch line {
	case 0:
		return v.tileTitleText(album, focused, artCols, width)
	case 1:
		return v.Styles.Dim.Render(text.Pad(v.Releases[album].Artist, width))
	default:
		pad := strings.Repeat(" ", max(width-artCols, 0))
		if focused {
			return v.Styles.AccentMark.Render(strings.Repeat(gridFocusBar, artCols)) + pad
		}
		return strings.Repeat(" ", artCols) + pad
	}
}

// tileTitleText styles the title over at least the cover's width, so the
// focused tile's highlight lines up with its cover and focus bar.
func (v View) tileTitleText(album string, focused bool, artCols, width int) string {
	_, isFav := v.FavoriteAlbums[album]
	title := tileTitle(album)
	if isFav {
		title = "★ " + title
	}
	title = text.Truncate(title, width)
	span := max(ansi.StringWidth(title), artCols)
	label, pad := text.Pad(title, span), strings.Repeat(" ", max(width-span, 0))
	switch {
	case focused && isFav:
		return v.Styles.FavAlbumAccent.Render(label) + pad
	case focused:
		return v.Styles.Accent.Render(label) + pad
	case isFav:
		return v.Styles.FavAlbum.Render(label) + pad
	default:
		return v.Styles.TextPrimary.Render(label) + pad
	}
}

// tileTitle is the album's own title, without the owner that disambiguates
// it in the list: the artist line under the title already says whose it is.
func tileTitle(album string) string {
	if title := library.AlbumTitle(album); title != "" {
		return title
	}
	return "(unknown album)"
}
