package tui

import (
	musiclib "github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// bulkSelectedHashes returns the hashes of all bulk-selected tracks for the
// active view. Returns nil if there is no bulk selection or the view doesn't
// support it.
func (m Model) bulkSelectedHashes() []string {
	switch m.activeView {
	case viewLibrary:
		return hashMapToSlice(m.library.SelectedHashes)
	case viewQueue:
		return m.queue.selectedHashes()
	case viewPlaylists:
		if len(m.playlists.SelectedTrackIDs) == 0 {
			return nil
		}
		hashes := make([]string, 0, len(m.playlists.SelectedTrackIDs))
		for _, t := range m.playlists.Tracks {
			if _, ok := m.playlists.SelectedTrackIDs[t.ID]; ok {
				hashes = append(hashes, t.Hash)
			}
		}
		return hashes
	case viewBrowse:
		switch m.browse.mode {
		case BrowseModeArtists:
			return hashMapToSlice(m.browse.artists.selectedHashes)
		case BrowseModeAlbums:
			return hashMapToSlice(m.browse.albums.selectedHashes)
		case BrowseModeGenres:
			return hashMapToSlice(m.browse.genres.selectedHashes)
		}
		return nil
	case viewFavorites:
		if len(m.favorites.selectedItems) == 0 {
			return nil
		}
		var items []musiclib.Track
		switch {
		case m.favorites.level == favtui.ListLevel && m.favorites.category == favtui.CategoryTracks:
			items = arttui.FilterTracks(m.favorites.tracks, m.favorites.filter.Text)
		case m.favorites.level == favtui.TrackLevel:
			items = arttui.FilterTracks(m.favorites.drilldownTracks, m.favorites.filter.Text)
		default:
			return nil
		}
		var hashes []string
		for _, idx := range sortedIntSetKeys(m.favorites.selectedItems) {
			if idx >= 0 && idx < len(items) {
				hashes = append(hashes, items[idx].Hash)
			}
		}
		if len(hashes) == 0 {
			return nil
		}
		return hashes
	case viewTags:
		return hashMapToSlice(m.tagsView.selectedHashes)
	case viewMixes:
		if m.mixes.level == 1 {
			return hashMapToSlice(m.mixes.trackView.SelectedHashes)
		}
		return nil
	case viewInsights:
		if m.insights.atTracks() {
			return hashMapToSlice(m.insights.trackView.SelectedHashes)
		}
		return nil
	default:
		return nil
	}
}

// commonValue returns the shared value if all strings are equal, or empty string otherwise.
func commonValue(vals []string) string {
	if len(vals) == 0 {
		return ""
	}
	first := vals[0]
	for _, v := range vals[1:] {
		if v != first {
			return ""
		}
	}
	return first
}

// syncStatusBarMetadata updates the status bar artist and title when the
// currently-playing track (identified by hash) has been edited. This ensures
// the persistent left zone reflects renamed metadata immediately.
func (m Model) syncStatusBarMetadata(hash, artist, title string) Model {
	if m.playback.currentHash != "" && m.playback.currentHash == hash {
		m.statusBar.Artist = artist
		m.statusBar.Title = title
	}
	return m
}

// refreshDerivedViews updates the artists and albums lists derived from the
// current library index. Call after any metadata change that may have renamed
// artists, albums, or titles. Also rebuilds the library sort/filter caches so
// that active sort orders and filter matches reflect the updated metadata.
func (m Model) refreshDerivedViews() Model {
	tracks := m.library.Index.Tracks()

	m.library = m.library.RefreshView()

	switch m.browse.artists.level {
	case arttui.ArtistLevel:
		m.browse.artists.artists = arttui.DeriveArtists(tracks)
		m.browse.artists.artistCounts = arttui.DeriveArtistCounts(tracks)
		m.browse.artists = m.browse.artists.recomputeArtistsSort()
		m.browse.artists = m.browse.artists.clampCursor()
	case arttui.AlbumLevel:
		m.browse.artists.albums = arttui.DeriveAlbums(tracks, m.browse.artists.selectedArtist)
		m.browse.artists = m.browse.artists.recomputeArtistsSort()
		m.browse.artists = m.browse.artists.clampCursor()
	case arttui.TrackLevel:
		m.browse.artists.tracks = arttui.DeriveTracks(tracks, m.browse.artists.selectedArtist, m.browse.artists.selectedAlbum)
		m.browse.artists = m.browse.artists.recomputeArtistsSort()
		m.browse.artists = m.browse.artists.clampCursor()
	}

	switch m.browse.albums.level {
	case albtui.AlbumLevel:
		m.browse.albums.albums = albtui.DeriveAllAlbums(tracks)
		m.browse.albums.albumCounts = albtui.DeriveAllAlbumCounts(tracks)
		m.browse.albums = m.browse.albums.recomputeAlbumsSort()
		m.browse.albums = m.browse.albums.clampCursor()
	case albtui.TrackLevel:
		m.browse.albums.tracks = albtui.DeriveAlbumTracks(tracks, m.browse.albums.selectedAlbum)
		m.browse.albums = m.browse.albums.recomputeAlbumsSort()
		m.browse.albums = m.browse.albums.clampCursor()
	}

	switch m.browse.genres.level {
	case gentui.GenreLevel:
		m.browse.genres.genres = gentui.DeriveGenres(tracks)
		m.browse.genres.genreCounts = gentui.DeriveGenreCounts(tracks)
		m.browse.genres = m.browse.genres.recomputeGenresSort()
		m.browse.genres = m.browse.genres.clampCursor()
	case gentui.ArtistLevel:
		m.browse.genres.artists = gentui.DeriveGenreArtists(tracks, m.browse.genres.selectedGenre)
		m.browse.genres = m.browse.genres.recomputeGenresSort()
		m.browse.genres = m.browse.genres.clampCursor()
	case gentui.TrackLevel:
		m.browse.genres.tracks = gentui.DeriveGenreArtistTracks(tracks, m.browse.genres.selectedGenre, m.browse.genres.selectedArtist)
		m.browse.genres = m.browse.genres.recomputeGenresSort()
		m.browse.genres = m.browse.genres.clampCursor()
	}

	if m.playlists.Level == pltui.TrackLevel {
		for i := range m.playlists.Tracks {
			if t, ok := m.library.Index.FindByHash(m.playlists.Tracks[i].Hash); ok {
				m.playlists.Tracks[i].Title = t.Title
				m.playlists.Tracks[i].Artist = t.Artist
				m.playlists.Tracks[i].Album = t.Album
				m.playlists.Tracks[i].DurationMs = int64(t.Duration * 1000)
			}
		}
		m.playlists = m.playlists.RecomputePlaylistSort()
	}

	refreshTrackMetadata := func(ts []musiclib.Track) {
		for i := range ts {
			if t, ok := m.library.Index.FindByHash(ts[i].Hash); ok {
				ts[i].Title = t.Title
				ts[i].Artist = t.Artist
				ts[i].Album = t.Album
				ts[i].Genre = t.Genre
				ts[i].Duration = t.Duration
				ts[i].TrackNumber = t.TrackNumber
				ts[i].DiscNumber = t.DiscNumber
			}
		}
	}

	if m.library.Mode != libtui.LibraryModeAll && len(m.library.SourceTracks) > 0 {
		refreshTrackMetadata(m.library.SourceTracks)
		m.library = m.library.RefreshView()
	}

	if m.tagsView.level == tagtui.TrackLevel {
		for i := range m.tagsView.tracks {
			if t, ok := m.library.Index.FindByHash(m.tagsView.tracks[i].Hash); ok {
				m.tagsView.tracks[i].Title = t.Title
				m.tagsView.tracks[i].Artist = t.Artist
				m.tagsView.tracks[i].Album = t.Album
				m.tagsView.tracks[i].DurationMs = int64(t.Duration * 1000)
			}
		}
		m.tagsView = m.tagsView.recomputeTagsSort()
	}

	if m.favorites.level == favtui.ListLevel && m.favorites.category == favtui.CategoryTracks {
		refreshTrackMetadata(m.favorites.tracks)
		m.favorites = m.favorites.recomputeFavoritesSort()
	}
	if m.favorites.level == favtui.TrackLevel {
		refreshTrackMetadata(m.favorites.drilldownTracks)
		m.favorites = m.favorites.recomputeFavoritesSort()
	}

	// Refresh metadata for every cached mix, not just the active one — when
	// the user navigates to a previously-loaded mix the cached slice is
	// reused as-is, so stale title/artist/album/duration would leak in.
	for id, cached := range m.mixes.cachedMixes {
		refreshTrackMetadata(cached)
		m.mixes.cachedMixes[id] = cached
	}
	if m.mixes.level == 1 {
		m.mixes.trackView = m.mixes.trackView.RefreshView()
	}

	// An open Insights drill holds a frozen snapshot taken by openDrill, so
	// without this the list the export/import loop is aimed at keeps rendering
	// pre-edit values until the drill is reopened.
	if m.insights.atTracks() && len(m.insights.trackView.SourceTracks) > 0 {
		refreshTrackMetadata(m.insights.trackView.SourceTracks)
		m.insights.trackView = m.insights.trackView.RefreshView().ClampCursor()
	}

	return m
}
