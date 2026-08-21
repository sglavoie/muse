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

	labels := make([]string, len(bars))
	labelled := false
	labelW := 0
	for i, b := range bars {
		labels[i] = SafeText(b.Label)
		if labels[i] != "" {
			labelled = true
			labelW = max(labelW, displayWidth(labels[i]))
		}
	}

	colW := opts.ColWidth
	if colW <= 0 {
		colW = 1
		if labelled {
			colW = labelW + 1
		}
		// A derived width narrows to fit rather than suppressing the chart; a
		// pinned one is the caller's assertion about the layout and is honoured
		// even when that leaves no room to draw.
		colW = min(colW, width)
	}
	fit := width / colW
	if fit == 0 {
		return nil
	}
	if drop := len(bars) - fit; drop > 0 {
		bars, labels = bars[drop:], labels[drop:]
	}

	// The scale is derived after trimming, so a narrowed chart is scaled to the
	// columns it actually shows rather than to a peak that is no longer on it.
	maxVal := SafeMagnitude(opts.Max)
	if maxVal <= 0 {
		for _, b := range bars {
			maxVal = math.Max(maxVal, SafeMagnitude(b.Value))
		}
	}

	height := max(opts.Height, 0)
	eighths := make([]int, len(bars))
	for i, b := range bars {
		if maxVal <= 0 {
			continue
		}
		e := int(math.Round(SafeFraction(SafeMagnitude(b.Value)/maxVal) * float64(height) * 8))
		if e == 0 && b.Value > 0 {
			e = 1
		}
		eighths[i] = e
	}

	out := make([][]seg, 0, height+1)
	for row := range height {
		// row 0 is the top of the chart, so the level a row represents counts
		// down from the tallest.
		level := height - 1 - row
		segs := make([]seg, 0, len(bars)+1)
		for i, b := range bars {
			glyph, r := vbarGlyph(eighths[i] - level*8)
			if b.Muted && r == roleBar {
				r = roleMuted
			}
			segs = append(segs, seg{SafeRepeat(glyph, colW), r})
		}
		out = append(out, padSegs(segs, len(bars)*colW, width))
	}

	if labelled {
		segs := make([]seg, 0, len(bars)+1)
		for _, l := range labels {
			segs = append(segs, seg{padRight(truncateTo(l, colW), colW), roleLabel})
		}
		out = append(out, padSegs(segs, len(bars)*colW, width))
	}
	return out
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
