package chart

import (
	"reflect"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func heatRows() []HeatRow {
	return []HeatRow{
		{Label: "Mon", Cells: []HeatCell{{Value: 3, Present: true}, {Present: true}, {}, {Value: 9, Present: true}}},
		{Label: "Sun", Cells: []HeatCell{{Value: 1, Present: true}}},
	}
}

func TestHeatmapPlain(t *testing.T) {
	cases := []struct {
		name string
		rows []HeatRow
		opts HeatmapOptions
		want []string
	}{
		{
			name: "a present-but-silent cell is a gap and an absent one is blank",
			rows: heatRows(),
			opts: HeatmapOptions{Width: 12},
			want: []string{
				"Mon ▒· █    ",
				"Sun ░       ",
			},
		},
		{
			name: "a pinned Max holds the scale down",
			rows: heatRows(),
			opts: HeatmapOptions{Width: 12, Max: 36},
			want: []string{
				"Mon ░· ░    ",
				"Sun ░       ",
			},
		},
		{
			name: "the label column shrinks rather than leaving no room for a cell",
			rows: heatRows(),
			opts: HeatmapOptions{Width: 4, LabelWidth: 6},
			want: []string{
				"Mo █",
				"Su ▒",
			},
		},
		{
			name: "wide cells repeat the glyph across the column",
			rows: heatRows()[:1],
			opts: HeatmapOptions{Width: 12, CellWidth: 2},
			want: []string{"Mon ▒▒··  ██"},
		},
		{
			name: "trailing columns are dropped and the scale follows what is left",
			rows: heatRows(),
			opts: HeatmapOptions{Width: 6, LabelWidth: 3},
			want: []string{
				"Mon █·",
				"Sun ▒ ",
			},
		},
		{
			name: "TrimOldest keeps the recent end of a time series instead",
			rows: heatRows(),
			opts: HeatmapOptions{Width: 6, LabelWidth: 3, TrimOldest: true},
			want: []string{
				"Mon  █",
				"Sun   ",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := heatmapPlain(tc.rows, tc.opts)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestCalendarPlain(t *testing.T) {
	// Ten days beginning on a Wednesday: the first week is a partial one, so
	// Sunday through Tuesday of it must render as blank padding rather than as
	// silent days.
	days := make([]HeatCell, 10)
	for i := range days {
		days[i] = HeatCell{Value: float64(i % 3), Present: true}
	}
	got := calendarPlain(days, CalendarOptions{Width: 6, FirstWeekday: time.Wednesday})
	want := []string{
		"Sun  ▒",
		"Mon  █",
		"Tue  ·",
		"Wed ·▒",
		"Thu ▒█",
		"Fri █·",
		"Sat · ",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestCalendarKeepsTheMostRecentWeeks(t *testing.T) {
	// Ten full weeks whose values fall over time, so keeping the recent end
	// yields light glyphs and keeping the old end would yield heavy ones.
	days := make([]HeatCell, 70)
	for i := range days {
		days[i] = HeatCell{Value: float64(len(days) - i), Present: true}
	}
	got := calendarPlain(days, CalendarOptions{Width: 7, FirstWeekday: time.Sunday})
	for _, line := range got {
		if ansi.StringWidth(line) != 7 {
			t.Fatalf("line %q is not 7 columns", line)
		}
	}
	if got[6] != "Sat ▓▒░" {
		t.Fatalf("want the last three Saturdays kept, got %q", got[6])
	}
}

func TestHeatGlyphsAreSingleWidth(t *testing.T) {
	for _, g := range heatLevels {
		if w := ansi.StringWidth(g); w != 1 {
			t.Fatalf("heat glyph %q measures %d columns; a heatmap cell must be exactly one", g, w)
		}
	}
}
