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
	if strings.TrimSpace(cp.Filter.Text) == "" {
		return cp.suggestedLines(budget, m, s)
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
	body := cp.buildRowContent(cmd, m.contentWidth, m.maxShortcutWidth, s)
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

// buildRowContent reserves a stable shortcut column beside the command name.
func (cp CommandPalette) buildRowContent(cmd PaletteCommand, contentWidth, maxShortcutWidth int, s styles.Styles) string {
	if maxShortcutWidth > 0 {
		nameWidth := max(0, contentWidth-maxShortcutWidth-2)
		name := text.Pad(cmd.Name, nameWidth)
		sc := text.PadLeft(cmd.Shortcut, maxShortcutWidth)
		return " " + clampPaletteRowBody(name+"  "+s.Dim.Render(sc), contentWidth) + " "
	}
	nameWidth := max(0, contentWidth)
	return " " + clampPaletteRowBody(text.Pad(cmd.Name, nameWidth), contentWidth) + " "
}

func clampPaletteRowBody(body string, width int) string {
	body = ansi.Truncate(body, max(width, 0), "")
	if w := ansi.StringWidth(body); w < width {
		body += strings.Repeat(" ", width-w)
	}
	return body
}

// Section headings replace repeated category abbreviations in the shortlist.
func (cp CommandPalette) suggestedLines(budget int, m cpBoxMetrics, s styles.Styles) []string {
	for start := 0; start <= cp.cursor; start++ {
		var lines []string
		previous := ""
		focused := false
		for i := start; i < len(cp.Filtered); i++ {
			group := suggestionGroup(cp.Filtered[i])
			if group != previous && budget > 1 {
				if len(lines)+2 > budget {
					break
				}
				lines = append(lines, cpBorderV+" "+s.Dim.Render(text.Pad(group, m.innerWidth-2))+" "+cpBorderV)
			}
			if len(lines) >= budget {
				break
			}
			lines = append(lines, cp.paletteRowLine(i, m, s))
			focused = focused || i == cp.cursor
			previous = group
		}
		if focused {
			return lines
		}
	}
	return nil
}

func suggestionGroup(c PaletteCommand) string {
	if c.Intent == IntentListening {
		return "Listening"
	}
	if c.Relevant {
		return "This view"
	}
	return "Other actions"
}
