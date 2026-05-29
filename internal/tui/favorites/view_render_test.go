package favorites

import (
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/library"
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

func TestRenderList_ClampsHighScrollOffset(t *testing.T) {
	v := View{
		Level:        ListLevel,
		Category:     CategoryArtists,
		Width:        20,
		Height:       4, // visibleRows = 2
		Cursor:       4,
		ScrollOffset: 50,
		Artists:      []string{"Alice", "Bob", "Carol", "Daphne", "Eve"},
	}
	out := v.Render()
	if !strings.Contains(out, "Daphne") || !strings.Contains(out, "Eve") {
		t.Fatalf("expected render to clamp to final visible window, got:\n%s", out)
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

func TestRenderTrackRows_ClampsHighScrollOffset(t *testing.T) {
	v := View{
		Level:        ListLevel,
		Category:     CategoryTracks,
		Width:        80,
		Height:       5, // visibleRows = 2
		Cursor:       4,
		ScrollOffset: 50,
		Tracks:       sampleTracks(5),
	}
	out := v.Render()
	if !strings.Contains(out, "t3") || !strings.Contains(out, "t4") {
		t.Fatalf("expected render to clamp to final visible track window, got:\n%s", out)
	}
}

func TestRenderTrackRows_NarrowWidthTruncatesHeaderCells(t *testing.T) {
	v := View{
		Level:    ListLevel,
		Category: CategoryTracks,
		Width:    10,
		Height:   8,
		Tracks:   sampleTracks(1),
	}
	out := v.Render()
	for _, disallowed := range []string{"Title", "Artist", "Album"} {
		if strings.Contains(out, disallowed) {
			t.Fatalf("expected narrow header cells to be truncated, found %q in:\n%s", disallowed, out)
		}
	}
}

func TestRenderTrackRows_TruncatesOverwideFixedCells(t *testing.T) {
	tracks := sampleTracks(1)
	tracks[0].TrackNumber = 123456
	tracks[0].PlayCount = 123456789
	v := View{
		Level:    ListLevel,
		Category: CategoryTracks,
		Width:    80,
		Height:   8,
		Tracks:   tracks,
	}
	out := v.Render()
	if strings.Contains(out, "123456 ") {
		t.Fatalf("expected overwide track position to be truncated, got:\n%s", out)
	}
	if strings.Contains(out, "123456789") {
		t.Fatalf("expected overwide play count to be truncated, got:\n%s", out)
	}
}

func TestRenderTrackRows_DisplaysUnknownArtistAndAlbum(t *testing.T) {
	v := View{
		Level:    ListLevel,
		Category: CategoryTracks,
		Width:    100,
		Height:   8,
		Tracks: []library.Track{
			{Hash: "h1", Title: "Mystery"},
		},
	}
	out := v.Render()
	if !strings.Contains(out, "(unknown artist)") {
		t.Fatalf("expected unknown artist sentinel in track row, got:\n%s", out)
	}
	if !strings.Contains(out, "(unknown album)") {
		t.Fatalf("expected unknown album sentinel in track row, got:\n%s", out)
	}
}
