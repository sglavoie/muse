package tui

import (
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

// jumpToArtist switches to the Artists view with the cursor pre-positioned on
// artistName. artist may be an empty string, in which case it is matched
// against the UnknownArtist sentinel in the derived list.
func (m Model) jumpToArtist(artistName string) Model {
	artists := arttui.DeriveArtists(m.library.index.Tracks())

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

	scrollOffset := max(0, idx-m.artists.height/2)

	m.artists = ArtistsView{
		level:        arttui.ArtistLevel,
		cursor:       idx,
		scrollOffset: scrollOffset,
		artists:      artists,
		width:        m.artists.width,
		height:       m.artists.height,
	}
	m.clearPendingConfirmations()
	m = m.resetAllSorts()
	m.activeView = viewArtists
	return m
}

// jumpToAlbum switches to the Albums view with the cursor pre-positioned on
// albumName. albumName may be an empty string, in which case it is matched
// against the UnknownAlbum sentinel in the derived list.
func (m Model) jumpToAlbum(albumName string) Model {
	albums := albtui.DeriveAllAlbums(m.library.index.Tracks())

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

	scrollOffset := max(0, idx-m.albums.height/2)

	m.albums = AlbumsView{
		level:        albtui.AlbumLevel,
		cursor:       idx,
		scrollOffset: scrollOffset,
		albums:       albums,
		width:        m.albums.width,
		height:       m.albums.height,
	}
	m.clearPendingConfirmations()
	m = m.resetAllSorts()
	m.activeView = viewAlbums
	return m
}

// jumpToGenre switches to the Genres view with the cursor pre-positioned on
// genreName. genreName may be an empty string, in which case it is matched
// against the UnknownGenre sentinel in the derived list.
func (m Model) jumpToGenre(genreName string) Model {
	genres := gentui.DeriveGenres(m.library.index.Tracks())

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

	scrollOffset := max(0, idx-m.genres.height/2)

	m.genres = GenresView{
		level:        gentui.GenreLevel,
		cursor:       idx,
		scrollOffset: scrollOffset,
		genres:       genres,
		width:        m.genres.width,
		height:       m.genres.height,
	}
	m.clearPendingConfirmations()
	m = m.resetAllSorts()
	m.activeView = viewGenres
	return m
}

// jumpToPlaylist switches to the Playlists view with the cursor
// pre-positioned on the named playlist at ListLevel.
func (m Model) jumpToPlaylist(playlistName string) Model {
	playlists := m.playlists.effectivePlaylists()
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

	scrollOffset := max(0, idx-m.playlists.height/2)
	m.playlists.level = pltui.ListLevel
	m.playlists.cursor = idx
	m.playlists.scrollOffset = scrollOffset
	m.playlists.tracks = nil
	m.playlists.sortedPlaylists = nil
	m.playlists.sortedTrackOrder = nil
	m.clearPendingConfirmations()
	m = m.resetAllSorts()
	m.activeView = viewPlaylists
	return m
}
