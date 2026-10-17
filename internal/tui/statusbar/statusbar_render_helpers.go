package statusbar

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sglavoie/muse/internal/tui/styles"
)

const progressCellSteps = 8

var progressPartialBlocks = [...]string{
	"",
	"▏",
	"▎",
	"▍",
	"▌",
	"▋",
	"▊",
	"▉",
}

func progressColorSet(c color.Color) bool {
	if c == nil {
		return false
	}
	_, noColor := c.(lipgloss.NoColor)
	return !noColor
}

// renderProgressBar builds a full-width progress bar string. Each terminal
// cell is subdivided into eighths using Unicode partial-block glyphs, giving
// finer progress precision while preserving the fixed display width.
func renderProgressBar(elapsed, total float64, width int, st styles.Styles) string {
	if width <= 0 {
		return ""
	}
	fullCells, partialSteps, emptyCells := progressCells(progressFraction(elapsed, total), width)

	fillColor := st.ProgressFilled.GetForeground()
	emptyColor := st.ProgressEmpty.GetBackground()
	if progressColorSet(fillColor) && progressColorSet(emptyColor) {
		return renderColoredProgress(st, fillColor, emptyColor, fullCells, partialSteps, emptyCells)
	}
	return renderGlyphProgress(st, fullCells, partialSteps, emptyCells)
}

// progressFraction returns elapsed/total clamped to [0,1], reading an unknown
// or non-finite total as no progress at all.
func progressFraction(elapsed, total float64) float64 {
	if total <= 0 {
		return 0
	}
	frac := elapsed / total
	if math.IsNaN(frac) || math.IsInf(frac, 0) || frac < 0 {
		return 0
	}
	return min(frac, 1)
}

// progressCells splits frac of width cells into whole filled cells, the
// eighths filled in the boundary cell, and the empty cells left after it.
func progressCells(frac float64, width int) (fullCells, partialSteps, emptyCells int) {
	maxSteps := width * progressCellSteps
	filledSteps := min(max(int(math.Round(frac*float64(maxSteps))), 0), maxSteps)

	fullCells = filledSteps / progressCellSteps
	partialSteps = filledSteps % progressCellSteps
	emptyCells = width - fullCells
	if partialSteps > 0 {
		emptyCells--
	}
	return fullCells, partialSteps, emptyCells
}

// renderColoredProgress paints the bar with background colours so the filled
// span reads as one solid block.
func renderColoredProgress(st styles.Styles, fillColor, emptyColor color.Color, fullCells, partialSteps, emptyCells int) string {
	filled := ""
	if fullCells > 0 {
		filled = st.ProgressFilled.Background(fillColor).Render(strings.Repeat(" ", fullCells))
	}
	partial := ""
	if partialSteps > 0 {
		partial = st.ProgressFilled.Background(emptyColor).Render(progressPartialBlocks[partialSteps])
	}
	empty := ""
	if emptyCells > 0 {
		empty = st.ProgressEmpty.Background(emptyColor).Render(strings.Repeat(" ", emptyCells))
	}
	return filled + partial + empty
}

// renderGlyphProgress is the fallback for styles that carry no colour: block
// glyphs stand in for the coloured spans.
func renderGlyphProgress(st styles.Styles, fullCells, partialSteps, emptyCells int) string {
	filled := strings.Repeat("█", fullCells)
	if partialSteps > 0 {
		filled += progressPartialBlocks[partialSteps]
	}
	return st.ProgressFilled.Render(filled) +
		st.ProgressEmpty.Render(strings.Repeat("░", emptyCells))
}

// modeLabels returns the editorial chip labels (visual mode, yank/cut count,
// selection count). When abbreviated is true short forms are used ("V",
// "Y:N", "Cut:N", "Sel:N"); otherwise verbose forms ("Visual", "Yank: N",
// "Cut: N", "Sel: N"). Playback modes (repeat/shuffle/stop-next/radio) are
// rendered separately as explicit labels, abbreviated in compact windows.
func (s StatusBar) modeLabels(abbreviated bool) []string {
	var out []string
	if s.VisualMode {
		out = append(out, pickLabel(abbreviated, "V", "Visual"))
	}
	if s.YankCount > 0 {
		out = append(out, fmt.Sprintf(yankLabelFormat(abbreviated, s.YankIsCut), s.YankCount))
	}
	if s.SelectionCount > 0 {
		out = append(out, fmt.Sprintf(pickLabel(abbreviated, "Sel:%d", "Sel: %d"), s.SelectionCount))
	}
	return out
}

// pickLabel chooses between the short and verbose form of a chip label.
func pickLabel(abbreviated bool, short, verbose string) string {
	if abbreviated {
		return short
	}
	return verbose
}

// yankLabelFormat returns the printf format for the yank/cut chip, which
// changes wording as well as length between the two forms.
func yankLabelFormat(abbreviated, isCut bool) string {
	if isCut {
		return pickLabel(abbreviated, "Cut:%d", "Cut: %d")
	}
	return pickLabel(abbreviated, "Y:%d", "Yank: %d")
}
