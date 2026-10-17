package albums

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/library"
)

func TestRenderTracksShowsTitles(t *testing.T) {
	v := View{
		Level:         TrackLevel,
		Width:         80,
		SelectedAlbum: "Alpha",
		Tracks: []library.Track{
			{Title: "Track A1", Duration: 183},
			{Title: "Track A2", Duration: 245},
		},
	}

	out := v.Render()
	if !strings.Contains(out, "Track A1") {
		t.Errorf("expected Track A1, got:\n%s", out)
	}
	if !strings.Contains(out, "Track A2") {
		t.Errorf("expected Track A2, got:\n%s", out)
	}
	if !strings.Contains(out, "Alpha") {
		t.Errorf("expected album heading 'Alpha', got:\n%s", out)
	}
}

func TestRenderTracksWithUnsetWidthShowsTitles(t *testing.T) {
	v := View{
		Level: TrackLevel,
		Tracks: []library.Track{
			{Title: "Track A1", Artist: "Alice", Duration: 183},
			{Title: "Track A2", Artist: "Alice", Duration: 245},
		},
	}

	out := v.Render()
	for _, want := range []string{"Track A1", "Track A2"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q with unset width, got:\n%s", want, out)
		}
	}
}

func TestRenderTracksNarrowHeaderDoesNotLeakHiddenArtistColumn(t *testing.T) {
	const width = 21

	v := View{
		Level: TrackLevel,
		Width: width,
		Tracks: []library.Track{{
			Title:    "A very long title",
			Artist:   "A very long artist",
			Duration: 60,
		}},
	}

	lines := strings.Split(ansi.Strip(v.Render()), "\n")
	var header string
	for _, line := range lines {
		if strings.Contains(line, "#") && strings.Contains(line, "Time") {
			header = line
			break
		}
	}
	if header == "" {
		t.Fatalf("expected track header in output:\n%s", v.Render())
	}
	if strings.Contains(header, "Artist") {
		t.Fatalf("artist header leaked despite zero-width artist column: %q", header)
	}
	if got := ansi.StringWidth(header); got > width {
		t.Fatalf("header width = %d, want <= %d: %q", got, width, header)
	}
}

func TestRenderTracksTruncatesFixedColumnsAtNarrowWidth(t *testing.T) {
	const width = 21

	v := View{
		Level: TrackLevel,
		Width: width,
		Tracks: []library.Track{{
			Title:       "A very long title",
			Artist:      "A very long artist",
			TrackNumber: 123456,
			Duration:    360000000,
		}},
	}

	lines := strings.Split(ansi.Strip(v.Render()), "\n")
	var row string
	for _, line := range lines {
		if strings.Contains(line, "…") {
			row = line
			break
		}
	}
	if row == "" {
		t.Fatalf("expected truncated track row in output:\n%s", v.Render())
	}
	if strings.Contains(row, "A very long artist") {
		t.Fatalf("artist text leaked despite zero-width artist column: %q", row)
	}
	if got := ansi.StringWidth(row); got > width {
		t.Fatalf("row width = %d, want <= %d: %q", got, width, row)
	}
}

func TestRenderTracksClampsHeaderAndRowWidthBelowMinTrackLayout(t *testing.T) {
	const width = 20

	v := View{
		Level: TrackLevel,
		Width: width,
		Tracks: []library.Track{{
			Title:    "A very long title",
			Artist:   "A very long artist",
			Duration: 60,
		}},
	}

	lines := strings.Split(ansi.Strip(v.Render()), "\n")
	var header, row string
	for i, line := range lines {
		if strings.Contains(line, "#") {
			header = line
			for _, candidate := range lines[i+1:] {
				if strings.TrimSpace(candidate) != "" {
					row = candidate
					break
				}
			}
			break
		}
	}
	if header == "" {
		t.Fatalf("expected track header in output:\n%s", v.Render())
	}
	if row == "" {
		t.Fatalf("expected track row in output:\n%s", v.Render())
	}
	if got := ansi.StringWidth(header); got > width {
		t.Fatalf("header width = %d, want <= %d: %q", got, width, header)
	}
	if got := ansi.StringWidth(row); got > width {
		t.Fatalf("row width = %d, want <= %d: %q", got, width, row)
	}
}

func TestRenderTracksClampsInvalidScrollOffset(t *testing.T) {
	tests := []struct {
		name         string
		scrollOffset int
	}{
		{name: "negative", scrollOffset: -1},
		{name: "past end", scrollOffset: 80},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := View{
				Level:        TrackLevel,
				Width:        80,
				Height:       20,
				ScrollOffset: tt.scrollOffset,
				Tracks: []library.Track{
					{Title: "Track A1", Duration: 183},
					{Title: "Track A2", Duration: 245},
				},
			}

			out := v.Render()
			for _, want := range []string{"Track A1", "Track A2"} {
				if !strings.Contains(out, want) {
					t.Fatalf("expected %q after clamping scroll offset %d, got:\n%s", want, tt.scrollOffset, out)
				}
			}
		})
	}
}

func TestRenderTracksEmpty(t *testing.T) {
	v := View{Level: TrackLevel, SelectedAlbum: "Alpha"}
	out := v.Render()
	if !strings.Contains(out, "No tracks") {
		t.Fatalf("expected tracks empty hint in output, got:\n%s", out)
	}
}

func TestRenderTracksShowsSelectionMarker(t *testing.T) {
	v := View{
		Level:         TrackLevel,
		Width:         80,
		SelectedAlbum: "Alpha",
		Tracks: []library.Track{
			{Title: "Selected", Hash: "h1", Duration: 60},
			{Title: "Not Selected", Hash: "h2", Duration: 60},
		},
		SelectedHashes: map[string]struct{}{"h1": {}},
	}

	out := v.Render()
	if !strings.Contains(out, "\n◉ ") {
		t.Errorf("expected selection marker ◉ in output for selected track, got:\n%s", out)
	}
}

func TestRenderTracksNoMarkersWhenNoneSelected(t *testing.T) {
	v := View{
		Level:         TrackLevel,
		Width:         80,
		SelectedAlbum: "Alpha",
		Tracks: []library.Track{
			{Title: "Track A", Hash: "h1", Duration: 60},
			{Title: "Track B", Hash: "h2", Duration: 60},
		},
	}

	out := v.Render()
	if strings.Contains(out, "●") {
		t.Errorf("did not expect selection marker ● when nothing is selected, got:\n%s", out)
	}
}
