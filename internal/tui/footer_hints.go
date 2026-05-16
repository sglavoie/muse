package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// footerHintSep is the visual divider between adjacent "key label" pairs in
// the footer hint strip. The middle-dot character matches the existing
// palette hint footer (command_palette.go).
const footerHintSep = " · "

// RenderFooterHints builds the single-line footer hint strip shown above the
// status bar. It returns an empty string when no primaries are registered for
// ctx (so the caller can avoid reserving a row). The whole line is rendered
// in the dim style.
//
// The returned string is at most width display columns wide. Pairs that
// don't fit are dropped from the right; any partially-rendered pair is
// elided with "…" so the strip never wraps.
func RenderFooterHints(width int, ctx bindingContext, r Registry, s styles.Styles) string {
	if width <= 0 {
		return ""
	}
	hints := r.Primaries(ctx)
	if len(hints) == 0 {
		return ""
	}

	const leading = "  "
	avail := width - ansi.StringWidth(leading)
	if avail <= 0 {
		return ""
	}

	var b strings.Builder
	used := 0
	for i, h := range hints {
		pair := h.Key + " " + h.Label
		piece := pair
		if i > 0 {
			piece = footerHintSep + pair
		}
		w := ansi.StringWidth(piece)
		if used+w > avail {
			break
		}
		b.WriteString(piece)
		used += w
	}
	if b.Len() == 0 {
		// Even the first pair didn't fit — truncate it so something shows.
		first := hints[0].Key + " " + hints[0].Label
		b.WriteString(ansi.Truncate(first, avail, "…"))
	}
	return leading + s.Dim.Render(b.String())
}
