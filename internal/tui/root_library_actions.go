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
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

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
		case viewQueue:
			if m.queue.filter.Text != "" {
				m.queue.filter = m.queue.filter.Deactivate()
				m.queue.cursor = 0
				m.queue.scrollOffset = 0
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
			case viewHistory:
				cmd = m.history.EnqueueFocused(m.queueWriter, len(m.queue.entries))
				if m.history.filter.Text != "" {
					m.history.filter = m.history.filter.Deactivate()
					m.history.cursor = 0
					m.history.scrollOffset = 0
					m.history = m.history.rebuildFilterCache()
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
			case viewHistory:
				cmd = m.history.PlayNextFocused(m.queueWriter, prevLen, insertIdx)
				if m.history.filter.Text != "" {
					m.history.filter = m.history.filter.Deactivate()
					m.history.cursor = 0
					m.history.scrollOffset = 0
					m.history = m.history.rebuildFilterCache()
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
			case viewHistory:
				if _, ok := m.history.view().FocusedTrack(); ok {
					m = m.clearQueueForReplace()
					cmd = m.history.EnqueueFocused(m.queueWriter, 0)
					if m.history.filter.Text != "" {
						m.history.filter = m.history.filter.Deactivate()
						m.history.cursor = 0
						m.history.scrollOffset = 0
						m.history = m.history.rebuildFilterCache()
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
		case viewHistory:
			historyOrder := m.history.baseTracks()
			if m.history.sortedTracks != nil {
				historyOrder = m.history.sortedTracks
			}
			if len(m.history.selectedHashes) > 0 {
				for _, t := range historyOrder {
					if _, ok := m.history.selectedHashes[t.Hash]; ok {
						tracks = append(tracks, t)
					}
				}
			} else if t, ok := m.history.view().FocusedTrack(); ok {
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
