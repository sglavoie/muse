package chart

import (
	"math"
	"reflect"
	"testing"
)

func weekdayRows() []BarRow {
	return []BarRow{
		{Label: "Mon", Value: 449, Note: "27%"},
		{Label: "Tue", Value: 312, Note: "19%"},
		{Label: "Sun", Value: 23, Note: "1%"},
	}
}

func TestBarListPlain(t *testing.T) {
	cases := []struct {
		name string
		rows []BarRow
		opts BarListOptions
		want []string
	}{
		{
			name: "label bar and right-aligned note",
			rows: weekdayRows(),
			opts: BarListOptions{Width: 30},
			want: []string{
				"Mon ██████████████████████ 27%",
				"Tue ███████████████▎░░░░░░ 19%",
				"Sun █▏░░░░░░░░░░░░░░░░░░░░  1%",
			},
		},
		{
			name: "pinned Max leaves headroom above the largest row",
			rows: weekdayRows(),
			opts: BarListOptions{Width: 30, Max: 600},
			want: []string{
				"Mon ████████████████▌░░░░░ 27%",
				"Tue ███████████▌░░░░░░░░░░ 19%",
				"Sun ▉░░░░░░░░░░░░░░░░░░░░░  1%",
			},
		},
		{
			name: "pinned LabelWidth widens the label column",
			rows: weekdayRows(),
			opts: BarListOptions{Width: 30, LabelWidth: 6},
			want: []string{
				"Mon    ███████████████████ 27%",
				"Tue    █████████████▎░░░░░ 19%",
				"Sun    █░░░░░░░░░░░░░░░░░░  1%",
			},
		},
		{
			name: "notes are dropped before the bar goes under three cells",
			rows: weekdayRows(),
			opts: BarListOptions{Width: 6},
			want: []string{"Mo ███", "Tu ██▏", "Su ▏░░"},
		},
		{
			name: "labels are dropped once notes are gone and the bar still does not fit",
			rows: weekdayRows(),
			opts: BarListOptions{Width: 3},
			want: []string{"███", "██▏", "▏░░"},
		},
		{
			name: "width two keeps only the bar",
			rows: weekdayRows(),
			opts: BarListOptions{Width: 2},
			want: []string{"██", "█▍", "▏░"},
		},
		{
			name: "width one still distinguishes magnitudes",
			rows: weekdayRows(),
			opts: BarListOptions{Width: 1},
			want: []string{"█", "▊", "▏"},
		},
		{
			name: "unlabelled rows use the whole width for bars",
			rows: []BarRow{{Value: 3}, {Value: 1}},
			opts: BarListOptions{Width: 10},
			want: []string{"██████████", "███▍░░░░░░"},
		},
		{
			name: "all-zero values render an empty track",
			rows: []BarRow{{Label: "a"}, {Label: "b"}},
			opts: BarListOptions{Width: 10},
			want: []string{"a ░░░░░░░░", "b ░░░░░░░░"},
		},
		{
			name: "wide runes are truncated by display width",
			rows: []BarRow{{Label: "坂本龍一", Value: 5}, {Label: "ab", Value: 2}},
			opts: BarListOptions{Width: 18},
			want: []string{"坂本龍 ███████████", "ab     ████▍░░░░░░"},
		},
		{
			name: "NaN value is treated as zero",
			rows: []BarRow{{Label: "a", Value: math.NaN()}, {Label: "b", Value: 4}},
			opts: BarListOptions{Width: 14},
			want: []string{"a ░░░░░░░░░░░░", "b ████████████"},
		},
		{
			name: "tabs in labels never reach the output",
			rows: []BarRow{{Label: "a\tb", Value: 1}},
			opts: BarListOptions{Width: 10},
			want: []string{"a b ██████"},
		},
		{
			name: "zero width renders nothing",
			rows: weekdayRows(),
			opts: BarListOptions{Width: 0},
			want: nil,
		},
		{
			name: "negative width renders nothing",
			rows: weekdayRows(),
			opts: BarListOptions{Width: -4},
			want: nil,
		},
		{
			name: "no rows renders nothing",
			rows: nil,
			opts: BarListOptions{Width: 20},
			want: nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := barListPlain(c.rows, c.opts)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("barListPlain() =\n%q\nwant\n%q", got, c.want)
			}
		})
	}
}

func TestBarTrackPlain(t *testing.T) {
	cases := []struct {
		name     string
		fraction float64
		width    int
		want     string
	}{
		{"zero width", 0.615, 0, ""},
		{"negative width", 0.615, -3, ""},
		{"one cell rounds to an eighth block", 0.615, 1, "▋"},
		{"two cells", 0.615, 2, "█▎"},
		{"eight cells", 0.615, 8, "████▉░░░"},
		{"thirteen cells", 0.615, 13, "████████░░░░░"},
		{"full", 1, 6, "██████"},
		{"empty", 0, 6, "░░░░░░"},
		{"a tiny non-zero fraction stays visible", 0.0001, 6, "▏░░░░░"},
		{"NaN renders an empty track", math.NaN(), 6, "░░░░░░"},
		{"infinity renders an empty track", math.Inf(1), 6, "░░░░░░"},
		{"negative clamps to empty", -2, 6, "░░░░░░"},
		{"above one clamps to full", 1.4, 6, "██████"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := barTrackPlain(c.fraction, c.width); got != c.want {
				t.Fatalf("barTrackPlain(%v, %d) = %q, want %q", c.fraction, c.width, got, c.want)
			}
		})
	}
}

func TestRatioPlain(t *testing.T) {
	cases := []struct {
		name        string
		part, whole float64
		width       int
		want        string
	}{
		{"zero width", 916, 2382, 0, ""},
		{"too narrow for the percentage", 916, 2382, 3, "█▏░"},
		{"still too narrow at eight cells", 916, 2382, 8, "███▏░░░░"},
		{"bar plus percentage", 916, 2382, 13, "██▊░░░░ 38.5%"},
		{"wider bar", 916, 2382, 20, "█████▍░░░░░░░░ 38.5%"},
		{"zero whole", 5, 0, 12, "░░░░░░░ 0.0%"},
		{"NaN whole", 5, math.NaN(), 12, "░░░░░░░ 0.0%"},
		{"part exceeding whole clamps to full", 9, 3, 12, "█████ 100.0%"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ratioPlain(c.part, c.whole, c.width); got != c.want {
				t.Fatalf("ratioPlain(%v, %v, %d) = %q, want %q", c.part, c.whole, c.width, got, c.want)
			}
		})
	}
}

func TestBarListLayoutSumsToWidth(t *testing.T) {
	labels := []string{"Mon", "Tuesday"}
	notes := []string{"27%", "1%"}
	for width := 1; width <= 60; width++ {
		for _, requested := range []int{0, 6, 100} {
			labelW, barW, noteW := barListLayout(labels, notes, requested, width)
			total := labelW + barW + noteW
			if labelW > 0 {
				total++
			}
			if noteW > 0 {
				total++
			}
			if total != width {
				t.Fatalf("width=%d requested=%d: label=%d bar=%d note=%d sums to %d",
					width, requested, labelW, barW, noteW, total)
			}
		}
	}
}
