package artists

import (
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestRenderArtistsEmpty(t *testing.T) {
	v := View{Level: ArtistLevel}
	out := v.Render()
	if !strings.Contains(out, "No artists") {
		t.Fatalf("expected artists empty hint in output, got:\n%s", out)
	}
}

func TestRenderArtistsShowsNames(t *testing.T) {
	v := View{
		Level:   ArtistLevel,
		Width:   80,
		Artists: []string{"Alice", "Bob", "Charlie"},
	}
	out := v.Render()
	for _, want := range []string{"Alice", "Bob", "Charlie"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestRenderArtistsClampsStaleScrollOffset(t *testing.T) {
	v := View{
		Level:        ArtistLevel,
		Width:        80,
		Height:       3,
		ScrollOffset: 50,
		Artists:      []string{"Alice", "Bob", "Charlie"},
	}

	out := v.Render()
	if !strings.Contains(out, "Bob") || !strings.Contains(out, "Charlie") {
		t.Fatalf("expected visible rows after clamping stale scroll offset, got:\n%s", out)
	}
}

func TestRenderArtistsShowsCounts(t *testing.T) {
	v := View{
		Level:        ArtistLevel,
		Width:        80,
		Artists:      []string{"Alice", "Bob", "Charlie"},
		ArtistCounts: map[string]int{"Alice": 3, "Bob": 7, "Charlie": 1},
	}
	out := v.Render()
	for _, want := range []string{"Alice", "Bob", "Charlie", "3", "7", "1"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestRenderArtistsCountDropsAtNarrowWidth(t *testing.T) {
	v := View{
		Level:        ArtistLevel,
		Width:        10, // below MinLabelWithCount+1+CountColWidth (6+1+5=12)
		Artists:      []string{"Beatles"},
		ArtistCounts: map[string]int{"Beatles": 99},
	}
	out := v.Render()
	if strings.Contains(out, "99") {
		t.Errorf("expected count to drop at narrow width, got count visible in output:\n%s", out)
	}
	if !strings.Contains(out, "Beatles") {
		t.Errorf("expected label to remain visible at narrow width, got:\n%s", out)
	}
}

func TestRenderArtistsZeroCountLabeled(t *testing.T) {
	v := View{
		Level:        ArtistLevel,
		Width:        80,
		Artists:      []string{"Alice", "Unknown"},
		ArtistCounts: map[string]int{"Alice": 5, "Unknown": 0},
	}
	out := v.Render()
	if !strings.Contains(out, "5") {
		t.Errorf("expected Alice count 5 visible, got:\n%s", out)
	}
	// Zero is an explicit count, not an absent value.
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "Unknown") && !strings.Contains(line, "0 tracks") {
			t.Errorf("expected zero-count to be labeled, got line: %q", line)
		}
	}
}

func TestRenderAlbumsShowsNames(t *testing.T) {
	v := View{
		Level:          AlbumLevel,
		Width:          80,
		Albums:         []string{"Alpha", "Beta"},
		SelectedArtist: "Alice",
	}
	out := v.Render()
	for _, want := range []string{"Alpha", "Beta", "Alice"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestRenderAlbumsClampsNegativeScrollOffset(t *testing.T) {
	v := View{
		Level:        AlbumLevel,
		Width:        80,
		Height:       3,
		ScrollOffset: -5,
		Albums:       []string{"Alpha", "Beta", "Gamma"},
	}

	out := v.Render()
	if !strings.Contains(out, "Alpha") || !strings.Contains(out, "Beta") {
		t.Fatalf("expected first rows after clamping negative scroll offset, got:\n%s", out)
	}
}

func TestRenderAlbumsEmpty(t *testing.T) {
	v := View{Level: AlbumLevel, SelectedArtist: "Alice"}
	out := v.Render()
	if !strings.Contains(out, "No albums") {
		t.Fatalf("expected albums empty hint in output, got:\n%s", out)
	}
}

func TestRenderTracksEmpty(t *testing.T) {
	v := View{Level: TrackLevel}
	out := v.Render()
	if !strings.Contains(out, "No tracks") {
		t.Fatalf("expected tracks empty hint in output, got:\n%s", out)
	}
}

func TestRenderTracksClampsStaleScrollOffset(t *testing.T) {
	v := View{
		Level:        TrackLevel,
		Width:        80,
		Height:       4,
		ScrollOffset: 50,
		Tracks: []library.Track{
			{Title: "First", Duration: 60},
			{Title: "Second", Duration: 120},
			{Title: "Third", Duration: 180},
		},
	}

	out := v.Render()
	if !strings.Contains(out, "Third") {
		t.Fatalf("expected last track after clamping stale scroll offset, got:\n%s", out)
	}
}

func TestRenderTracksShowsColumns(t *testing.T) {
	v := View{
		Level: TrackLevel,
		Width: 80,
		Tracks: []library.Track{
			{Title: "Song A", Duration: 183},
			{Title: "Song B", Duration: 245},
		},
	}
	out := v.Render()
	for _, want := range []string{"Song A", "3:03", "Song B", "4:05"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestRenderTracksSequentialPositions(t *testing.T) {
	v := View{
		Level: TrackLevel,
		Width: 80,
		Tracks: []library.Track{
			{Title: "First", Duration: 60},
			{Title: "Second", Duration: 120},
			{Title: "Third", Duration: 180},
		},
	}
	out := v.Render()
	posFirst := strings.Index(out, "First")
	posSecond := strings.Index(out, "Second")
	posThird := strings.Index(out, "Third")
	if posFirst < 0 || posSecond < 0 || posThird < 0 {
		t.Fatalf("expected all track titles in output, got:\n%s", out)
	}
	if !(posFirst < posSecond && posSecond < posThird) {
		t.Errorf("tracks not in ascending sequential order:\n%s", out)
	}
}

func TestViewStylesCompileTimeAssertion(t *testing.T) {
	v := View{}
	var s styles.Styles = v.Styles
	if s.HasDark {
		t.Log("HasDark is true (unexpected for zero value but not an error)")
	}
}

func TestRenderTracksShowsSelectionMarker(t *testing.T) {
	tracks := []library.Track{
		{Title: "Selected", Hash: "h1", Duration: 60},
		{Title: "Not Selected", Hash: "h2", Duration: 60},
	}
	v := View{
		Level:          TrackLevel,
		Width:          80,
		Tracks:         tracks,
		SelectedHashes: map[string]struct{}{"h1": {}},
	}
	out := v.Render()
	if !strings.Contains(out, "●") {
		t.Errorf("expected selection marker ● in output for selected track, got:\n%s", out)
	}
}

func TestRenderTracksNoMarkersWhenNoneSelected(t *testing.T) {
	tracks := []library.Track{
		{Title: "Track A", Hash: "h1", Duration: 60},
		{Title: "Track B", Hash: "h2", Duration: 60},
	}
	v := View{
		Level:  TrackLevel,
		Width:  80,
		Tracks: tracks,
	}
	out := v.Render()
	if strings.Contains(out, "●") {
		t.Errorf("did not expect selection marker ● when nothing is selected, got:\n%s", out)
	}
}
