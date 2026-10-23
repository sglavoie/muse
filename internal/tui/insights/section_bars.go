package insights

import (
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
		// Extra terminal space belongs to names, rather than oversized bars.
		chartWidth := min(width, labelWidth+32+noteWidth+2)
		copy(bodies[start:end], chart.BarList(bars,
			chart.BarListOptions{Width: chartWidth, LabelWidth: labelWidth, Max: 1}, chart.FromStyles(s)))
		start = end
	}
	return bodies
}
