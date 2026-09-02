package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/browse"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/queue"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

func (m Model) handleFavoriteTargetPickerKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.overlay.favoriteTargetPicker == nil {
		return m, nil, false
	}
	if msg.String() == "ctrl+c" {
		return m, tea.Quit, true
	}
	updated, cmd := m.overlay.favoriteTargetPicker.HandleKey(msg.String())
	m.overlay.favoriteTargetPicker = &updated
	return m, cmd, true
}

func (m Model) openFavoriteTargetPicker() Model {
	tracks := m.favoritePickerTracks()
	if len(tracks) == 0 {
		return m
	}
	origin := favtui.PickerOrigin{View: int(m.ActiveView)}
	if m.ActiveView == viewFavorites {
		origin.FavLevel = int(m.favorites.Level)
		origin.FavCategory = int(m.favorites.Category)
		origin.DrilldownName = m.favorites.DrilldownName
	}
	bulk := m.activeViewSelectionCount() > 0
	picker := favtui.NewTargetPicker(
		tracks, m.favoriteHashes, m.favoriteArtists, m.favoriteAlbums, bulk, origin,
	)
	m.overlay.favoriteTargetPicker = &picker
	return m
}

func (m Model) favoritePickerTracks() []library.Track {
	switch m.ActiveView {
	case viewLibrary:
		return m.bulkLibraryTracks()
	case viewQueue:
		filtered := m.queue.FilteredEntries()
		if m.queue.HasSelection() {
			var tracks []library.Track
			for _, entry := range filtered {
				if entry.Selected {
					tracks = append(tracks, queueEntryAsTrack(entry))
				}
			}
			return tracks
		}
		if m.queue.Cursor >= 0 && m.queue.Cursor < len(filtered) {
			return []library.Track{queueEntryAsTrack(filtered[m.queue.Cursor])}
		}
	case viewPlaylists:
		if m.playlists.Level != pltui.TrackLevel {
			return nil
		}
		if len(m.playlists.SelectedTrackIDs) > 0 {
			var tracks []library.Track
			for _, row := range m.playlists.Tracks {
				if _, ok := m.playlists.SelectedTrackIDs[row.ID]; ok {
					tracks = append(tracks, playlistTrackAsTrack(row))
				}
			}
			return tracks
		}
		if track, ok := m.playlists.FocusedTrack(); ok {
			return []library.Track{track}
		}
	case viewBrowse:
		switch m.browse.Mode {
		case browse.BrowseModeArtists:
			if m.browse.Artists.Level == arttui.TrackLevel {
				return m.bulkArtistTracks()
			}
		case browse.BrowseModeAlbums:
			if m.browse.Albums.Level == albtui.TrackLevel {
				return m.bulkAlbumTracks()
			}
		case browse.BrowseModeGenres:
			if m.browse.Genres.Level == gentui.TrackLevel {
				return m.bulkGenreTracks()
			}
		}
	case viewFavorites:
		return m.bulkFavoriteTracks()
	case viewTags:
		if m.tagsView.Level == tagtui.TrackLevel {
			return m.bulkTagTracks()
		}
	case viewMixes:
		if m.mixes.Level != 1 {
			return nil
		}
		trackOrder := m.mixes.TrackView.BaseTracks()
		if m.mixes.TrackView.SortedTracks != nil {
			trackOrder = m.mixes.TrackView.SortedTracks
		}
		if len(m.mixes.TrackView.SelectedHashes) > 0 {
			return collectSelectedTracks(trackOrder, m.mixes.TrackView.SelectedHashes)
		}
		if track, ok := m.mixes.TrackView.Panel().FocusedTrack(); ok {
			return []library.Track{track}
		}
	case viewInsights:
		if !m.insights.AtTracks() {
			return nil
		}
		trackOrder := m.insights.TrackView.BaseTracks()
		if m.insights.TrackView.SortedTracks != nil {
			trackOrder = m.insights.TrackView.SortedTracks
		}
		if len(m.insights.TrackView.SelectedHashes) > 0 {
			return collectSelectedTracks(trackOrder, m.insights.TrackView.SelectedHashes)
		}
		if track, ok := m.insights.TrackView.Panel().FocusedTrack(); ok {
			return []library.Track{track}
		}
	}
	return nil
}

