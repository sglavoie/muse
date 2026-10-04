package tui

import (
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/browse"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

// jumpToArtist switches to the Artists view with the cursor pre-positioned on
// artistName. artist may be an empty string, in which case it is matched
// against the UnknownArtist sentinel in the derived list.
func (m Model) jumpToArtist(artistName string) Model {
	artists := arttui.DeriveArtists(m.library.Index.Tracks())

	target := artistName
	if target == "" {
		target = arttui.UnknownArtist
	}

	idx := -1
	for i, artist := range artists {
		if artist == target {
			idx = i
			break
		}
	}
	if idx < 0 {
		return m
	}

	scrollOffset := max(0, idx-m.browse.Artists.Height/2)

	m.browse.Artists = browse.ArtistsView{
		Level:        arttui.ArtistLevel,
		Cursor:       idx,
		ScrollOffset: scrollOffset,
		Artists:      artists,
		Width:        m.browse.Artists.Width,
		Height:       m.browse.Artists.Height,
	}
	m.clearPendingConfirmations()
	m = exitVisualMode(m)
	m.browse.Mode = browse.BrowseModeArtists
	m.ActiveView = viewBrowse
	return m
}

// jumpToAlbum switches to the Albums view with the cursor pre-positioned on
// albumName. albumName may be an empty string, in which case it is matched
// against the UnknownAlbum sentinel in the derived list.
func (m Model) jumpToAlbum(albumName string) Model {
	albums := albtui.DeriveAllAlbums(m.library.Index.Tracks())

	target := albumName
	if target == "" {
		target = arttui.UnknownAlbum
	}

	idx := -1
	for i, album := range albums {
		if album == target {
			idx = i
			break
		}
	}
	if idx < 0 {
		return m
	}

	scrollOffset := max(0, idx-m.browse.Albums.Height/2)

	m.browse.Albums = browse.AlbumsView{
		Level:        albtui.AlbumLevel,
		Cursor:       idx,
		ScrollOffset: scrollOffset,
		Albums:       albums,
		Width:        m.browse.Albums.Width,
		Height:       m.browse.Albums.Height,
	}
	m.clearPendingConfirmations()
	m = exitVisualMode(m)
	m.browse.Mode = browse.BrowseModeAlbums
	m.ActiveView = viewBrowse
	return m
}

// jumpToGenre switches to the Genres view with the cursor pre-positioned on
// genreName. genreName may be an empty string, in which case it is matched
// against the UnknownGenre sentinel in the derived list.
func (m Model) jumpToGenre(genreName string) Model {
	genres := gentui.DeriveGenres(m.library.Index.Tracks())

	target := genreName
	if target == "" {
		target = gentui.UnknownGenre
	}

	idx := -1
	for i, genre := range genres {
		if genre == target {
			idx = i
			break
		}
	}
	if idx < 0 {
		return m
	}

	scrollOffset := max(0, idx-m.browse.Genres.Height/2)

	m.browse.Genres = browse.GenresView{
		Level:        gentui.GenreLevel,
		Cursor:       idx,
		ScrollOffset: scrollOffset,
		Genres:       genres,
		Width:        m.browse.Genres.Width,
		Height:       m.browse.Genres.Height,
	}
	m.clearPendingConfirmations()
	m = exitVisualMode(m)
	m.browse.Mode = browse.BrowseModeGenres
	m.ActiveView = viewBrowse
	return m
}

// jumpToPlaylist switches to the Playlists view with the cursor
// pre-positioned on the named playlist at ListLevel.
func (m Model) jumpToPlaylist(playlistName string) Model {
	playlists := m.playlists.Playlists
	idx := -1
	for i, playlist := range playlists {
		if playlist.Name == playlistName {
			idx = i
			break
		}
	}
	if idx < 0 {
		return m
	}

	scrollOffset := max(0, idx-m.playlists.Height/2)
	m.playlists = m.playlists.BackToList()
	m.playlists.Filter = m.playlists.Filter.Deactivate()
	m.playlists.Level = pltui.ListLevel
	m.playlists.Cursor = idx
	m.playlists.ScrollOffset = scrollOffset
	m.playlists.Tracks = nil
	m = m.resetPlaylistsSort()
	m.clearPendingConfirmations()
	m = exitVisualMode(m)
	m.ActiveView = viewPlaylists
	return m
}
