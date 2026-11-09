package statusbar

import (
	"math"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func renderMeterPlain(m Meter, width int) string {
	ctx := newStatusBarRenderContext(width, styles.Styles{}, playback.StatePlaying)
	out, _ := StatusBar{}.renderLevelMeter(ctx, m)
	return ansi.Strip(out)
}

func TestLevelMeterDrawsOneBarPerBand(t *testing.T) {
	cases := []struct {
		name  string
		bands []float64
		want  string
	}{
		{"rest", make([]float64, 10), "▁▁▁▁▁▁▁▁▁▁"},
		{"bass to treble", []float64{1, 0.86, 0.72, 0.58, 0.43, 0.29, 0.15, 0, 0, 0}, "█▇▆▅▄▃▂▁▁▁"},
		{"out of range clamps", []float64{-1, 2, math.NaN()}, "▁█▁"},
	}
	for _, tc := range cases {
		if got := renderMeterPlain(Meter{Bands: tc.bands}, 100); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestLevelMeterPairsBandsInNarrowWindows(t *testing.T) {
	bands := []float64{0, 1, 0.43, 0.29, 0, 0, 0.58, 0.15, 1, 0}
	if got := renderMeterPlain(Meter{Bands: bands}, levelMeterNarrowBelow-1); got != "█▄▁▅█" {
		t.Fatalf("narrow meter = %q, want the louder of each pair", got)
	}
	if got := renderMeterPlain(Meter{Bands: bands}, 0); ansi.StringWidth(got) != len(bands) {
		t.Fatalf("unsized meter = %q, want one bar per band", got)
	}
}

func TestLevelMeterReplacesPlayIcon(t *testing.T) {
	sb := StatusBar{PlaybackState: playback.StatePlaying, Title: "Song", Artist: "Band",
		LevelMeter: &Meter{Bands: []float64{1, 0.5, 0}}}
	line := strings.SplitN(sb.Render(100, styles.NewStyles(true)), "\n", 2)[0]
	plain := ansi.Strip(line)
	if ansi.StringWidth(plain) != 100 {
		t.Fatalf("line 1 is %d cells wide, want 100: %q", ansi.StringWidth(plain), plain)
	}
	if !strings.HasPrefix(plain, "█▅▁ ") || strings.Contains(plain, "▶") {
		t.Fatalf("the meter should replace ▶ at the start of the line: %q", plain)
	}
}
