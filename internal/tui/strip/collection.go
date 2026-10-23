package strip

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// Collection labels the current collection and its visible/total item count,
// including zero, using the same alignment as the track-list state strip.
func Collection(width int, label, unit string, visible, total int, s styles.Styles, window ...int) string {
	plural := unit + "s"
	if unit == "mix" {
		plural = "mixes"
	}
	if unit == "category" {
		plural = "categories"
	}
	count := fmt.Sprintf("%d %s", total, text.Pluralize(total, unit, plural))
	if visible != total {
		count = fmt.Sprintf("%d/%d %s", visible, total, text.Pluralize(total, unit, plural))
	}
	if len(window) == 2 {
		counts := CountsChunk{Total: total, Filtered: visible, Filtering: visible != total, WindowStart: window[0], WindowEnd: window[1]}
		if position := counts.windowLabel(); position != "" {
			count = position
		}
	}
	if width <= 0 {
		return s.TextSecondary.Render(label) + " · " + s.Dim.Render(count)
	}
	label = text.Truncate(label, max(0, width-ansi.StringWidth(count)-1))
	gap := max(1, width-ansi.StringWidth(label)-ansi.StringWidth(count))
	return ansi.Truncate(s.TextSecondary.Render(label)+strings.Repeat(" ", gap)+s.Dim.Render(count), width, "…")
}
