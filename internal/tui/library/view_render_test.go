package library

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	musiclib "github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// TestRenderColumnsDoNotOverflow verifies that each row in the rendered output
// has exactly the expected byte length at a given terminal width.
func TestRenderColumnsDoNotOverflow(t *testing.T) {
	idx := buildIndex(sampleTracks)

	for _, width := range []int{40, 80, 120, 160} {
		v := View{Index: idx, Width: width, Height: 0}
		rendered := v.Render()
		lines := strings.Split(rendered, "\n")

		// The header row is lines[0], track rows follow.
		if len(lines) < 1 {
			t.Fatalf("width=%d: expected at least 1 line, got %d", width, len(lines))
		}

		for _, line := range lines {
			got := ansi.StringWidth(line)
			if got != width {
				t.Errorf("width=%d: row has %d display columns, want %d: %q", width, got, width, line)
			}
		}
	}
}

// TestRenderEmptyIndex verifies the empty-library card surfaces its CTAs.
func TestRenderEmptyIndex(t *testing.T) {
	v := View{Width: 80, Height: 24}
	got := v.Render()
	if !strings.Contains(got, "Library is empty") {
		t.Errorf("empty library must show title; got: %q", got)
	}
	for _, want := range []string{"scan folders", "import", "help"} {
		if !strings.Contains(got, want) {
			t.Errorf("empty library card missing CTA %q; got: %q", want, got)
		}
	}
}

// TestRenderZeroWidthFallback verifies that a zero Width renders a simple
// "artist – title" format without panic.
func TestRenderZeroWidthFallback(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Width: 0, Height: 0}
	got := v.Render()
	// In fallback mode each row is "artist – title", not a padded columnar row.
	// The column header row starts with the literal "#" track-number header.
	for line := range strings.SplitSeq(got, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			t.Errorf("zero-width render should not contain a column header row, got: %q", line)
		}
	}
}

// TestRenderSortOrderUnknownArtistLast verifies that "Unknown Artist" entries
// sort below named artists in the rendered output.
func TestRenderSortOrderUnknownArtistLast(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Width: 80, Height: 0}
	got := v.Render()
	alicePos := strings.Index(got, "Alice")
	unknownPos := strings.Index(got, "Unknown Artist")
	if alicePos == -1 || unknownPos == -1 {
		t.Fatalf("could not find expected artist names in render:\n%s", got)
	}
	if alicePos > unknownPos {
		t.Errorf("expected named artists before Unknown Artist; alice at %d, unknown at %d", alicePos, unknownPos)
	}
}

func TestRenderShowsDateAddedColumnWhenEnabled(t *testing.T) {
	tracks := []musiclib.Track{
		{Artist: "Alice", Title: "Song One", Album: "Debut", Duration: 185, CreatedAt: 1711929600},
	}
	idx := buildIndex(tracks)
	v := View{Index: idx, Width: 100, Height: 10, ShowDateAdded: true}
	got := ansi.Strip(v.Render())
	wantDate := time.Unix(1711929600, 0).Local().Format("2006-01-02")
	if !strings.Contains(got, "Added") {
		t.Fatalf("expected Added header in render, got:\n%s", got)
	}
	if !strings.Contains(got, wantDate) {
		t.Fatalf("expected formatted date in render, got:\n%s", got)
	}
}

// TestCursorRowHighlighted verifies that the focused row carries styling
// (i.e. ANSI escapes are present) while non-focused rows are plain text.
func TestCursorRowHighlighted(t *testing.T) {
	idx := buildIndex(sampleTracks)
	s := styles.NewStyles(true).Accent
	accentMarker := styledText(s, "x") // produces a reverse-video ANSI sequence
	if ansi.Strip(accentMarker) == accentMarker {
		t.Skip("lipgloss produced no ANSI output in this terminal environment")
	}

	v := View{Index: idx, Width: 80, Height: 0, Cursor: 0}
	v.Styles.Accent = s
	got := v.Render()
	lines := strings.Split(got, "\n")
	// Track rows begin after the header row (index 1).
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines, got %d", len(lines))
	}
	focusedRow := lines[1]
	if ansi.Strip(focusedRow) == focusedRow {
		t.Errorf("focused row should contain ANSI styling, got: %q", focusedRow)
	}
	// Non-focused rows must not carry styling.
	for _, line := range lines[2:] {
		if ansi.Strip(line) != line {
			t.Errorf("non-focused row should not contain ANSI styling, got: %q", line)
		}
	}
}

