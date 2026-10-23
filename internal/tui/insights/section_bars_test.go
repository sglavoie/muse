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
