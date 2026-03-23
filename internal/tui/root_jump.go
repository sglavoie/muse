package tui

import (
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
)

// focusedTrackMeta returns the artist and album of the focused item in the
// active view. Either value may be empty if no meaningful context exists
// (e.g., Artists view at ArtistLevel has no album context).
func (m Model) focusedTrackMeta() (artist, album string) {
	switch m.activeView {
	case viewLibrary:
		if track, ok := m.library.view().FocusedTrack(); ok {
			return track.Artist, track.Album
		}

	case viewQueue:
		if m.queue.cursor >= 0 && m.queue.cursor < len(m.queue.filteredEntries()) {
			filtered := m.queue.filteredEntries()
			e := filtered[m.queue.cursor]
			return e.Artist, e.Album
		}

	case viewArtists:
		switch m.artists.level {
		case arttui.ArtistLevel:
			if name, ok := m.artists.focusedArtist(); ok {
				return name, ""
			}
		case arttui.AlbumLevel:
			album := ""
			if name, ok := m.artists.focusedAlbum(); ok {
				album = name
			}
			return m.artists.selectedArtist, album
		case arttui.TrackLevel:
			if track, ok := m.artists.focusedTrack(); ok {
				return track.Artist, track.Album
			}
		}

	case viewAlbums:
		switch m.albums.level {
		case albtui.AlbumLevel:
			if name, ok := m.albums.focusedAlbum(); ok {
				return "", name
			}
		case albtui.TrackLevel:
			if track, ok := m.albums.focusedTrack(); ok {
				return track.Artist, track.Album
			}
		}

	case viewPlaylists:
		if track, ok := m.playlists.focusedTrack(); ok {
			return track.Artist, track.Album
		}

	case viewFavorites:
		switch m.favorites.level {
		case favtui.ListLevel:
			switch m.favorites.category {
			case favtui.CategoryArtists:
				if name, ok := m.favorites.focusedArtist(); ok {
					return name, ""
				}
			case favtui.CategoryAlbums:
				if name, ok := m.favorites.focusedAlbum(); ok {
					return "", name
				}
			case favtui.CategoryTracks:
				if track, ok := m.favorites.focusedTrack(); ok {
					return track.Artist, track.Album
				}
			}
		case favtui.TrackLevel:
			if track, ok := m.favorites.focusedTrack(); ok {
				return track.Artist, track.Album
			}
		}
	}
	return "", ""
}

// jumpToArtist switches to the Artists view with the cursor pre-positioned on
// artistName. artist may be an empty string, in which case it is matched
// against the UnknownArtist sentinel in the derived list.
func (m Model) jumpToArtist(artistName string) Model {
	artists := arttui.DeriveArtists(m.library.index.Tracks())

	// Map empty raw name to the display sentinel.
	target := artistName
	if target == "" {
		target = arttui.UnknownArtist
	}

	idx := -1
	for i, a := range artists {
		if a == target {
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

	// Map empty raw name to the display sentinel.
	target := albumName
	if target == "" {
		target = arttui.UnknownAlbum
	}

	idx := -1
	for i, a := range albums {
		if a == target {
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
