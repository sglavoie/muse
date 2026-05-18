package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// handleWatchBatchMsg processes filesystem watcher events, removing deleted
// tracks and upserting changed ones.
func (m Model) handleWatchBatchMsg(msg watchBatchMsg) (Model, tea.Cmd) {
	m = m.watchApplyDeletes(msg.deleted)
	m, msg.tracks = m.watchUpsertTracks(msg.tracks)
	m = m.syncRecentlyAddedFromLibrary()
	m = m.watchSyncQueueMetadata(msg.tracks)
	m = m.watchSyncHistoryMetadata(msg.tracks)
	m = m.watchSyncPlaylistMetadata(msg.tracks)
	m = m.watchSyncFavoritesMetadata(msg.tracks)
	m = m.watchSyncTagsMetadata(msg.tracks)
	m = m.watchRecomputeLibrary()
	m = m.refreshBrowseDerivedViews()
	if m.watchCh != nil {
		return m, waitForWatchBatch(m.watchCh)
	}
	return m, nil
}

// watchApplyDeletes removes deleted paths from the library index and marks
// the corresponding queue entries as missing.
func (m Model) watchApplyDeletes(deleted []string) Model {
	for _, path := range deleted {
		m.library.index.RemoveByPath(path)
	}
	if len(deleted) > 0 {
		deletedSet := make(map[string]struct{}, len(deleted))
		for _, p := range deleted {
			deletedSet[p] = struct{}{}
		}
		for i := range m.queue.entries {
			if _, ok := deletedSet[m.queue.entries[i].Path]; ok {
				m.queue.entries[i].Missing = true
			}
		}
	}
	return m
}

// watchUpsertTracks applies user-edited overlays and preserved metadata, then
// upserts each track into the library index. It returns the updated model and
// the (possibly modified) track slice for downstream consumers.
func (m Model) watchUpsertTracks(tracks []library.Track) (Model, []library.Track) {
	for i, t := range tracks {
		t = m.preserveTrackMetadata(t)
		if edited, ok := m.userEditedOverlay[t.Hash]; ok {
			t.Title = edited.Title
			t.Artist = edited.Artist
			t.Album = edited.Album
			t.Genre = edited.Genre
		}
		tracks[i] = t
		m.library.index.UpsertByPath(t.Path, t)
	}
	return m, tracks
}

// watchSyncQueueMetadata updates in-memory queue entries whose track metadata
// was refreshed by the watcher, keeping the queue view in sync with on-disk
// changes.
func (m Model) watchSyncQueueMetadata(tracks []library.Track) Model {
	queueDirty := false
	for _, t := range tracks {
		display := trackDisplay(t.Artist, t.Title, t.Path)
		for i := range m.queue.entries {
			if m.queue.entries[i].Hash == t.Hash {
				m.queue.entries[i].Title = t.Title
				m.queue.entries[i].Artist = t.Artist
				m.queue.entries[i].Album = t.Album
				m.queue.entries[i].DurationMs = int64(t.Duration * 1000)
				m.queue.entries[i].Display = display
				queueDirty = true
				if m.queue.entries[i].Missing {
					m.queue.entries[i].Missing = false
				}
			}
		}
		if m.playback.currentHash == t.Hash {
			m.statusBar.artist = t.Artist
			m.statusBar.title = t.Title
		}
	}
	if queueDirty {
		m.syncPlayerQueue()
		m = m.preserveQueueSort()
	}
	return m
}

// watchSyncHistoryMetadata refreshes history track metadata so the library
// History mode view reflects watcher-detected changes.
func (m Model) watchSyncHistoryMetadata(tracks []library.Track) Model {
	if m.library.mode != LibraryModeHistory || len(m.library.sourceTracks) == 0 {
		return m
	}
	for _, t := range tracks {
		for i := range m.library.sourceTracks {
			if m.library.sourceTracks[i].Hash == t.Hash {
				m.library.sourceTracks[i].Title = t.Title
				m.library.sourceTracks[i].Artist = t.Artist
				m.library.sourceTracks[i].Album = t.Album
				m.library.sourceTracks[i].Genre = t.Genre
				m.library.sourceTracks[i].Duration = t.Duration
			}
		}
	}
	m.library = m.library.refreshView()
	return m
}

// watchSyncPlaylistMetadata refreshes playlist track metadata so the
// playlists view reflects watcher-detected changes.
func (m Model) watchSyncPlaylistMetadata(tracks []library.Track) Model {
	if m.playlists.level != pltui.TrackLevel {
		return m
	}
	for _, t := range tracks {
		for i := range m.playlists.tracks {
			if m.playlists.tracks[i].Hash == t.Hash {
				m.playlists.tracks[i].Title = t.Title
				m.playlists.tracks[i].Artist = t.Artist
				m.playlists.tracks[i].Album = t.Album
				m.playlists.tracks[i].DurationMs = int64(t.Duration * 1000)
			}
		}
	}
	m = m.preservePlaylistsSort()
	return m
}

