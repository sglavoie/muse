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
	Tracks   int
	Artists  int
	Albums   int
	Sources  int
	RemainMs int64 // remaining duration from current track onward
	RemainN  int   // remaining track count
	TotalMs  int64 // total queue duration
}

// Render produces a centered overlay string for the queue summary.
func (q QueueSummaryOverlay) Render(termWidth, termHeight int, s styles.Styles) string {
	remaining := text.FormatDurationMs(q.RemainMs)
	if q.RemainN > 0 {
		remaining = fmt.Sprintf("%s (%d %s)", remaining, q.RemainN, text.Pluralize(q.RemainN, "track", "tracks"))
	}

	rows := []Row{
		{Label: "Tracks:", Text: strconv.Itoa(q.Tracks)},
		{Label: "Artists:", Text: strconv.Itoa(q.Artists)},
		{Label: "Albums:", Text: strconv.Itoa(q.Albums)},
		{Label: "Sources:", Text: strconv.Itoa(q.Sources)},
		{Blank: true},
		{Label: "Remaining:", Text: remaining},
		{Label: "Total:", Text: text.FormatDurationMs(q.TotalMs)},
	}

	box := Box{Title: " Queue Summary ", Rows: rows, Hint: "q or esc to dismiss"}
	return box.Render(termWidth, termHeight, s)
}
