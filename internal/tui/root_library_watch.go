package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
	"github.com/sglavoie/muse/internal/tui/text"
)

// handleWatchBatchMsg processes filesystem watcher events, removing deleted
// tracks and upserting changed ones.
func (m Model) handleWatchBatchMsg(msg watchBatchMsg) (Model, tea.Cmd) {
	m = m.watchApplyDeletes(msg.deleted)
	m, msg.tracks = m.watchUpsertTracks(msg.tracks)
	m = m.syncRecentlyAddedFromLibrary()
	m = m.reconcileTrackCaches(msg.tracks)
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
		m.library.Index.RemoveByPath(path)
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
		t = m.normalizeScannedTrack(t)
		tracks[i] = t
		m.library.Index.UpsertByPath(t.Path, t)
	}
	return m, tracks
}

// watchSyncQueueMetadata updates in-memory queue entries whose track metadata
// or file path was refreshed by the watcher, keeping the queue view in sync
// with on-disk changes. A same-hash move/rename updates QueueEntry.Path before
// syncPlayerQueue rebuilds the player queue, so playback resolves the new path
// immediately. Source, selection, and position state are preserved; the missing
// flag is cleared for a recovered entry.
func (m Model) watchSyncQueueMetadata(tracks []library.Track) Model {
	queueDirty := false
	for _, t := range tracks {
		display := text.TrackDisplay(t.Artist, t.Title, t.Path)
		durationMs := int64(t.Duration * 1000)
		for i := range m.queue.entries {
			e := &m.queue.entries[i]
			if e.Hash != t.Hash {
				continue
			}
			if e.Path != t.Path || e.Title != t.Title || e.Artist != t.Artist ||
				e.Album != t.Album || e.DurationMs != durationMs ||
				e.Display != display || e.Missing {
				queueDirty = true
			}
			e.Path = t.Path
			e.Title = t.Title
			e.Artist = t.Artist
			e.Album = t.Album
			e.DurationMs = durationMs
			e.Display = display
			e.Missing = false
		}
		if m.playback.currentHash == t.Hash {
			m.statusBar.Artist = t.Artist
			m.statusBar.Title = t.Title
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
	if m.library.Mode != libtui.LibraryModeHistory || len(m.library.SourceTracks) == 0 {
		return m
	}
	for _, t := range tracks {
		for i := range m.library.SourceTracks {
			if m.library.SourceTracks[i].Hash == t.Hash {
				m.library.SourceTracks[i].Path = t.Path
				m.library.SourceTracks[i].Title = t.Title
				m.library.SourceTracks[i].Artist = t.Artist
				m.library.SourceTracks[i].Album = t.Album
				m.library.SourceTracks[i].Genre = t.Genre
				m.library.SourceTracks[i].Duration = t.Duration
				m.library.SourceTracks[i].TrackNumber = t.TrackNumber
				m.library.SourceTracks[i].DiscNumber = t.DiscNumber
			}
		}
	}
	m.library = m.library.RefreshView()
	return m
}

// watchSyncPlaylistMetadata refreshes playlist track metadata so the
// playlists view reflects watcher-detected changes.
func (m Model) watchSyncPlaylistMetadata(tracks []library.Track) Model {
	if m.playlists.Level != pltui.TrackLevel {
		return m
	}
	for _, t := range tracks {
		for i := range m.playlists.Tracks {
			if m.playlists.Tracks[i].Hash == t.Hash {
				m.playlists.Tracks[i].Path = t.Path
				m.playlists.Tracks[i].Title = t.Title
				m.playlists.Tracks[i].Artist = t.Artist
				m.playlists.Tracks[i].Album = t.Album
				m.playlists.Tracks[i].DurationMs = int64(t.Duration * 1000)
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
					m.favorites.tracks[i].Path = t.Path
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
					m.favorites.drilldownTracks[i].Path = t.Path
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
				m.tagsView.tracks[i].Path = t.Path
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

// watchSyncMixesMetadata propagates same-hash path and metadata changes into
// every cached mix slice (not just the active one) so that a later drill-down
// into a previously-loaded mix reuses reconciled paths and metadata rather than
// stale values. When the mixes track-level view is open its derived view is
// refreshed in place, since trackView.sourceTracks aliases the active mix's
// cached slice.
func (m Model) watchSyncMixesMetadata(tracks []library.Track) Model {
	if len(m.mixes.cachedMixes) == 0 {
		return m
	}
	updated := false
	for _, t := range tracks {
		for id, cached := range m.mixes.cachedMixes {
			changed := false
			for i := range cached {
				if cached[i].Hash == t.Hash {
					cached[i].Path = t.Path
					cached[i].Title = t.Title
					cached[i].Artist = t.Artist
					cached[i].Album = t.Album
					cached[i].Genre = t.Genre
					cached[i].Duration = t.Duration
					changed = true
				}
			}
			if changed {
				m.mixes.cachedMixes[id] = cached
				updated = true
			}
		}
	}
	if updated && m.mixes.level == 1 {
		m.mixes.trackView = m.mixes.trackView.RefreshView()
	}
	return m
}

// watchSyncInsightsMetadata refreshes the open Insights drill track list so it
// reflects scan-detected changes. trackView.sourceTracks aliases whatever slice
// insightsDrillTracks returned — m.insights.dormant for a dormant drill — so
// entries are mutated in place and the slice is never appended to or resliced.
func (m Model) watchSyncInsightsMetadata(tracks []library.Track) Model {
	src := m.insights.trackView.SourceTracks
	if !m.insights.atTracks() || len(src) == 0 {
		return m
	}
	updated := false
	for _, t := range tracks {
		// A session drill can list the same hash more than once, so every index
		// is checked rather than stopping at the first match.
		for i := range src {
			if src[i].Hash != t.Hash {
				continue
			}
			src[i].Path = t.Path
			src[i].Title = t.Title
			src[i].Artist = t.Artist
			src[i].Album = t.Album
			src[i].Genre = t.Genre
			src[i].Duration = t.Duration
			src[i].TrackNumber = t.TrackNumber
			src[i].DiscNumber = t.DiscNumber
			updated = true
		}
	}
	if updated {
		m.insights.trackView = m.insights.trackView.RefreshView().ClampCursor()
	}
	return m
}

// reconcileTrackCaches propagates same-hash path and metadata updates into
// every in-memory cache that holds a playable Path: the queue (and its player
// mirror), the History-mode library source, and the loaded playlists,
// favorites, tags, mix, and Insights drill track lists. Artist/album/genre
// browse views are intentionally not touched here; they rederive from the live
// index via refreshBrowseDerivedViews. This is the single fan-out shared by
// the live watcher, the startup diff scan, and manual rescans so that a
// move/rename heals identically regardless of which pipeline observed it.
func (m Model) reconcileTrackCaches(tracks []library.Track) Model {
	m = m.watchSyncQueueMetadata(tracks)
	m = m.watchSyncHistoryMetadata(tracks)
	m = m.watchSyncPlaylistMetadata(tracks)
	m = m.watchSyncFavoritesMetadata(tracks)
	m = m.watchSyncTagsMetadata(tracks)
	m = m.watchSyncMixesMetadata(tracks)
	m = m.watchSyncInsightsMetadata(tracks)
	return m
}

// watchRecomputeLibrary recomputes sorted tracks, filter cache, and clamps
// the library cursor after watcher-driven insertions and deletions.
func (m Model) watchRecomputeLibrary() Model {
	m.library = m.library.RefreshView()
	if m.library.Filter.Text != "" && m.library.FilteredMatches != nil {
		n := len(m.library.FilteredMatches)
		if n == 0 {
			m.library.Cursor = 0
			m.library.ScrollOffset = 0
		} else if m.library.Cursor >= n {
			m.library.Cursor = n - 1
		}
	} else {
		n := m.library.Index.Len()
		if n == 0 {
			m.library.Cursor = 0
			m.library.ScrollOffset = 0
		} else if m.library.Cursor >= n {
			m.library.Cursor = n - 1
		}
	}
	if m.library.ScrollOffset > m.library.Cursor {
		m.library.ScrollOffset = m.library.Cursor
	}
	return m
}

// refreshBrowseDerivedViews re-derives and re-sorts artist, album, and genre
// data at the current drill-down level from the live library index.
func (m Model) refreshBrowseDerivedViews() Model {
	allTracks := m.library.Index.Tracks()
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
