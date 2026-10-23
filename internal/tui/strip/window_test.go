package strip

import (
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
	"strings"
	"testing"
)

func TestWindowCountsAfterFilteringAndResize(t *testing.T) {
	counts := CountsChunk{Unit: "tracks", Total: 100, Filtered: 20, Filtering: true, WindowStart: 5, WindowEnd: 12}
	if got := buildCountsPlain(counts); got != "6–12 of 20 · 100 total" {
		t.Fatal(got)
	}
	counts.WindowStart, counts.WindowEnd = 0, 20
	if got := buildCountsPlain(counts); got != "20/100 tracks" {
		t.Fatal(got)
	}
	counts.Filtered, counts.WindowEnd = 0, 0
	if got := buildCountsPlain(counts); got != "0/100 tracks" {
		t.Fatal(got)
	}
	for _, width := range []int{60, 100} {
		out := ansi.Strip(Render(width, []string{"Library"}, SortChunk{}, FilterChunk{Text: strings.Repeat("query", 30)}, CountsChunk{Total: 100, WindowEnd: 5}, &SelectionChunk{Count: 4, Hidden: 2}, "", styles.NewStyles(true)))
		if !strings.Contains(out, "4 selected") || !strings.Contains(out, "2 hidden") || ansi.StringWidth(out) > width {
			t.Fatal(out)
		}
	}
}

func TestCollectionRangeReplacesCountWithoutTruncatingPadding(t *testing.T) {
	out := ansi.Strip(Collection(60, "Tags", "tag", 30, 60, styles.NewStyles(true), 3, 10))
	if !strings.Contains(out, "4–10 of 30 · 60 total") || strings.Contains(out, "…") || ansi.StringWidth(out) != 60 {
		t.Fatal(out)
	}
}
