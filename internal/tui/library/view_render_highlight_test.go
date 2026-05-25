package library

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	musiclib "github.com/sglavoie/muse/internal/library"
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
