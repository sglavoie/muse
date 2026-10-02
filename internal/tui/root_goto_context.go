package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/browse"
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
	switch m.ActiveView {
	case viewLibrary:
		if track, ok := m.library.Panel().FocusedTrack(); ok {
			return goToContext{artist: track.Artist, album: track.Album, genre: track.Genre, found: true}
		}

	case viewQueue:
		filtered := m.queue.FilteredEntries()
		if m.queue.Cursor >= 0 && m.queue.Cursor < len(filtered) {
			entry := filtered[m.queue.Cursor]
			ctx := goToContext{artist: entry.Artist, album: entry.Album, sourcePlaylist: entry.SourcePlaylist, found: true}
			if track, ok := m.library.Index.FindByHash(entry.Hash); ok {
				ctx.genre = track.Genre
			}
			return ctx
		}

	case viewBrowse:
		switch m.browse.Mode {
		case browse.BrowseModeArtists:
			switch m.browse.Artists.Level {
			case arttui.ArtistLevel:
				if name, ok := m.browse.Artists.FocusedArtist(); ok {
					ctx := goToContext{artist: name, found: true}
					rawArtist := arttui.RawName(name)
					tracks := arttui.DeriveArtistTracks(m.library.Index.Tracks(), rawArtist)
					if len(tracks) > 0 {
						ctx.genre = tracks[0].Genre
						ctx.album = tracks[0].Album
					}
					return ctx
				}
			case arttui.AlbumLevel:
				if name, ok := m.browse.Artists.FocusedAlbum(); ok {
					ctx := goToContext{artist: m.browse.Artists.SelectedArtist, album: name, found: true}
					rawArtist := arttui.RawName(m.browse.Artists.SelectedArtist)
					rawAlbum := arttui.RawName(name)
					tracks := arttui.DeriveTracks(m.library.Index.Tracks(), rawArtist, rawAlbum)
					if len(tracks) > 0 {
						ctx.genre = tracks[0].Genre
					}
					return ctx
				}
				if m.browse.Artists.SelectedArtist != "" {
					return goToContext{artist: m.browse.Artists.SelectedArtist, found: true}
				}
			case arttui.TrackLevel:
				if track, ok := m.browse.Artists.FocusedTrack(); ok {
					return goToContext{artist: track.Artist, album: track.Album, genre: track.Genre, found: true}
				}
			}
		case browse.BrowseModeAlbums:
			switch m.browse.Albums.Level {
			case albtui.AlbumLevel:
				if name, ok := m.browse.Albums.FocusedAlbum(); ok {
					ctx := goToContext{album: name, found: true}
					rawAlbum := arttui.RawName(name)
					tracks := albtui.DeriveAlbumTracks(m.library.Index.Tracks(), rawAlbum)
					if len(tracks) > 0 {
						ctx.artist = tracks[0].Artist
						ctx.genre = tracks[0].Genre
					}
					return ctx
				}
			case albtui.TrackLevel:
				if track, ok := m.browse.Albums.FocusedTrack(); ok {
					return goToContext{artist: track.Artist, album: track.Album, genre: track.Genre, found: true}
				}
			}
		case browse.BrowseModeGenres:
			switch m.browse.Genres.Level {
			case gentui.GenreLevel:
				if name, ok := m.browse.Genres.FocusedGenre(); ok {
					ctx := goToContext{genre: name, found: true}
					rawGenre := gentui.RawGenre(name)
					tracks := gentui.DeriveGenreTracks(m.library.Index.Tracks(), rawGenre)
					if len(tracks) > 0 {
						ctx.artist = tracks[0].Artist
						ctx.album = tracks[0].Album
					}
					return ctx
				}
			case gentui.ArtistLevel:
				if name, ok := m.browse.Genres.FocusedArtist(); ok {
					ctx := goToContext{artist: name, genre: m.browse.Genres.SelectedGenre, found: true}
					rawArtist := arttui.RawName(name)
					rawGenre := gentui.RawGenre(m.browse.Genres.SelectedGenre)
					tracks := gentui.DeriveGenreArtistTracks(m.library.Index.Tracks(), rawGenre, rawArtist)
					if len(tracks) > 0 {
						ctx.album = tracks[0].Album
					}
					return ctx
				}
			case gentui.TrackLevel:
				if track, ok := m.browse.Genres.FocusedTrack(); ok {
					return goToContext{artist: track.Artist, album: track.Album, genre: track.Genre, found: true}
				}
			}
		}

	case viewPlaylists:
		if track, ok := m.playlists.FocusedTrack(); ok {
			ctx := goToContext{artist: track.Artist, album: track.Album, found: true}
			if libTrack, ok := m.library.Index.FindByHash(track.Hash); ok {
				ctx.genre = libTrack.Genre
			}
			return ctx
		}

	case viewFavorites:
		switch m.favorites.Level {
		case favtui.ListLevel:
			switch m.favorites.Category {
			case favtui.CategoryArtists:
				if name, ok := m.favorites.FocusedArtist(); ok {
					ctx := goToContext{artist: name, found: true}
					rawArtist := arttui.RawName(name)
					tracks := arttui.DeriveArtistTracks(m.library.Index.Tracks(), rawArtist)
					if len(tracks) > 0 {
						ctx.album = tracks[0].Album
						ctx.genre = tracks[0].Genre
					}
					return ctx
				}
			case favtui.CategoryAlbums:
				if name, ok := m.favorites.FocusedAlbum(); ok {
					ctx := goToContext{album: name, found: true}
					rawAlbum := arttui.RawName(name)
					tracks := albtui.DeriveAlbumTracks(m.library.Index.Tracks(), rawAlbum)
					if len(tracks) > 0 {
						ctx.artist = tracks[0].Artist
						ctx.genre = tracks[0].Genre
					}
					return ctx
				}
			case favtui.CategoryTracks:
				if track, ok := m.favorites.FocusedTrack(); ok {
					return goToContext{artist: track.Artist, album: track.Album, genre: track.Genre, found: true}
				}
			}
		case favtui.TrackLevel:
			if track, ok := m.favorites.FocusedTrack(); ok {
				return goToContext{artist: track.Artist, album: track.Album, genre: track.Genre, found: true}
			}
		}

	case viewTags:
		if track, ok := m.tagsView.FocusedTrack(); ok {
			ctx := goToContext{artist: track.Artist, album: track.Album, found: true}
			if libTrack, found := m.library.Index.FindByHash(track.Hash); found {
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

	switch m.ActiveView {
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
		switch m.browse.Mode {
		case browse.BrowseModeArtists:
			switch m.browse.Artists.Level {
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
		case browse.BrowseModeAlbums:
			addArtist()
			addGenres()
		case browse.BrowseModeGenres:
			switch m.browse.Genres.Level {
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
		switch m.favorites.Level {
		case favtui.ListLevel:
			switch m.favorites.Category {
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
		return m.markQuitting(), tea.Quit, true
	}
	updated, cmd := m.overlay.goTo.HandleKey(msg.String())
	m.overlay.goTo = &updated
	return m, cmd, true
}
