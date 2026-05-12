package tui

import (
	musiclib "github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// bulkSelectedHashes returns the hashes of all bulk-selected tracks for the
// active view. Returns nil if there is no bulk selection or the view doesn't
// support it.
func (m Model) bulkSelectedHashes() []string {
	switch m.activeView {
	case viewLibrary:
		return hashMapToSlice(m.library.selectedHashes)
	case viewQueue:
		return m.queue.selectedHashes()
	case viewPlaylists:
		if len(m.playlists.selectedTrackIDs) == 0 {
			return nil
		}
		hashes := make([]string, 0, len(m.playlists.selectedTrackIDs))
		for _, t := range m.playlists.tracks {
			if _, ok := m.playlists.selectedTrackIDs[t.ID]; ok {
				hashes = append(hashes, t.Hash)
			}
		}
		return hashes
	case viewArtists:
		return hashMapToSlice(m.artists.selectedHashes)
	case viewAlbums:
		return hashMapToSlice(m.albums.selectedHashes)
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
	case viewGenres:
		return hashMapToSlice(m.genres.selectedHashes)
	case viewTags:
		return hashMapToSlice(m.tagsView.selectedHashes)
	case viewMixes:
		if m.mixes.level == 1 {
			return hashMapToSlice(m.mixes.trackView.selectedHashes)
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
		m.statusBar.artist = artist
		m.statusBar.title = title
	}
	return m
}

// refreshDerivedViews updates the artists and albums lists derived from the
// current library index. Call after any metadata change that may have renamed
// artists, albums, or titles. Also rebuilds the library sort/filter caches so
// that active sort orders and filter matches reflect the updated metadata.
func (m Model) refreshDerivedViews() Model {
	tracks := m.library.index.Tracks()

	if m.library.sortState.Field != SortDefault {
		m.library = m.library.recomputeSortedTracks()
	}
	if m.library.filter.Text != "" {
		m.library = m.library.rebuildFilterCache()
	}

	switch m.artists.level {
	case arttui.ArtistLevel:
		m.artists.artists = arttui.DeriveArtists(tracks)
		m.artists = m.artists.recomputeArtistsSort()
		m.artists = m.artists.clampCursor()
	case arttui.AlbumLevel:
		m.artists.albums = arttui.DeriveAlbums(tracks, m.artists.selectedArtist)
		m.artists = m.artists.recomputeArtistsSort()
		m.artists = m.artists.clampCursor()
	case arttui.TrackLevel:
		m.artists.tracks = arttui.DeriveTracks(tracks, m.artists.selectedArtist, m.artists.selectedAlbum)
		m.artists = m.artists.recomputeArtistsSort()
		m.artists = m.artists.clampCursor()
	}

	switch m.albums.level {
	case albtui.AlbumLevel:
		m.albums.albums = albtui.DeriveAllAlbums(tracks)
		m.albums = m.albums.recomputeAlbumsSort()
		m.albums = m.albums.clampCursor()
	case albtui.TrackLevel:
		m.albums.tracks = albtui.DeriveAlbumTracks(tracks, m.albums.selectedAlbum)
		m.albums = m.albums.recomputeAlbumsSort()
		m.albums = m.albums.clampCursor()
	}

	switch m.genres.level {
	case gentui.GenreLevel:
		m.genres.genres = gentui.DeriveGenres(tracks)
		m.genres = m.genres.recomputeGenresSort()
		m.genres = m.genres.clampCursor()
	case gentui.ArtistLevel:
		m.genres.artists = gentui.DeriveGenreArtists(tracks, m.genres.selectedGenre)
		m.genres = m.genres.recomputeGenresSort()
		m.genres = m.genres.clampCursor()
	case gentui.TrackLevel:
		m.genres.tracks = gentui.DeriveGenreArtistTracks(tracks, m.genres.selectedGenre, m.genres.selectedArtist)
		m.genres = m.genres.recomputeGenresSort()
		m.genres = m.genres.clampCursor()
	}

	if m.playlists.level == pltui.TrackLevel {
		for i := range m.playlists.tracks {
			if t, ok := m.library.index.FindByHash(m.playlists.tracks[i].Hash); ok {
				m.playlists.tracks[i].Title = t.Title
				m.playlists.tracks[i].Artist = t.Artist
				m.playlists.tracks[i].Album = t.Album
				m.playlists.tracks[i].DurationMs = int64(t.Duration * 1000)
			}
		}
		m.playlists = m.playlists.recomputePlaylistSort()
	}

	refreshTrackMetadata := func(ts []musiclib.Track) {
		for i := range ts {
			if t, ok := m.library.index.FindByHash(ts[i].Hash); ok {
				ts[i].Title = t.Title
				ts[i].Artist = t.Artist
				ts[i].Album = t.Album
				ts[i].Genre = t.Genre
				ts[i].Duration = t.Duration
			}
		}
	}

	if m.library.mode != LibraryModeAll && len(m.library.sourceTracks) > 0 {
		refreshTrackMetadata(m.library.sourceTracks)
		if m.library.sortState.Field != SortDefault {
			m.library = m.library.recomputeSortedTracks()
		}
		if m.library.filter.Text != "" {
			m.library = m.library.rebuildFilterCache()
		}
	}

	if m.tagsView.level == tagtui.TrackLevel {
		for i := range m.tagsView.tracks {
			if t, ok := m.library.index.FindByHash(m.tagsView.tracks[i].Hash); ok {
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

	if m.mixes.level == 1 {
		if cached, ok := m.mixes.cachedMixes[m.mixes.activeMix]; ok {
			refreshTrackMetadata(cached)
		}
		if m.mixes.trackView.sortState.Field != SortDefault {
			m.mixes.trackView = m.mixes.trackView.recomputeSortedTracks()
		}
		if m.mixes.trackView.filter.Text != "" {
			m.mixes.trackView = m.mixes.trackView.rebuildFilterCache()
		}
	}

	return m
}
