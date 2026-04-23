package tui

import (
	tea "charm.land/bubbletea/v2"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
)

func (m Model) handleLibraryNavigationAction(action action) (tea.Model, tea.Cmd, bool) {
	switch action {
	case actionArtistOpen:
		return m.handleArtistOpenAction()
	case actionArtistBack:
		m = exitVisualMode(m)
		m.artists = m.artists.back()
		return m, nil, true
	case actionAlbumOpen:
		return m.handleAlbumOpenAction()
	case actionAlbumBack:
		m = exitVisualMode(m)
		m.albums = m.albums.back()
		return m, nil, true
	case actionGenreOpen:
		return m.handleGenreOpenAction()
	case actionGenreBack:
		m = exitVisualMode(m)
		m.genres = m.genres.back()
		return m, nil, true
	default:
		return m, nil, false
	}
}

func (m Model) handleArtistOpenAction() (tea.Model, tea.Cmd, bool) {
	allTracks := m.library.index.Tracks()
	switch m.artists.level {
	case arttui.ArtistLevel:
		artist, ok := m.artists.focusedArtist()
		if ok {
			albums := arttui.DeriveAlbums(allTracks, artist)
			m.artists.selectedArtist = artist
			if len(albums) == 0 {
				tracks := arttui.DeriveArtistTracks(allTracks, artist)
				m.artists.selectedAlbum = ""
				m.artists = m.artists.openAlbums(nil)
				m.artists = m.artists.openTracks(tracks)
			} else {
				m.artists = m.artists.openAlbums(albums)
			}
		}
	case arttui.AlbumLevel:
		album, ok := m.artists.focusedAlbum()
		if ok {
			tracks := arttui.DeriveTracks(allTracks, m.artists.selectedArtist, album)
			m.artists.selectedAlbum = album
			m.artists = m.artists.openTracks(tracks)
		}
	case arttui.TrackLevel:
		if m.queueWriter != nil {
			if cmd := m.artists.enqueueFocusedTrack(m.queueWriter, len(m.queue.entries)); cmd != nil {
				return m, cmd, true
			}
		}
	}
	return m, nil, true
}

func (m Model) handleAlbumOpenAction() (tea.Model, tea.Cmd, bool) {
	allTracks := m.library.index.Tracks()
	switch m.albums.level {
	case albtui.AlbumLevel:
		album, ok := m.albums.focusedAlbum()
		if ok {
			tracks := albtui.DeriveAlbumTracks(allTracks, album)
			m.albums.selectedAlbum = album
			m.albums = m.albums.openTracks(tracks)
		}
	case albtui.TrackLevel:
		if m.queueWriter != nil {
			if cmd := m.albums.enqueueFocusedTrack(m.queueWriter, len(m.queue.entries)); cmd != nil {
				return m, cmd, true
			}
		}
	}
	return m, nil, true
}

func (m Model) handleGenreOpenAction() (tea.Model, tea.Cmd, bool) {
	allTracks := m.library.index.Tracks()
	switch m.genres.level {
	case gentui.GenreLevel:
		genre, ok := m.genres.focusedGenre()
		if ok {
			artists := gentui.DeriveGenreArtists(allTracks, genre)
			m.genres.selectedGenre = genre
			if len(artists) == 0 {
				tracks := gentui.DeriveGenreTracks(allTracks, genre)
				m.genres = m.genres.openArtists(nil)
				m.genres = m.genres.openTracks(tracks)
			} else {
				m.genres = m.genres.openArtists(artists)
			}
		}
	case gentui.ArtistLevel:
		artist, ok := m.genres.focusedArtist()
		if ok {
			tracks := gentui.DeriveGenreArtistTracks(allTracks, m.genres.selectedGenre, artist)
			m.genres.selectedArtist = artist
			m.genres = m.genres.openTracks(tracks)
		}
	case gentui.TrackLevel:
		if m.queueWriter != nil {
			if cmd := m.genres.enqueueFocusedTrack(m.queueWriter, len(m.queue.entries)); cmd != nil {
				return m, cmd, true
			}
		}
	}
	return m, nil, true
}
