package tui

import (
	"fmt"

	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/browse"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// filterRowKeys names every visible row in display order, independently of
// bulk selection. Queue IDs distinguish multiple occurrences of the same track.
func (m Model) filterRowKeys() []string {
	switch m.ActiveView {
	case viewPlaylists:
		return m.playlistFilterRowKeys()
	case viewLibrary:
		return filterTrackKeys(m.library.Panel().VisibleTracks())
	case viewMixes:
		return filterTrackKeys(m.mixes.TrackView.Panel().VisibleTracks())
	case viewInsights:
		return filterTrackKeys(m.insights.TrackView.Panel().VisibleTracks())
	case viewQueue:
		return m.queueFilterRowKeys()
	case viewBrowse:
		return m.browseFilterRowKeys()
	case viewFavorites:
		return m.favoritesFilterRowKeys()
	case viewTags:
		return m.tagsFilterRowKeys()
	default:
		return nil
	}
}

func filterTrackKeys(tracks []library.Track) []string {
	keys := make([]string, len(tracks))
	for i, t := range tracks {
		keys[i] = t.Hash
		if keys[i] == "" {
			keys[i] = t.Path
		}
	}
	return keys
}

func (m Model) browseFilterRowKeys() []string {
	switch m.browse.Mode {
	case browse.BrowseModeArtists:
		v := m.browse.Artists
		switch v.Level {
		case arttui.ArtistLevel:
			return arttui.FilterStrings(v.Artists, v.Filter.Text)
		case arttui.AlbumLevel:
			return arttui.FilterStrings(v.Albums, v.Filter.Text)
		default:
			return filterTrackKeys(arttui.FilterTracks(v.Tracks, v.Filter.Text))
		}
	case browse.BrowseModeAlbums:
		v := m.browse.Albums
		if v.Level == albtui.AlbumLevel {
			return arttui.FilterStrings(v.Albums, v.Filter.Text)
		}
		return filterTrackKeys(arttui.FilterTracks(v.Tracks, v.Filter.Text))
	default:
		return m.genreFilterRowKeys()
	}
}

func (m Model) genreFilterRowKeys() []string {
	v := m.browse.Genres
	switch v.Level {
	case gentui.GenreLevel:
		return arttui.FilterStrings(v.Genres, v.Filter.Text)
	case gentui.ArtistLevel:
		return arttui.FilterStrings(v.Artists, v.Filter.Text)
	default:
		return filterTrackKeys(arttui.FilterTracks(v.Tracks, v.Filter.Text))
	}
}

func (m Model) favoritesFilterRowKeys() []string {
	v := m.favorites
	switch {
	case v.Level == favtui.CategoryLevel:
		return []string{"artists", "albums", "tracks"}
	case v.Level == favtui.TrackLevel:
		return filterTrackKeys(arttui.FilterTracks(v.DrilldownTracks, v.Filter.Text))
	case v.Category == favtui.CategoryArtists:
		return arttui.FilterStrings(v.Artists, v.Filter.Text)
	case v.Category == favtui.CategoryAlbums:
		return arttui.FilterStrings(v.Albums, v.Filter.Text)
	default:
		return filterTrackKeys(arttui.FilterTracks(v.Tracks, v.Filter.Text))
	}
}

func (m Model) tagsFilterRowKeys() []string {
	v := m.tagsView
	if v.Level == tagtui.TrackLevel {
		rows := v.DisplayTracks()
		keys := make([]string, len(rows))
		for i, row := range rows {
			keys[i] = row.Hash
		}
		return keys
	}
	rows := (tagtui.View{Tags: v.Tags, TagCounts: v.TagCounts, FilterText: v.Filter.Text}).EffectiveTags()
	keys := make([]string, len(rows))
	for i, row := range rows {
		keys[i] = fmt.Sprintf("tag:%d", row.ID)
	}
	return keys
}

func (m Model) queueFilterRowKeys() []string {
	entries := m.queue.FilteredEntries()
	keys := make([]string, len(entries))
	for i, e := range entries {
		keys[i] = fmt.Sprintf("id:%d", e.ID)
		if e.ID == 0 {
			keys[i] = fmt.Sprintf("row:%d:%s", e.OrigIdx, e.Path)
		}
	}
	return keys
}

func (m Model) playlistFilterRowKeys() []string {
	var keys []string
	if m.playlists.AtListLevel() {
		for _, row := range m.playlists.EffectivePlaylists() {
			keys = append(keys, fmt.Sprintf("playlist:%d", row.ID))
		}
	} else {
		for _, row := range m.playlists.DisplayTracks() {
			keys = append(keys, fmt.Sprintf("playlist-track:%d", row.ID))
		}
	}
	return keys
}
