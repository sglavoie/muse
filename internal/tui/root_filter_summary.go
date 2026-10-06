package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/browse"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// Track lists interpret field prefixes. Collection names use literal matching
// and must not advertise a field constraint they do not actually apply.
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
	return m.collectionFieldFilterText()
}

func (m Model) collectionFieldFilterText() string {
	switch m.ActiveView {
	case viewBrowse:
		return m.browseFieldFilterText()
	case viewFavorites:
		v := m.favorites
		if v.Level == favtui.TrackLevel || (v.Level == favtui.ListLevel && v.Category == favtui.CategoryTracks) {
			return v.Filter.Text
		}
	case viewTags:
		if m.tagsView.Level == tagtui.TrackLevel {
			return m.tagsView.Filter.Text
		}
	}
	return ""
}

func (m Model) browseFieldFilterText() string {
	switch m.browse.Mode {
	case browse.BrowseModeArtists:
		if m.browse.Artists.Level == arttui.TrackLevel {
			return m.browse.Artists.Filter.Text
		}
	case browse.BrowseModeAlbums:
		if m.browse.Albums.Level == albtui.TrackLevel {
			return m.browse.Albums.Filter.Text
		}
	case browse.BrowseModeGenres:
		if m.browse.Genres.Level == gentui.TrackLevel {
			return m.browse.Genres.Filter.Text
		}
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
		{"Album", fq.Album}, {"Album Artist", fq.AlbumArtist}, {"Genre", fq.Genre},
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