// watchSyncFavoritesMetadata refreshes favorites track metadata (both the
// top-level track list and the drill-down track list) so the favorites view
// reflects watcher-detected changes.
func (m Model) watchSyncFavoritesMetadata(tracks []library.Track) Model {
	if m.favorites.level == favtui.ListLevel && m.favorites.category == favtui.CategoryTracks {
		for _, t := range tracks {
			for i := range m.favorites.tracks {
				if m.favorites.tracks[i].Hash == t.Hash {
					m.favorites.tracks[i].Title = t.Title
					m.favorites.tracks[i].Artist = t.Artist
					m.favorites.tracks[i].Album = t.Album
					m.favorites.tracks[i].Genre = t.Genre
					m.favorites.tracks[i].Duration = t.Duration
				}
			}
		}
		m.favorites = m.favorites.recomputeFavoritesSort()
	}
	if m.favorites.level == favtui.TrackLevel {
		for _, t := range tracks {
			for i := range m.favorites.drilldownTracks {
				if m.favorites.drilldownTracks[i].Hash == t.Hash {
					m.favorites.drilldownTracks[i].Title = t.Title
					m.favorites.drilldownTracks[i].Artist = t.Artist
					m.favorites.drilldownTracks[i].Album = t.Album
					m.favorites.drilldownTracks[i].Genre = t.Genre
					m.favorites.drilldownTracks[i].Duration = t.Duration
				}
			}
		}
		m.favorites = m.favorites.recomputeFavoritesSort()
	}
	return m
}

// watchSyncTagsMetadata refreshes tags track metadata so the tags view
// reflects watcher-detected changes.
func (m Model) watchSyncTagsMetadata(tracks []library.Track) Model {
	if m.tagsView.level != tagtui.TrackLevel {
		return m
	}
	for _, t := range tracks {
		for i := range m.tagsView.tracks {
			if m.tagsView.tracks[i].Hash == t.Hash {
				m.tagsView.tracks[i].Title = t.Title
				m.tagsView.tracks[i].Artist = t.Artist
				m.tagsView.tracks[i].Album = t.Album
				m.tagsView.tracks[i].DurationMs = int64(t.Duration * 1000)
			}
		}
	}
	m.tagsView = m.tagsView.recomputeTagsSort()
	return m
}

// watchRecomputeLibrary recomputes sorted tracks, filter cache, and clamps
// the library cursor after watcher-driven insertions and deletions.
func (m Model) watchRecomputeLibrary() Model {
	m.library = m.library.refreshView()
	if m.library.filter.Text != "" && m.library.filteredMatches != nil {
		n := len(m.library.filteredMatches)
		if n == 0 {
			m.library.cursor = 0
			m.library.scrollOffset = 0
		} else if m.library.cursor >= n {
			m.library.cursor = n - 1
		}
	} else {
		n := m.library.index.Len()
		if n == 0 {
			m.library.cursor = 0
			m.library.scrollOffset = 0
		} else if m.library.cursor >= n {
			m.library.cursor = n - 1
		}
	}
	if m.library.scrollOffset > m.library.cursor {
		m.library.scrollOffset = m.library.cursor
	}
	return m
}

// refreshBrowseDerivedViews re-derives and re-sorts artist, album, and genre
// data at the current drill-down level from the live library index.
func (m Model) refreshBrowseDerivedViews() Model {
	allTracks := m.library.index.Tracks()
	switch m.browse.artists.level {
	case arttui.ArtistLevel:
		m.browse.artists.artists = arttui.DeriveArtists(allTracks)
		m.browse.artists.artistCounts = arttui.DeriveArtistCounts(allTracks)
		m.browse.artists = m.browse.artists.recomputeArtistsSort()
		m.browse.artists = m.browse.artists.clampCursor()
	case arttui.AlbumLevel:
		m.browse.artists.albums = arttui.DeriveAlbums(allTracks, m.browse.artists.selectedArtist)
		m.browse.artists = m.browse.artists.recomputeArtistsSort()
		m.browse.artists = m.browse.artists.clampCursor()
	case arttui.TrackLevel:
		m.browse.artists.tracks = arttui.DeriveTracks(allTracks, m.browse.artists.selectedArtist, m.browse.artists.selectedAlbum)
		m.browse.artists = m.browse.artists.recomputeArtistsSort()
		m.browse.artists = m.browse.artists.clampCursor()
	}
	switch m.browse.albums.level {
	case albtui.AlbumLevel:
		m.browse.albums.albums = albtui.DeriveAllAlbums(allTracks)
		m.browse.albums.albumCounts = albtui.DeriveAllAlbumCounts(allTracks)
		m.browse.albums = m.browse.albums.recomputeAlbumsSort()
		m.browse.albums = m.browse.albums.clampCursor()
	case albtui.TrackLevel:
		m.browse.albums.tracks = albtui.DeriveAlbumTracks(allTracks, m.browse.albums.selectedAlbum)
		m.browse.albums = m.browse.albums.recomputeAlbumsSort()
		m.browse.albums = m.browse.albums.clampCursor()
	}
	switch m.browse.genres.level {
	case gentui.GenreLevel:
		m.browse.genres.genres = gentui.DeriveGenres(allTracks)
		m.browse.genres.genreCounts = gentui.DeriveGenreCounts(allTracks)
		m.browse.genres = m.browse.genres.recomputeGenresSort()
		m.browse.genres = m.browse.genres.clampCursor()
	case gentui.ArtistLevel:
		m.browse.genres.artists = gentui.DeriveGenreArtists(allTracks, m.browse.genres.selectedGenre)
		m.browse.genres = m.browse.genres.recomputeGenresSort()
		m.browse.genres = m.browse.genres.clampCursor()
	case gentui.TrackLevel:
		m.browse.genres.tracks = gentui.DeriveGenreArtistTracks(allTracks, m.browse.genres.selectedGenre, m.browse.genres.selectedArtist)
		m.browse.genres = m.browse.genres.recomputeGenresSort()
		m.browse.genres = m.browse.genres.clampCursor()
	}
	return m
}
