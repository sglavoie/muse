package albums

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/library"
)

func gridView(n, width, height int) View {
	albums := make([]string, n)
	for i := range albums {
		albums[i] = library.AlbumKey(library.Track{Path: fmt.Sprintf("/m/%04d/t.flac", i), Album: fmt.Sprintf("Album %04d", i), AlbumArtist: "Artist"})
	}
	return View{Albums: albums, Artwork: true, Width: width, Height: height}
}

func TestComputeGridAdaptsToThePanel(t *testing.T) {
	cases := []struct {
		width, height     int
		artRows, minCols  int
		wantGrid, wantTwo bool
	}{
		{60, 10, 6, 4, true, false},  // 60x15 terminal: one row of 12x6 covers
		{80, 19, 6, 5, true, true},   // 80x24: two rows of 12x6
		{120, 31, 6, 8, true, true},  // 120x36: three rows fill better than two of 16x8
		{200, 45, 8, 11, true, true}, // 200x50: large covers
		{24, 30, 4, 2, true, true},   // narrow: small covers before giving up
		{16, 30, 0, 0, false, false}, // too narrow for two tiles: the list
		{80, 6, 0, 0, false, false},  // too short for any row: the list
	}
	for _, tc := range cases {
		g, ok := ComputeGrid(tc.width, tc.height)
		if ok != tc.wantGrid {
			t.Fatalf("%dx%d: grid=%v, want %v", tc.width, tc.height, ok, tc.wantGrid)
		}
		if !ok {
			continue
		}
		if g.ArtRows != tc.artRows || g.ArtCols != 2*g.ArtRows || g.Cols < tc.minCols || (g.Rows >= 2) != tc.wantTwo {
			t.Fatalf("%dx%d: got %+v", tc.width, tc.height, g)
		}
		if used := g.Cols*g.ArtCols + (g.Cols-1)*g.Gap; used > tc.width || g.Gap < gridMinGap {
			t.Fatalf("%dx%d: tiles use %d columns with gap %d", tc.width, tc.height, used, g.Gap)
		}
	}
}

func TestGridOnlyWhenEnabledAndAtAlbumLevel(t *testing.T) {
	v := gridView(10, 80, 20)
	if _, ok := v.Grid(); !ok {
		t.Fatal("grid layout with artwork should draw a grid")
	}
	for name, mod := range map[string]func(View) View{
		"list layout": func(v View) View { v.Layout = LayoutList; return v },
		"no artwork":  func(v View) View { v.Artwork = false; return v },
		"track level": func(v View) View { v.Level = TrackLevel; return v },
	} {
		if _, ok := mod(v).Grid(); ok {
			t.Fatalf("%s must fall back to the list", name)
		}
	}
}

func TestMoveInGridNavigatesIn2D(t *testing.T) {
	v := gridView(12, 80, 20) // 5 across, 2 rows: rows hold 0-4, 5-9, 10-11
	g, _ := v.Grid()
	if g.Cols != 5 {
		t.Fatalf("want 5 columns, got %+v", g)
	}
	steps := []struct {
		dRows, dItems, want int
	}{
		{0, 1, 1},   // right
		{1, 0, 6},   // down one row
		{1, 0, 11},  // down into the short last row lands on its last tile
		{1, 0, 11},  // already on the last row: stays
		{-1, 0, 6},  // up
		{0, -2, 4},  // left wraps to the previous row
		{-1, 0, 4},  // up from the first row stays
		{0, -9, 0},  // clamps at the first tile
		{-5, 0, 0},  // a page up from the top stays
		{9, 0, 11},  // a long page down lands on the last tile
		{-9, 0, 1},  // a long page up keeps the column
		{0, 99, 11}, // clamps at the last tile
	}
	for i, s := range steps {
		v = v.MoveInGrid(s.dRows, s.dItems)
		if v.Cursor != s.want {
			t.Fatalf("step %d (%d,%d): cursor %d, want %d", i, s.dRows, s.dItems, v.Cursor, s.want)
		}
	}
}

func TestGridScrollKeepsTheFocusedRowVisible(t *testing.T) {
	v := gridView(100, 80, 20) // 5 across, 2 rows on screen
	v = v.MoveInGrid(4, 0)
	if v.Cursor != 20 || v.ScrollOffset != 15 {
		t.Fatalf("cursor %d scroll %d, want 20 and the row above it on top (15)", v.Cursor, v.ScrollOffset)
	}
	out := ansi.Strip(v.Render())
	if !strings.Contains(out, "Album 0020") || strings.Contains(out, "Album 0010") {
		t.Fatalf("visible rows are wrong:\n%s", out)
	}
	if !strings.Contains(out, "16–25 of 100") {
		t.Fatalf("strip should count the visible tiles:\n%s", out)
	}
	v = v.MoveInGrid(-1, 0).MoveInGrid(-1, 0)
	if v.Cursor != 10 || v.ScrollOffset != 10 {
		t.Fatalf("scrolling up: cursor %d scroll %d", v.Cursor, v.ScrollOffset)
	}
}