func queueEntryAsTrack(entry queue.Entry) library.Track {
	return library.Track{
		Hash: entry.Hash, Path: entry.Path, Artist: entry.Artist, Title: entry.Title,
		Album: entry.Album, Duration: float64(entry.DurationMs) / 1000,
	}
}

func playlistTrackAsTrack(row db.PlaylistTrackRow) library.Track {
	return library.Track{
		Hash: row.Hash, Path: row.Path, Artist: row.Artist, Title: row.Title,
		Album: row.Album, Duration: float64(row.DurationMs) / 1000,
	}
}

func (m Model) handleFavoriteTargetPicked(msg favtui.TargetPickedMsg) (Model, tea.Cmd) {
	m.overlay.favoriteTargetPicker = nil
	m = exitVisualMode(m)
	m = m.clearActiveViewSelection()
	if m.favoriteWriter == nil || len(msg.Values) == 0 {
		return m, nil
	}

	writer := m.favoriteWriter
	result := favoriteToggledMsg{
		typ:                 msg.Typ,
		added:               msg.Add,
		count:               len(msg.Values),
		removedIdx:          -1,
		fromTargetPicker:    true,
		pickerOriginView:    viewID(msg.Origin.View),
		pickerOriginLevel:   favtui.Level(msg.Origin.FavLevel),
		pickerDrilldownName: msg.Origin.DrilldownName,
		favLevel:            favtui.Level(msg.Origin.FavLevel),
		favCategory:         favtui.Category(msg.Origin.FavCategory),
	}
	if len(msg.Values) == 1 {
		result.value = msg.Values[0]
	}
	if msg.Add {
		if len(msg.Values) > 1 {
			result.addedValues = append([]string(nil), msg.Values...)
		}
	} else {
		if len(msg.Values) > 1 {
			result.removedValues = append([]string(nil), msg.Values...)
		}
		result = m.decoratePickerRemoval(result)
	}

	return m, func() tea.Msg {
		var err error
		switch {
		case msg.Add && len(msg.Values) == 1:
			err = writer.AddFavorite(msg.Typ, msg.Values[0])
		case msg.Add:
			err = writer.AddFavoritesByTypeAndValues(msg.Typ, msg.Values)
		case len(msg.Values) == 1:
			err = writer.RemoveFavorite(msg.Typ, msg.Values[0])
		default:
			err = writer.RemoveFavoritesByTypeAndValues(msg.Typ, msg.Values)
		}
		result.err = err
		return result
	}
}

func (m Model) decoratePickerRemoval(msg favoriteToggledMsg) favoriteToggledMsg {
	if m.ActiveView != viewFavorites {
		return msg
	}
	values := msg.removedValues
	if len(values) == 0 && msg.value != "" {
		values = []string{msg.value}
	}
	if msg.typ == "track" {
		var source []library.Track
		switch {
		case m.favorites.Level == favtui.ListLevel && m.favorites.Category == favtui.CategoryTracks:
			source = m.favorites.Tracks
		case m.favorites.Level == favtui.TrackLevel:
			source = m.favorites.DrilldownTracks
		}
		for _, value := range values {
			for i, track := range source {
				if track.Hash == value {
					msg.removedTracks = append(msg.removedTracks, track)
					msg.removedIndices = append(msg.removedIndices, i)
					break
				}
			}
		}
		if len(values) == 1 && len(msg.removedTracks) == 1 {
			msg.removedTrack = msg.removedTracks[0]
			msg.removedIdx = msg.removedIndices[0]
		} else if len(values) > 1 {
			sortBulkRemovalByIndex(values, msg.removedTracks, msg.removedIndices)
		}
	}

	if m.favorites.Level == favtui.TrackLevel && msg.value == m.favorites.DrilldownName {
		isParent := (m.favorites.Category == favtui.CategoryArtists && msg.typ == "artist") ||
			(m.favorites.Category == favtui.CategoryAlbums && msg.typ == "album")
		if isParent {
			msg.favLevel = favtui.ListLevel
			switch msg.typ {
			case "artist":
				msg.removedIdx = indexOfString(m.favorites.Artists, msg.value)
			case "album":
				msg.removedIdx = indexOfString(m.favorites.Albums, msg.value)
			}
		}
	}
	return msg
}
