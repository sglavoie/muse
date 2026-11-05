package insights

import (
	"github.com/charmbracelet/x/ansi"
	insightsdata "github.com/sglavoie/muse/internal/insights"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// A window with no plays at all leaves every listening panel saying "nothing
// here" in its own words. Rather than a column of those, the briefing keeps
// the panels that stand on the library alone (Coverage, and Data Health when
// it found something) and says once what the rest are waiting for.

const (
	silentMessage      = "Nothing played in this window yet — play something and your listening patterns show up here."
	silentMessageShort = "Nothing played in this window yet."
)

// silentLines is how many lines the silent note adds to the briefing.
const silentLines = 2

// silent reports that no listening panel has anything to show: the window has
// no plays, and neither do the all-time trends Momentum and Discovery draw.
// When only some panels are empty, each keeps its own empty line.
func (v View) silent() bool {
	s := v.Snap
	if s.TotalPlays > 0 || s.Rhythm.TotalPlays > 0 || s.SessionStats.Count > 0 ||
		s.Rankings.Ladder.TotalArtists > 0 || len(s.Rankings.Deltas) > 0 {
		return false
	}
	return !trendsHavePlays(s.Trends)
}

// trendsHavePlays reports whether the all-time trends saw any play.
func trendsHavePlays(t insightsdata.Trends) bool {
	if t.AvgSecs > 0 || t.Current.Plays > 0 || t.CurrentFirstPlays > 0 {
		return true
	}
	for _, b := range t.Buckets {
		if b.Plays > 0 {
			return true
		}
	}
	return false
}

// healthHasFindings reports whether Data Health has anything to say.
func (v View) healthHasFindings() bool {
	return len(v.Health) > 0 || len(v.Snap.Quality.Issues) > 0
}

// visiblePanels lists the panels the briefing and the panel keys offer. In a
// silent window that is Coverage, Data Health with findings, and whichever
// panel is already active, so switching windows never strands the cursor.
func (v View) visiblePanels() []PanelSpec {
	if !v.silent() {
		return insightsPanelSpecs
	}
	var out []PanelSpec
	for _, spec := range insightsPanelSpecs {
		keep := spec.id == PanelCoverage || spec.id == v.Panel ||
			(spec.id == PanelDataHealth && v.healthHasFindings())
		if keep {
			out = append(out, spec)
		}
	}
	return out
}

// briefingLines counts the briefing's lines: one per offered panel, plus the
// silent note when the listening panels are folded away.
func (v View) briefingLines() int {
	n := len(v.visiblePanels())
	if v.silent() {
		n += silentLines
	}
	return n
}

// silentNote is the friendly line that replaces the empty listening panels,
// followed by the window it describes and the key that changes it.
func (v View) silentNote(s styles.Styles) []string {
	msg := silentMessage
	if v.width > 0 && ansi.StringWidth(msg) > v.width {
		msg = text.Truncate(silentMessageShort, v.width)
	}
	hint := v.window().Label + " · w/W change window · 1 library"
	if v.width > 0 {
		hint = text.Truncate(hint, v.width)
	}
	return []string{s.TextSecondary.Render(msg), s.Dim.Render(hint)}
}
