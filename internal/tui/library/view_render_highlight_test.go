package library

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	musiclib "github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// TestCursorRowHighlighted verifies that the focused row carries styling
// (i.e. ANSI escapes are present) while non-focused rows are plain text.
func TestCursorRowHighlighted(t *testing.T) {
	accent := styles.NewStyles(true).Accent
	requireANSIStyleSupport(t, styledText(accent, "x"))

	v := newRenderView(sampleTracks)
	v.Cursor = 0
	v.Styles.Accent = accent

	rows := renderTrackRows(t, v, len(sampleTracks))
	focusedRow := rows[0]
	if ansi.Strip(focusedRow) == focusedRow {
		t.Errorf("focused row should contain ANSI styling, got: %q", focusedRow)
	}
	for _, row := range rows[1:] {
		if ansi.Strip(row) != row {
			t.Errorf("non-focused row should not contain ANSI styling, got: %q", row)
		}
	}
}

// TestFavoriteColorSurvivesSearchHighlight verifies that a favorited artist's
// foreground colour is preserved across bold-highlighted characters — the bug
// where inner SGR resets from bold rendering cleared the outer favourite colour.
func TestFavoriteColorSurvivesSearchHighlight(t *testing.T) {
	tracks := []musiclib.Track{
		{Artist: "Alice", Title: "Song One", Album: "Debut", Duration: 185},
	}
	s := styles.NewStyles(true)
	v := newRenderView(tracks)
	v.Fuzzy = highlightFake{results: []musiclib.Match{{TrackIndex: 0, ArtistPositions: []int{0, 1}}}}
	v.Styles = s
	v.FavoriteArtists = map[string]struct{}{"Alice": {}}
	v = activateFilterQuery(v, "al")

	got := v.Render()
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
	v := newRenderView(tracks)
	v.Fuzzy = highlightFake{results: []musiclib.Match{{TrackIndex: 0, MatchedIndexes: []int{0, 1}}}}
	v = activateFilterQuery(v, "Al")

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
	v := newRenderView(tracks)
	v.Fuzzy = highlightFake{results: []musiclib.Match{{TrackIndex: 0, MatchedIndexes: []int{0}}}}
	v = activateFilterQuery(v, "A")

	got := v.Render()
	boldA := styledText(lipgloss.NewStyle().Bold(true), "A")
	if !strings.Contains(got, boldA) {
		t.Errorf("expected bold-wrapped 'A' (%q) in rendered output, got:\n%q", boldA, got)
	}
}

// TestGeneralSearchHighlightsAlbumColumn verifies that when a general search
// matches text within the album field, the album column now shows bold
// highlights (previously album was not highlighted at all).
func TestGeneralSearchHighlightsAlbumColumn(t *testing.T) {
	tracks := []musiclib.Track{
		{Artist: "Alice", Title: "Song One", Album: "Debut", Duration: 185},
	}
	v := newRenderView(tracks)
	v.Fuzzy = highlightFake{results: []musiclib.Match{{TrackIndex: 0, MatchedIndexes: []int{15, 16, 17, 18, 19}}}}
	v = activateFilterQuery(v, "Debut")

	got := v.Render()
	boldDebut := styledText(lipgloss.NewStyle().Bold(true), "Debut")
	if !strings.Contains(got, boldDebut) {
		t.Errorf("expected bold 'Debut' in album column for general search matching album, got:\n%q", got)
	}
}

