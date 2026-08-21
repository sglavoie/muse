package chart

import (
	"math"
	"reflect"
	"testing"
)

// monthBars is the real library's monthly listening hours, the series Momentum
// plots.
func monthBars() []VBar {
	return []VBar{
		{Label: "Mar", Value: 14.6},
		{Label: "Apr", Value: 66.2},
		{Label: "May", Value: 30.5},
		{Label: "Jun", Value: 15.0},
		{Label: "Jul", Value: 2.5},
		{Label: "Aug", Value: 23.8, Muted: true},
	}
}

func TestVBarChartPlain(t *testing.T) {
	cases := []struct {
		name string
		bars []VBar
		opts VBarOptions
		want []string
	}{
		{
			name: "column width derives from the labels plus a separator",
			bars: monthBars(),
			opts: VBarOptions{Width: 24, Height: 4},
			want: []string{
				"    ████                ",
				"    ████                ",
				"    ████▇▇▇▇        ▄▄▄▄",
				"▇▇▇▇████████▇▇▇▇▁▁▁▁████",
				"Mar Apr May Jun Jul Aug ",
			},
		},
		{
			name: "a pinned Max leaves headroom above the tallest column",
			bars: monthBars(),
			opts: VBarOptions{Width: 24, Height: 4, Max: 132.4},
			want: []string{
				"                        ",
				"                        ",
				"    ████                ",
				"▄▄▄▄████▇▇▇▇▄▄▄▄▁▁▁▁▆▆▆▆",
				"Mar Apr May Jun Jul Aug ",
			},
		},
		{
			name: "the oldest columns are dropped and the scale follows what is left",
			bars: monthBars(),
			opts: VBarOptions{Width: 9, Height: 2, ColWidth: 3},
			want: []string{
				"▂▂▂   ███",
				"███▂▂▂███",
				"JunJulAug",
			},
		},
		{
			name: "an unlabelled series packs one column per cell",
			bars: []VBar{{Value: 1}, {Value: 4}, {Value: 2}},
			opts: VBarOptions{Width: 5, Height: 2},
			want: []string{
				" █   ",
				"▄██  ",
			},
		},
		{
			name: "a series with no values at all draws an empty frame",
			bars: []VBar{{Label: "a"}, {Label: "b"}},
			opts: VBarOptions{Width: 6, Height: 2},
			want: []string{
				"      ",
				"      ",
				"a b   ",
			},
		},
		{
			name: "zero height leaves the label row alone",
			bars: monthBars()[:2],
			opts: VBarOptions{Width: 8},
			want: []string{"Mar Apr "},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := vbarChartPlain(tc.bars, tc.opts)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestVBarChartDropsTheOldestColumnsNotTheNewest(t *testing.T) {
	bars := monthBars()
	got := vbarChartPlain(bars, VBarOptions{Width: 8, Height: 1, ColWidth: 4})
	if len(got) != 2 {
		t.Fatalf("want a body row and a label row, got %q", got)
	}
	if got[1] != "Jul Aug " {
		t.Fatalf("want the two most recent months kept, got %q", got[1])
	}
}

func TestVBarChartRefusesToDrawWhenNoColumnFits(t *testing.T) {
	if got := vbarChartPlain(monthBars(), VBarOptions{Width: 3, Height: 2, ColWidth: 4}); got != nil {
		t.Fatalf("want no output when a single column cannot fit, got %q", got)
	}
}

func TestTrendPlain(t *testing.T) {
	cases := []struct {
		change float64
		want   string
	}{
		{1.2, "▲"},
		{-0.001, "▼"},
		{0, "·"},
		{math.NaN(), "·"},
		{math.Inf(1), "·"},
	}
	for _, tc := range cases {
		if got := trendPlain(tc.change); got != tc.want {
			t.Fatalf("Trend(%v) = %q, want %q", tc.change, got, tc.want)
		}
	}
}
