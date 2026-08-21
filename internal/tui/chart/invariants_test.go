package chart

import (
	"image/color"
	"math"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/tui/styles"
)

// primitive pairs a styled renderer with its plain core so the shared
// invariants can be asserted once for every chart primitive. width is the
// caller-supplied cap; -1 means the primitive takes no width.
type primitive struct {
	name   string
	width  int
	styled func(Palette) []string
	plain  func() []string
}

func one(s string) []string { return []string{s} }

func invariantPrimitives(width int) []primitive {
	rows := []BarRow{
		{Label: "Toumani Diabaté", Value: 58, Note: "9.0h"},
		{Label: "坂本龍一", Value: 40, Note: "8.8h"},
		{Label: "A Winged Victory for the Sullen and Then Some", Value: 0, Note: ""},
		{Label: "tab\there", Value: math.NaN(), Note: "—"},
	}
	opts := BarListOptions{Width: width}
	values := []float64{14.6, 66.2, 30.5, math.NaN(), 15.0, 2.5, 23.8, math.Inf(1), -4}
	points := []SparkPoint{
		{Value: 14.6, Present: true}, {Present: false}, {Value: math.NaN(), Present: true},
		{Value: 2.5, Present: true}, {Value: math.Inf(-1), Present: true}, {Present: false},
	}
	ticks := []string{"0", "130", "261", "392"}
	items := []LegendItem{
		{Glyph: "█", Label: "heard 916"},
		{Glyph: "░", Label: "unheard 1,466", Muted: true},
		{Label: "tab\there"},
	}
	vbars := []VBar{
		{Label: "Mar", Value: 14.6},
		{Label: "坂本", Value: math.NaN()},
		{Label: "tab\there", Value: math.Inf(1)},
		{Label: "Aug", Value: 23.8, Muted: true},
	}
	grid := []HeatRow{
		{Label: "15:00", Cells: []HeatCell{{Value: 261, Present: true}, {Present: true}, {}}},
		{Label: "坂本龍一", Cells: []HeatCell{{Value: math.NaN(), Present: true}}},
		{Label: "tab\there"},
	}
	calendarDays := []HeatCell{
		{Value: 63, Present: true}, {Present: true}, {Value: math.Inf(-1), Present: true}, {},
		{Value: 12, Present: true}, {Value: 4, Present: true}, {Present: true}, {Value: 9, Present: true},
	}
	return []primitive{
		{
			name:   "BarList",
			width:  width,
			styled: func(p Palette) []string { return BarList(rows, opts, p) },
			plain:  func() []string { return barListPlain(rows, opts) },
		},
		{
			name:   "BarTrack",
			width:  width,
			styled: func(p Palette) []string { return one(BarTrack(0.615, width, p)) },
			plain:  func() []string { return one(barTrackPlain(0.615, width)) },
		},
		{
			name:   "Ratio",
			width:  width,
			styled: func(p Palette) []string { return one(Ratio(916, 2382, width, p)) },
			plain:  func() []string { return one(ratioPlain(916, 2382, width)) },
		},
		{
			name:   "Sparkline",
			width:  width,
			styled: func(p Palette) []string { return one(Sparkline(values, width, p)) },
			plain:  func() []string { return one(sparklinePlain(values, width)) },
		},
		{
			name:   "SparklineGaps",
			width:  width,
			styled: func(p Palette) []string { return one(SparklineGaps(points, width, p)) },
			plain:  func() []string { return one(sparklineGapsPlain(points, width)) },
		},
		{
			name:   "Scale",
			width:  width,
			styled: func(p Palette) []string { return one(Scale(ticks, width, p)) },
			plain:  func() []string { return one(scalePlain(ticks, width)) },
		},
		{
			name:   "Legend",
			width:  width,
			styled: func(p Palette) []string { return one(Legend(items, width, p)) },
			plain:  func() []string { return one(legendPlain(items, width)) },
		},
		{
			name:   "Delta",
			width:  -1,
			styled: func(p Palette) []string { return one(Delta(-12, p)) },
			plain:  func() []string { return one(deltaPlain(-12)) },
		},
		{
			name:   "Trend",
			width:  -1,
			styled: func(p Palette) []string { return one(Trend(-0.4, p)) },
			plain:  func() []string { return one(trendPlain(-0.4)) },
		},
		{
			name:  "VBarChart",
			width: width,
			styled: func(p Palette) []string {
				return VBarChart(vbars, VBarOptions{Width: width, Height: 3}, p)
			},
			plain: func() []string { return vbarChartPlain(vbars, VBarOptions{Width: width, Height: 3}) },
		},
		{
			name:   "Heatmap",
			width:  width,
			styled: func(p Palette) []string { return Heatmap(grid, HeatmapOptions{Width: width}, p) },
			plain:  func() []string { return heatmapPlain(grid, HeatmapOptions{Width: width}) },
		},
		{
			name:  "Calendar",
			width: width,
			styled: func(p Palette) []string {
				return Calendar(calendarDays, CalendarOptions{Width: width, FirstWeekday: time.Friday}, p)
			},
			plain: func() []string {
				return calendarPlain(calendarDays, CalendarOptions{Width: width, FirstWeekday: time.Friday})
			},
		},
	}
}

func invariantPalettes() map[string]Palette {
	return map[string]Palette{
		"dark":  FromStyles(styles.NewStyles(true)),
		"light": FromStyles(styles.NewStyles(false)),
		"zero":  {},
	}
}