// TestNowPlayingRowKeepsBackgroundUnderFieldHighlight regression-tests a bug
// where a now-playing row that also had per-field match highlighting (e.g.
// the user was filtering and the current track matched the query) lost its
// NowPlayingBg on the artist/title/album cells. The markers and separators
// were rendered through NowPlayingRow but the cells went through padHighlight
// with no base style, leaving the row visually broken with mismatched
// backgrounds between the marker/separators and the content cells.
func TestNowPlayingRowKeepsBackgroundUnderFieldHighlight(t *testing.T) {
	tracks := []musiclib.Track{
		{Artist: "Alice", Title: "Song", Album: "Debut", Duration: 185, Hash: "h1"},
		{Artist: "Anna", Title: "Other", Album: "AlbX", Duration: 200, Hash: "h2"},
	}
	s := styles.NewStyles(true)
	requireANSIStyleSupport(t, s.NowPlayingRow.Render("x"))

	v := newRenderView(tracks)
	v.Styles = s
	v.NowPlayingHash = "h1"
	v.NowPlayingIcon = "▶"
	// Cursor must NOT be on the now-playing row, otherwise the focused branch
	// (Accent) takes over and the now-playing styling is bypassed.
	v.Cursor = 1
	v.Filter = filter.State{Text: "A"}
	v.FilteredMatches = []musiclib.Match{
		{TrackIndex: 0, ArtistPositions: []int{0}},
		{TrackIndex: 1, ArtistPositions: []int{0}},
	}

	got := v.Render()
	rows := strings.Split(got, "\n")
	if len(rows) < 3 {
		t.Fatalf("expected at least 3 rows (strip + header + 2 tracks), got %d", len(rows))
	}
	nowPlayingRow := rows[2] // first track row = Alice (now playing)

	// The bg ANSI emitted by lipgloss for the NowPlayingBg colour. The cells
	// that carry highlighting must all include this exact background.
	nowPlayingBgANSI := s.NowPlayingRow.Render("X")
	// Extract the SGR prefix that introduces the background sequence (e.g.
	// "\x1b[1;38;2;...;48;2;42;38;24m") — find the substring up through the
	// closing 'm' of the leading escape.
	prefixEnd := strings.IndexByte(nowPlayingBgANSI, 'm')
	if prefixEnd <= 0 {
		t.Fatalf("could not locate SGR prefix in NowPlayingRow render: %q", nowPlayingBgANSI)
	}
	// The substring "48;2;<r>;<g>;<b>" identifies the now-playing background.
	bgStart := strings.Index(nowPlayingBgANSI[:prefixEnd], "48;2;")
	if bgStart < 0 {
		t.Fatalf("could not locate 48;2; bg sequence: %q", nowPlayingBgANSI)
	}
	bgEnd := strings.IndexByte(nowPlayingBgANSI[bgStart:], 'm')
	bgSequence := nowPlayingBgANSI[bgStart : bgStart+bgEnd]

	// Confirm that the artist cell content "Alice" is followed by the
	// NowPlayingBg colour somewhere in the surrounding ANSI sequence. The
	// pre-fix bug emitted just "\x1b[1mA\x1b[m" with no background for the
	// matched rune; ensure both the matched character "A" and the trailing
	// "lice" are wrapped with NowPlayingBg.
	if !strings.Contains(nowPlayingRow, "A"+"\x1b[m") {
		// sanity check: ensure we are actually looking at the right row
		t.Fatalf("expected highlighted A in now-playing row, got:\n%q", nowPlayingRow)
	}
	// Look for the sub-pattern: bgSequence followed (within a short window) by
	// "lice" (the unhighlighted tail of the artist cell). The buggy version
	// rendered "lice" as plain text without the background.
	idx := strings.Index(nowPlayingRow, "lice")
	if idx < 0 {
		t.Fatalf("expected 'lice' in now-playing row, got:\n%q", nowPlayingRow)
	}
	// Check the segment leading up to "lice" includes the now-playing bg.
	prefix := nowPlayingRow[:idx]
	// Find the last ANSI escape before "lice".
	lastEsc := strings.LastIndex(prefix, "\x1b[")
	if lastEsc < 0 {
		t.Fatalf("expected ANSI escape before 'lice', got prefix:\n%q", prefix)
	}
	escAround := prefix[lastEsc:]
	if !strings.Contains(escAround, bgSequence) {
		t.Errorf("expected NowPlayingBg %q in ANSI escape preceding 'lice'; got:\n%q\nfull row:\n%q",
			bgSequence, escAround, nowPlayingRow)
	}
}
