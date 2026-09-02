package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	"github.com/sglavoie/muse/internal/tui/overlay"
)

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
// active view for use by the Go To popup.
func (m Model) focusedGoToContext() goToContext {
	switch m.activeView {
	case viewLibrary:
		if track, ok := m.library.view().FocusedTrack(); ok {
			return goToContext{artist: track.Artist, album: track.Album, genre: track.Genre, found: true}
		}

	case viewQueue:
		filtered := m.queue.filteredEntries()
		if m.queue.cursor >= 0 && m.queue.cursor < len(filtered) {
			entry := filtered[m.queue.cursor]
			ctx := goToContext{artist: entry.Artist, album: entry.Album, sourcePlaylist: entry.SourcePlaylist, found: true}
			if track, ok := m.library.index.FindByHash(entry.Hash); ok {
				ctx.genre = track.Genre
			}
			return ctx
		}

	case viewBrowse:
		switch m.browse.mode {
		case BrowseModeArtists:
			switch m.browse.artists.level {
			case arttui.ArtistLevel:
				if name, ok := m.browse.artists.focusedArtist(); ok {
					ctx := goToContext{artist: name, found: true}
					rawArtist := arttui.RawName(name)
					tracks := arttui.DeriveArtistTracks(m.library.index.Tracks(), rawArtist)
					if len(tracks) > 0 {
						ctx.genre = tracks[0].Genre
						ctx.album = tracks[0].Album
					}
					return ctx
				}
			case arttui.AlbumLevel:
				if name, ok := m.browse.artists.focusedAlbum(); ok {
					ctx := goToContext{artist: m.browse.artists.selectedArtist, album: name, found: true}
					rawArtist := arttui.RawName(m.browse.artists.selectedArtist)
					rawAlbum := arttui.RawName(name)
					tracks := arttui.DeriveTracks(m.library.index.Tracks(), rawArtist, rawAlbum)
					if len(tracks) > 0 {
						ctx.genre = tracks[0].Genre
					}
					return ctx
				}
				if m.browse.artists.selectedArtist != "" {
					return goToContext{artist: m.browse.artists.selectedArtist, found: true}
				}
			case arttui.TrackLevel:
				if track, ok := m.browse.artists.focusedTrack(); ok {
					return goToContext{artist: track.Artist, album: track.Album, genre: track.Genre, found: true}
				}
			}
		case BrowseModeAlbums:
			switch m.browse.albums.level {
			case albtui.AlbumLevel:
				if name, ok := m.browse.albums.focusedAlbum(); ok {
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
				if track, ok := m.browse.albums.focusedTrack(); ok {
					return goToContext{artist: track.Artist, album: track.Album, genre: track.Genre, found: true}
				}
			}
		case BrowseModeGenres:
			switch m.browse.genres.level {
			case gentui.GenreLevel:
				if name, ok := m.browse.genres.focusedGenre(); ok {
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
				if name, ok := m.browse.genres.focusedArtist(); ok {
					ctx := goToContext{artist: name, genre: m.browse.genres.selectedGenre, found: true}
					rawArtist := arttui.RawName(name)
					rawGenre := gentui.RawGenre(m.browse.genres.selectedGenre)
					tracks := gentui.DeriveGenreArtistTracks(m.library.index.Tracks(), rawGenre, rawArtist)
					if len(tracks) > 0 {
						ctx.album = tracks[0].Album
					}
					return ctx
				}
			case gentui.TrackLevel:
				if track, ok := m.browse.genres.focusedTrack(); ok {
					return goToContext{artist: track.Artist, album: track.Album, genre: track.Genre, found: true}
				}
			}
		}

	case viewPlaylists:
		if track, ok := m.playlists.focusedTrack(); ok {
			ctx := goToContext{artist: track.Artist, album: track.Album, found: true}
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
					rawArtist := arttui.RawName(name)
					tracks := arttui.DeriveArtistTracks(m.library.index.Tracks(), rawArtist)
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

	case viewTags:
		if track, ok := m.tagsView.focusedTrack(); ok {
			ctx := goToContext{artist: track.Artist, album: track.Album, found: true}
			if libTrack, found := m.library.index.FindByHash(track.Hash); found {
				ctx.genre = libTrack.Genre
			}
			return ctx
		}
	}

	return goToContext{}
}

// buildGoToOptions generates the list of GoToOption for the Go To popup based
// on the gathered context and current view/level. Options pointing to the
// current view position are suppressed.
func (m Model) buildGoToOptions(ctx goToContext) []overlay.GoToOption {
	var opts []overlay.GoToOption

	addArtist := func() {
		if ctx.artist != "" {
			opts = append(opts, overlay.GoToOption{Kind: overlay.GoToArtist, Value: ctx.artist})
		}
	}
	addAlbum := func() {
		if ctx.album != "" {
			opts = append(opts, overlay.GoToOption{Kind: overlay.GoToAlbum, Value: ctx.album})
		}
	}
	addGenres := func() {
		genres := library.SplitGenres(ctx.genre)
		for _, genre := range genres {
			opts = append(opts, overlay.GoToOption{Kind: overlay.GoToGenre, Value: genre})
		}
	}
	addSource := func() {
		if ctx.sourcePlaylist != "" {
			opts = append(opts, overlay.GoToOption{Kind: overlay.GoToPlaylist, Value: ctx.sourcePlaylist})
		}
	}

	switch m.activeView {
	case viewLibrary:
		addAlbum()
		addArtist()
		addGenres()

	case viewQueue:
		addAlbum()
		addArtist()
		addGenres()
		addSource()

	case viewBrowse:
		switch m.browse.mode {
		case BrowseModeArtists:
			switch m.browse.artists.level {
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
		case BrowseModeAlbums:
			addArtist()
			addGenres()
		case BrowseModeGenres:
			switch m.browse.genres.level {
			case gentui.GenreLevel:
				addArtist()
				addAlbum()
			case gentui.ArtistLevel:
				addAlbum()
			case gentui.TrackLevel:
				addAlbum()
				addArtist()
			}
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

	case viewTags:
		addAlbum()
		addArtist()
		addGenres()
	}

	return opts
}

// handleGoToKeyPress intercepts key presses when the Go To popup is active.
func (m Model) handleGoToKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.overlay.goTo == nil {
		return m, nil, false
	}
	if msg.String() == "ctrl+c" {
		return m, tea.Quit, true
	}
	updated, cmd := m.overlay.goTo.HandleKey(msg.String())
	m.overlay.goTo = &updated
	return m, cmd, true
}