// TestRenderScrollOffsetBeyondTrackCount verifies that Render does not panic
// when ScrollOffset exceeds the number of tracks (e.g. after tracks are removed).
func TestRenderScrollOffsetBeyondTrackCount(t *testing.T) {
	idx := buildIndex(sampleTracks) // 3 tracks
	v := View{Index: idx, Width: 80, Height: 24, ScrollOffset: 100}
	got := v.Render()
	// The output should be the header + empty body (no panic, no crash).
	if !strings.Contains(got, "Artist") {
		t.Errorf("expected column header in render even with out-of-bounds scroll, got:\n%s", got)
	}
}

// TestFavoriteColorSurvivesSearchHighlight verifies that a favorited artist's
// foreground colour is preserved across bold-highlighted characters — the bug
// where inner SGR resets from bold rendering cleared the outer favourite colour.
func TestFavoriteColorSurvivesSearchHighlight(t *testing.T) {
	tracks := []musiclib.Track{
		{Artist: "Alice", Title: "Song One", Album: "Debut", Duration: 185},
	}
	idx := buildIndex(tracks)
	// Match positions 0,1 in Artist → "Al" bold-highlighted.
	fake := highlightFake{
		results: []musiclib.Match{
			{TrackIndex: 0, ArtistPositions: []int{0, 1}},
		},
	}
	s := styles.NewStyles(true) // dark theme for deterministic colour codes
	v := View{
		Index:           idx,
		Width:           80,
		Height:          24,
		Fuzzy:           fake,
		Styles:          s,
		FavoriteArtists: map[string]struct{}{"Alice": {}},
	}
	v = v.ActivateFilter()
	v = v.AppendFilterChar('a')
	v = v.AppendFilterChar('l')
	got := v.Render()

	// The first row is focused (cursor=0), so per-cell accent styling uses
	// FavArtistAccent (combined fg+bg) as the base for padHighlight.
	boldAccent := s.FavArtistAccent.Bold(true)
	plainAccent := s.FavArtistAccent
	if !strings.Contains(got, boldAccent.Render("Al")) {
		t.Errorf("expected bold+FavArtistAccent 'Al', got:\n%q", got)
	}
	if !strings.Contains(got, plainAccent.Render("ice")) {
		t.Errorf("expected FavArtistAccent 'ice' (non-bold remainder), got:\n%q", got)
	}
}

// TestHighlightedRowContainsBoldANSI verifies that a rendered row with
// non-nil MatchedIndexes contains bold ANSI codes at the matched positions.
func TestHighlightedRowContainsBoldANSI(t *testing.T) {
	tracks := []musiclib.Track{
		{Artist: "Alice", Title: "Song One", Album: "Debut", Duration: 185},
	}
	idx := buildIndex(tracks)
	// Positions 0 and 1 in the concatenated "Alice Song One …" string are
	// 'A' and 'l' in the Artist field.
	fake := highlightFake{
		results: []musiclib.Match{
			{TrackIndex: 0, MatchedIndexes: []int{0, 1}},
		},
	}
	v := View{
		Index:  idx,
		Width:  80,
		Height: 24,
		Fuzzy:  fake,
	}
	v = v.ActivateFilter()
	v = v.AppendFilterChar('A')
	v = v.AppendFilterChar('l')
	got := v.Render()
	boldAl := styledText(lipgloss.NewStyle().Bold(true), "Al")
	if !strings.Contains(got, boldAl) {
		t.Errorf("rendered row should contain bold-styled match highlights, got:\n%q", got)
	}
}

