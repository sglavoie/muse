package tui

import (
	"fmt"
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

// rescanEntry holds per-track values snapshotted before a full rescan clears
// the in-memory library index. Used to restore created_at and duration when
// the scanner cannot provide them (e.g. duration=0 for tag-less files).
type rescanEntry struct {
	CreatedAt int64
	Duration  float64
}

// handleScanProgressMsg updates the status bar with the current scan progress.
func (m Model) handleScanProgressMsg(msg library.ScanProgressMsg) Model {
	m.statusBar.handleScanProgress(msg)
	return m
}

// handleScanTotalMsg records the expected total and updates the progress display.
func (m Model) handleScanTotalMsg(msg library.ScanTotalMsg) Model {
	m.scanTotal = msg.Total
	if m.scanning {
		m.statusBar.handleScanProgress(library.ScanProgressMsg{Scanned: m.scanned, Total: m.scanTotal})
	}
	return m
}

// handleScanBatchMsg inserts scanned tracks into the library index and refreshes
// derived views (artists, albums) as needed.
func (m Model) handleScanBatchMsg(msg library.ScanBatchMsg) (Model, tea.Cmd) {
	for _, t := range msg.Tracks {
		t = m.preserveTrackMetadata(t)
		if edited, ok := m.userEditedOverlay[t.Hash]; ok {
			t.Title = edited.Title
			t.Artist = edited.Artist
			t.Album = edited.Album
			t.Genre = edited.Genre
		}
		m.library.index.Insert(t)
	}
	m.scanned += len(msg.Tracks)
	m.statusBar.handleScanProgress(library.ScanProgressMsg{Scanned: m.scanned, Total: m.scanTotal})
	// Recompute sorted tracks if a sort is active so the cached copy
	// includes newly scanned tracks.
	if m.library.sortState.Field != SortDefault {
		m.library = m.library.recomputeSortedTracks()
	}
	m = m.syncRecentlyAddedFromLibrary()
	// Invalidate the cached fuzzy-match results so that stale TrackIndex
	// values from the previous index state cannot cause an out-of-bounds
	// panic when the filter is active and the user navigates or renders.
	if m.library.filter.Text != "" {
		m.library = m.library.rebuildFilterCache()
	}
	// Refresh artist/album derived views at all drill-down levels so
	// the view stays current as the library grows during a scan.
	scanTracks := m.library.index.Tracks()
	switch m.artists.level {
	case arttui.ArtistLevel:
		m.artists.artists = arttui.DeriveArtists(scanTracks)
		m.artists.sortState = SortState{}
		m.artists = m.artists.clampCursor()
	case arttui.AlbumLevel:
		m.artists.albums = arttui.DeriveAlbums(scanTracks, m.artists.selectedArtist)
		m.artists = m.artists.recomputeArtistsSort()
		m.artists = m.artists.clampCursor()
	case arttui.TrackLevel:
		m.artists.tracks = arttui.DeriveTracks(scanTracks, m.artists.selectedArtist, m.artists.selectedAlbum)
		m.artists = m.artists.recomputeArtistsSort()
		m.artists = m.artists.clampCursor()
	}
	switch m.albums.level {
	case albtui.AlbumLevel:
		m.albums.albums = albtui.DeriveAllAlbums(scanTracks)
		m.albums.sortState = SortState{}
		m.albums = m.albums.clampCursor()
	case albtui.TrackLevel:
		m.albums.tracks = albtui.DeriveAlbumTracks(scanTracks, m.albums.selectedAlbum)
		m.albums = m.albums.recomputeAlbumsSort()
		m.albums = m.albums.clampCursor()
	}
	switch m.genres.level {
	case gentui.GenreLevel:
		m.genres.genres = gentui.DeriveGenres(scanTracks)
		m.genres.sortState = SortState{}
		m.genres = m.genres.clampCursor()
	case gentui.ArtistLevel:
		m.genres.artists = gentui.DeriveGenreArtists(scanTracks, m.genres.selectedGenre)
		m.genres = m.genres.recomputeGenresSort()
		m.genres = m.genres.clampCursor()
	case gentui.TrackLevel:
		m.genres.tracks = gentui.DeriveGenreArtistTracks(scanTracks, m.genres.selectedGenre, m.genres.selectedArtist)
		m.genres = m.genres.recomputeGenresSort()
		m.genres = m.genres.clampCursor()
	}
	if m.scanCh != nil {
		return m, waitForNextBatch(m.scanCh)
	}
	return m, nil
}

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

// handleScanDoneMsg clears the scanning state.
func (m Model) handleScanDoneMsg() Model {
	m.scanning = false
	m.scanCh = nil
	m.rescanSnapshot = nil
	return m
}

// handleLibraryKeyAction handles library, artist, and album navigation key
// actions. It returns true in the third value when the action was handled.
func (m Model) handleLibraryKeyAction(action action) (tea.Model, tea.Cmd, bool) {
	switch action {
	case actionRescan:
		if len(m.libraryRoots) > 0 && m.extractor != nil && !m.scanning {
			// Refresh user-edited overlay so manually edited metadata
			// survives the full rescan that is about to clear the index.
			if m.userEditedReader != nil {
				rows, err := m.userEditedReader.ReadUserEditedTracks()
				switch {
				case err != nil:
					slog.Warn("rescan: read user-edited tracks", "err", err)
				case len(rows) > 0:
					overlay := make(map[string]db.UserEditedRow, len(rows))
					for _, r := range rows {
						overlay[r.Hash] = r
					}
					m.userEditedOverlay = overlay
				default:
					m.userEditedOverlay = nil
				}
			}
			// Snapshot created_at and duration before clearing the index
			// so preserveTrackMetadata can restore them for rescanned tracks.
			tracks := m.library.index.Tracks()
			snap := make(map[string]rescanEntry, len(tracks))
			for _, t := range tracks {
				snap[t.Hash] = rescanEntry{CreatedAt: t.CreatedAt, Duration: t.Duration}
			}
			m.rescanSnapshot = snap
			rawCh := library.Scan(m.appCtx, m.libraryRoots, m.cfg.ExcludedPaths, m.extractor)
			ch := m.bridgeScanChannel(rawCh)
			m.library.index = library.LibraryIndex{}
			m.library.cursor = 0
			m.library.scrollOffset = 0
			m.library.filter = m.library.filter.Deactivate()
			m.library.filteredMatches = nil
			m.library.selectedHashes = nil
			m.library.sortState = SortState{}
			m.library.sortedTracks = nil
			m.recentlyAdded = newRecentlyAddedView()
			m.recentlyAdded.width = m.library.width
			m.recentlyAdded.height = m.library.height
			m.scanCh = ch
			m.scanning = true
			m.scanned = 0
			m.scanTotal = 0
			// Reset the artists view so it gets re-derived from the new
			// library index as scan batches arrive.
			m.artists = ArtistsView{width: m.artists.width, height: m.artists.height}
			// Reset the albums view so it gets re-derived from the new
			// library index as scan batches arrive.
			m.albums = AlbumsView{width: m.albums.width, height: m.albums.height}
			// Reset the genres view so stale selectedGenre/selectedArtist
			// from the previous scan don't persist into the new scan.
			m.genres = GenresView{width: m.genres.width, height: m.genres.height}
			return m, tea.Batch(waitForNextBatch(ch), countAudioFiles(m.appCtx, m.libraryRoots, m.cfg.ExcludedPaths)), true
		}
		return m, nil, true
	case actionLibraryFilterClear:
		switch m.activeView {
		case viewLibrary:
			if m.library.filter.Text != "" {
				m.library.filter = m.library.filter.Deactivate()
				m.library.cursor = 0
				m.library.scrollOffset = 0
				m.library = m.library.rebuildFilterCache()
			}
		case viewRecentlyAdded:
			if m.recentlyAdded.filter.Text != "" {
				m.recentlyAdded.filter = m.recentlyAdded.filter.Deactivate()
				m.recentlyAdded.cursor = 0
				m.recentlyAdded.scrollOffset = 0
				m.recentlyAdded = m.recentlyAdded.rebuildFilterCache()
			}
		}
		return m, nil, true
	case actionLibraryEnqueue:
		if m.queueWriter != nil {
			var cmd tea.Cmd
			switch m.activeView {
			case viewLibrary:
				cmd = m.library.EnqueueFocused(m.queueWriter, len(m.queue.entries))
				if m.library.filter.Text != "" {
					m.library.filter = m.library.filter.Deactivate()
					m.library.cursor = 0
					m.library.scrollOffset = 0
					m.library = m.library.rebuildFilterCache()
				}
			case viewRecentlyAdded:
				cmd = m.recentlyAdded.EnqueueFocused(m.queueWriter, len(m.queue.entries))
				if m.recentlyAdded.filter.Text != "" {
					m.recentlyAdded.filter = m.recentlyAdded.filter.Deactivate()
					m.recentlyAdded.cursor = 0
					m.recentlyAdded.scrollOffset = 0
					m.recentlyAdded = m.recentlyAdded.rebuildFilterCache()
				}
			}
			return m, cmd, true
		}
		return m, nil, true
	case actionPlayNext:
		if m.queueWriter != nil {
			insertIdx := m.playNextIndex()
			prevLen := len(m.queue.entries)
			var cmd tea.Cmd
			switch m.activeView {
			case viewLibrary:
				cmd = m.library.PlayNextFocused(m.queueWriter, prevLen, insertIdx)
				if m.library.filter.Text != "" {
					m.library.filter = m.library.filter.Deactivate()
					m.library.cursor = 0
					m.library.scrollOffset = 0
					m.library = m.library.rebuildFilterCache()
				}
			case viewRecentlyAdded:
				cmd = m.recentlyAdded.PlayNextFocused(m.queueWriter, prevLen, insertIdx)
				if m.recentlyAdded.filter.Text != "" {
					m.recentlyAdded.filter = m.recentlyAdded.filter.Deactivate()
					m.recentlyAdded.cursor = 0
					m.recentlyAdded.scrollOffset = 0
					m.recentlyAdded = m.recentlyAdded.rebuildFilterCache()
				}
			case viewArtists:
				if m.artists.level == arttui.TrackLevel {
					cmd = m.artists.playNextFocusedTrack(m.queueWriter, prevLen, insertIdx)
				}
			case viewAlbums:
				if m.albums.level == albtui.TrackLevel {
					cmd = m.albums.playNextFocusedTrack(m.queueWriter, prevLen, insertIdx)
				}
			case viewPlaylists:
				if m.playlists.level == pltui.TrackLevel {
					cmd = m.playlists.playNextFocusedTrack(m.queueWriter, prevLen, insertIdx)
				}
			case viewFavorites:
				if _, ok := m.favorites.focusedTrack(); ok {
					cmd = m.favorites.playNextFocusedTrack(m.queueWriter, prevLen, insertIdx)
				}
			case viewGenres:
				if _, ok := m.genres.focusedTrack(); ok {
					cmd = m.genres.playNextFocusedTrack(m.queueWriter, prevLen, insertIdx)
				}
			}
			return m, cmd, true
		}
		return m, nil, true
	case actionLibraryEnqueueReplace:
		if m.queueWriter != nil {
			var cmd tea.Cmd
			switch m.activeView {
			case viewLibrary:
				if _, ok := m.library.view().FocusedTrack(); ok {
					m = m.clearQueueForReplace()
					cmd = m.library.EnqueueFocused(m.queueWriter, 0)
					if m.library.filter.Text != "" {
						m.library.filter = m.library.filter.Deactivate()
						m.library.cursor = 0
						m.library.scrollOffset = 0
						m.library = m.library.rebuildFilterCache()
					}
				}
			case viewRecentlyAdded:
				if _, ok := m.recentlyAdded.view().FocusedTrack(); ok {
					m = m.clearQueueForReplace()
					cmd = m.recentlyAdded.EnqueueFocused(m.queueWriter, 0)
					if m.recentlyAdded.filter.Text != "" {
						m.recentlyAdded.filter = m.recentlyAdded.filter.Deactivate()
						m.recentlyAdded.cursor = 0
						m.recentlyAdded.scrollOffset = 0
						m.recentlyAdded = m.recentlyAdded.rebuildFilterCache()
					}
				}
			case viewArtists:
				if m.artists.level == arttui.TrackLevel {
					m = m.clearQueueForReplace()
					cmd = m.artists.enqueueFocusedTrack(m.queueWriter, 0)
				}
			case viewAlbums:
				if m.albums.level == albtui.TrackLevel {
					m = m.clearQueueForReplace()
					cmd = m.albums.enqueueFocusedTrack(m.queueWriter, 0)
				}
			case viewPlaylists:
				if m.playlists.level == pltui.TrackLevel {
					m = m.clearQueueForReplace()
					cmd = m.playlists.enqueueFocusedTrack(m.queueWriter, 0)
				}
			case viewFavorites:
				if _, ok := m.favorites.focusedTrack(); ok {
					m = m.clearQueueForReplace()
					cmd = m.favorites.enqueueFocusedTrack(m.queueWriter, 0)
				}
			case viewGenres:
				if _, ok := m.genres.focusedTrack(); ok {
					m = m.clearQueueForReplace()
					cmd = m.genres.enqueueFocusedTrack(m.queueWriter, 0)
				}
			}
			return m, cmd, true
		}
		return m, nil, true
	case actionBulkAction:
		var tracks []library.Track
		switch m.activeView {
		case viewLibrary:
			if len(m.library.selectedHashes) > 0 {
				for _, t := range m.library.index.Tracks() {
					if _, ok := m.library.selectedHashes[t.Hash]; ok {
						tracks = append(tracks, t)
					}
				}
			} else {
				if t, ok := m.library.view().FocusedTrack(); ok {
					tracks = []library.Track{t}
				}
			}
		case viewRecentlyAdded:
			trackOrder := m.recentlyAdded.baseTracks()
			if m.recentlyAdded.sortedTracks != nil {
				trackOrder = m.recentlyAdded.sortedTracks
			}
			if len(m.recentlyAdded.selectedHashes) > 0 {
				for _, t := range trackOrder {
					if _, ok := m.recentlyAdded.selectedHashes[t.Hash]; ok {
						tracks = append(tracks, t)
					}
				}
			} else if t, ok := m.recentlyAdded.view().FocusedTrack(); ok {
				tracks = []library.Track{t}
			}
		case viewArtists:
			allTracks := m.library.index.Tracks()
			switch m.artists.level {
			case arttui.ArtistLevel:
				if artist, ok := m.artists.focusedArtist(); ok {
					raw := arttui.RawName(artist)
					for _, t := range allTracks {
						if t.Artist == raw {
							tracks = append(tracks, t)
						}
					}
				}
			case arttui.AlbumLevel:
				if album, ok := m.artists.focusedAlbum(); ok {
					tracks = arttui.DeriveTracks(allTracks, m.artists.selectedArtist, album)
				}
			case arttui.TrackLevel:
				if len(m.artists.selectedHashes) > 0 {
					for _, t := range m.artists.tracks {
						if _, ok := m.artists.selectedHashes[t.Hash]; ok {
							tracks = append(tracks, t)
						}
					}
				} else if t, ok := m.artists.focusedTrack(); ok {
					tracks = []library.Track{t}
				}
			}
		case viewAlbums:
			allTracks := m.library.index.Tracks()
			switch m.albums.level {
			case albtui.AlbumLevel:
				if album, ok := m.albums.focusedAlbum(); ok {
					tracks = albtui.DeriveAlbumTracks(allTracks, album)
				}
			case albtui.TrackLevel:
				if len(m.albums.selectedHashes) > 0 {
					for _, t := range m.albums.tracks {
						if _, ok := m.albums.selectedHashes[t.Hash]; ok {
							tracks = append(tracks, t)
						}
					}
				} else if t, ok := m.albums.focusedTrack(); ok {
					tracks = []library.Track{t}
				}
			}
		case viewPlaylists:
			if m.playlists.level != pltui.TrackLevel || !m.playlists.hasTrackSelection() {
				return m, nil, true
			}
			n := len(m.playlists.selectedTrackIDs)
			now := time.Now()
			if m.undo.bulkRemovePending && now.Before(m.undo.bulkRemovePendingAt.Add(confirmationWindow)) {
				m.undo.bulkRemovePending = false
				// Capture removed entries and their indices for undo.
				var removedEntries []db.PlaylistTrackRow
				var removedIndices []int
				for i, t := range m.playlists.tracks {
					if _, ok := m.playlists.selectedTrackIDs[t.ID]; ok {
						removedEntries = append(removedEntries, t)
						removedIndices = append(removedIndices, i)
					}
				}
				pd := m.playlistDeleter
				cancel := m.scheduleSoftDelete(
					func() {
						if pd != nil {
							for _, t := range removedEntries {
								if err := pd.RemovePlaylistTrackSoft(t.ID); err != nil {
									slog.Warn("playlist track soft-delete failed", "err", err)
								}
							}
						}
					},
					func(before int64) {
						if pd != nil {
							if err := pd.PurgeDeletedPlaylistTracks(before); err != nil {
								slog.Warn("playlist track purge failed", "err", err)
							}
						}
					},
				)
				var remaining []db.PlaylistTrackRow
				for _, t := range m.playlists.tracks {
					if _, ok := m.playlists.selectedTrackIDs[t.ID]; !ok {
						remaining = append(remaining, t)
					}
				}
				m.playlists.tracks = remaining
				m = m.resetPlaylistsSort()
				m.playlists.cursor, m.playlists.scrollOffset = clampAfterRemoval(
					m.playlists.cursor, m.playlists.scrollOffset, len(m.playlists.tracks))
				m.playlists.selectedTrackIDs = nil
				m.setUndoSlot(undoBulkRemovePlaylistTracksAction{
					entries: removedEntries,
					indices: removedIndices,
				}, cancel)
				player.PlaySound(player.SoundRemoved)
				return m, nil, true
			}
			m.undo.bulkRemovePending = true
			m.undo.bulkRemovePendingAt = now
			m.statusBar.notification = newNotificationFor(
				fmt.Sprintf("Press a again to remove %d selected %s", n, pluralize(n, "track", "tracks")),
				confirmationWindow,
			)
			return m, nil, true
		case viewGenres:
			allTracks := m.library.index.Tracks()
			switch m.genres.level {
			case gentui.GenreLevel:
				if genre, ok := m.genres.focusedGenre(); ok {
					tracks = gentui.DeriveGenreTracks(allTracks, genre)
				}
			case gentui.ArtistLevel:
				if artist, ok := m.genres.focusedArtist(); ok {
					tracks = gentui.DeriveGenreArtistTracks(allTracks, m.genres.selectedGenre, artist)
				}
			case gentui.TrackLevel:
				if len(m.genres.selectedHashes) > 0 {
					for _, t := range m.genres.tracks {
						if _, ok := m.genres.selectedHashes[t.Hash]; ok {
							tracks = append(tracks, t)
						}
					}
				} else if t, ok := m.genres.focusedTrack(); ok {
					tracks = []library.Track{t}
				}
			}
		}
		if len(tracks) > 0 {
			modal := pltui.NewDestinationPicker(m.playlists.playlists)
			m.overlay.destPicker = &modal
			m.overlay.destPickerTracks = tracks
		}
		return m, nil, true
	case actionArtistOpen:
		allTracks := m.library.index.Tracks()
		switch m.artists.level {
		case arttui.ArtistLevel:
			artist, ok := m.artists.focusedArtist()
			if ok {
				albums := arttui.DeriveAlbums(allTracks, artist)
				m.artists.selectedArtist = artist
				if len(albums) == 0 {
					tracks := arttui.DeriveArtistTracks(allTracks, artist)
					m.artists.selectedAlbum = ""
					m.artists = m.artists.openAlbums(nil)
					m.artists = m.artists.openTracks(tracks)
				} else {
					m.artists = m.artists.openAlbums(albums)
				}
			}
		case arttui.AlbumLevel:
			album, ok := m.artists.focusedAlbum()
			if ok {
				tracks := arttui.DeriveTracks(allTracks, m.artists.selectedArtist, album)
				m.artists.selectedAlbum = album
				m.artists = m.artists.openTracks(tracks)
			}
		case arttui.TrackLevel:
			if m.queueWriter != nil {
				if cmd := m.artists.enqueueFocusedTrack(m.queueWriter, len(m.queue.entries)); cmd != nil {
					return m, cmd, true
				}
			}
		}
		return m, nil, true
	case actionArtistBack:
		m.artists = m.artists.back()
		return m, nil, true
	case actionAlbumOpen:
		allTracks := m.library.index.Tracks()
		switch m.albums.level {
		case albtui.AlbumLevel:
			album, ok := m.albums.focusedAlbum()
			if ok {
				tracks := albtui.DeriveAlbumTracks(allTracks, album)
				m.albums.selectedAlbum = album
				m.albums = m.albums.openTracks(tracks)
			}
		case albtui.TrackLevel:
			if m.queueWriter != nil {
				if cmd := m.albums.enqueueFocusedTrack(m.queueWriter, len(m.queue.entries)); cmd != nil {
					return m, cmd, true
				}
			}
		}
		return m, nil, true
	case actionAlbumBack:
		m.albums = m.albums.back()
		return m, nil, true
	case actionGenreOpen:
		allTracks := m.library.index.Tracks()
		switch m.genres.level {
		case gentui.GenreLevel:
			genre, ok := m.genres.focusedGenre()
			if ok {
				artists := gentui.DeriveGenreArtists(allTracks, genre)
				m.genres.selectedGenre = genre
				if len(artists) == 0 {
					tracks := gentui.DeriveGenreTracks(allTracks, genre)
					m.genres = m.genres.openArtists(nil)
					m.genres = m.genres.openTracks(tracks)
				} else {
					m.genres = m.genres.openArtists(artists)
				}
			}
		case gentui.ArtistLevel:
			artist, ok := m.genres.focusedArtist()
			if ok {
				tracks := gentui.DeriveGenreArtistTracks(allTracks, m.genres.selectedGenre, artist)
				m.genres.selectedArtist = artist
				m.genres = m.genres.openTracks(tracks)
			}
		case gentui.TrackLevel:
			if m.queueWriter != nil {
				if cmd := m.genres.enqueueFocusedTrack(m.queueWriter, len(m.queue.entries)); cmd != nil {
					return m, cmd, true
				}
			}
		}
		return m, nil, true
	case actionGenreBack:
		m.genres = m.genres.back()
		return m, nil, true
	}
	return m, nil, false
}

// bridgeScanChannel wraps raw with a goroutine that persists each track to the
// database via m.scanWriter before forwarding it. When no scanWriter is
// configured, the raw channel is returned as-is.
func (m Model) bridgeScanChannel(raw <-chan library.Track) <-chan library.Track {
	if m.scanWriter == nil {
		return raw
	}
	ch := make(chan library.Track, 256)
	go func() {
		defer close(ch)
		batch := make([]library.Track, 0, m.upsertBatchSize)
		flush := func() {
			if len(batch) == 0 {
				return
			}
			if err := m.scanWriter.UpsertBatch(batch); err != nil {
				slog.Warn("rescan: batch upsert", "err", err, "count", len(batch))
			}
			batch = batch[:0]
		}
		for t := range raw {
			ch <- t
			batch = append(batch, t)
			if len(batch) >= m.upsertBatchSize {
				flush()
			}
		}
		flush()
	}()
	return ch
}

func (m Model) preserveTrackMetadata(track library.Track) library.Track {
	// Try the live index first (works for watcher-driven inserts where the
	// index has not been cleared).
	if existing, ok := m.library.index.FindByHash(track.Hash); ok {
		if existing.CreatedAt != 0 {
			track.CreatedAt = existing.CreatedAt
		}
		if track.Duration == 0 && existing.Duration != 0 {
			track.Duration = existing.Duration
		}
		return track
	}
	if existing, ok := m.library.index.FindByPath(track.Path); ok {
		if existing.CreatedAt != 0 {
			track.CreatedAt = existing.CreatedAt
		}
		if track.Duration == 0 && existing.Duration != 0 {
			track.Duration = existing.Duration
		}
		return track
	}
	// Fall back to the pre-rescan snapshot (index was cleared for a full
	// rescan but we captured these values beforehand).
	if entry, ok := m.rescanSnapshot[track.Hash]; ok {
		if entry.CreatedAt != 0 {
			track.CreatedAt = entry.CreatedAt
		}
		if track.Duration == 0 && entry.Duration != 0 {
			track.Duration = entry.Duration
		}
	}
	if track.CreatedAt == 0 {
		track.CreatedAt = time.Now().Unix()
	}
	return track
}
