package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/browse"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	"github.com/sglavoie/muse/internal/tui/keys"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
	"github.com/sglavoie/muse/internal/tui/text"
)

func (m Model) handleLibraryBulkAction(action keys.Action) (tea.Model, tea.Cmd, bool) {
	if action != keys.ActionBulkAction {
		return m, nil, false
	}
	if m.ActiveView == viewPlaylists {
		m.handlePlaylistBulkRemove()
		return m, nil, true
	}

	tracks := m.bulkActionTracks()
	if len(tracks) > 0 {
		var duration text.DurationTotal
		for _, track := range tracks {
			duration.Add(track.Duration)
		}
		modal := pltui.NewDestinationPicker(m.playlists.Playlists).WithTrackSummary(len(tracks), duration.String())
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
	switch m.ActiveView {
	case viewLibrary:
		return m.bulkLibraryTracks()
	case viewBrowse:
		switch m.browse.Mode {
		case browse.BrowseModeArtists:
			return m.bulkArtistTracks()
		case browse.BrowseModeAlbums:
			return m.bulkAlbumTracks()
		case browse.BrowseModeGenres:
			return m.bulkGenreTracks()
		}
		return nil
	case viewFavorites:
		return m.bulkFavoriteTracks()
	case viewTags:
		return m.bulkTagTracks()
	case viewMixes:
		if m.mixes.Level == 1 {
			view := m.mixes.TrackView
			tracks := view.SortedTracks
			if tracks == nil {
				tracks = view.BaseTracks()
			}
			if len(view.SelectedHashes) > 0 {
				return collectSelectedTracks(tracks, view.SelectedHashes)
			}
			if track, ok := view.Panel().FocusedTrack(); ok {
				return []library.Track{track}
			}
		}
		return nil
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
	switch m.browse.Artists.Level {
	case arttui.ArtistLevel:
		if artist, ok := m.browse.Artists.FocusedArtist(); ok {
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
		if album, ok := m.browse.Artists.FocusedAlbum(); ok {
			return arttui.DeriveTracks(allTracks, m.browse.Artists.SelectedArtist, album)
		}
	case arttui.TrackLevel:
		if len(m.browse.Artists.SelectedHashes) > 0 {
			return collectSelectedTracks(m.browse.Artists.Tracks, m.browse.Artists.SelectedHashes)
		}
		if track, ok := m.browse.Artists.FocusedTrack(); ok {
			return []library.Track{track}
		}
	}
	return nil
}

func (m Model) bulkAlbumTracks() []library.Track {
	allTracks := m.library.Index.Tracks()
	switch m.browse.Albums.Level {
	case albtui.AlbumLevel:
		if album, ok := m.browse.Albums.FocusedAlbum(); ok {
			return albtui.DeriveAlbumTracks(allTracks, album)
		}
	case albtui.TrackLevel:
		if len(m.browse.Albums.SelectedHashes) > 0 {
			return collectSelectedTracks(m.browse.Albums.Tracks, m.browse.Albums.SelectedHashes)
		}
		if track, ok := m.browse.Albums.FocusedTrack(); ok {
			return []library.Track{track}
		}
	}
	return nil
}

func (m Model) bulkGenreTracks() []library.Track {
	allTracks := m.library.Index.Tracks()
	switch m.browse.Genres.Level {
	case gentui.GenreLevel:
		if genre, ok := m.browse.Genres.FocusedGenre(); ok {
			return gentui.DeriveGenreTracks(allTracks, genre)
		}
	case gentui.ArtistLevel:
		if artist, ok := m.browse.Genres.FocusedArtist(); ok {
			return gentui.DeriveGenreArtistTracks(allTracks, m.browse.Genres.SelectedGenre, artist)
		}
	case gentui.TrackLevel:
		if len(m.browse.Genres.SelectedHashes) > 0 {
			return collectSelectedTracks(m.browse.Genres.Tracks, m.browse.Genres.SelectedHashes)
		}
		if track, ok := m.browse.Genres.FocusedTrack(); ok {
			return []library.Track{track}
		}
	}
	return nil
}

func (m Model) bulkFavoriteTracks() []library.Track {
	if m.favorites.HasSelection() {
		return m.favorites.SelectedTracks()
	}
	if track, ok := m.favorites.FocusedTrack(); ok {
		return []library.Track{track}
	}
	return nil
}

func (m Model) bulkTagTracks() []library.Track {
	if m.tagsView.Level != tagtui.TrackLevel {
		return nil
	}
	items := m.tagsView.DisplayTracks()
	if len(m.tagsView.SelectedHashes) > 0 {
		var tracks []library.Track
		for _, t := range items {
			if _, ok := m.tagsView.SelectedHashes[t.Hash]; ok {
				tracks = append(tracks, library.Track{
					Hash:        t.Hash,
					Path:        t.Path,
					Artist:      t.Artist,
					Title:       t.Title,
					Album:       t.Album,
					AlbumArtist: t.AlbumArtist,
					Duration:    float64(t.DurationMs) / 1000,
				})
			}
		}
		return tracks
	}
	if t, ok := m.tagsView.FocusedTrack(); ok {
		return []library.Track{{
			Hash:        t.Hash,
			Path:        t.Path,
			Artist:      t.Artist,
			Title:       t.Title,
			Album:       t.Album,
			AlbumArtist: t.AlbumArtist,
			Duration:    float64(t.DurationMs) / 1000,
		}}
	}
	return nil
}

func (m *Model) handlePlaylistBulkRemove() bool {
	if m.playlists.Level != pltui.TrackLevel || !m.playlists.HasTrackSelection() {
		return false
	}
	newM, _ := m.handlePlaylistDeleteBulkTracks()
	*m = newM
	return true
}
