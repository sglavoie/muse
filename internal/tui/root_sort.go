package tui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/layout"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

// handleSortAction handles sort-related key actions. Returns (model, cmd, true)
// when the action was handled, or (zero, nil, false) to let the caller continue
// the handler chain.
func (m Model) handleSortAction(a action) (tea.Model, tea.Cmd, bool) {
	switch a {
	case actionSortCycle:
		return m.handleSortCycle()
	case actionSortAsc:
		return m.handleSortAsc()
	case actionSortDescOrPageDown:
		return m.handleSortDescOrPageDown()
	default:
		return m, nil, false
	}
}

// handleSortCycle cycles to the next sort field for the active view.
func (m Model) handleSortCycle() (tea.Model, tea.Cmd, bool) {
	switch m.activeView {
	case viewLibrary:
		next := CycleSortField(m.library.sortState.Field, librarySortFields)
		m.library.sortState.Field = next
		if next == SortDefault {
			m.library.sortState.Dir = SortAsc
		}
		m.library = m.library.recomputeSortedTracks()
		m.library.cursor = 0
		m.library.scrollOffset = 0
		if m.library.filter.Text != "" {
			m.library = m.library.rebuildFilterCache()
		}
		m.statusBar.notification = transientNotification{
			msg:       sortNotification(m.library.sortState),
			expiresAt: time.Now().Add(transientDuration),
		}
		return m, nil, true

	case viewQueue:
		if m.queue.savingAsPlaylist {
			return m, nil, false
		}
		next := CycleSortField(m.queue.sortState.Field, queueSortFields)
		m.queue.sortState.Field = next
		if next == SortDefault {
			m.queue.sortState.Dir = SortAsc
		}
		m.queue = m.queue.recomputeDisplayOrder()
		m.queue.cursor = 0
		m.queue.scrollOffset = 0
		m.statusBar.notification = transientNotification{
			msg:       sortNotification(m.queue.sortState),
			expiresAt: time.Now().Add(transientDuration),
		}
		return m, nil, true

	case viewPlaylists:
		if m.playlists.creating || m.playlists.renaming {
			return m, nil, false
		}
		fields := playlistListSortFields
		if m.playlists.level == pltui.TrackLevel {
			fields = playlistTrackSortFields
		}
		next := CycleSortField(m.playlists.sortState.Field, fields)
		m.playlists.sortState.Field = next
		if next == SortDefault {
			m.playlists.sortState.Dir = SortAsc
		}
		m.playlists = m.playlists.recomputePlaylistSort()
		m.playlists.cursor = 0
		m.playlists.scrollOffset = 0
		m.statusBar.notification = transientNotification{
			msg:       sortNotification(m.playlists.sortState),
			expiresAt: time.Now().Add(transientDuration),
		}
		return m, nil, true

	case viewArtists:
		fields := artistsSortFieldsForLevel(m.artists.level)
		next := CycleSortField(m.artists.sortState.Field, fields)
		m.artists.sortState.Field = next
		if next == SortDefault {
			m.artists.sortState.Dir = SortAsc
		}
		m.artists = m.artists.recomputeArtistsSort()
		m.artists.cursor = 0
		m.artists.scrollOffset = 0
		m.statusBar.notification = transientNotification{
			msg:       sortNotification(m.artists.sortState),
			expiresAt: time.Now().Add(transientDuration),
		}
		return m, nil, true

	case viewAlbums:
		fields := albumsSortFieldsForLevel(m.albums.level)
		next := CycleSortField(m.albums.sortState.Field, fields)
		m.albums.sortState.Field = next
		if next == SortDefault {
			m.albums.sortState.Dir = SortAsc
		}
		m.albums = m.albums.recomputeAlbumsSort()
		m.albums.cursor = 0
		m.albums.scrollOffset = 0
		m.statusBar.notification = transientNotification{
			msg:       sortNotification(m.albums.sortState),
			expiresAt: time.Now().Add(transientDuration),
		}
		return m, nil, true

	default:
		return m, nil, false
	}
}

