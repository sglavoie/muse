package tui

import (
	"fmt"
	"math"
	"strings"

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

// renderProgressBar builds a full-width progress bar string. The filled portion
// uses "█" styled with ProgressFilled; the empty portion uses "░" with
// ProgressEmpty. elapsed/total are seconds; fraction is clamped to [0,1].
// Returns "" when width <= 0 so that callers using calculated/clamped widths
// cannot trigger a strings.Repeat panic on negative counts.
func renderProgressBar(elapsed, total float64, width int, st styles.Styles) string {
	if width <= 0 {
		return ""
	}
	frac := 0.0
	if total > 0 {
		frac = elapsed / total
		if frac < 0 {
			frac = 0
		} else if frac > 1 {
			frac = 1
		}
	}
	filled := int(math.Round(frac * float64(width)))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return st.ProgressFilled.Render(strings.Repeat("█", filled)) +
		st.ProgressEmpty.Render(strings.Repeat("░", width-filled))
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
