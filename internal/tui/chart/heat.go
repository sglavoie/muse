package chart

import (
	"math"
	"time"
)

// heatLevels indexes intensity glyphs from lightest to heaviest. Intensity is
// carried by glyph density rather than by colour so the grid stays readable in
// a monochrome terminal and needs no palette entries of its own.
var heatLevels = [4]string{"░", "▒", "▓", "█"}

// HeatCell is one cell of a Heatmap. Present distinguishes a cell that exists
// and recorded nothing — drawn as the same gap glyph a sparkline uses — from
// one that falls outside the data range and is drawn as blank padding.
type HeatCell struct {
	Value   float64
	Present bool
}

// HeatRow is one labelled row of a Heatmap.
type HeatRow struct {
	Label string
	Cells []HeatCell
}

// HeatmapOptions configures Heatmap's column allocation.
type HeatmapOptions struct {
	// Width is the exact display width of every rendered line. Zero or
	// negative produces no output.
	Width int
	// LabelWidth pins the row-label column. Zero or negative derives it from
	// the longest label, capped at a third of Width.
	LabelWidth int
	// CellWidth is the display width of one cell. Zero or negative means 1.
	CellWidth int
	// Max pins the value the heaviest glyph represents. Zero or negative
	// derives it from the largest cell.
	Max float64
	// TrimOldest drops leading columns rather than trailing ones when the grid
	// is wider than the space the label column leaves. A time-series grid wants
	// its recent end kept.
	TrimOldest bool
}

// Heatmap renders a labelled grid of intensity glyphs, one line per row, each
// line exactly Width columns wide.
func Heatmap(rows []HeatRow, opts HeatmapOptions, p Palette) []string {
	return styleLines(heatmapSegs(rows, opts), p)
}

func heatmapPlain(rows []HeatRow, opts HeatmapOptions) []string {
	return plainLines(heatmapSegs(rows, opts))
}

func heatmapSegs(rows []HeatRow, opts HeatmapOptions) [][]seg {
	width := SafeWidth(opts.Width)
	if width == 0 || len(rows) == 0 {
		return nil
	}

	labels := make([]string, len(rows))
	gridCols := 0
	for i, r := range rows {
		labels[i] = SafeText(r.Label)
		gridCols = max(gridCols, len(r.Cells))
	}
	if gridCols == 0 {
		return nil
	}

	cellW := max(opts.CellWidth, 1)
	labelW, cols := heatmapLayout(labels, opts.LabelWidth, cellW, width)
	if cols == 0 {
		return nil
	}
	from, gridCols := heatmapVisibleColumns(gridCols, cols, opts.TrimOldest)
	maxVal := heatmapMax(rows, opts.Max, from, gridCols)

	out := make([][]seg, 0, len(rows))
	for i, r := range rows {
		out = append(out, heatRowSegs(labels[i], r.Cells, maxVal, heatRowLayout{
			from:     from,
			gridCols: gridCols,
			cellW:    cellW,
			labelW:   labelW,
			width:    width,
		}))
	}
	return out
}

// heatmapVisibleColumns narrows a grid of gridCols columns to the cols the
// layout left room for, returning the half-open column range that survives.
// Columns are dropped, never folded: a heat cell stands for one concrete bucket
// that enter must be able to open, so averaging two into one would leave a
// glyph with no query behind it.
func heatmapVisibleColumns(gridCols, cols int, trimOldest bool) (from, to int) {
	if gridCols <= cols {
		return 0, gridCols
	}
	if trimOldest {
		from = gridCols - cols
	}
	return from, from + cols
}

// heatmapMax resolves the value the heaviest glyph represents. Only the
// surviving columns set the scale, so a trimmed grid is measured against what
// it shows rather than against a peak scrolled off the left.
func heatmapMax(rows []HeatRow, pinned float64, from, gridCols int) float64 {
	maxVal := SafeMagnitude(pinned)
	if maxVal > 0 {
		return maxVal
	}
	for _, r := range rows {
		for col := from; col < min(gridCols, len(r.Cells)); col++ {
			if c := r.Cells[col]; c.Present {
				maxVal = math.Max(maxVal, SafeMagnitude(c.Value))
			}
		}
	}
	return maxVal
}

