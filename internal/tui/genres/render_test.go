package genres

import (
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestRenderGenresShowsCounts(t *testing.T) {
	v := View{
		Level:       GenreLevel,
		Width:       80,
		Genres:      []string{"Rock", "Jazz"},
		GenreCounts: map[string]int{"Rock": 42, "Jazz": 7},
	}
	out := v.Render()
	for _, want := range []string{"Rock", "Jazz", "42", "7"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestRenderGenresCountDropsAtNarrowWidth(t *testing.T) {
	v := View{
		Level:       GenreLevel,
		Width:       10,
		Genres:      []string{"Rock"},
		GenreCounts: map[string]int{"Rock": 99},
	}
	out := v.Render()
	if strings.Contains(out, "99") {
		t.Errorf("expected count to drop at narrow width, got:\n%s", out)
	}
	if !strings.Contains(out, "Rock") {
		t.Errorf("expected label visible at narrow width, got:\n%s", out)
	}
}

func TestRenderGenresZeroCountHidden(t *testing.T) {
	v := View{
		Level:       GenreLevel,
		Width:       80,
		Genres:      []string{"Rock", "MysteryGenre"},
		GenreCounts: map[string]int{"Rock": 5},
	}
	out := v.Render()
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "MysteryGenre") && strings.Contains(line, "0") {
			t.Errorf("expected zero-count to render blank, got line: %q", line)
		}
	}
}

func TestRenderGenresClampsStaleScrollAfterFilter(t *testing.T) {
	v := View{
		Level:        GenreLevel,
		Width:        80,
		Height:       4,
		Genres:       []string{"Ambient", "Jazz", "Rock"},
		Cursor:       20,
		ScrollOffset: 20,
		FilterText:   "rock",
	}
	out := v.Render()
	if !strings.Contains(out, "Rock") {
		t.Fatalf("expected filtered match to remain visible with stale scroll, got:\n%s", out)
	}
	if strings.Contains(out, "(no matches)") {
		t.Fatalf("expected a visible match, got:\n%s", out)
	}
}

func TestRenderArtistsClampsNegativeScroll(t *testing.T) {
	v := View{
		Level:        ArtistLevel,
		Width:        80,
		Height:       4,
		Styles:       styles.NewStyles(true),
		Artists:      []string{"Alice", "Bob"},
		Cursor:       -3,
		ScrollOffset: -10,
	}
	out := v.Render()
	if !strings.Contains(out, "Alice") {
		t.Fatalf("expected first artist to render with negative scroll clamped, got:\n%s", out)
	}
}

func TestRenderTracksClampsStaleScrollAfterFilter(t *testing.T) {
	v := View{
		Level:  TrackLevel,
		Width:  80,
		Height: 5,
		Styles: styles.NewStyles(true),
		Tracks: tracks(
			struct{ genre, artist, title string }{"Rock", "Alice", "First"},
			struct{ genre, artist, title string }{"Rock", "Alice", "Second"},
			struct{ genre, artist, title string }{"Rock", "Alice", "Third"},
		),
		Cursor:       99,
		ScrollOffset: 99,
		FilterText:   "third",
	}
	out := v.Render()
	if !strings.Contains(out, "Third") {
		t.Fatalf("expected filtered track to remain visible with stale scroll, got:\n%s", out)
	}
	if strings.Contains(out, "(no matches)") {
		t.Fatalf("expected a visible track match, got:\n%s", out)
	}
}