// handleSortAsc sets the sort direction to ascending for the active view.
// The focused item is preserved at its new position after re-sort.
func (m Model) handleSortAsc() (tea.Model, tea.Cmd, bool) {
	switch m.activeView {
	case viewLibrary:
		if m.library.sortState.Field == SortDefault {
			return m, nil, false
		}
		var focusedHash string
		if t, ok := m.library.view().FocusedTrack(); ok {
			focusedHash = t.Hash
		}
		m.library.sortState.Dir = SortAsc
		m.library = m.library.recomputeSortedTracks()
		if m.library.filter.Text != "" {
			m.library = m.library.rebuildFilterCache()
		}
		m.library.cursor = m.library.findTrackPosition(focusedHash)
		m.library.scrollOffset = adjustScroll(m.library.cursor, m.library.scrollOffset, layout.VisibleRows(m.library.height, 1))
		m.statusBar.notification = transientNotification{
			msg:       sortNotification(m.library.sortState),
			expiresAt: time.Now().Add(transientDuration),
		}
		return m, nil, true

	case viewQueue:
		if m.queue.sortState.Field == SortDefault {
			return m, nil, false
		}
		prevEntryIdx := m.queue.entryIndex(m.queue.cursor)
		m.queue.sortState.Dir = SortAsc
		m.queue = m.queue.recomputeDisplayOrder()
		m.queue.cursor = m.queue.findEntryDisplayPosition(prevEntryIdx)
		m.queue.scrollOffset = adjustScroll(m.queue.cursor, m.queue.scrollOffset, layout.VisibleRows(m.queue.height, 2))
		m.statusBar.notification = transientNotification{
			msg:       sortNotification(m.queue.sortState),
			expiresAt: time.Now().Add(transientDuration),
		}
		return m, nil, true

	case viewPlaylists:
		if m.playlists.sortState.Field == SortDefault {
			return m, nil, false
		}
		m = m.preservePlaylistCursorOnDirChange(SortAsc)
		m.statusBar.notification = transientNotification{
			msg:       sortNotification(m.playlists.sortState),
			expiresAt: time.Now().Add(transientDuration),
		}
		return m, nil, true

	case viewArtists:
		if m.artists.sortState.Field == SortDefault {
			return m, nil, false
		}
		m = m.preserveArtistsCursorOnDirChange(SortAsc)
		m.statusBar.notification = transientNotification{
			msg:       sortNotification(m.artists.sortState),
			expiresAt: time.Now().Add(transientDuration),
		}
		return m, nil, true

	case viewAlbums:
		if m.albums.sortState.Field == SortDefault {
			return m, nil, false
		}
		m = m.preserveAlbumsCursorOnDirChange(SortAsc)
		m.statusBar.notification = transientNotification{
			msg:       sortNotification(m.albums.sortState),
			expiresAt: time.Now().Add(transientDuration),
		}
		return m, nil, true

	default:
		return m, nil, false
	}
}

// handleSortDescOrPageDown sets descending sort when sort is active, otherwise
// dispatches half-page-down.
func (m Model) handleSortDescOrPageDown() (tea.Model, tea.Cmd, bool) {
	switch m.activeView {
	case viewLibrary:
		if m.library.sortState.Field != SortDefault {
			var focusedHash string
			if t, ok := m.library.view().FocusedTrack(); ok {
				focusedHash = t.Hash
			}
			m.library.sortState.Dir = SortDesc
			m.library = m.library.recomputeSortedTracks()
			if m.library.filter.Text != "" {
				m.library = m.library.rebuildFilterCache()
			}
			m.library.cursor = m.library.findTrackPosition(focusedHash)
			m.library.scrollOffset = adjustScroll(m.library.cursor, m.library.scrollOffset, layout.VisibleRows(m.library.height, 1))
			m.statusBar.notification = transientNotification{
				msg:       sortNotification(m.library.sortState),
				expiresAt: time.Now().Add(transientDuration),
			}
			return m, nil, true
		}
		m.library = m.library.HandleAction(actionHalfPageDown)
		return m, nil, true

	case viewQueue:
		if m.queue.sortState.Field != SortDefault {
			prevEntryIdx := m.queue.entryIndex(m.queue.cursor)
			m.queue.sortState.Dir = SortDesc
			m.queue = m.queue.recomputeDisplayOrder()
			m.queue.cursor = m.queue.findEntryDisplayPosition(prevEntryIdx)
			m.queue.scrollOffset = adjustScroll(m.queue.cursor, m.queue.scrollOffset, layout.VisibleRows(m.queue.height, 2))
			m.statusBar.notification = transientNotification{
				msg:       sortNotification(m.queue.sortState),
				expiresAt: time.Now().Add(transientDuration),
			}
			return m, nil, true
		}
		var cmd tea.Cmd
		m.queue, cmd = m.queue.HandleAction(actionHalfPageDown)
		return m, cmd, true

	case viewPlaylists:
		if m.playlists.sortState.Field != SortDefault {
			m = m.preservePlaylistCursorOnDirChange(SortDesc)
			m.statusBar.notification = transientNotification{
				msg:       sortNotification(m.playlists.sortState),
				expiresAt: time.Now().Add(transientDuration),
			}
			return m, nil, true
		}
		m.playlists = m.playlists.HandleAction(actionHalfPageDown)
		return m, nil, true

	case viewArtists:
		if m.artists.sortState.Field != SortDefault {
			m = m.preserveArtistsCursorOnDirChange(SortDesc)
			m.statusBar.notification = transientNotification{
				msg:       sortNotification(m.artists.sortState),
				expiresAt: time.Now().Add(transientDuration),
			}
			return m, nil, true
		}
		m.artists = m.artists.HandleAction(actionHalfPageDown)
		return m, nil, true

	case viewAlbums:
		if m.albums.sortState.Field != SortDefault {
			m = m.preserveAlbumsCursorOnDirChange(SortDesc)
			m.statusBar.notification = transientNotification{
				msg:       sortNotification(m.albums.sortState),
				expiresAt: time.Now().Add(transientDuration),
			}
			return m, nil, true
		}
		m.albums = m.albums.HandleAction(actionHalfPageDown)
		return m, nil, true

	default:
		return m, nil, false
	}
}

