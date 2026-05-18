package genres

import (
	"strings"
	"testing"
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