// heatRowLayout carries the column allocation heatmapSegs resolved once for the
// whole grid down to each row.
type heatRowLayout struct {
	from     int
	gridCols int
	cellW    int
	labelW   int
	width    int
}

// heatRowSegs renders one grid row: the optional label column, one glyph run
// per surviving cell, and the trailing padding that squares the line off.
func heatRowSegs(label string, cells []HeatCell, maxVal float64, l heatRowLayout) []seg {
	segs := make([]seg, 0, l.gridCols-l.from+3)
	if l.labelW > 0 {
		segs = append(segs,
			seg{padRight(truncateTo(label, l.labelW), l.labelW), roleLabel},
			seg{" ", rolePlain})
	}
	for col := l.from; col < l.gridCols; col++ {
		cell := HeatCell{}
		if col < len(cells) {
			cell = cells[col]
		}
		glyph, role := heatGlyph(cell, maxVal)
		segs = append(segs, seg{SafeRepeat(glyph, l.cellW), role})
	}
	used := l.labelW + (l.gridCols-l.from)*l.cellW
	if l.labelW > 0 {
		used++
	}
	return padSegs(segs, used, l.width)
}

// heatmapLayout divides width between the row-label column and the grid,
// shrinking the label column a column at a time rather than emitting a grid
// with no cells in it.
func heatmapLayout(labels []string, requestedLabelW, cellW, width int) (labelW, cols int) {
	labelW = min(derivedLabelWidth(labels, requestedLabelW, width), width)

	for {
		gap := 0
		if labelW > 0 {
			gap = 1
		}
		cols = (width - labelW - gap) / cellW
		if cols > 0 || labelW == 0 {
			break
		}
		labelW--
	}
	return labelW, max(cols, 0)
}

func heatGlyph(cell HeatCell, maxVal float64) (string, role) {
	if !cell.Present {
		return " ", rolePlain
	}
	v := SafeMagnitude(cell.Value)
	if v <= 0 || maxVal <= 0 {
		return glyphGap, roleMuted
	}
	level := int(math.Ceil(SafeFraction(v/maxVal) * float64(len(heatLevels))))
	return heatLevels[min(max(level, 1), len(heatLevels))-1], roleBar
}

// weekdayLabels are the Calendar's row labels, Sunday first.
var weekdayLabels = [7]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}

// CalendarOptions configures Calendar.
type CalendarOptions struct {
	// Width is the exact display width of every rendered line.
	Width int
	// FirstWeekday is the weekday days[0] falls on.
	FirstWeekday time.Weekday
	// LabelWidth pins the weekday-label column; zero pins it to the width of
	// the three-letter weekday names.
	LabelWidth int
	// Max pins the value the heaviest glyph represents.
	Max float64
}

// Calendar lays a contiguous run of days out as seven weekday rows and one
// column per week, keeping the most recent weeks when the history is wider
// than Width. days must be contiguous — one entry per calendar day including
// the silent ones — so a gap renders as a gap instead of collapsing.
func Calendar(days []HeatCell, opts CalendarOptions, p Palette) []string {
	return styleLines(calendarSegs(days, opts), p)
}

func calendarPlain(days []HeatCell, opts CalendarOptions) []string {
	return plainLines(calendarSegs(days, opts))
}

func calendarSegs(days []HeatCell, opts CalendarOptions) [][]seg {
	if len(days) == 0 {
		return nil
	}
	offset := int(opts.FirstWeekday)
	if offset < 0 || offset > 6 {
		offset = 0
	}
	weeks := (offset + len(days) + 6) / 7

	rows := make([]HeatRow, 7)
	for i := range rows {
		rows[i] = HeatRow{Label: weekdayLabels[i], Cells: make([]HeatCell, weeks)}
	}
	for i, d := range days {
		slot := offset + i
		rows[slot%7].Cells[slot/7] = d
	}
	// The weekday names are what make the grid readable at all, so the label
	// column is pinned to them rather than derived and capped the way a
	// free-form Heatmap's is.
	labelW := opts.LabelWidth
	if labelW <= 0 {
		labelW = len(weekdayLabels[0])
	}
	return heatmapSegs(rows, HeatmapOptions{
		Width:      opts.Width,
		LabelWidth: labelW,
		Max:        opts.Max,
		TrimOldest: true,
	})
}