// resetLibrarySort clears the library sort state and cached sorted tracks.
func (m Model) resetLibrarySort() Model {
	m.library.sortState = SortState{}
	m.library.sortedTracks = nil
	return m
}

// resetQueueSort clears the queue sort state and display order.
func (m Model) resetQueueSort() Model {
	m.queue.sortState = SortState{}
	m.queue.displayOrder = nil
	return m
}

// resetPlaylistsSort clears the playlists sort state and sorted caches.
func (m Model) resetPlaylistsSort() Model {
	m.playlists.sortState = SortState{}
	m.playlists.sortedPlaylists = nil
	m.playlists.sortedTrackOrder = nil
	return m
}

// resetArtistsSort clears the artists sort state.
func (m Model) resetArtistsSort() Model {
	m.artists.sortState = SortState{}
	return m
}

// resetAlbumsSort clears the albums sort state.
func (m Model) resetAlbumsSort() Model {
	m.albums.sortState = SortState{}
	return m
}

// resetAllSorts clears sort state for all views.
func (m Model) resetAllSorts() Model {
	m = m.resetLibrarySort()
	m = m.resetQueueSort()
	m = m.resetPlaylistsSort()
	m = m.resetArtistsSort()
	m = m.resetAlbumsSort()
	return m
}

// artistsSortFieldsForLevel returns the sort field cycle for the given artists level.
func artistsSortFieldsForLevel(level arttui.Level) []SortField {
	switch level {
	case arttui.ArtistLevel:
		return artistArtistSortFields
	case arttui.AlbumLevel:
		return artistAlbumSortFields
	case arttui.TrackLevel:
		return artistTrackSortFields
	default:
		return artistArtistSortFields
	}
}

// albumsSortFieldsForLevel returns the sort field cycle for the given albums level.
func albumsSortFieldsForLevel(level albtui.Level) []SortField {
	switch level {
	case albtui.AlbumLevel:
		return albumAlbumSortFields
	case albtui.TrackLevel:
		return albumTrackSortFields
	default:
		return albumAlbumSortFields
	}
}

// preservePlaylistCursorOnDirChange changes the playlists sort direction while
// keeping the focused item at its new position.
func (m Model) preservePlaylistCursorOnDirChange(dir SortDir) Model {
	if m.playlists.level == pltui.ListLevel {
		var focusedID int64
		pl := m.playlists.effectivePlaylists()
		if m.playlists.cursor >= 0 && m.playlists.cursor < len(pl) {
			focusedID = pl[m.playlists.cursor].ID
		}
		m.playlists.sortState.Dir = dir
		m.playlists = m.playlists.recomputePlaylistSort()
		m.playlists.cursor = findPlaylistPosition(m.playlists.effectivePlaylists(), focusedID)
	} else {
		prevTrackIdx := m.playlists.trackIndex(m.playlists.cursor)
		m.playlists.sortState.Dir = dir
		m.playlists = m.playlists.recomputePlaylistSort()
		m.playlists.cursor = m.playlists.findTrackDisplayPosition(prevTrackIdx)
	}
	m.playlists.scrollOffset = adjustScroll(m.playlists.cursor, m.playlists.scrollOffset, layout.VisibleRows(m.playlists.height, 1))
	return m
}

