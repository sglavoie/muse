package strip

import (
	"strings"

	"github.com/sglavoie/muse/internal/tui/styles"
)

// renderKeyLabelPair styles a "key label" hint string by rendering the
// first whitespace-delimited token in HelpKey (bold + accent foreground)
// and the remainder in TextSecondary, joined by a single plain space.
// A hint with no space is treated as a key-only label. Width arithmetic
// upstream uses the unstyled string, so this function never changes
// display width.
func renderKeyLabelPair(hint string, s styles.Styles) string {
	hint = plainHintLine(hint)
	if hint == "" {
		return ""
	}

	idx := strings.IndexByte(hint, ' ')
	if idx < 0 {
		return s.HelpKey.Render(hint)
	}

	key := hint[:idx]
	label := hint[idx+1:]
	if label == "" {
		return s.HelpKey.Render(key)
	}
	return s.HelpKey.Render(key) + " " + s.TextSecondary.Render(label)
}
