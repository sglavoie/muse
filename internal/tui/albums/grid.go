package albums

import (
	"strings"
	"unicode"

	"github.com/sglavoie/muse/internal/library"
)

// Layout selects how the album level is drawn: a grid of cover tiles or the
// plain list. The zero value is the grid, which falls back to the list when
// covers are off or the panel is too small for two tiles side by side.
type Layout uint8

const (
	LayoutGrid Layout = iota
	LayoutList
)

// Release describes one album for its grid tile: a track to look the cover
// up by, and the artist line shown under the title.
type Release struct {
	Track  library.Track
	Artist string
}

// DeriveReleases maps every album key to the track its cover is looked up by
// and the artist its tile names. An album whose tracks credit different
// artists without an Album Artist tag reads "Various artists".
func DeriveReleases(tracks []library.Track) map[string]Release {
	if len(tracks) == 0 {
		return nil
	}
	out := make(map[string]Release)
	for _, t := range tracks {
		key := library.AlbumKey(t)
		r, seen := out[key]
		if !seen {
			artist := t.AlbumArtist
			if artist == "" {
				artist = t.Artist
			}
			out[key] = Release{Track: t, Artist: artist}
			continue
		}
		if t.AlbumArtist == "" && r.Track.AlbumArtist == "" && t.Artist != r.Track.Artist {
			r.Artist = "Various artists"
			out[key] = r
		}
	}
	return out
}

// Grid is the geometry of the cover grid inside the panel.
type Grid struct {
	Cols    int // tiles per row
	Rows    int // tile rows that fit
	ArtCols int // cover width in cells; also the tile width
	ArtRows int // cover height in cells
	Gap     int // blank columns between tiles
}

// gridTextRows is the title, artist and focus-bar lines under each cover.
// The focus bar row doubles as the gutter between tile rows, so the focused
// tile is marked without shifting anything.
const gridTextRows = 3

// gridMinGap is the narrowest gutter between tiles; wider panels spread the
// spare columns into the gutters, up to gridMaxGap.
const (
	gridMinGap = 2
	gridMaxGap = 4
)

// gridArtSizes are the preferred cover heights, largest first. Terminal
// cells are about twice as tall as wide, so a cover is 2×rows columns wide.
// gridSmallArtRows is the last resort for panels too short for either.
var gridArtSizes = []int{8, 6}

const gridSmallArtRows = 4

// gridFillMargin is how much more of the panel height a smaller cover size
// must fill before it wins over a larger one.
const gridFillMargin = 0.1

// TileHeight is the number of lines one tile row takes.
func (g Grid) TileHeight() int { return g.ArtRows + gridTextRows }

// ComputeGrid picks the cover size that fills width×height best, favouring
// larger covers unless a smaller size leaves clearly less empty space below
// the last row. It reports false when fewer than two tiles fit across or no
// tile row fits at all.
func ComputeGrid(width, height int) (Grid, bool) {
	var best Grid
	bestFill, found := 0.0, false
	for _, rows := range gridArtSizes {
		g, ok := gridFor(width, height, rows)
		if !ok {
			continue
		}
		fill := float64(g.Rows*g.TileHeight()) / float64(height)
		if !found || fill > bestFill+gridFillMargin {
			best, bestFill, found = g, fill, true
		}
	}
	if found {
		return best, true
	}
	return gridFor(width, height, gridSmallArtRows)
}

func gridFor(width, height, artRows int) (Grid, bool) {
	g := Grid{ArtRows: artRows, ArtCols: 2 * artRows}
	g.Rows = height / g.TileHeight()
	g.Cols = (width + gridMinGap) / (g.ArtCols + gridMinGap)
	if g.Rows < 1 || g.Cols < 2 {
		return Grid{}, false
	}
	g.Gap = min((width-g.Cols*g.ArtCols)/(g.Cols-1), gridMaxGap)
	return g, true
}

