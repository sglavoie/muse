package library

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	musiclib "github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// TestDuplicateArtistDimmedRegardlessOfSortField verifies that consecutive
// tracks sharing an artist receive TextTertiary styling on the second
// occurrence regardless of which column is sorted, and that the repeated
// cell collapses to a "·" continuation glyph rather than the literal artist
// name.
func TestDuplicateArtistDimmedRegardlessOfSortField(t *testing.T) {
	tracks := []musiclib.Track{
		{Artist: "Alice", Title: "AAA", Album: "Debut", Duration: 185},
		{Artist: "Alice", Title: "BBB", Album: "Debut", Duration: 200},
	}
	s := styles.NewStyles(true)
	v := newRenderView(tracks)
	v.Styles = s
	v.SortField = 2

	row2 := renderTrackRow(t, v, 1)
	styled := styles.WithZebraBg(s.TextTertiary, s.ZebraBg, true).Render("·")
	requireANSIStyleSupport(t, styled)

	ansiCode := strings.SplitN(styled, "·", 2)[0]
	if !strings.Contains(row2, ansiCode) {
		t.Errorf("expected TextTertiary+ZebraBg ANSI for duplicate artist; row: %q", row2)
	}
	plainRow := ansi.Strip(row2)
	if strings.Contains(plainRow, "Alice") {
		t.Errorf("duplicate artist row should not render 'Alice' literal; row: %q", row2)
	}
	if !strings.Contains(plainRow, "·") {
		t.Errorf("duplicate artist row should contain continuation glyph '·'; row: %q", row2)
	}
}

// TestDuplicateAlbumDimmed verifies that consecutive tracks sharing an
// album collapse the repeated album cell into a continuation glyph, mirroring
// the artist-dim behaviour.
func TestDuplicateAlbumDimmed(t *testing.T) {
	tracks := []musiclib.Track{
		{Artist: "Alice", Title: "AAA", Album: "Debut", Duration: 185},
		{Artist: "Bob", Title: "BBB", Album: "Debut", Duration: 200},
	}
	v := newRenderView(tracks)
	v.Styles = styles.NewStyles(true)

	row2 := ansi.Strip(renderTrackRow(t, v, 1))
	if strings.Contains(row2, "Debut") {
		t.Errorf("duplicate album row should not render 'Debut' literal; row: %q", row2)
	}
	if !strings.Contains(row2, "·") {
		t.Errorf("duplicate album row should contain continuation glyph '·'; row: %q", row2)
	}
}

// TestDimmingDisabledWhenFilterActive verifies that repeated-cell dimming is
// suspended while a filter query is active, since filtered results may be
// non-contiguous and dimming would be misleading.
func TestDimmingDisabledWhenFilterActive(t *testing.T) {
	tracks := []musiclib.Track{
		{Artist: "Alice", Title: "AAA", Album: "Debut", Duration: 185},
		{Artist: "Alice", Title: "BBB", Album: "Debut", Duration: 200},
	}
	v := newRenderView(tracks)
	v.Styles = styles.NewStyles(true)
	v.Fuzzy = musiclib.FakeFuzzySearcher{}
	v = activateFilterQuery(v, "A")

	plain := renderPlain(v)
	occurrences := strings.Count(plain, "Alice")
	if occurrences < 2 {
		t.Errorf("filtered render should keep 'Alice' on both rows (got %d occurrences); render:\n%s", occurrences, plain)
	}
}
