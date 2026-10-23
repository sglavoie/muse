package tui

import (
	"github.com/charmbracelet/x/ansi"
	"path/filepath"
	"strings"

	"github.com/sglavoie/muse/internal/tui/text"
)

// Use otherwise empty space above the footer for the focused song, without
// taking rows from a full table or moving the cursor. Information follows focus,
// independently of the song playing in the status bar.
func (m Model) renderFocusedIdentity(maxLines int) string {
	if m.hasActiveOverlay() || maxLines < 2 {
		return ""
	}
	track, ok := m.focusedTrackForInspection()
	if !ok {
		return ""
	}
	title := track.Title
	if title == "" {
		title = filepath.Base(track.Path)
	}
	var context []string
	for _, value := range []string{track.Artist, track.Album} {
		if value != "" {
			context = append(context, value)
		}
	}
	width := max(m.termWidth-2, 0)
	maxLines = min(maxLines, 4)
	titleLines := focusedIdentityLines("Focused · "+title, width, maxLines-1)
	contextLines := focusedIdentityLines(strings.Join(context, " — "), width, maxLines-len(titleLines))
	var lines []string
	for _, line := range titleLines {
		lines = append(lines, "  "+m.styles.TextSecondary.Render(line))
	}
	for _, line := range contextLines {
		lines = append(lines, "  "+m.styles.Dim.Render(line))
	}
	return strings.Join(lines, "\n")
}

func focusedIdentityLines(value string, width, budget int) []string {
	lines := strings.Split(ansi.Wrap(value, width, " "), "\n")
	if len(lines) > budget {
		lines[budget-1] = text.Truncate(strings.Join(lines[budget-1:], " "), width)
		lines = lines[:budget]
	}
	return lines
}
