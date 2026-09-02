package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	"github.com/sglavoie/muse/internal/tui/keys"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

func (m Model) handleLibraryBulkAction(action keys.Action) (tea.Model, tea.Cmd, bool) {
	if action != keys.ActionBulkAction {
		return m, nil, false
	}
	if m.activeView == viewPlaylists {
		m.handlePlaylistBulkRemove()
		return m, nil, true
	}

	tracks := m.bulkActionTracks()
	if len(tracks) > 0 {
		modal := pltui.NewDestinationPicker(m.playlists.playlists)
		m.overlay.destPicker = &modal
		m.overlay.destPickerTracks = tracks
	}
	return m, nil, true
}

// collectSelectedTracks returns tracks whose Hash is in the selected set,
// preserving the order of the input slice.
func collectSelectedTracks(tracks []library.Track, selected map[string]struct{}) []library.Track {
	out := make([]library.Track, 0, len(selected))
	for _, t := range tracks {
		if _, ok := selected[t.Hash]; ok {
			out = append(out, t)
		}
	}
	return out
}

func (m Model) bulkActionTracks() []library.Track {
	switch m.activeView {
	case viewLibrary:
		return m.bulkLibraryTracks()
	case viewBrowse:
		switch m.browse.mode {
		case BrowseModeArtists:
			return m.bulkArtistTracks()
		case BrowseModeAlbums:
			return m.bulkAlbumTracks()
		case BrowseModeGenres:
			return m.bulkGenreTracks()
		}
		return nil
	case viewFavorites:
		return m.bulkFavoriteTracks()
	case viewTags:
		return m.bulkTagTracks()
	case viewInsights:
		return m.insightsBulkTracks()
	default:
		return nil
	}
}

func (m Model) bulkLibraryTracks() []library.Track {
	trackOrder := m.library.BaseTracks()
	if m.library.SortedTracks != nil {
		trackOrder = m.library.SortedTracks
	}
	if len(m.library.SelectedHashes) > 0 {
		return collectSelectedTracks(trackOrder, m.library.SelectedHashes)
	}
	if track, ok := m.library.Panel().FocusedTrack(); ok {
		return []library.Track{track}
	}
	return nil
}

func (m Model) bulkArtistTracks() []library.Track {
	allTracks := m.library.Index.Tracks()
	switch m.browse.artists.level {
	case arttui.ArtistLevel:
		if artist, ok := m.browse.artists.focusedArtist(); ok {
			var tracks []library.Track
			raw := arttui.RawName(artist)
			for _, track := range allTracks {
				if track.Artist == raw {
					tracks = append(tracks, track)
				}
			}
			return tracks
		}
	case arttui.AlbumLevel:
		if album, ok := m.browse.artists.focusedAlbum(); ok {
			return arttui.DeriveTracks(allTracks, m.browse.artists.selectedArtist, album)
		}
	case arttui.TrackLevel:
		if len(m.browse.artists.selectedHashes) > 0 {
			return collectSelectedTracks(m.browse.artists.tracks, m.browse.artists.selectedHashes)
		}
		if track, ok := m.browse.artists.focusedTrack(); ok {
			return []library.Track{track}
		}
	}
	return nil
}

func (m Model) bulkAlbumTracks() []library.Track {
	allTracks := m.library.Index.Tracks()
	switch m.browse.albums.level {
	case albtui.AlbumLevel:
		if album, ok := m.browse.albums.focusedAlbum(); ok {
			return albtui.DeriveAlbumTracks(allTracks, album)
		}
	case albtui.TrackLevel:
		if len(m.browse.albums.selectedHashes) > 0 {
			return collectSelectedTracks(m.browse.albums.tracks, m.browse.albums.selectedHashes)
		}
		if track, ok := m.browse.albums.focusedTrack(); ok {
			return []library.Track{track}
		}
	}
	return nil
}

func (m Model) bulkGenreTracks() []library.Track {
	allTracks := m.library.Index.Tracks()
	switch m.browse.genres.level {
	case gentui.GenreLevel:
		if genre, ok := m.browse.genres.focusedGenre(); ok {
			return gentui.DeriveGenreTracks(allTracks, genre)
		}
	case gentui.ArtistLevel:
		if artist, ok := m.browse.genres.focusedArtist(); ok {
			return gentui.DeriveGenreArtistTracks(allTracks, m.browse.genres.selectedGenre, artist)
		}
	case gentui.TrackLevel:
		if len(m.browse.genres.selectedHashes) > 0 {
			return collectSelectedTracks(m.browse.genres.tracks, m.browse.genres.selectedHashes)
		}
		if track, ok := m.browse.genres.focusedTrack(); ok {
			return []library.Track{track}
		}
	}
	return nil
}

func (m Model) bulkFavoriteTracks() []library.Track {
	switch {
	case m.favorites.level == favtui.ListLevel && m.favorites.category == favtui.CategoryTracks:
		if len(m.favorites.selectedItems) > 0 {
			items := arttui.FilterTracks(m.favorites.tracks, m.favorites.filter.Text)
			var tracks []library.Track
			for _, idx := range sortedIntSetKeys(m.favorites.selectedItems) {
				if idx >= 0 && idx < len(items) {
					tracks = append(tracks, items[idx])
				}
			}
			return tracks
		}
		if track, ok := m.favorites.focusedTrack(); ok {
			return []library.Track{track}
		}
	case m.favorites.level == favtui.TrackLevel:
		if len(m.favorites.selectedItems) > 0 {
			items := arttui.FilterTracks(m.favorites.drilldownTracks, m.favorites.filter.Text)
			var tracks []library.Track
			for _, idx := range sortedIntSetKeys(m.favorites.selectedItems) {
				if idx >= 0 && idx < len(items) {
					tracks = append(tracks, items[idx])
				}
			}
			return tracks
		}
		if track, ok := m.favorites.focusedTrack(); ok {
			return []library.Track{track}
		}
	}
	return nil
}

func (m Model) bulkTagTracks() []library.Track {
	if m.tagsView.level != tagtui.TrackLevel {
		return nil
	}
	items := m.tagsView.displayTracks()
	if len(m.tagsView.selectedHashes) > 0 {
		var tracks []library.Track
		for _, t := range items {
			if _, ok := m.tagsView.selectedHashes[t.Hash]; ok {
				tracks = append(tracks, library.Track{
					Hash:     t.Hash,
					Path:     t.Path,
					Artist:   t.Artist,
					Title:    t.Title,
					Album:    t.Album,
					Duration: float64(t.DurationMs) / 1000,
				})
			}
		}
		return tracks
	}
	if t, ok := m.tagsView.focusedTrack(); ok {
		return []library.Track{{
			Hash:     t.Hash,
			Path:     t.Path,
			Artist:   t.Artist,
			Title:    t.Title,
			Album:    t.Album,
			Duration: float64(t.DurationMs) / 1000,
		}}
	}
	return nil
}

func (m *Model) handlePlaylistBulkRemove() bool {
	if m.playlists.level != pltui.TrackLevel || !m.playlists.hasTrackSelection() {
		return false
	}
	newM, _ := m.handlePlaylistDeleteBulkTracks()
	*m = newM
	return true
}