// Grid returns the grid geometry when the album level draws as a grid.
func (v View) Grid() (Grid, bool) {
	if v.Level != AlbumLevel || v.Layout != LayoutGrid || !v.Artwork {
		return Grid{}, false
	}
	return ComputeGrid(contentWidth(v.Width), max(v.Height-1, 0))
}

// gridTop returns the first visible tile row: the row ScrollOffset points
// into, moved just enough to keep the cursor's row on screen.
func (v View) gridTop(g Grid, n int) int {
	top := max(v.ScrollOffset, 0) / g.Cols
	cur := v.Cursor / g.Cols
	if cur < top {
		top = cur
	} else if cur >= top+g.Rows {
		top = cur - g.Rows + 1
	}
	lastTop := max((n-1)/g.Cols-g.Rows+1, 0)
	return max(min(top, lastTop), 0)
}

// KeepGridVisible scrolls the grid so the cursor's row is visible. In the
// grid ScrollOffset is the index of the first tile on screen, so switching
// to the list keeps roughly the same albums in view.
func (v View) KeepGridVisible() View {
	g, ok := v.Grid()
	if !ok {
		return v
	}
	n := len(v.EffectiveAlbums())
	if n == 0 {
		v.Cursor, v.ScrollOffset = 0, 0
		return v
	}
	v.Cursor = max(min(v.Cursor, n-1), 0)
	v.ScrollOffset = v.gridTop(g, n) * g.Cols
	return v
}

// MoveInGrid moves the cursor dRows tile rows down (negative: up) and then
// dItems tiles along the reading order. A vertical move that would leave the
// grid stays put, except that moving down from above a short last row lands
// on its last tile.
func (v View) MoveInGrid(dRows, dItems int) View {
	g, ok := v.Grid()
	n := len(v.EffectiveAlbums())
	if !ok || n == 0 {
		return v
	}
	cur := max(min(v.Cursor, n-1), 0)
	if dRows != 0 {
		next := cur + dRows*g.Cols
		lastRow := (n - 1) / g.Cols
		switch {
		case next < 0:
			next = cur % g.Cols
		case next > n-1 && cur/g.Cols < lastRow:
			next = n - 1
		case next > n-1:
			next = cur
		}
		cur = next
	}
	v.Cursor = max(min(cur+dItems, n-1), 0)
	return v.KeepGridVisible()
}

// PageRows returns how many tile rows a half (or full) page moves.
func (g Grid) PageRows(full bool) int {
	if full {
		return max(g.Rows, 1)
	}
	return max(g.Rows/2, 1)
}

// ToggleLayout switches between the grid and the list, keeping the focused
// album on screen in the new layout.
func (v View) ToggleLayout() View {
	if v.Layout == LayoutGrid {
		v.Layout = LayoutList
		return v.keepCursorVisible()
	}
	v.Layout = LayoutGrid
	return v.KeepGridVisible()
}

// initials returns up to two letters naming an album for its placeholder
// tile: the first letters of its first two words.
func initials(title string) string {
	var out []rune
	for _, word := range strings.Fields(title) {
		for _, r := range word {
			if isInitial(r) {
				out = append(out, r)
				break
			}
		}
		if len(out) == 2 {
			break
		}
	}
	if len(out) == 0 {
		return "♪"
	}
	return strings.ToUpper(string(out))
}

func isInitial(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// VisibleTiles returns the albums whose tiles are on screen, in order, with
// the grid geometry; it reports false when the album level is not a grid.
func (v View) VisibleTiles() ([]string, Grid, bool) {
	g, ok := v.Grid()
	if !ok {
		return nil, Grid{}, false
	}
	items := v.EffectiveAlbums()
	if len(items) == 0 {
		return nil, g, true
	}
	start := v.gridTop(g, len(items)) * g.Cols
	return items[start:min(len(items), start+g.Rows*g.Cols)], g, true
}
