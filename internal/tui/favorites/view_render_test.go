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

func TestRenderList_ClampsNegativeScrollOffset(t *testing.T) {
	v := View{
		Level:        ListLevel,
		Category:     CategoryArtists,
		Width:        20,
		Height:       10,
		ScrollOffset: -2,
		Artists:      []string{"Alice", "Bob"},
	}
	out := v.Render()
	if !strings.Contains(out, "Alice") {
		t.Fatalf("expected render to start from first item, got:\n%s", out)
	}
}

func TestRenderTrackRows_ClampsNegativeScrollOffset(t *testing.T) {
	v := View{
		Level:        ListLevel,
		Category:     CategoryTracks,
		Width:        80,
		Height:       10,
		ScrollOffset: -2,
		Tracks:       sampleTracks(2),
	}
	out := v.Render()
	if !strings.Contains(out, "t0") {
		t.Fatalf("expected render to start from first track, got:\n%s", out)
	}
}
