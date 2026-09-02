package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
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
	origin := favoritePickerOrigin{view: m.activeView}
	if m.activeView == viewFavorites {
		origin.favLevel = int(m.favorites.level)
		origin.favCategory = int(m.favorites.category)
		origin.drilldownName = m.favorites.drilldownName
	}
	bulk := m.activeViewSelectionCount() > 0
	picker := newFavoriteTargetPicker(
		tracks, m.favoriteHashes, m.favoriteArtists, m.favoriteAlbums, bulk, origin,
	)
	m.overlay.favoriteTargetPicker = &picker
	return m
}

func (m Model) favoritePickerTracks() []library.Track {
	switch m.activeView {
	case viewLibrary:
		return m.bulkLibraryTracks()
	case viewQueue:
		filtered := m.queue.filteredEntries()
		if m.queue.hasSelection() {
			var tracks []library.Track
			for _, entry := range filtered {
				if entry.Selected {
					tracks = append(tracks, queueEntryAsTrack(entry))
				}
			}
			return tracks
		}
		if m.queue.cursor >= 0 && m.queue.cursor < len(filtered) {
			return []library.Track{queueEntryAsTrack(filtered[m.queue.cursor])}
		}
	case viewPlaylists:
		if m.playlists.level != pltui.TrackLevel {
			return nil
		}
		if len(m.playlists.selectedTrackIDs) > 0 {
			var tracks []library.Track
			for _, row := range m.playlists.tracks {
				if _, ok := m.playlists.selectedTrackIDs[row.ID]; ok {
					tracks = append(tracks, playlistTrackAsTrack(row))
				}
			}
			return tracks
		}
		if track, ok := m.playlists.focusedTrack(); ok {
			return []library.Track{track}
		}
	case viewBrowse:
		switch m.browse.mode {
		case BrowseModeArtists:
			if m.browse.artists.level == arttui.TrackLevel {
				return m.bulkArtistTracks()
			}
		case BrowseModeAlbums:
			if m.browse.albums.level == albtui.TrackLevel {
				return m.bulkAlbumTracks()
			}
		case BrowseModeGenres:
			if m.browse.genres.level == gentui.TrackLevel {
				return m.bulkGenreTracks()
			}
		}
	case viewFavorites:
		return m.bulkFavoriteTracks()
	case viewTags:
		if m.tagsView.level == tagtui.TrackLevel {
			return m.bulkTagTracks()
		}
	case viewMixes:
		if m.mixes.level != 1 {
			return nil
		}
		trackOrder := m.mixes.trackView.BaseTracks()
		if m.mixes.trackView.SortedTracks != nil {
			trackOrder = m.mixes.trackView.SortedTracks
		}
		if len(m.mixes.trackView.SelectedHashes) > 0 {
			return collectSelectedTracks(trackOrder, m.mixes.trackView.SelectedHashes)
		}
		if track, ok := m.mixes.trackView.Panel().FocusedTrack(); ok {
			return []library.Track{track}
		}
	case viewInsights:
		if !m.insights.atTracks() {
			return nil
		}
		trackOrder := m.insights.trackView.BaseTracks()
		if m.insights.trackView.SortedTracks != nil {
			trackOrder = m.insights.trackView.SortedTracks
		}
		if len(m.insights.trackView.SelectedHashes) > 0 {
			return collectSelectedTracks(trackOrder, m.insights.trackView.SelectedHashes)
		}
		if track, ok := m.insights.trackView.Panel().FocusedTrack(); ok {
			return []library.Track{track}
		}
	}
	return nil
}

func queueEntryAsTrack(entry QueueEntry) library.Track {
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

func (m Model) handleFavoriteTargetPicked(msg favoriteTargetPickedMsg) (Model, tea.Cmd) {
	m.overlay.favoriteTargetPicker = nil
	m = exitVisualMode(m)
	m = m.clearActiveViewSelection()
	if m.favoriteWriter == nil || len(msg.values) == 0 {
		return m, nil
	}

	writer := m.favoriteWriter
	result := favoriteToggledMsg{
		typ:                 msg.typ,
		added:               msg.add,
		count:               len(msg.values),
		removedIdx:          -1,
		fromTargetPicker:    true,
		pickerOriginView:    msg.origin.view,
		pickerOriginLevel:   favtui.Level(msg.origin.favLevel),
		pickerDrilldownName: msg.origin.drilldownName,
		favLevel:            favtui.Level(msg.origin.favLevel),
		favCategory:         favtui.Category(msg.origin.favCategory),
	}
	if len(msg.values) == 1 {
		result.value = msg.values[0]
	}
	if msg.add {
		if len(msg.values) > 1 {
			result.addedValues = append([]string(nil), msg.values...)
		}
	} else {
		if len(msg.values) > 1 {
			result.removedValues = append([]string(nil), msg.values...)
		}
		result = m.decoratePickerRemoval(result)
	}

	return m, func() tea.Msg {
		var err error
		switch {
		case msg.add && len(msg.values) == 1:
			err = writer.AddFavorite(msg.typ, msg.values[0])
		case msg.add:
			err = writer.AddFavoritesByTypeAndValues(msg.typ, msg.values)
		case len(msg.values) == 1:
			err = writer.RemoveFavorite(msg.typ, msg.values[0])
		default:
			err = writer.RemoveFavoritesByTypeAndValues(msg.typ, msg.values)
		}
		result.err = err
		return result
	}
}

func (m Model) decoratePickerRemoval(msg favoriteToggledMsg) favoriteToggledMsg {
	if m.activeView != viewFavorites {
		return msg
	}
	values := msg.removedValues
	if len(values) == 0 && msg.value != "" {
		values = []string{msg.value}
	}
	if msg.typ == "track" {
		var source []library.Track
		switch {
		case m.favorites.level == favtui.ListLevel && m.favorites.category == favtui.CategoryTracks:
			source = m.favorites.tracks
		case m.favorites.level == favtui.TrackLevel:
			source = m.favorites.drilldownTracks
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

	if m.favorites.level == favtui.TrackLevel && msg.value == m.favorites.drilldownName {
		isParent := (m.favorites.category == favtui.CategoryArtists && msg.typ == "artist") ||
			(m.favorites.category == favtui.CategoryAlbums && msg.typ == "album")
		if isParent {
			msg.favLevel = favtui.ListLevel
			switch msg.typ {
			case "artist":
				msg.removedIdx = indexOfString(m.favorites.artists, msg.value)
			case "album":
				msg.removedIdx = indexOfString(m.favorites.albums, msg.value)
			}
		}
	}
	return msg
}
