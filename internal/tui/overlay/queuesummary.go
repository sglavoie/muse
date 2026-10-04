package overlay

import (
	"fmt"
	"strconv"

	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// QueueSummaryOverlay renders a centered overlay box showing aggregate
// statistics about the current play queue.
type QueueSummaryOverlay struct {
	Tracks        int
	Artists       int
	Albums        int
	Sources       int
	RemainMs      int64 // remaining duration from current track onward
	RemainUnknown bool  // at least one scheduled track has no known duration
	RemainN       int   // remaining track count
	TotalMs       int64 // total queue duration
	TotalUnknown  bool  // at least one queue row has no known duration
}

// Render produces a centered overlay string for the queue summary.
func (q QueueSummaryOverlay) Render(termWidth, termHeight int, s styles.Styles) string {
	remaining := text.FormatDurationMs(q.RemainMs)
	if q.RemainUnknown {
		remaining += "+"
	}
	if q.RemainN > 0 {
		remaining = fmt.Sprintf("%s (%d %s)", remaining, q.RemainN, text.Pluralize(q.RemainN, "track", "tracks"))
	}

	total := text.FormatDurationMs(q.TotalMs)
	if q.TotalUnknown {
		total += "+"
	}

	// Keep timing visible when the box truncates rows in a short terminal.
	rows := []Row{
		{Label: "Remaining:", Text: remaining},
		{Label: "Total:", Text: total},
		{Blank: true},
		{Label: "Tracks:", Text: strconv.Itoa(q.Tracks)},
		{Label: "Artists:", Text: strconv.Itoa(q.Artists)},
		{Label: "Albums:", Text: strconv.Itoa(q.Albums)},
		{Label: "Sources:", Text: strconv.Itoa(q.Sources)},
	}

	box := Box{Title: " Queue Summary ", Rows: rows, Hint: "q or esc to dismiss"}
	return box.Render(termWidth, termHeight, s)
}