// TestHighlightedPositionsWithinArtistColumn verifies that bold codes appear
// before the correct rune positions within the artist column.
func TestHighlightedPositionsWithinArtistColumn(t *testing.T) {
	tracks := []musiclib.Track{
		{Artist: "Alice", Title: "Song One", Album: "Debut", Duration: 185},
	}
	idx := buildIndex(tracks)
	// Match position 0 → 'A' in Artist.
	fake := highlightFake{
		results: []musiclib.Match{
			{TrackIndex: 0, MatchedIndexes: []int{0}},
		},
	}
	v := View{
		Index:  idx,
		Width:  80,
		Height: 24,
		Fuzzy:  fake,
	}
	v = v.ActivateFilter()
	v = v.AppendFilterChar('A')
	got := v.Render()
	// The bold sequence should precede 'A' somewhere in the output.
	boldA := styledText(lipgloss.NewStyle().Bold(true), "A")
	if !strings.Contains(got, boldA) {
		t.Errorf("expected bold-wrapped 'A' (%q) in rendered output, got:\n%q", boldA, got)
	}
}

// TestDuplicateArtistDimmedRegardlessOfSortField verifies that consecutive
// tracks sharing an artist receive TextTertiary styling on the second
// occurrence regardless of which column is sorted (previously only triggered
// when SortField == 1).
func TestDuplicateArtistDimmedRegardlessOfSortField(t *testing.T) {
	tracks := []musiclib.Track{
		{Artist: "Alice", Title: "AAA", Album: "Debut", Duration: 185},
		{Artist: "Alice", Title: "BBB", Album: "Debut", Duration: 200},
	}
	idx := buildIndex(tracks)
	s := styles.NewStyles(true)
	// SortField=2 means sort-by-title, not sort-by-artist — dedup must still
	// apply under the new unconditional sameArtist check.
	v := View{Index: idx, Width: 80, Height: 24, Styles: s, SortField: 2}
	got := v.Render()
	lines := strings.Split(got, "\n")
	if len(lines) < 3 {
		t.Fatalf("want ≥3 lines (header + 2 tracks), got %d", len(lines))
	}
	row2 := lines[2] // absIdx=1 (isOdd=true), duplicate Alice
	// Duplicate artist must be styled with TextTertiary + ZebraBg.
	styled := styles.WithZebraBg(s.TextTertiary, s.ZebraBg, true).Render("Alice")
	if styled == "Alice" {
		t.Skip("lipgloss produced no ANSI output in this environment")
	}
	ansiCode := strings.SplitN(styled, "Alice", 2)[0]
	if !strings.Contains(row2, ansiCode) {
		t.Errorf("expected TextTertiary+ZebraBg ANSI for duplicate artist; row: %q", row2)
	}
}

// TestGeneralSearchHighlightsAlbumColumn verifies that when a general search
// matches text within the album field, the album column now shows bold
// highlights (previously album was not highlighted at all).
func TestGeneralSearchHighlightsAlbumColumn(t *testing.T) {
	tracks := []musiclib.Track{
		{Artist: "Alice", Title: "Song One", Album: "Debut", Duration: 185},
	}
	idx := buildIndex(tracks)
	// Positions 15-19 in "Alice Song One Debut file" correspond to "Debut".
	fake := highlightFake{
		results: []musiclib.Match{
			{TrackIndex: 0, MatchedIndexes: []int{15, 16, 17, 18, 19}},
		},
	}
	v := View{
		Index:  idx,
		Width:  80,
		Height: 24,
		Fuzzy:  fake,
	}
	v = v.ActivateFilter()
	for _, ch := range "Debut" {
		v = v.AppendFilterChar(ch)
	}
	got := v.Render()
	boldDebut := styledText(lipgloss.NewStyle().Bold(true), "Debut")
	if !strings.Contains(got, boldDebut) {
		t.Errorf("expected bold 'Debut' in album column for general search matching album, got:\n%q", got)
	}
}
