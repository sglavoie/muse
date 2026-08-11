package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// renderQueueDivider builds a dim horizontal divider line spanning the
// queue panel width.
func renderQueueDivider(s styles.Styles, width int) string {
	if width <= 0 {
		width = 1
	}
	return s.Divider.Render(strings.Repeat("─", width))
}

// renderQueueBucketHeader builds a bucket-header line padded to the queue
// panel width so neighbouring rows align.
func renderQueueBucketHeader(s styles.Styles, label string, width int) string {
	if label == "" {
		return ""
	}
	padded := label
	if width > 0 {
		if pw := ansi.StringWidth(label); pw < width {
			padded = label + strings.Repeat(" ", width-pw)
		}
	}
	return s.BucketHeader.Render(padded)
}

func clampQueueLine(line string, width int) string {
	if width <= 0 || ansi.StringWidth(line) <= width {
		return line
	}
	return ansi.Truncate(line, width, "")
}

func appendQueueSuffix(line, suffix string, width int) string {
	if width <= 0 {
		return line + suffix
	}
	suffix = text.Truncate(suffix, width)
	lineWidth := max(width-ansi.StringWidth(suffix), 0)
	return clampQueueLine(line, lineWidth) + suffix
}

// padQueue truncates or right-pads s with spaces to exactly width display columns.
func padQueue(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = truncateTitle(s, width)
	n := ansi.StringWidth(s)
	if n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return s
}
