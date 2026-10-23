package overlay

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// PanelInnerWidth is shared by tabbed editors so titles, padding, and footer
// placement remain consistent between panels.
func PanelInnerWidth(width int) int { return max(min(width-8, 80), MinInnerWidth) }

func PanelHeight(height int) int {
	if height <= 0 {
		return 27
	}
	return min(height, 27)
}

func PanelHintRows(height int) int {
	if height >= 12 {
		return 2
	}
	return 1
}

// PanelContentHeight adapts renderers that reserve one hint row themselves.
func PanelContentHeight(height int) int { return height - PanelHintRows(height) + 1 }

// RenderPanelLines pins the bottom border and hints, filling unused body rows.
// Callers keep their own scrolling rules and budget for PanelHintRows.
func RenderPanelLines(lines []string, width, height int) string {
	if len(lines) == 0 {
		return ""
	}
	bottom := -1
	for i, line := range lines {
		if strings.HasPrefix(ansi.Strip(line), CornerBL) {
			bottom = i
		}
	}
	if bottom < 0 {
		return RenderTopLines(lines, "", width)
	}
	inner := ansi.StringWidth(lines[0]) - 2
	target := height - PanelHintRows(height) - 1
	out := append([]string(nil), lines[:bottom]...)
	for len(out) < target {
		out = append(out, BorderV+strings.Repeat(" ", inner)+BorderV)
	}
	out = append(out, lines[bottom:]...)
	for len(out) < height {
		out = append(out, "")
	}
	return RenderTopLines(out, "", width)
}
