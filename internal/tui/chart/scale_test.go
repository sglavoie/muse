package chart

import (
	"math"
	"strings"
	"testing"
)

func TestScalePlain(t *testing.T) {
	ticks := []string{"0", "130", "261"}
	cases := []struct {
		name   string
		labels []string
		width  int
		want   string
	}{
		{"endpoints anchored with an evenly spaced middle", ticks, 24, "0          130       261"},
		{"two labels straddle the axis", []string{"Mar", "Aug"}, 12, "Mar      Aug"},
		{"a single label sits flush left", []string{"all-time"}, 12, "all-time"},
		{"a crowded middle label is dropped before the endpoints", ticks, 9, "0     261"},
		{"only the endpoints survive at minimum width", ticks, 5, "0 261"},
		{"empty labels are skipped", []string{"", "Aug"}, 8, "Aug"},
		{"a tab in a label never reaches the output", []string{"a\tb"}, 8, "a b"},
		{"an over-long label is truncated to width", []string{"all-time (fixed)"}, 6, "all-ti"},
		{"zero width", ticks, 0, ""},
		{"negative width", ticks, -4, ""},
		{"no labels", nil, 12, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := scalePlain(c.labels, c.width); got != c.want {
				t.Fatalf("scalePlain(%q, %d) = %q, want %q", c.labels, c.width, got, c.want)
			}
		})
	}
}

func TestLegendPlain(t *testing.T) {
	coverage := []LegendItem{
		{Glyph: "█", Label: "heard 916"},
		{Glyph: "░", Label: "unheard 1,466", Muted: true},
	}
	cases := []struct {
		name  string
		items []LegendItem
		width int
		want  string
	}{
		{"swatches joined by a separator", coverage, 40, "█ heard 916 · ░ unheard 1,466"},
		{"an entry that does not fit is dropped", coverage, 20, "█ heard 916"},
		{"labels without swatches", []LegendItem{{Label: "weekly"}, {Label: "Mar–Aug"}}, 30, "weekly · Mar–Aug"},
		{"a swatch without a label", []LegendItem{{Glyph: "⚠"}}, 10, "⚠"},
		{"fully empty entries are skipped", []LegendItem{{}, {Label: "Aug"}}, 10, "Aug"},
		{"zero width", coverage, 0, ""},
		{"no items", nil, 20, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := legendPlain(c.items, c.width); got != c.want {
				t.Fatalf("legendPlain(%v, %d) = %q, want %q", c.items, c.width, got, c.want)
			}
		})
	}
}

func TestDeltaPlain(t *testing.T) {
	cases := []struct {
		name   string
		change int
		want   string
	}{
		{"rise", 4, "▲4"},
		{"fall", -2, "▼2"},
		{"flat", 0, "·"},
		{"large rise", 12, "▲12"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := deltaPlain(c.change); got != c.want {
				t.Fatalf("deltaPlain(%d) = %q, want %q", c.change, got, c.want)
			}
		})
	}
}

func TestDeltaPlainMostNegativeIntDoesNotOverflow(t *testing.T) {
	got := deltaPlain(math.MinInt)
	if !strings.HasPrefix(got, glyphDown) {
		t.Fatalf("deltaPlain(math.MinInt) = %q, want a down arrow prefix", got)
	}
	if strings.Contains(got, "-") {
		t.Fatalf("deltaPlain(math.MinInt) = %q, want no sign in the magnitude", got)
	}
}

func TestFormatPercent(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0.615, "61.5%"},
		{0, "0.0%"},
		{1, "100.0%"},
		{math.NaN(), "0.0%"},
		{math.Inf(1), "0.0%"},
		{1.4, "100.0%"},
		{-0.5, "0.0%"},
	}
	for _, c := range cases {
		if got := formatPercent(c.in); got != c.want {
			t.Fatalf("formatPercent(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
