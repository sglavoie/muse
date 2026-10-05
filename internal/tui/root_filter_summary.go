package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/library"
)

// Only the Library-based track panels and Queue interpret field prefixes.
// Other lists perform literal substring matching and must not advertise a
// field constraint they do not actually apply.
func (m Model) fieldFilterText() string {
	switch m.ActiveView {
	case viewLibrary:
		return m.library.Filter.Text
	case viewMixes:
		return m.mixes.TrackView.Filter.Text
	case viewInsights:
		return m.insights.TrackView.Filter.Text
	case viewQueue:
		return m.queue.Filter.Text
	}
	return ""
}

func (m Model) filterQuerySummary() string {
	raw := m.fieldFilterText()
	fq := library.ParseFilterQuery(raw)
	if !fq.IsFieldSpecific() && fq.General == strings.TrimSpace(raw) {
		return ""
	}
	genreLabel := "Genre"
	if m.ActiveView == viewQueue {
		genreLabel = "Genre (ignored in Queue)"
	}
	parts := []string{}
	for _, field := range []struct{ label, value string }{
		{"Text", fq.General}, {"Artist", fq.Artist}, {"Title", fq.Title},
		{"Album", fq.Album}, {genreLabel, fq.Genre},
	} {
		if field.value != "" {
			value := strings.Join(strings.Fields(ansi.Strip(field.value)), " ")
			parts = append(parts, field.label+": "+value)
		}
	}
	if len(parts) == 0 {
		return "Empty prefixes: no text constraint"
	}
	return strings.Join(parts, " · ")
}
