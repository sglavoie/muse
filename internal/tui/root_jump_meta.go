package tui

import (
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// focusedTrackMeta returns the artist and album of the focused item in the
// active view, plus a bool indicating whether any item is focused. Either
// value may be empty if no meaningful context exists (e.g., Artists view at
// ArtistLevel has no album context), but found will be true as long as an
// item exists under the cursor.
func (m Model) focusedTrackMeta() (artist, album string, found bool) {
	switch m.activeView {
	case viewLibrary:
		if track, ok := m.library.view().FocusedTrack(); ok {
			return track.Artist, track.Album, true
		}

	case viewRecentlyAdded:
		if track, ok := m.recentlyAdded.view().FocusedTrack(); ok {
			return track.Artist, track.Album, true
		}

	case viewHistory:
		if track, ok := m.history.view().FocusedTrack(); ok {
			return track.Artist, track.Album, true
		}

	case viewQueue:
		filtered := m.queue.filteredEntries()
		if m.queue.cursor >= 0 && m.queue.cursor < len(filtered) {
			e := filtered[m.queue.cursor]
			return e.Artist, e.Album, true
		}

	case viewArtists:
		switch m.artists.level {
		case arttui.ArtistLevel:
			if name, ok := m.artists.focusedArtist(); ok {
				return name, "", true
			}
		case arttui.AlbumLevel:
			if name, ok := m.artists.focusedAlbum(); ok {
				return m.artists.selectedArtist, name, true
			}
			if m.artists.selectedArtist != "" {
				return m.artists.selectedArtist, "", true
			}
		case arttui.TrackLevel:
			if track, ok := m.artists.focusedTrack(); ok {
				return track.Artist, track.Album, true
			}
		}

	case viewAlbums:
		switch m.albums.level {
		case albtui.AlbumLevel:
			if name, ok := m.albums.focusedAlbum(); ok {
				return "", name, true
			}
		case albtui.TrackLevel:
			if track, ok := m.albums.focusedTrack(); ok {
				return track.Artist, track.Album, true
			}
		}

	case viewPlaylists:
		if track, ok := m.playlists.focusedTrack(); ok {
			return track.Artist, track.Album, true
		}

	case viewFavorites:
		switch m.favorites.level {
		case favtui.ListLevel:
			switch m.favorites.category {
			case favtui.CategoryArtists:
				if name, ok := m.favorites.focusedArtist(); ok {
					return name, "", true
				}
			case favtui.CategoryAlbums:
				if name, ok := m.favorites.focusedAlbum(); ok {
					return "", name, true
				}
			case favtui.CategoryTracks:
				if track, ok := m.favorites.focusedTrack(); ok {
					return track.Artist, track.Album, true
				}
			}
		case favtui.TrackLevel:
			if track, ok := m.favorites.focusedTrack(); ok {
				return track.Artist, track.Album, true
			}
		}

	case viewGenres:
		switch m.genres.level {
		case gentui.ArtistLevel:
			if name, ok := m.genres.focusedArtist(); ok {
				return name, "", true
			}
		case gentui.TrackLevel:
			if track, ok := m.genres.focusedTrack(); ok {
				return track.Artist, track.Album, true
			}
		}

	case viewTags:
		if m.tagsView.level == tagtui.TrackLevel {
			if track, ok := m.tagsView.focusedTrack(); ok {
				return track.Artist, track.Album, true
			}
		}
	}

	return "", "", false
}
