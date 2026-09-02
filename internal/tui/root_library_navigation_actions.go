package tui

import (
	tea "charm.land/bubbletea/v2"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	"github.com/sglavoie/muse/internal/tui/keys"
)

func (m Model) handleLibraryNavigationAction(action keys.Action) (tea.Model, tea.Cmd, bool) {
	switch action {
	case keys.ActionArtistOpen:
		return m.handleArtistOpenAction()
	case keys.ActionArtistBack:
		m = exitVisualMode(m)
		m.browse.artists = m.browse.artists.back()
		return m, nil, true
	case keys.ActionAlbumOpen:
		return m.handleAlbumOpenAction()
	case keys.ActionAlbumBack:
		m = exitVisualMode(m)
		m.browse.albums = m.browse.albums.back()
		return m, nil, true
	case keys.ActionGenreOpen:
		return m.handleGenreOpenAction()
	case keys.ActionGenreBack:
		m = exitVisualMode(m)
		m.browse.genres = m.browse.genres.back()
		return m, nil, true
	default:
		return m, nil, false
	}
}

func (m Model) handleArtistOpenAction() (tea.Model, tea.Cmd, bool) {
	allTracks := m.library.Index.Tracks()
	switch m.browse.artists.level {
	case arttui.ArtistLevel:
		artist, ok := m.browse.artists.focusedArtist()
		if ok {
			albums := arttui.DeriveAlbums(allTracks, artist)
			m.browse.artists.selectedArtist = artist
			if len(albums) == 0 {
				tracks := arttui.DeriveArtistTracks(allTracks, artist)
				m.browse.artists.selectedAlbum = ""
				m.browse.artists = m.browse.artists.openAlbums(nil)
				m.browse.artists = m.browse.artists.openTracks(tracks)
			} else {
				m.browse.artists = m.browse.artists.openAlbums(albums)
			}
		}
	case arttui.AlbumLevel:
		album, ok := m.browse.artists.focusedAlbum()
		if ok {
			tracks := arttui.DeriveTracks(allTracks, m.browse.artists.selectedArtist, album)
			m.browse.artists.selectedAlbum = album
			m.browse.artists = m.browse.artists.openTracks(tracks)
		}
	case arttui.TrackLevel:
		if m.queueWriter != nil {
			if cmd := m.browse.artists.enqueueFocusedTrack(m.queueWriter, len(m.queue.entries)); cmd != nil {
				return m, cmd, true
			}
		}
	}
	return m, nil, true
}

func (m Model) handleAlbumOpenAction() (tea.Model, tea.Cmd, bool) {
	allTracks := m.library.Index.Tracks()
	switch m.browse.albums.level {
	case albtui.AlbumLevel:
		album, ok := m.browse.albums.focusedAlbum()
		if ok {
			tracks := albtui.DeriveAlbumTracks(allTracks, album)
			m.browse.albums.selectedAlbum = album
			m.browse.albums = m.browse.albums.openTracks(tracks)
		}
	case albtui.TrackLevel:
		if m.queueWriter != nil {
			if cmd := m.browse.albums.enqueueFocusedTrack(m.queueWriter, len(m.queue.entries)); cmd != nil {
				return m, cmd, true
			}
		}
	}
	return m, nil, true
}

func (m Model) handleGenreOpenAction() (tea.Model, tea.Cmd, bool) {
	allTracks := m.library.Index.Tracks()
	switch m.browse.genres.level {
	case gentui.GenreLevel:
		genre, ok := m.browse.genres.focusedGenre()
		if ok {
			artists := gentui.DeriveGenreArtists(allTracks, genre)
			m.browse.genres.selectedGenre = genre
			if len(artists) == 0 {
				tracks := gentui.DeriveGenreTracks(allTracks, genre)
				m.browse.genres = m.browse.genres.openArtists(nil)
				m.browse.genres = m.browse.genres.openTracks(tracks)
			} else {
				m.browse.genres = m.browse.genres.openArtists(artists)
			}
		}
	case gentui.ArtistLevel:
		artist, ok := m.browse.genres.focusedArtist()
		if ok {
			tracks := gentui.DeriveGenreArtistTracks(allTracks, m.browse.genres.selectedGenre, artist)
			m.browse.genres.selectedArtist = artist
			m.browse.genres = m.browse.genres.openTracks(tracks)
		}
	case gentui.TrackLevel:
		if m.queueWriter != nil {
			if cmd := m.browse.genres.enqueueFocusedTrack(m.queueWriter, len(m.queue.entries)); cmd != nil {
				return m, cmd, true
			}
		}
	}
	return m, nil, true
}