// preserveArtistsCursorOnDirChange changes the artists sort direction while
// keeping the focused item at its new position.
func (m Model) preserveArtistsCursorOnDirChange(dir SortDir) Model {
	switch m.artists.level {
	case arttui.ArtistLevel:
		var name string
		items := arttui.FilterStrings(m.artists.artists, m.artists.filter.Text)
		if m.artists.cursor >= 0 && m.artists.cursor < len(items) {
			name = items[m.artists.cursor]
		}
		m.artists.sortState.Dir = dir
		m.artists = m.artists.recomputeArtistsSort()
		items = arttui.FilterStrings(m.artists.artists, m.artists.filter.Text)
		m.artists.cursor = findStringPosition(items, name)
	case arttui.AlbumLevel:
		var name string
		items := arttui.FilterStrings(m.artists.albums, m.artists.filter.Text)
		if m.artists.cursor >= 0 && m.artists.cursor < len(items) {
			name = items[m.artists.cursor]
		}
		m.artists.sortState.Dir = dir
		m.artists = m.artists.recomputeArtistsSort()
		items = arttui.FilterStrings(m.artists.albums, m.artists.filter.Text)
		m.artists.cursor = findStringPosition(items, name)
	case arttui.TrackLevel:
		var focusedHash string
		items := arttui.FilterTracks(m.artists.tracks, m.artists.filter.Text)
		if m.artists.cursor >= 0 && m.artists.cursor < len(items) {
			focusedHash = items[m.artists.cursor].Hash
		}
		m.artists.sortState.Dir = dir
		m.artists = m.artists.recomputeArtistsSort()
		items = arttui.FilterTracks(m.artists.tracks, m.artists.filter.Text)
		m.artists.cursor = findTrackHashPosition(items, focusedHash)
	}
	reserve := artistsVisibleReserve(m.artists)
	m.artists.scrollOffset = adjustScroll(m.artists.cursor, m.artists.scrollOffset, layout.VisibleRows(m.artists.height, reserve))
	return m
}

// preserveAlbumsCursorOnDirChange changes the albums sort direction while
// keeping the focused item at its new position.
func (m Model) preserveAlbumsCursorOnDirChange(dir SortDir) Model {
	switch m.albums.level {
	case albtui.AlbumLevel:
		var name string
		items := arttui.FilterStrings(m.albums.albums, m.albums.filter.Text)
		if m.albums.cursor >= 0 && m.albums.cursor < len(items) {
			name = items[m.albums.cursor]
		}
		m.albums.sortState.Dir = dir
		m.albums = m.albums.recomputeAlbumsSort()
		items = arttui.FilterStrings(m.albums.albums, m.albums.filter.Text)
		m.albums.cursor = findStringPosition(items, name)
	case albtui.TrackLevel:
		var focusedHash string
		items := arttui.FilterTracks(m.albums.tracks, m.albums.filter.Text)
		if m.albums.cursor >= 0 && m.albums.cursor < len(items) {
			focusedHash = items[m.albums.cursor].Hash
		}
		m.albums.sortState.Dir = dir
		m.albums = m.albums.recomputeAlbumsSort()
		items = arttui.FilterTracks(m.albums.tracks, m.albums.filter.Text)
		m.albums.cursor = findTrackHashPosition(items, focusedHash)
	}
	reserve := albumsVisibleReserve(m.albums)
	m.albums.scrollOffset = adjustScroll(m.albums.cursor, m.albums.scrollOffset, layout.VisibleRows(m.albums.height, reserve))
	return m
}

// artistsVisibleReserve returns the line reserve for the artists view's visibleRows.
func artistsVisibleReserve(v ArtistsView) int {
	var reserve int
	switch v.level {
	case arttui.ArtistLevel:
		if v.filter.Active || v.filter.Text != "" {
			reserve = 1
		}
	default:
		reserve = 2
	}
	if v.sortState.Field != SortDefault {
		reserve++
	}
	return reserve
}

// albumsVisibleReserve returns the line reserve for the albums view's visibleRows.
func albumsVisibleReserve(v AlbumsView) int {
	var reserve int
	switch v.level {
	case albtui.AlbumLevel:
		if v.filter.Active || v.filter.Text != "" {
			reserve = 1
		}
	default:
		reserve = 2
	}
	if v.sortState.Field != SortDefault {
		reserve++
	}
	return reserve
}

// sortNotification formats a transient notification string for the current sort state.
func sortNotification(s SortState) string {
	if s.Field == SortDefault {
		return "Sort: default"
	}
	return fmt.Sprintf("Sort: %s %s", SortFieldLabel(s.Field), SortDirIndicator(s.Dir))
}
