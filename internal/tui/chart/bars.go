package chart

// minBarWidth is the narrowest bar worth drawing. Below it the layout gives up
// notes, then labels, rather than emitting a bar too short to read.
const minBarWidth = 3

// BarRow is one labelled row of a BarList.
type BarRow struct {
	Label string
	Value float64
	// Note is an optional annotation rendered right-aligned after the bar,
	// e.g. a play count or a percentage.
	Note string
}

// BarListOptions configures BarList's column allocation.
type BarListOptions struct {
	// Width is the exact display width of every rendered line. Zero or
	// negative produces no output.
	Width int
	// LabelWidth pins the label column so several lists can align. Zero or
	// negative derives it from the longest label, capped at a third of Width.
	LabelWidth int
	// Max pins the value the longest bar represents, so lists rendered from
	// different data stay comparable. Zero or negative derives it from the
	// largest row value, making the longest bar fill its track.
	Max float64
}

// BarList renders one horizontal bar per row as "label bar note", each line
// exactly Width columns wide.
func BarList(rows []BarRow, opts BarListOptions, p Palette) []string {
	return styleLines(barListSegs(rows, opts), p)
}

func barListPlain(rows []BarRow, opts BarListOptions) []string {
	return plainLines(barListSegs(rows, opts))
}

func barListSegs(rows []BarRow, opts BarListOptions) [][]seg {
	width := SafeWidth(opts.Width)
	if width == 0 || len(rows) == 0 {
		return nil
	}
	labels := make([]string, len(rows))
	notes := make([]string, len(rows))
	for i, r := range rows {
		labels[i] = SafeText(r.Label)
		notes[i] = SafeText(r.Note)
	}
	labelW, barW, noteW := barListLayout(labels, notes, opts.LabelWidth, width)

	maxVal := SafeMagnitude(opts.Max)
	if maxVal <= 0 {
		for _, r := range rows {
			if v := SafeMagnitude(r.Value); v > maxVal {
				maxVal = v
			}
		}
	}

	out := make([][]seg, 0, len(rows))
	for i, r := range rows {
		fraction := 0.0
		if maxVal > 0 {
			fraction = SafeMagnitude(r.Value) / maxVal
		}
		fill, track := fillSegs(fraction, barW)
		segs := make([]seg, 0, 6)
		if labelW > 0 {
			segs = append(segs,
				seg{padRight(truncateTo(labels[i], labelW), labelW), roleLabel},
				seg{" ", rolePlain})
		}
		segs = append(segs, seg{fill, roleBar}, seg{track, roleTrack})
		if noteW > 0 {
			segs = append(segs,
				seg{" ", rolePlain},
				seg{padLeft(truncateTo(notes[i], noteW), noteW), roleValue})
		}
		out = append(out, segs)
	}
	return out
}

// barListLayout divides width between the label column, the bar, and the note
// column. The three plus their separating spaces always sum to exactly width.
// When the bar would fall below minBarWidth the notes are dropped first, then
// the label column shrinks a column at a time.
func barListLayout(labels, notes []string, requestedLabelW, width int) (labelW, barW, noteW int) {
	for _, n := range notes {
		noteW = max(noteW, displayWidth(n))
	}
	if requestedLabelW > 0 {
		labelW = requestedLabelW
	} else {
		for _, l := range labels {
			labelW = max(labelW, displayWidth(l))
		}
		labelW = min(labelW, width/3)
	}
	labelW = min(labelW, width)
	noteW = min(noteW, width)

	for {
		gap, noteGap := 0, 0
		if labelW > 0 {
			gap = 1
		}
		if noteW > 0 {
			noteGap = 1
		}
		barW = width - labelW - gap - noteW - noteGap
		if barW >= minBarWidth || (labelW == 0 && noteW == 0) {
			break
		}
		if noteW > 0 {
			noteW = 0
			continue
		}
		labelW--
	}
	return labelW, max(barW, 0), noteW
}

// BarTrack renders one fraction as a filled bar over a track of exactly width
// cells, with no label and no annotation.
func BarTrack(fraction float64, width int, p Palette) string {
	return styleLine(barTrackSegs(fraction, width), p)
}

func barTrackPlain(fraction float64, width int) string {
	return plainLine(barTrackSegs(fraction, width))
}

func barTrackSegs(fraction float64, width int) []seg {
	fill, track := fillSegs(fraction, width)
	return []seg{{fill, roleBar}, {track, roleTrack}}
}

// Ratio renders part out of whole as a bar followed by the percentage, within
// width columns. When the percentage cannot fit, the bar takes the full width
// on its own. A non-positive or non-finite whole renders an empty track.
func Ratio(part, whole float64, width int, p Palette) string {
	return styleLine(ratioSegs(part, whole, width), p)
}

func ratioPlain(part, whole float64, width int) string {
	return plainLine(ratioSegs(part, whole, width))
}

func ratioSegs(part, whole float64, width int) []seg {
	width = SafeWidth(width)
	if width == 0 {
		return nil
	}
	fraction := 0.0
	if w := SafeMagnitude(whole); w > 0 {
		fraction = SafeMagnitude(part) / w
	}
	pct := formatPercent(fraction)
	barW := width - displayWidth(pct) - 1
	if barW < minBarWidth {
		return barTrackSegs(fraction, width)
	}
	fill, track := fillSegs(fraction, barW)
	return []seg{
		{fill, roleBar},
		{track, roleTrack},
		{" ", rolePlain},
		{pct, roleValue},
	}
}
