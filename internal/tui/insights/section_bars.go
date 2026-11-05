package insights

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/chart"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// Measure complete sections, including offscreen rows, so scrolling never
// moves their chart columns. All bars use the same 0–100% scale.
func sectionBarBodies(rows []Row, width int, s styles.Styles) []string {
	bodies := make([]string, len(rows))
	for start := 0; start < len(rows); {
		end := start + 1
		for end < len(rows) && rows[end].Section == rows[start].Section {
			end++
		}
		var bars []chart.BarRow
		labelWidth, noteWidth := 0, 0
		for _, row := range rows[start:end] {
			bars = append(bars, chart.BarRow{Label: row.Label, Value: row.fraction, Note: row.note})
			labelWidth = max(labelWidth, ansi.StringWidth(row.Label))
			noteWidth = max(noteWidth, ansi.StringWidth(row.note))
		}
		labelWidth = min(labelWidth, max(width/2, 1))
		if uniformFractions(rows[start:end]) {
			// Bars that are all full or all empty compare nothing; the notes
			// alone say it better.
			for i, row := range rows[start:end] {
				bodies[start+i] = uniformRowBody(row, labelWidth, width, s)
			}
			start = end
			continue
		}
		// Extra terminal space belongs to names, rather than oversized bars.
		chartWidth := min(width, labelWidth+32+noteWidth+2)
		copy(bodies[start:end], chart.BarList(bars,
			chart.BarListOptions{Width: chartWidth, LabelWidth: labelWidth, Max: 1}, chart.FromStyles(s)))
		start = end
	}
	return bodies
}

// uniformFractions reports whether a section of two or more rows would draw
// bars that are all completely full or all empty: on the shared 0–100% scale
// those say nothing the notes do not, while equal partial bars still show how
// far along each row is.
func uniformFractions(rows []Row) bool {
	if len(rows) < 2 || (rows[0].fraction != 0 && rows[0].fraction != 1) {
		return false
	}
	for _, row := range rows[1:] {
		if row.fraction != rows[0].fraction {
			return false
		}
	}
	return true
}

// uniformRowBody renders a row without its bar: the label, then its note.
func uniformRowBody(row Row, labelWidth, width int, s styles.Styles) string {
	label := ansi.Truncate(row.Label, labelWidth, "…")
	label += strings.Repeat(" ", max(0, labelWidth-ansi.StringWidth(label)))
	line := s.TextPrimary.Render(label)
	if row.note != "" {
		line += "  " + s.TextSecondary.Render(row.note)
	}
	return ansi.Truncate(line, max(width, 0), "…")
}
