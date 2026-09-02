package chart

import "math"

// VBar is one column of a VBarChart.
type VBar struct {
	// Label is drawn under the column. An empty label on every bar omits the
	// label row entirely.
	Label string
	Value float64
	// Muted draws the column in the muted role, for a bucket that is still
	// filling up and should not read as a finished comparison.
	Muted bool
}

// VBarOptions configures VBarChart's column allocation.
type VBarOptions struct {
	// Width is the exact display width of every rendered line. Zero or
	// negative produces no output.
	Width int
	// Height is how many rows the chart body occupies. Zero or negative
	// produces no body, leaving only the label row.
	Height int
	// ColWidth pins the display width of one column. Zero or negative derives
	// it from the longest label plus a separating column, or 1 when there are
	// no labels.
	ColWidth int
	// Max pins the value a full-height column represents. Zero or negative
	// derives it from the largest bar.
	Max float64
}

// VBarChart renders values as vertical columns of block glyphs, oldest first.
// When more bars are supplied than fit, the *leading* ones are dropped: every
// caller here plots a time series, where the recent end is the part worth
// keeping.
func VBarChart(bars []VBar, opts VBarOptions, p Palette) []string {
	return styleLines(vbarSegs(bars, opts), p)
}

func vbarChartPlain(bars []VBar, opts VBarOptions) []string {
	return plainLines(vbarSegs(bars, opts))
}

func vbarSegs(bars []VBar, opts VBarOptions) [][]seg {
	width := SafeWidth(opts.Width)
	if width == 0 || len(bars) == 0 {
		return nil
	}

	labels, labelled, labelW := vbarLabels(bars)
	colW := vbarColWidth(opts.ColWidth, labelled, labelW, width)
	fit := width / colW
	if fit == 0 {
		return nil
	}
	if drop := len(bars) - fit; drop > 0 {
		bars, labels = bars[drop:], labels[drop:]
	}

	height := max(opts.Height, 0)
	eighths := vbarEighths(bars, vbarMax(bars, opts.Max), height)

	out := make([][]seg, 0, height+1)
	for row := range height {
		// row 0 is the top of the chart, so the level a row represents counts
		// down from the tallest.
		out = append(out, vbarBodyRow(bars, eighths, height-1-row, colW, width))
	}
	if labelled {
		out = append(out, vbarLabelRow(labels, colW, width))
	}
	return out
}

// vbarLabels sanitizes each column's label and reports the widest one, plus
// whether any bar carried a label at all — an all-blank set omits the label
// row entirely.
func vbarLabels(bars []VBar) (labels []string, labelled bool, labelW int) {
	labels = make([]string, len(bars))
	for i, b := range bars {
		labels[i] = SafeText(b.Label)
		if labels[i] != "" {
			labelled = true
			labelW = max(labelW, displayWidth(labels[i]))
		}
	}
	return labels, labelled, labelW
}

// vbarColWidth resolves the width of one column. A derived width fits the
// widest label plus a separating column and narrows to fit the line rather than
// suppressing the chart; a pinned one is the caller's assertion about the
// layout and is honoured even when that leaves no room to draw.
func vbarColWidth(requested int, labelled bool, labelW, width int) int {
	if requested > 0 {
		return requested
	}
	colW := 1
	if labelled {
		colW = labelW + 1
	}
	return min(colW, width)
}

// vbarMax resolves the value a full-height column represents. It is derived
// after trimming, so a narrowed chart is scaled to the columns it actually
// shows rather than to a peak that is no longer on it.
func vbarMax(bars []VBar, pinned float64) float64 {
	maxVal := SafeMagnitude(pinned)
	if maxVal > 0 {
		return maxVal
	}
	for _, b := range bars {
		maxVal = math.Max(maxVal, SafeMagnitude(b.Value))
	}
	return maxVal
}

// vbarEighths converts each bar's value into the number of filled eighths of a
// cell its column occupies. A non-zero value always claims at least one eighth
// so a small bar is visible rather than silently rounding away.
func vbarEighths(bars []VBar, maxVal float64, height int) []int {
	eighths := make([]int, len(bars))
	if maxVal <= 0 {
		return eighths
	}
	for i, b := range bars {
		e := int(math.Round(SafeFraction(SafeMagnitude(b.Value)/maxVal) * float64(height) * 8))
		if e == 0 && b.Value > 0 {
			e = 1
		}
		eighths[i] = e
	}
	return eighths
}

// vbarBodyRow renders one chart row, where level is how many whole cells of a
// column sit below it.
func vbarBodyRow(bars []VBar, eighths []int, level, colW, width int) []seg {
	segs := make([]seg, 0, len(bars)+1)
	for i, b := range bars {
		glyph, r := vbarGlyph(eighths[i] - level*8)
		if b.Muted && r == roleBar {
			r = roleMuted
		}
		segs = append(segs, seg{SafeRepeat(glyph, colW), r})
	}
	return padSegs(segs, len(bars)*colW, width)
}

// vbarLabelRow renders the label row drawn under the chart body.
func vbarLabelRow(labels []string, colW, width int) []seg {
	segs := make([]seg, 0, len(labels)+1)
	for _, l := range labels {
		segs = append(segs, seg{padRight(truncateTo(l, colW), colW), roleLabel})
	}
	return padSegs(segs, len(labels)*colW, width)
}

// vbarGlyph picks the glyph for one cell from how many eighths of it are
// filled, so a column keeps sub-row precision at its top.
func vbarGlyph(remaining int) (string, role) {
	switch {
	case remaining >= 8:
		return glyphFull, roleBar
	case remaining <= 0:
		return " ", rolePlain
	default:
		return sparkLevels[remaining-1], roleBar
	}
}

// padSegs appends the trailing spaces that bring a line of known width up to
// the full requested width, so every emitted line measures exactly width.
func padSegs(segs []seg, used, width int) []seg {
	if pad := width - used; pad > 0 {
		segs = append(segs, seg{SafeRepeat(" ", pad), rolePlain})
	}
	return segs
}

// Trend renders a bare direction arrow with no magnitude, for the places where
// a Delta's number would only repeat the percentage next to it.
func Trend(change float64, p Palette) string {
	return styleLine(trendSegs(change), p)
}

func trendPlain(change float64) string {
	return plainLine(trendSegs(change))
}

func trendSegs(change float64) []seg {
	switch c := SafeFloat(change); {
	case c > 0:
		return []seg{{glyphUp, roleUp}}
	case c < 0:
		return []seg{{glyphDown, roleDown}}
	default:
		return []seg{{glyphFlat, roleFlat}}
	}
}
