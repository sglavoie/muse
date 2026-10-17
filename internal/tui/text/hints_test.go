package text

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestCompactHintsPreservesEssentialWholePairs(t *testing.T) {
	const hint = "j/k scrolls · enter edit · tab/⇧tab tabs · ctrl+s save · esc cancel"
	for _, width := range []int{27, 40, 56, 60, 80} {
		got := CompactHints(hint, width, "ctrl+s", "esc")
		if ansi.StringWidth(got) > width || strings.Contains(got, "…") {
			t.Fatalf("width %d: clipped hint %q", width, got)
		}
		for _, essential := range []string{"ctrl+s save", "esc cancel"} {
			if !strings.Contains(got, essential) {
				t.Errorf("width %d: missing %q in %q", width, essential, got)
			}
		}
		for _, pair := range strings.Split(got, " · ") {
			if !strings.Contains(hint, pair) {
				t.Errorf("partial pair: %q", pair)
			}
		}
	}
	if got := CompactHints(hint, 100, "ctrl+s", "esc"); got != hint {
		t.Fatalf("wide hint changed order: %q", got)
	}
}