// TestStyledRenderersPreserveDisplayWidth pins the same contract
// renderHintKeyLabel documents in footer_hints.go: width arithmetic upstream
// uses the unstyled string, so styling must never change display width.
func TestStyledRenderersPreserveDisplayWidth(t *testing.T) {
	for _, width := range []int{0, 1, 2, 3, 7, 20, 60, 110} {
		for name, p := range invariantPalettes() {
			for _, prim := range invariantPrimitives(width) {
				t.Run(prim.name+"/"+name, func(t *testing.T) {
					styled := prim.styled(p)
					plain := prim.plain()
					if len(styled) != len(plain) {
						t.Fatalf("width=%d: styled has %d lines, plain has %d", width, len(styled), len(plain))
					}
					for i := range styled {
						gotW := ansi.StringWidth(styled[i])
						wantW := ansi.StringWidth(plain[i])
						if gotW != wantW {
							t.Fatalf("width=%d line %d: styled width %d != plain width %d\nstyled=%q\nplain=%q",
								width, i, gotW, wantW, styled[i], plain[i])
						}
					}
				})
			}
		}
	}
}

func TestRenderedLinesNeverExceedRequestedWidth(t *testing.T) {
	for _, width := range []int{0, 1, 2, 3, 7, 20, 60, 110} {
		for name, p := range invariantPalettes() {
			for _, prim := range invariantPrimitives(width) {
				if prim.width < 0 {
					continue
				}
				t.Run(prim.name+"/"+name, func(t *testing.T) {
					for _, lines := range [][]string{prim.styled(p), prim.plain()} {
						for i, line := range lines {
							if w := ansi.StringWidth(line); w > prim.width {
								t.Fatalf("width=%d line %d has width %d: %q", prim.width, i, w, line)
							}
						}
					}
				})
			}
		}
	}
}

func TestRenderedOutputNeverContainsATab(t *testing.T) {
	for _, width := range []int{0, 1, 3, 20, 110} {
		for name, p := range invariantPalettes() {
			for _, prim := range invariantPrimitives(width) {
				t.Run(prim.name+"/"+name, func(t *testing.T) {
					for _, lines := range [][]string{prim.styled(p), prim.plain()} {
						for i, line := range lines {
							if strings.ContainsAny(line, "\t\n\r") {
								t.Fatalf("width=%d line %d contains a control character: %q", width, i, line)
							}
						}
					}
				})
			}
		}
	}
}

// TestDegenerateInputsDoNotPanic sweeps the inputs that would otherwise reach
// strings.Repeat as a NaN-derived count.
func TestDegenerateInputsDoNotPanic(t *testing.T) {
	widths := []int{-3, 0, 1, 2, 3, 5, 80}
	values := []float64{math.NaN(), math.Inf(1), math.Inf(-1), -1e18, -1, 0, 1e18}
	palettes := invariantPalettes()
	for _, width := range widths {
		for _, v := range values {
			for name, p := range palettes {
				t.Run(name, func(t *testing.T) {
					BarList([]BarRow{{Label: "", Value: v}, {Label: "x", Value: 0}},
						BarListOptions{Width: width, LabelWidth: width * 2, Max: v}, p)
					BarList(nil, BarListOptions{Width: width}, p)
					BarTrack(v, width, p)
					Ratio(v, v, width, p)
					Ratio(v, 0, width, p)
					Sparkline([]float64{v, 0, -v}, width, p)
					Sparkline(nil, width, p)
					SparklineGaps([]SparkPoint{{Value: v}, {Value: v, Present: true}}, width, p)
					SparklineGaps(nil, width, p)
					Scale([]string{"", "0", "\t"}, width, p)
					Scale(nil, width, p)
					Legend([]LegendItem{{}, {Glyph: "█", Label: "x"}}, width, p)
					Legend(nil, width, p)
					Delta(width, p)
					Trend(v, p)
					VBarChart([]VBar{{Value: v}, {Label: "x", Value: -v, Muted: true}},
						VBarOptions{Width: width, Height: width, ColWidth: width, Max: v}, p)
					VBarChart(nil, VBarOptions{Width: width, Height: 3}, p)
					Heatmap([]HeatRow{{Label: "\t", Cells: []HeatCell{{Value: v, Present: true}, {Value: v}}}, {}},
						HeatmapOptions{Width: width, LabelWidth: width * 2, CellWidth: width, Max: v, TrimOldest: true}, p)
					Heatmap(nil, HeatmapOptions{Width: width}, p)
					Calendar([]HeatCell{{Value: v, Present: true}, {}, {Value: -v, Present: true}},
						CalendarOptions{Width: width, FirstWeekday: time.Weekday(width), Max: v}, p)
					Calendar(nil, CalendarOptions{Width: width}, p)
				})
			}
		}
	}
}

func TestFromStylesUsesForegroundOnlyStyles(t *testing.T) {
	for _, hasDark := range []bool{true, false} {
		p := FromStyles(styles.NewStyles(hasDark))
		backgrounds := map[string]color.Color{
			"Bar":   p.Bar.GetBackground(),
			"Track": p.Track.GetBackground(),
			"Label": p.Label.GetBackground(),
			"Value": p.Value.GetBackground(),
			"Muted": p.Muted.GetBackground(),
			"Up":    p.Up.GetBackground(),
			"Down":  p.Down.GetBackground(),
			"Flat":  p.Flat.GetBackground(),
		}
		for name, bg := range backgrounds {
			if bg != nil && bg != (lipgloss.NoColor{}) {
				t.Fatalf("hasDark=%v: Palette.%s carries background %v; chart styles must be "+
					"foreground-only so they do not bleed over the row background", hasDark, name, bg)
			}
		}
	}
}
