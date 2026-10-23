package overlay

import (
	"charm.land/lipgloss/v2"

	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// Column widths for a legend row's value zone. The name column is two columns
// wider than the longest indicator name ("Stop after current", 18) and the
// state column two wider than the longest state ("off"/"all"/"one", 3), so
// every row keeps a visible gutter between name, state and key.
const (
	modesLegendNameWidth  = 20
	modesLegendStateWidth = 5
)

// ModesLegendOverlay renders a centered overlay box explaining the status-bar
// mode cluster: one row per indicator, showing its glyph, what it means, its
// live state and the key that toggles it.
type ModesLegendOverlay struct {
	Repeat    playback.RepeatMode
	Shuffle   bool
	StopAfter bool
	Radio     bool
}

// modesLegendGlyphStyles returns the per-indicator glyph style, positionally
// paired with modeIndicators. These are the unblended semantic palette styles:
// the overlay sits on the normal background, not the status bar's, so the
// background-blended variants from statusBarRenderContext must not be used.
func modesLegendGlyphStyles(s styles.Styles) []lipgloss.Style {
	return []lipgloss.Style{s.ModeRepeat, s.ModeShuffle, s.ModeStopAfter, s.ModeRadio}
}

// Render produces a centered overlay string for the playback-mode legend.
func (o ModesLegendOverlay) Render(termWidth, termHeight int, s styles.Styles) string {
	indicators := playback.ModeIndicators(o.Repeat, o.Shuffle, o.StopAfter, o.Radio)
	glyphStyles := modesLegendGlyphStyles(s)

	rows := make([]Row, 0, len(indicators))
	for i, ind := range indicators {
		// Mirror the cluster's own encoding: dim when off, colored when on.
		style := s.TextTertiary
		if ind.Active && i < len(glyphStyles) {
			style = glyphStyles[i]
		}
		rows = append(rows, Row{
			Text: style.Render(text.Pad(ind.Name, modesLegendNameWidth)) + text.Pad(ind.State, modesLegendStateWidth) + ind.Key,
		})
	}

	box := Box{Title: " ♪ Playback Modes ", Rows: rows, Hint: "space play/pause · n/p skip · esc close"}
	return box.Render(termWidth, termHeight, s)
}
