package favorites

import (
	"strings"
	"testing"
)

func TestRenderTracks_UsesDrilldownNameForHeading(t *testing.T) {
	v := View{
		Level:           TrackLevel,
		Category:        CategoryArtists,
		Width:           80,
		Height:          20,
		DrilldownTracks: sampleTracks(3),
		DrilldownName:   "Correct Artist",
		Artists:         []string{"Alice", "Bob", "Carol", "Correct Artist"},
		ListCursor:      0,
	}
	out := v.Render()
	if !strings.Contains(out, "Correct Artist") {
		t.Errorf("expected heading to contain DrilldownName 'Correct Artist', got:\n%s", out)
	}
}

func TestRenderTracks_EmptyDrilldownNameFallback(t *testing.T) {
	v := View{
		Level:           TrackLevel,
		Category:        CategoryArtists,
		Width:           80,
		Height:          20,
		DrilldownTracks: sampleTracks(2),
	}
	out := v.Render()
	if out == "" {
		t.Error("expected non-empty render output")
	}
}
