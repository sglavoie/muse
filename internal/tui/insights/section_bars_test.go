package insights

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestSectionBarsAlignNamesFractionsAndNotes(t *testing.T) {
	rows := []Row{
		{Section: "Artists", Label: "Al", fraction: .5, note: "1/2 unheard"},
		{Section: "Artists", Label: "Much longer artist", fraction: .5, note: "10/20 unheard"},
		{Section: "Artists", Label: "全角名前", fraction: .5, note: "2/4 unheard"},
	}
	for _, width := range []int{58, 78, 150} {
		bodies := sectionBarBodies(rows, width, styles.NewStyles(true))
		start, end := -1, -1
		for _, body := range bodies {
			plain := ansi.Strip(body)
			at := strings.Index(plain, "█")
			last := strings.LastIndex(plain, "░")
			if at < 0 || last < 0 {
				t.Fatal(plain)
			}
			left := ansi.StringWidth(plain[:at])
			right := ansi.StringWidth(plain[:last]) + 1
			if start >= 0 && (left != start || right != end) {
				t.Fatalf("unaligned: %q", plain)
			}
			if right-left > 32 || ansi.StringWidth(plain) > width {
				t.Fatalf("oversized: %q", plain)
			}
			start, end = left, right
		}
	}
}

func TestSectionBarsOmitAllFullBars(t *testing.T) {
	rows := []Row{
		{Section: "Artists", Label: "Al", fraction: 1, note: "1/1 unheard"},
		{Section: "Artists", Label: "Bea", fraction: 1, note: "3/3 unheard"},
		{Section: "Albums", Label: "Solo", fraction: 1, note: "2/2 unheard"},
	}
	bodies := sectionBarBodies(rows, 60, styles.NewStyles(true))
	for i, body := range bodies[:2] {
		plain := ansi.Strip(body)
		if strings.ContainsAny(plain, "█░") {
			t.Fatalf("row %d drew a bar that compares nothing: %q", i, plain)
		}
		if !strings.Contains(plain, rows[i].note) || !strings.HasPrefix(plain, rows[i].Label) {
			t.Fatalf("row %d lost its label or note: %q", i, plain)
		}
	}
	// A section of one keeps its bar: there is nothing to be uniform with.
	if plain := ansi.Strip(bodies[2]); !strings.Contains(plain, "█") {
		t.Fatalf("single-row section lost its bar: %q", plain)
	}
}
