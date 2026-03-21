package tui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

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
		m.library.index.Insert(t)
	}
	m.scanned += len(msg.Tracks)
	m.statusBar.handleScanProgress(library.ScanProgressMsg{Scanned: m.scanned, Total: m.scanTotal})
	// Invalidate the cached fuzzy-match results so that stale TrackIndex
	// values from the previous index state cannot cause an out-of-bounds
	// panic when the filter is active and the user navigates or renders.
	if m.library.filter.Text != "" {
		m.library = m.library.rebuildFilterCache()
	}
	// Refresh the artists list when at ArtistLevel so the view stays
	// current as the library grows during a scan.
	if m.artists.level == arttui.ArtistLevel {
		m.artists.artists = arttui.DeriveArtists(m.library.index.Tracks())
		m.artists.sortState = SortState{}
		m.artists = m.artists.clampCursor()
	}
	// Refresh the albums list when at AlbumLevel so the view stays
	// current as the library grows during a scan.
	if m.albums.level == albtui.AlbumLevel {
		m.albums.albums = albtui.DeriveAllAlbums(m.library.index.Tracks())
		m.albums.sortState = SortState{}
		m.albums = m.albums.clampCursor()
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
	for _, t := range msg.tracks {
		m.library.index.UpsertByPath(t.Path, t)
	}
	// Invalidate the cached fuzzy-match results so that stale TrackIndex
	// values from the previous index state cannot cause an out-of-bounds
	// panic when the filter is active and the user navigates or renders.
	if m.library.filter.Text != "" {
		m.library = m.library.rebuildFilterCache()
		// Clamp cursor to the new (possibly shorter) match set.
		n := len(m.library.filteredMatches)
		if n == 0 {
			m.library.cursor = 0
			m.library.scrollOffset = 0
		} else if m.library.cursor >= n {
			m.library.cursor = n - 1
		}
	}
	// Refresh the artists list when at ArtistLevel so the view stays
	// current when the filesystem watcher detects changes.
	if m.artists.level == arttui.ArtistLevel {
		m.artists.artists = arttui.DeriveArtists(m.library.index.Tracks())
		m.artists.sortState = SortState{}
		m.artists = m.artists.clampCursor()
	}
	// Refresh the albums list when at AlbumLevel so the view stays
	// current when the filesystem watcher detects changes.
	if m.albums.level == albtui.AlbumLevel {
		m.albums.albums = albtui.DeriveAllAlbums(m.library.index.Tracks())
		m.albums.sortState = SortState{}
		m.albums = m.albums.clampCursor()
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
	return m
}

// handleLibraryKeyAction handles library, artist, and album navigation key
// actions. It returns true in the third value when the action was handled.
func (m Model) handleLibraryKeyAction(action action) (tea.Model, tea.Cmd, bool) {
	switch action {
	case actionRescan:
		if m.libraryRoot != "" && m.extractor != nil && !m.scanning {
			ch := library.Scan(m.appCtx, m.libraryRoot, m.extractor)
			m.library.index = library.LibraryIndex{}
			m.library.cursor = 0
			m.library.scrollOffset = 0
			m.library.filter.Active = false
			m.library.filter.Text = ""
			m.library.filteredMatches = nil
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
			return m, tea.Batch(waitForNextBatch(ch), countAudioFiles(m.appCtx, m.libraryRoot)), true
		}
		return m, nil, true
	case actionLibraryFilterClear:
		if m.library.filter.Text != "" {
			m.library.filter = m.library.filter.Deactivate()
			m.library.cursor = 0
			m.library.scrollOffset = 0
			m.library = m.library.rebuildFilterCache()
		}
		return m, nil, true
	case actionLibraryEnqueue:
		if m.queueWriter != nil {
			cmd := m.library.EnqueueFocused(m.queueWriter, len(m.queue.entries))
			if m.library.filter.Text != "" {
				m.library.filter = m.library.filter.Deactivate()
				m.library.cursor = 0
				m.library.scrollOffset = 0
				m.library = m.library.rebuildFilterCache()
			}
			return m, cmd, true
		}
		return m, nil, true
	case actionLibraryEnqueueReplace:
		if m.queueWriter != nil {
			var cmd tea.Cmd
			switch m.activeView {
			case viewLibrary:
				m = m.clearQueueForReplace()
				cmd = m.library.EnqueueFocused(m.queueWriter, 0)
				if m.library.filter.Text != "" {
					m.library.filter = m.library.filter.Deactivate()
					m.library.cursor = 0
					m.library.scrollOffset = 0
					m.library = m.library.rebuildFilterCache()
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
				m = m.clearQueueForReplace()
				cmd = m.favorites.enqueueFocusedTrack(m.queueWriter, 0)
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
		case viewArtists:
			allTracks := m.library.index.Tracks()
			switch m.artists.level {
			case arttui.ArtistLevel:
				if artist, ok := m.artists.focusedArtist(); ok {
					for _, t := range allTracks {
						if t.Artist == artist {
							tracks = append(tracks, t)
						}
					}
				}
			case arttui.AlbumLevel:
				if album, ok := m.artists.focusedAlbum(); ok {
					tracks = arttui.DeriveTracks(allTracks, m.artists.selectedArtist, album)
				}
			case arttui.TrackLevel:
				if t, ok := m.artists.focusedTrack(); ok {
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
				if t, ok := m.albums.focusedTrack(); ok {
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
								_ = pd.RemovePlaylistTrackSoft(t.ID)
							}
						}
					},
					func(before int64) {
						if pd != nil {
							_ = pd.PurgeDeletedPlaylistTracks(before)
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
				m.playlists.cursor, m.playlists.scrollOffset = clampAfterRemoval(
					m.playlists.cursor, m.playlists.scrollOffset, len(m.playlists.tracks))
				m.playlists.selectedTrackIDs = nil
				m.setUndoSlot(undoSlot{
					kind:                   undoKindBulkRemovePlaylistTracks,
					expiresAt:              time.Now().Add(undoWindowDuration),
					cancelPurge:            cancel,
					bulkRemoveTrackEntries: removedEntries,
					bulkRemoveTrackIndices: removedIndices,
				}, fmt.Sprintf("%d %s removed — press u to undo", n, pluralize(n, "track", "tracks")))
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
	}
	return m, nil, false
}
