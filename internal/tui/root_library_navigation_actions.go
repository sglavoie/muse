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
		m.browse.Artists = m.browse.Artists.Back()
		return m, nil, true
	case keys.ActionAlbumOpen:
		return m.handleAlbumOpenAction()
	case keys.ActionAlbumBack:
		m = exitVisualMode(m)
		m.browse.Albums = m.browse.Albums.Back()
		return m, nil, true
	case keys.ActionGenreOpen:
		return m.handleGenreOpenAction()
	case keys.ActionGenreBack:
		m = exitVisualMode(m)
		m.browse.Genres = m.browse.Genres.Back()
		return m, nil, true
	default:
		return m, nil, false
	}
}

func (m Model) handleArtistOpenAction() (tea.Model, tea.Cmd, bool) {
	allTracks := m.library.Index.Tracks()
	switch m.browse.Artists.Level {
	case arttui.ArtistLevel:
		artist, ok := m.browse.Artists.FocusedArtist()
		if ok {
			albums := arttui.DeriveAlbums(allTracks, artist)
			m.browse.Artists.SelectedArtist = artist
			if len(albums) == 0 {
				tracks := arttui.DeriveArtistTracks(allTracks, artist)
				m.browse.Artists.SelectedAlbum = ""
				m.browse.Artists = m.browse.Artists.OpenAlbums(nil)
				m.browse.Artists = m.browse.Artists.OpenTracks(tracks)
			} else {
				m.browse.Artists = m.browse.Artists.OpenAlbums(albums)
			}
		}
	case arttui.AlbumLevel:
		album, ok := m.browse.Artists.FocusedAlbum()
		if ok {
			tracks := arttui.DeriveTracks(allTracks, m.browse.Artists.SelectedArtist, album)
			m.browse.Artists.SelectedAlbum = album
			m.browse.Artists = m.browse.Artists.OpenTracks(tracks)
		}
	case arttui.TrackLevel:
		if m.queueWriter != nil {
			if cmd := m.browse.Artists.EnqueueFocusedTrack(m.queueWriter, len(m.queue.Entries)); cmd != nil {
				return m, cmd, true
			}
		}
	}
	return m, nil, true
}

func (m Model) handleAlbumOpenAction() (tea.Model, tea.Cmd, bool) {
	allTracks := m.library.Index.Tracks()
	switch m.browse.Albums.Level {
	case albtui.AlbumLevel:
		album, ok := m.browse.Albums.FocusedAlbum()
		if ok {
			tracks := albtui.DeriveAlbumTracks(allTracks, album)
			m.browse.Albums.SelectedAlbum = album
			m.browse.Albums = m.browse.Albums.OpenTracks(tracks)
		}
	case albtui.TrackLevel:
		if m.queueWriter != nil {
			if cmd := m.browse.Albums.EnqueueFocusedTrack(m.queueWriter, len(m.queue.Entries)); cmd != nil {
				return m, cmd, true
			}
		}
	}
	return m, nil, true
}

func (m Model) handleGenreOpenAction() (tea.Model, tea.Cmd, bool) {
	allTracks := m.library.Index.Tracks()
	switch m.browse.Genres.Level {
	case gentui.GenreLevel:
		genre, ok := m.browse.Genres.FocusedGenre()
		if ok {
			artists := gentui.DeriveGenreArtists(allTracks, genre)
			m.browse.Genres.SelectedGenre = genre
			if len(artists) == 0 {
				tracks := gentui.DeriveGenreTracks(allTracks, genre)
				m.browse.Genres = m.browse.Genres.OpenArtists(nil)
				m.browse.Genres = m.browse.Genres.OpenTracks(tracks)
			} else {
				m.browse.Genres = m.browse.Genres.OpenArtists(artists)
			}
		}
	case gentui.ArtistLevel:
		artist, ok := m.browse.Genres.FocusedArtist()
		if ok {
			tracks := gentui.DeriveGenreArtistTracks(allTracks, m.browse.Genres.SelectedGenre, artist)
			m.browse.Genres.SelectedArtist = artist
			m.browse.Genres = m.browse.Genres.OpenTracks(tracks)
		}
	case gentui.TrackLevel:
		if m.queueWriter != nil {
			if cmd := m.browse.Genres.EnqueueFocusedTrack(m.queueWriter, len(m.queue.Entries)); cmd != nil {
				return m, cmd, true
			}
		}
	}
	return m, nil, true
}
