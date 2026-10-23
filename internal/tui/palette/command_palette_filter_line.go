package palette

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// filterLine renders the always-active "/ query" input row.
func (cp CommandPalette) filterLine(m cpBoxMetrics) string {
	filterText := cp.Filter.View()
	if !cp.Filter.Active {
		filterText = cp.Filter.Text
	}
	return cpBorderV + "  " + text.Pad(" / "+filterText, m.contentWidth) + "  " + cpBorderV
}

// commandLines renders the command area: either the "(no matches)" row or the
// rows the height budget leaves room for.
func (cp CommandPalette) commandLines(budget int, m cpBoxMetrics, s styles.Styles) []string {
	if len(cp.Filtered) == 0 {
		return []string{cpBorderV + "  " + text.Pad("(no matches)", m.contentWidth) + "  " + cpBorderV}
	}
	start := clamp(cp.cursor-budget/2, 0, max(len(cp.Filtered)-budget, 0))
	var lines []string
	for i := start; i < min(start+budget, len(cp.Filtered)); i++ {
		lines = append(lines, cp.paletteRowLine(i, m, s))
	}
	return lines
}

// paletteRowLine styles one task according to focus and availability.
func (cp CommandPalette) paletteRowLine(index int, m cpBoxMetrics, s styles.Styles) string {
	cmd := cp.Filtered[index]
	body := cp.buildRowContent(cmd, m.contentWidth, m.maxCategoryWidth, m.maxShortcutWidth, s)
	body = text.FocusMarker(index == cp.cursor) + strings.TrimPrefix(body, " ")
	switch {
	case index == cp.cursor && !cmd.IsEnabled():
		body = s.Accent.Faint(true).Italic(true).Render(body)
	case index == cp.cursor:
		body = s.Accent.Render(body)
	case !cmd.IsEnabled():
		body = s.Dim.Italic(true).Render(body)
	}
	return cpBorderV + body + " " + cpBorderV
}

func cpCommandRowBudget(termHeight int) int {
	if termHeight <= 0 {
		return 1 << 30
	}
	return max(termHeight-6, 1) // title, filter, separator, details, two hints
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// buildRowContent renders a single command row's inner text: leading space,
// optional "[category] " prefix, padded name, and right-aligned shortcut.
// The category prefix is styled dim and the shortcut dim.
func (cp CommandPalette) buildRowContent(cmd PaletteCommand, contentWidth, maxCategoryWidth, maxShortcutWidth int, s styles.Styles) string {
	// Category column: fixed-width "[cat] " or all-spaces when no categories
	// are present anywhere in the list.
	categoryPart := ""
	categoryWidth := 0
	if maxCategoryWidth > 0 {
		categoryWidth = maxCategoryWidth + 3 // "[" + cat + "] "
		if cmd.Category != "" {
			padded := text.PadLeft(cmd.Category, maxCategoryWidth)
			categoryPart = s.PaletteCategory.Render("["+padded+"]") + " "
		} else {
			categoryPart = strings.Repeat(" ", categoryWidth)
		}
	}

	if maxShortcutWidth > 0 {
		nameWidth := max(0, contentWidth-maxShortcutWidth-2-categoryWidth)
		name := text.Pad(cmd.Name, nameWidth)
		sc := text.PadLeft(cmd.Shortcut, maxShortcutWidth)
		return " " + clampPaletteRowBody(categoryPart+name+"  "+s.Dim.Render(sc), contentWidth) + " "
	}
	nameWidth := max(0, contentWidth-categoryWidth)
	return " " + clampPaletteRowBody(categoryPart+text.Pad(cmd.Name, nameWidth), contentWidth) + " "
}

func clampPaletteRowBody(body string, width int) string {
	body = ansi.Truncate(body, max(width, 0), "")
	if w := ansi.StringWidth(body); w < width {
		body += strings.Repeat(" ", width-w)
	}
	return body
}
