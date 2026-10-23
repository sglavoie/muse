package palette

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

func (cp CommandPalette) compactBoxLines(width, height int, m cpBoxMetrics, s styles.Styles) []string {
	title := fmt.Sprintf("Command Palette · %d results", len(cp.Filtered))
	if len(cp.Filtered) > 0 {
		title = fmt.Sprintf("Command Palette · %d/%d", cp.cursor+1, len(cp.Filtered))
	}
	lines := []string{compactRule(title, cpCornerTL, cpCornerTR, m.innerWidth), cp.filterLine(m), cpCrossL + strings.Repeat(cpBorderH, m.innerWidth) + cpCrossR}
	lines = append(lines, cp.commandLines(cpCommandRowBudget(height), m, s)...)
	reason := ""
	if cp.cursor < len(cp.Filtered) {
		reason = cp.Filtered[cp.cursor].detailText()
	}
	return append(lines, s.Dim.Render(compactRule(reason, cpCornerBL, cpCornerBR, m.innerWidth)),
		"  "+s.Dim.Render(text.Truncate("↑/↓ pgup/pgdown · enter run · esc close", max(width-2, 0))),
		"  "+s.Dim.Render(text.Truncate("F1 help · ctrl+c quit", max(width-2, 0))))
}

func compactRule(label, left, right string, width int) string {
	if label == "" {
		return left + strings.Repeat(cpBorderH, width) + right
	}
	label = text.Truncate(" "+label+" ", width)
	return left + label + strings.Repeat(cpBorderH, max(width-ansi.StringWidth(label), 0)) + right
}
