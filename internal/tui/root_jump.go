package tui

import (
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

// focusedTrackMeta returns the artist and album of the focused item in the
// active view, plus a bool indicating whether any item is focused.  Either
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
			// No focused album (empty list) — still report the artist context.
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
	}
	return "", "", false
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
	for i, g := range genres {
		if g == target {
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
	pl := m.playlists.effectivePlaylists()
	idx := -1
	for i, p := range pl {
		if p.Name == playlistName {
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

// goToContext holds the metadata extracted from the focused item for the Go To
// popup. Fields may be empty when not applicable.
type goToContext struct {
	artist         string
	album          string
	genre          string // raw genre field (may contain multiple comma-separated genres)
	sourcePlaylist string // only non-empty for Queue entries enqueued from a playlist
	found          bool
}

// focusedGoToContext extracts the full metadata from the focused item in the
// active view for use by the Go To popup. It extends focusedTrackMeta with
// genre and source-playlist data.
func (m Model) focusedGoToContext() goToContext {
	switch m.activeView {
	case viewLibrary:
		if track, ok := m.library.view().FocusedTrack(); ok {
			return goToContext{artist: track.Artist, album: track.Album, genre: track.Genre, found: true}
		}

	case viewRecentlyAdded:
		if track, ok := m.recentlyAdded.view().FocusedTrack(); ok {
			return goToContext{artist: track.Artist, album: track.Album, genre: track.Genre, found: true}
		}

	case viewHistory:
		if track, ok := m.history.view().FocusedTrack(); ok {
			return goToContext{artist: track.Artist, album: track.Album, genre: track.Genre, found: true}
		}

	case viewQueue:
		filtered := m.queue.filteredEntries()
		if m.queue.cursor >= 0 && m.queue.cursor < len(filtered) {
			e := filtered[m.queue.cursor]
			ctx := goToContext{artist: e.Artist, album: e.Album, sourcePlaylist: e.SourcePlaylist, found: true}
			// Queue entries lack genre — look it up from the library.
			if track, ok := m.library.index.FindByHash(e.Hash); ok {
				ctx.genre = track.Genre
			}
			return ctx
		}

	case viewArtists:
		switch m.artists.level {
		case arttui.ArtistLevel:
			if name, ok := m.artists.focusedArtist(); ok {
				ctx := goToContext{artist: name, found: true}
				// Get genre from first track by this artist.
				raw := arttui.RawName(name)
				tracks := arttui.DeriveArtistTracks(m.library.index.Tracks(), raw)
				if len(tracks) > 0 {
					ctx.genre = tracks[0].Genre
					ctx.album = tracks[0].Album
				}
				return ctx
			}
		case arttui.AlbumLevel:
			if name, ok := m.artists.focusedAlbum(); ok {
				ctx := goToContext{artist: m.artists.selectedArtist, album: name, found: true}
				rawArtist := arttui.RawName(m.artists.selectedArtist)
				rawAlbum := arttui.RawName(name)
				tracks := arttui.DeriveTracks(m.library.index.Tracks(), rawArtist, rawAlbum)
				if len(tracks) > 0 {
					ctx.genre = tracks[0].Genre
				}
				return ctx
			}
			if m.artists.selectedArtist != "" {
				return goToContext{artist: m.artists.selectedArtist, found: true}
			}
		case arttui.TrackLevel:
			if track, ok := m.artists.focusedTrack(); ok {
				return goToContext{artist: track.Artist, album: track.Album, genre: track.Genre, found: true}
			}
		}

	case viewAlbums:
		switch m.albums.level {
		case albtui.AlbumLevel:
			if name, ok := m.albums.focusedAlbum(); ok {
				ctx := goToContext{album: name, found: true}
				rawAlbum := arttui.RawName(name)
				tracks := albtui.DeriveAlbumTracks(m.library.index.Tracks(), rawAlbum)
				if len(tracks) > 0 {
					ctx.artist = tracks[0].Artist
					ctx.genre = tracks[0].Genre
				}
				return ctx
			}
		case albtui.TrackLevel:
			if track, ok := m.albums.focusedTrack(); ok {
				return goToContext{artist: track.Artist, album: track.Album, genre: track.Genre, found: true}
			}
		}

	case viewPlaylists:
		if track, ok := m.playlists.focusedTrack(); ok {
			ctx := goToContext{artist: track.Artist, album: track.Album, found: true}
			// Playlist tracks lack genre — look it up from the library.
			if libTrack, ok := m.library.index.FindByHash(track.Hash); ok {
				ctx.genre = libTrack.Genre
			}
			return ctx
		}

	case viewFavorites:
		switch m.favorites.level {
		case favtui.ListLevel:
			switch m.favorites.category {
			case favtui.CategoryArtists:
				if name, ok := m.favorites.focusedArtist(); ok {
					ctx := goToContext{artist: name, found: true}
					raw := arttui.RawName(name)
					tracks := arttui.DeriveArtistTracks(m.library.index.Tracks(), raw)
					if len(tracks) > 0 {
						ctx.album = tracks[0].Album
						ctx.genre = tracks[0].Genre
					}
					return ctx
				}
			case favtui.CategoryAlbums:
				if name, ok := m.favorites.focusedAlbum(); ok {
					ctx := goToContext{album: name, found: true}
					rawAlbum := arttui.RawName(name)
					tracks := albtui.DeriveAlbumTracks(m.library.index.Tracks(), rawAlbum)
					if len(tracks) > 0 {
						ctx.artist = tracks[0].Artist
						ctx.genre = tracks[0].Genre
					}
					return ctx
				}
			case favtui.CategoryTracks:
				if track, ok := m.favorites.focusedTrack(); ok {
					return goToContext{artist: track.Artist, album: track.Album, genre: track.Genre, found: true}
				}
			}
		case favtui.TrackLevel:
			if track, ok := m.favorites.focusedTrack(); ok {
				return goToContext{artist: track.Artist, album: track.Album, genre: track.Genre, found: true}
			}
		}

	case viewGenres:
		switch m.genres.level {
		case gentui.GenreLevel:
			if name, ok := m.genres.focusedGenre(); ok {
				ctx := goToContext{genre: name, found: true}
				rawGenre := gentui.RawGenre(name)
				tracks := gentui.DeriveGenreTracks(m.library.index.Tracks(), rawGenre)
				if len(tracks) > 0 {
					ctx.artist = tracks[0].Artist
					ctx.album = tracks[0].Album
				}
				return ctx
			}
		case gentui.ArtistLevel:
			if name, ok := m.genres.focusedArtist(); ok {
				ctx := goToContext{artist: name, genre: m.genres.selectedGenre, found: true}
				rawArtist := arttui.RawName(name)
				rawGenre := gentui.RawGenre(m.genres.selectedGenre)
				tracks := gentui.DeriveGenreArtistTracks(m.library.index.Tracks(), rawGenre, rawArtist)
				if len(tracks) > 0 {
					ctx.album = tracks[0].Album
				}
				return ctx
			}
		case gentui.TrackLevel:
			if track, ok := m.genres.focusedTrack(); ok {
				return goToContext{artist: track.Artist, album: track.Album, genre: track.Genre, found: true}
			}
		}
	}

	return goToContext{}
}

// buildGoToOptions generates the list of GoToOption for the Go To popup based
// on the gathered context and current view/level. Options pointing to the
// current view position are suppressed.
func (m Model) buildGoToOptions(ctx goToContext) []GoToOption {
	var opts []GoToOption

	addArtist := func() {
		if ctx.artist != "" {
			opts = append(opts, GoToOption{Kind: goToArtist, Value: ctx.artist})
		}
	}
	addAlbum := func() {
		if ctx.album != "" {
			opts = append(opts, GoToOption{Kind: goToAlbum, Value: ctx.album})
		}
	}
	addGenres := func() {
		genres := library.SplitGenres(ctx.genre)
		for _, g := range genres {
			opts = append(opts, GoToOption{Kind: goToGenre, Value: g})
		}
	}
	addSource := func() {
		if ctx.sourcePlaylist != "" {
			opts = append(opts, GoToOption{Kind: goToPlaylist, Value: ctx.sourcePlaylist})
		}
	}

	switch m.activeView {
	case viewLibrary, viewRecentlyAdded, viewHistory:
		addAlbum()
		addArtist()
		addGenres()

	case viewQueue:
		addAlbum()
		addArtist()
		addGenres()
		addSource()

	case viewArtists:
		switch m.artists.level {
		case arttui.ArtistLevel:
			addAlbum()
			addGenres()
		case arttui.AlbumLevel:
			addArtist()
			addGenres()
		case arttui.TrackLevel:
			addAlbum()
			addGenres()
		}

	case viewAlbums:
		addArtist()
		addGenres()

	case viewGenres:
		switch m.genres.level {
		case gentui.GenreLevel:
			addArtist()
			addAlbum()
		case gentui.ArtistLevel:
			addAlbum()
		case gentui.TrackLevel:
			addAlbum()
			addArtist()
		}

	case viewPlaylists:
		addAlbum()
		addArtist()
		addGenres()

	case viewFavorites:
		switch m.favorites.level {
		case favtui.ListLevel:
			switch m.favorites.category {
			case favtui.CategoryArtists:
				addAlbum()
				addGenres()
			case favtui.CategoryAlbums:
				addArtist()
				addGenres()
			case favtui.CategoryTracks:
				addAlbum()
				addArtist()
				addGenres()
			}
		case favtui.TrackLevel:
			addAlbum()
			addArtist()
			addGenres()
		}
	}

	return opts
}
