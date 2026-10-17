package tui

import (
	"path/filepath"
	"strings"

	"github.com/sglavoie/muse/internal/tui/text"
)

// Use otherwise empty space above the footer for the focused song, without
// taking rows from a full table or moving the cursor. Information follows focus,
// independently of the song playing in the status bar.
func (m Model) renderFocusedIdentity() string {
	if m.hasActiveOverlay() || m.termHeight < 24 {
		return ""
	}
	track, ok := m.focusedTrackForPreview()
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
	return "  " + m.styles.TextSecondary.Render(text.Truncate(title, width)) + "\n  " +
		m.styles.Dim.Render(text.Truncate(strings.Join(context, " — "), width))
}
