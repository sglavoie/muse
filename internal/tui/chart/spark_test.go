package chart

import (
	"math"
	"testing"
)

func TestSparklinePlain(t *testing.T) {
	cases := []struct {
		name   string
		values []float64
		width  int
		want   string
	}{
		{
			name:   "one glyph per value when they fit",
			values: []float64{1, 2, 3, 8, 7, 2, 3, 5, 6, 4, 7, 9, 8, 4, 3, 6, 1, 1},
			width:  20,
			want:   "▂▃▃▇▆▃▃▅▆▄▆█▇▄▃▆▂▂",
		},
		{
			name:   "excess values are averaged into buckets",
			values: []float64{1, 2, 3, 4, 5, 6, 7, 8},
			width:  4,
			want:   "▂▄▆█",
		},
		{
			name:   "all zero renders the lowest level",
			values: []float64{0, 0, 0},
			width:  8,
			want:   "▁▁▁",
		},
		{
			name:   "negative values are treated as zero",
			values: []float64{-5, 4, 8},
			width:  8,
			want:   "▁▅█",
		},
		{
			name:   "NaN and infinity are treated as zero",
			values: []float64{math.NaN(), math.Inf(1), math.Inf(-1), 4},
			width:  8,
			want:   "▁▁▁█",
		},
		{"no values", nil, 8, ""},
		{"zero width", []float64{1, 2}, 0, ""},
		{"negative width", []float64{1, 2}, -3, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := sparklinePlain(c.values, c.width); got != c.want {
				t.Fatalf("sparklinePlain(%v, %d) = %q, want %q", c.values, c.width, got, c.want)
			}
		})
	}
}

func TestSparklineGapsPlain(t *testing.T) {
	monthly := []SparkPoint{
		{Value: 14.6, Present: true},
		{Value: 66.2, Present: true},
		{Value: 30.5, Present: true},
		{Present: false},
		{Value: 15.0, Present: true},
		{Value: 2.5, Present: true},
		{Value: 23.8, Present: true},
	}
	cases := []struct {
		name   string
		points []SparkPoint
		width  int
		want   string
	}{
		{"an absent month reads as a gap not a zero", monthly, 20, "▃█▄·▃▁▄"},
		{
			name:   "a bucket is absent only when every point in it is",
			points: []SparkPoint{{Value: 1, Present: true}, {}, {}, {}, {Value: 4, Present: true}, {Value: 8, Present: true}},
			width:  3,
			want:   "▂·█",
		},
		{
			name:   "an absent point carries no value even when one is set",
			points: []SparkPoint{{Value: 99}, {Value: 4, Present: true}},
			width:  4,
			want:   "·█",
		},
		{"all absent", []SparkPoint{{}, {}}, 4, "··"},
		{"no points", nil, 8, ""},
		{"zero width", monthly, 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := sparklineGapsPlain(c.points, c.width); got != c.want {
				t.Fatalf("sparklineGapsPlain(%v, %d) = %q, want %q", c.points, c.width, got, c.want)
			}
		})
	}
}
