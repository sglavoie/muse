package tui

import (
	"charm.land/lipgloss/v2"

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
	repeat    RepeatMode
	shuffle   bool
	stopAfter bool
	radio     bool
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
	indicators := modeIndicators(o.repeat, o.shuffle, o.stopAfter, o.radio)
	glyphStyles := modesLegendGlyphStyles(s)

	rows := make([]OverlayRow, 0, len(indicators))
	for i, ind := range indicators {
		// Mirror the cluster's own encoding: dim when off, colored when on.
		style := s.TextTertiary
		if ind.active && i < len(glyphStyles) {
			style = glyphStyles[i]
		}
		rows = append(rows, OverlayRow{
			Label: style.Render(ind.glyph),
			Text:  text.Pad(ind.name, modesLegendNameWidth) + text.Pad(ind.state, modesLegendStateWidth) + ind.key,
		})
	}

	box := OverlayBox{Title: " ♪ Playback Modes ", Rows: rows, Hint: "; or esc to dismiss"}
	return box.Render(termWidth, termHeight, s)
}
