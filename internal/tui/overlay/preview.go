package overlay

import (
	"fmt"

	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// PreviewOverlay renders a centered overlay box showing preview track info
// and live playback progress.
type PreviewOverlay struct {
	Hash      string
	Artist    string
	Title     string
	Album     string
	Elapsed   float64
	Total     float64
	PlayCount int
	TagsLine  string // pre-rendered colored tag names; empty when none
}

// Render produces a centered overlay string for the preview. The caller should
// pass the current terminal dimensions so the box is positioned in the middle
// of the screen.
func (p PreviewOverlay) Render(termWidth, termHeight int, s styles.Styles) string {
	progress := fmt.Sprintf("▶ %s / %s", text.FormatPositionSecs(p.Elapsed), text.FormatPositionSecs(p.Total))
	return renderTrackOverlay(p.Title, p.Artist, p.Album, progress, p.PlayCount, p.TagsLine,
		" ♪ Preview ", "space pause, arrows seek, shift+arrows long seek, P/esc dismiss", termWidth, termHeight, s)
}
