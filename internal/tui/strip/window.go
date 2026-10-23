package strip

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
	"strings"
)

// WindowLabel describes visible data rows only when the list needs scrolling.
func WindowLabel(start, end, total int) string {
	start, end = max(0, start), min(end, total)
	if end <= start || (start == 0 && end == total) {
		return ""
	}
	return fmt.Sprintf("%d–%d of %d", start+1, end, total)
}

// WithWindow adds a quiet range to an existing heading without adding a row.
func WithWindow(heading string, width, start, end, total int, s styles.Styles) string {
	label := WindowLabel(start, end, total)
	if label == "" || width <= 0 {
		return heading
	}
	heading = strings.TrimRight(heading, " ")
	room := max(0, width-ansi.StringWidth(label)-2)
	heading = text.Truncate(heading, room)
	return heading + strings.Repeat(" ", max(1, width-ansi.StringWidth(heading)-ansi.StringWidth(label))) + s.Dim.Render(label)
}
