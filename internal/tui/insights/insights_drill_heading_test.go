package insights

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// TestDrillHeadingShowsDrillLabel guards that a drilled track list is headed
// by what was drilled into, "Insights — <label>", instead of the embedded
// library view's fallback "Library".
func TestDrillHeadingShowsDrillLabel(t *testing.T) {
	v := View{}
	v.SetSize(100, 20)
	v.OpenDrill("Never played", []library.Track{{Hash: "h1", Title: "Song", Artist: "A"}})

	out := ansi.Strip(v.TrackView.Render(styles.NewStyles(true), nil, nil, nil, "", ""))
	first, _, _ := strings.Cut(out, "\n")
	if !strings.HasPrefix(first, "Insights — Never played") {
		t.Errorf("expected the strip to name the drill, got %q", first)
	}
}
