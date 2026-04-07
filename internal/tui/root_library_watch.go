package tui

import (
	tea "charm.land/bubbletea/v2"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

// handleWatchBatchMsg processes filesystem watcher events, removing deleted
// tracks and upserting changed ones.
func (m Model) handleWatchBatchMsg(msg watchBatchMsg) (Model, tea.Cmd) {
	for _, path := range msg.deleted {
		m.library.index.RemoveByPath(path)
	}
	// Mark queue entries whose file was deleted as missing so they appear
	// grayed out immediately rather than waiting until playback tries them.
	if len(msg.deleted) > 0 {
		deletedSet := make(map[string]struct{}, len(msg.deleted))
		for _, p := range msg.deleted {
			deletedSet[p] = struct{}{}
		}
		for i := range m.queue.entries {
			if _, ok := deletedSet[m.queue.entries[i].Path]; ok {
				m.queue.entries[i].Missing = true
			}
		}
	}
	for i, t := range msg.tracks {
		t = m.preserveTrackMetadata(t)
		if edited, ok := m.userEditedOverlay[t.Hash]; ok {
			t.Title = edited.Title
			t.Artist = edited.Artist
			t.Album = edited.Album
			t.Genre = edited.Genre
		}
		msg.tracks[i] = t
		m.library.index.UpsertByPath(t.Path, t)
	}
	m = m.syncRecentlyAddedFromLibrary()
	// Update queue entries whose track metadata was refreshed by the watcher
	// so the queue view stays in sync with on-disk changes.
	queueDirty := false
	for _, t := range msg.tracks {
		display := trackDisplay(t.Artist, t.Title, t.Path)
		for i := range m.queue.entries {
			if m.queue.entries[i].Hash == t.Hash {
				m.queue.entries[i].Title = t.Title
				m.queue.entries[i].Artist = t.Artist
				m.queue.entries[i].Album = t.Album
				m.queue.entries[i].Display = display
				queueDirty = true
				// If the track was previously missing and the watcher
				// now found it again (re-created file), clear the flag.
				if m.queue.entries[i].Missing {
					m.queue.entries[i].Missing = false
				}
			}
		}
		// If the currently-playing track was updated, refresh the status bar.
		if m.playback.currentHash == t.Hash {
			m.statusBar.artist = t.Artist
			m.statusBar.title = t.Title
		}
	}
	// Keep the player's in-memory queue in sync when entries changed.
	if queueDirty {
		m.syncPlayerQueue()
	}
	// Refresh playlist track metadata so the playlists view reflects
	// watcher-detected changes.
	if m.playlists.level == pltui.TrackLevel {
		for _, t := range msg.tracks {
			for i := range m.playlists.tracks {
				if m.playlists.tracks[i].Hash == t.Hash {
					m.playlists.tracks[i].Title = t.Title
					m.playlists.tracks[i].Artist = t.Artist
					m.playlists.tracks[i].Album = t.Album
				}
			}
		}
	}
	// Refresh favorites track metadata so the favorites view reflects
	// watcher-detected changes.
	if m.favorites.level == favtui.ListLevel && m.favorites.category == favtui.CategoryTracks {
		for _, t := range msg.tracks {
			for i := range m.favorites.tracks {
				if m.favorites.tracks[i].Hash == t.Hash {
					m.favorites.tracks[i].Title = t.Title
					m.favorites.tracks[i].Artist = t.Artist
					m.favorites.tracks[i].Album = t.Album
					m.favorites.tracks[i].Genre = t.Genre
				}
			}
		}
	}
	if m.favorites.level == favtui.TrackLevel {
		for _, t := range msg.tracks {
			for i := range m.favorites.drilldownTracks {
				if m.favorites.drilldownTracks[i].Hash == t.Hash {
					m.favorites.drilldownTracks[i].Title = t.Title
					m.favorites.drilldownTracks[i].Artist = t.Artist
					m.favorites.drilldownTracks[i].Album = t.Album
					m.favorites.drilldownTracks[i].Genre = t.Genre
				}
			}
		}
	}
	// Recompute sorted tracks if a sort is active so the cached copy
	// reflects insertions and deletions from the watcher.
	if m.library.sortState.Field != SortDefault {
		m.library = m.library.recomputeSortedTracks()
	}
	// Invalidate the cached fuzzy-match results so that stale TrackIndex
	// values from the previous index state cannot cause an out-of-bounds
	// panic when the filter is active and the user navigates or renders.
	if m.library.filter.Text != "" {
		m.library = m.library.rebuildFilterCache()
	}
	// Clamp library cursor to the effective track count (filtered or
	// unfiltered) so it cannot point beyond the end after deletions.
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
	// Keep scrollOffset at or below cursor so the cursor is always visible.
	if m.library.scrollOffset > m.library.cursor {
		m.library.scrollOffset = m.library.cursor
	}
	// Refresh artist/album derived views so they stay current when the
	// filesystem watcher detects changes. At deeper drill-down levels
	// (AlbumLevel, TrackLevel) the owned slices must also be refreshed;
	// otherwise the user sees stale metadata until they navigate away
	// and back.
	allTracks := m.library.index.Tracks()
	switch m.artists.level {
	case arttui.ArtistLevel:
		m.artists.artists = arttui.DeriveArtists(allTracks)
		m.artists.sortState = SortState{}
		m.artists = m.artists.clampCursor()
	case arttui.AlbumLevel:
		m.artists.albums = arttui.DeriveAlbums(allTracks, m.artists.selectedArtist)
		m.artists = m.artists.recomputeArtistsSort()
		m.artists = m.artists.clampCursor()
	case arttui.TrackLevel:
		m.artists.tracks = arttui.DeriveTracks(allTracks, m.artists.selectedArtist, m.artists.selectedAlbum)
		m.artists = m.artists.recomputeArtistsSort()
		m.artists = m.artists.clampCursor()
	}
	switch m.albums.level {
	case albtui.AlbumLevel:
		m.albums.albums = albtui.DeriveAllAlbums(allTracks)
		m.albums.sortState = SortState{}
		m.albums = m.albums.clampCursor()
	case albtui.TrackLevel:
		m.albums.tracks = albtui.DeriveAlbumTracks(allTracks, m.albums.selectedAlbum)
		m.albums = m.albums.recomputeAlbumsSort()
		m.albums = m.albums.clampCursor()
	}
	switch m.genres.level {
	case gentui.GenreLevel:
		m.genres.genres = gentui.DeriveGenres(allTracks)
		m.genres.sortState = SortState{}
		m.genres = m.genres.clampCursor()
	case gentui.ArtistLevel:
		m.genres.artists = gentui.DeriveGenreArtists(allTracks, m.genres.selectedGenre)
		m.genres = m.genres.recomputeGenresSort()
		m.genres = m.genres.clampCursor()
	case gentui.TrackLevel:
		m.genres.tracks = gentui.DeriveGenreArtistTracks(allTracks, m.genres.selectedGenre, m.genres.selectedArtist)
		m.genres = m.genres.recomputeGenresSort()
		m.genres = m.genres.clampCursor()
	}
	if m.watchCh != nil {
		return m, waitForWatchBatch(m.watchCh)
	}
	return m, nil
}
