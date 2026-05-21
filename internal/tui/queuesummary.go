package tui

import (
	"fmt"
	"strconv"

	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// QueueSummaryOverlay renders a centered overlay box showing aggregate
// statistics about the current play queue.
type QueueSummaryOverlay struct {
	tracks   int
	artists  int
	albums   int
	sources  int
	remainMs int64 // remaining duration from current track onward
	remainN  int   // remaining track count
	totalMs  int64 // total queue duration
}

// Render produces a centered overlay string for the queue summary.
func (q QueueSummaryOverlay) Render(termWidth, termHeight int, s styles.Styles) string {
	remaining := text.FormatDurationMs(q.remainMs)
	if q.remainN > 0 {
		remaining = fmt.Sprintf("%s (%d %s)", remaining, q.remainN, pluralize(q.remainN, "track", "tracks"))
	}

	rows := []OverlayRow{
		{Label: "Tracks:", Text: strconv.Itoa(q.tracks)},
		{Label: "Artists:", Text: strconv.Itoa(q.artists)},
		{Label: "Albums:", Text: strconv.Itoa(q.albums)},
		{Label: "Sources:", Text: strconv.Itoa(q.sources)},
		{Blank: true},
		{Label: "Remaining:", Text: remaining},
		{Label: "Total:", Text: text.FormatDurationMs(q.totalMs)},
	}

	box := OverlayBox{Title: " Queue Summary ", Rows: rows, Hint: "q or esc to dismiss"}
	return box.Render(termWidth, termHeight, s)
}