func TestGridRenderFitsThePanelAndMarksFocus(t *testing.T) {
	for _, size := range [][2]int{{60, 10}, {80, 19}, {120, 31}, {200, 45}} {
		v := gridView(40, size[0], size[1])
		v.Cursor = 1
		v.Releases = map[string]Release{v.Albums[1]: {Artist: "Somebody"}}
		out := v.Render()
		lines := strings.Split(out, "\n")
		if len(lines) != size[1] {
			t.Fatalf("%v: %d lines, want the panel height", size, len(lines))
		}
		for _, line := range lines {
			if w := ansi.StringWidth(line); w > size[0] {
				t.Fatalf("%v: line overflows (%d): %q", size, w, ansi.Strip(line))
			}
		}
		plain := ansi.Strip(out)
		if strings.Count(plain, gridFocusBar) == 0 || !strings.Contains(plain, "Somebody") {
			t.Fatalf("%v: focus bar or artist missing:\n%s", size, plain)
		}
		// Without covers every tile is a placeholder carrying initials.
		if !strings.Contains(plain, "A0") {
			t.Fatalf("%v: placeholder initials missing:\n%s", size, plain)
		}
	}
}

func TestGridDrawsCoversFromTheCallback(t *testing.T) {
	v := gridView(3, 60, 10)
	calls := 0
	v.Covers = func(_ string, cols, rows int) string {
		calls++
		return strings.TrimSuffix(strings.Repeat(strings.Repeat("#", cols)+"\n", rows), "\n")
	}
	out := ansi.Strip(v.Render())
	if calls != 3 || !strings.Contains(out, "############") {
		t.Fatalf("covers drawn %d times:\n%s", calls, out)
	}
}

func TestToggleLayoutKeepsFocusVisible(t *testing.T) {
	v := gridView(100, 80, 20).MoveInGrid(10, 0)
	v = v.ToggleLayout()
	if v.Layout != LayoutList || v.Cursor != 50 || v.ScrollOffset > 50 || v.ScrollOffset+v.visibleRows() <= 50 {
		t.Fatalf("list after toggle: %+v", v)
	}
	v = v.ToggleLayout()
	if v.Layout != LayoutGrid || v.ScrollOffset%5 != 0 {
		t.Fatalf("grid after toggle: scroll %d", v.ScrollOffset)
	}
}

func TestInitials(t *testing.T) {
	for in, want := range map[string]string{"Kind of Blue": "KO", "(unknown album)": "UA", "混雑": "混", "": "♪", "42": "4"} {
		if got := initials(in); got != want {
			t.Errorf("initials(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDeriveReleases(t *testing.T) {
	tracks := []library.Track{
		{Path: "/m/a/1.flac", Album: "Solo", Artist: "Ann"},
		{Path: "/m/a/2.flac", Album: "Solo", Artist: "Ann"},
		{Path: "/m/b/1.flac", Album: "Mix", Artist: "Ann"},
		{Path: "/m/b/2.flac", Album: "Mix", Artist: "Bob"},
		{Path: "/m/c/1.flac", Album: "Duo", Artist: "Cy", AlbumArtist: "The Duo"},
	}
	rel := DeriveReleases(tracks)
	if got := rel[library.AlbumKey(tracks[0])]; got.Artist != "Ann" || got.Track.Path != "/m/a/1.flac" {
		t.Fatalf("solo album: %+v", got)
	}
	if got := rel[library.AlbumKey(tracks[2])].Artist; got != "Various artists" {
		t.Fatalf("mixed album artist: %q", got)
	}
	if got := rel[library.AlbumKey(tracks[4])].Artist; got != "The Duo" {
		t.Fatalf("album artist tag: %q", got)
	}
}

// BenchmarkGridRenderLargeLibrary guards that a frame costs one screenful of
// tiles however many albums there are.
func BenchmarkGridRenderLargeLibrary(b *testing.B) {
	v := gridView(5000, 200, 45)
	v.Cursor = 2500
	v = v.KeepGridVisible()
	b.ResetTimer()
	for b.Loop() {
		_ = v.Render()
	}
}

// TestGridBuildsOnlyVisibleTiles guards rendering cost in a large library:
// only the tiles on screen are drawn, wherever the cursor is.
func TestGridBuildsOnlyVisibleTiles(t *testing.T) {
	v := gridView(20000, 200, 45)
	v.Cursor = 12345
	v = v.KeepGridVisible()
	g, _ := v.Grid()
	calls := 0
	v.Covers = func(string, int, int) string { calls++; return "" }
	out := ansi.Strip(v.Render())
	if calls != g.Cols*g.Rows {
		t.Fatalf("drew %d covers, want one screenful (%d)", calls, g.Cols*g.Rows)
	}
	if !strings.Contains(out, "Album 12345") {
		t.Fatalf("focused album not on screen:\n%s", out)
	}
}
