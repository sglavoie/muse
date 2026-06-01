package tui

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// formatTime converts a duration in seconds to M:SS or H:MM:SS format.
// Unlike FormatDurationSecs, zero is treated as a valid position (start of
// playback) and returns "0:00". NaN and infinite values are clamped to zero.
func formatTime(secs float64) string {
	if secs < 0 || math.IsNaN(secs) || math.IsInf(secs, 0) {
		secs = 0
	}
	total := int(secs)
	hours := total / 3600
	minutes := (total % 3600) / 60
	seconds := total % 60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

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
	frac := 0.0
	if total > 0 {
		frac = elapsed / total
		if math.IsNaN(frac) || math.IsInf(frac, 0) || frac < 0 {
			frac = 0
		} else if frac > 1 {
			frac = 1
		}
	}
	filledSteps := int(math.Round(frac * float64(width*progressCellSteps)))
	if filledSteps > width*progressCellSteps {
		filledSteps = width * progressCellSteps
	}
	if filledSteps < 0 {
		filledSteps = 0
	}

	fullCells := filledSteps / progressCellSteps
	partialSteps := filledSteps % progressCellSteps
	emptyCells := width - fullCells
	if partialSteps > 0 {
		emptyCells--
	}

	fillColor := st.ProgressFilled.GetForeground()
	emptyColor := st.ProgressEmpty.GetBackground()
	if progressColorSet(fillColor) && progressColorSet(emptyColor) {
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

	filled := strings.Repeat("█", fullCells)
	if partialSteps > 0 {
		filled += progressPartialBlocks[partialSteps]
	}
	return st.ProgressFilled.Render(filled) +
		st.ProgressEmpty.Render(strings.Repeat("░", emptyCells))
}

// truncateTitle shortens s so its display width (measured in terminal columns)
// does not exceed maxWidth. When truncation occurs an ellipsis "…" (width 1)
// is appended. Returns an empty string when maxWidth <= 0.
func truncateTitle(s string, maxWidth int) string {
	return text.Truncate(s, maxWidth)
}

// modeLabels returns the editorial chip labels (visual mode, yank/cut count,
// selection count). When abbreviated is true short forms are used ("V",
// "Y:N", "Cut:N", "Sel:N"); otherwise verbose forms ("Visual", "Yank: N",
// "Cut: N", "Sel: N"). Playback modes (repeat/shuffle/stop-next/radio) are
// rendered separately by render() as the 4-glyph RSND cluster.
func (s StatusBar) modeLabels(abbreviated bool) []string {
	var out []string
	if abbreviated {
		if s.visualMode {
			out = append(out, "V")
		}
		if s.yankCount > 0 {
			if s.yankIsCut {
				out = append(out, fmt.Sprintf("Cut:%d", s.yankCount))
			} else {
				out = append(out, fmt.Sprintf("Y:%d", s.yankCount))
			}
		}
		if s.selectionCount > 0 {
			out = append(out, fmt.Sprintf("Sel:%d", s.selectionCount))
		}
		return out
	}
	if s.visualMode {
		out = append(out, "Visual")
	}
	if s.yankCount > 0 {
		if s.yankIsCut {
			out = append(out, fmt.Sprintf("Cut: %d", s.yankCount))
		} else {
			out = append(out, fmt.Sprintf("Yank: %d", s.yankCount))
		}
	}
	if s.selectionCount > 0 {
		out = append(out, fmt.Sprintf("Sel: %d", s.selectionCount))
	}
	return out
}
