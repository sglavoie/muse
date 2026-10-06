package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/library"
)

// Library-based track panels, Queue, and open playlists interpret field prefixes.
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
	case viewPlaylists:
		if !m.playlists.AtListLevel() {
			return m.playlists.Filter.Text
		}
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
	parts := []string{}
	for _, field := range []struct{ label, value string }{
		{"Text", fq.General}, {"Artist", fq.Artist}, {"Title", fq.Title},
		{"Album", fq.Album}, {"Genre", fq.Genre},
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
