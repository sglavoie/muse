package tui

import (
	"log/slog"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

func (m Model) handleLibraryBulkAction(action action) (tea.Model, tea.Cmd, bool) {
	if action != actionBulkAction {
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
	default:
		return nil
	}
}

func (m Model) bulkLibraryTracks() []library.Track {
	trackOrder := m.library.baseTracks()
	if m.library.sortedTracks != nil {
		trackOrder = m.library.sortedTracks
	}
	if len(m.library.selectedHashes) > 0 {
		return collectSelectedTracks(trackOrder, m.library.selectedHashes)
	}
	if track, ok := m.library.view().FocusedTrack(); ok {
		return []library.Track{track}
	}
	return nil
}

func (m Model) bulkArtistTracks() []library.Track {
	allTracks := m.library.index.Tracks()
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
	allTracks := m.library.index.Tracks()
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
	allTracks := m.library.index.Tracks()
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

	var removedEntries []db.PlaylistTrackRow
	var removedIndices []int
	for index, track := range m.playlists.tracks {
		if _, ok := m.playlists.selectedTrackIDs[track.ID]; ok {
			removedEntries = append(removedEntries, track)
			removedIndices = append(removedIndices, index)
		}
	}

	pd := m.playlistDeleter
	cancel := m.scheduleSoftDelete(
		func() {
			if pd != nil {
				for _, track := range removedEntries {
					if err := pd.RemovePlaylistTrackSoft(track.ID); err != nil {
						slog.Warn("playlist track soft-delete failed", "err", err)
					}
				}
			}
		},
		func(before int64) {
			if pd != nil {
				if err := pd.PurgeDeletedPlaylistTracks(before); err != nil {
					slog.Warn("playlist track purge failed", "err", err)
				}
			}
		},
	)

	var remaining []db.PlaylistTrackRow
	for _, track := range m.playlists.tracks {
		if _, ok := m.playlists.selectedTrackIDs[track.ID]; !ok {
			remaining = append(remaining, track)
		}
	}
	m.playlists.tracks = remaining
	*m = m.preservePlaylistsSort()
	m.playlists.cursor, m.playlists.scrollOffset = clampAfterRemoval(
		m.playlists.cursor, m.playlists.scrollOffset, len(m.playlists.tracks))
	*m = exitVisualMode(*m)
	m.playlists.selectedTrackIDs = nil
	m.setUndoSlot(undoBulkRemovePlaylistTracksAction{entries: removedEntries, indices: removedIndices}, cancel)
	player.PlaySound(player.SoundRemoved)
	return true
}
