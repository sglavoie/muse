package tui

import (
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// visualState holds the state for Vim-like visual line selection mode.
type visualState struct {
	active   bool
	view     viewID
	anchor   int
	snapshot selectionSnapshot
}

// selectionSnapshot captures the selection state before visual mode was entered
// so it can be restored on each cursor move to compute the new visual range.
type selectionSnapshot struct {
	hashes    map[string]struct{} // Library, RecentlyAdded, Artists, Albums, Genres
	queueSel  map[int]bool        // Queue: entries index → was selected
	trackIDs  map[int64]struct{}  // Playlists: track row IDs
	favIdxSet map[int]struct{}    // Favorites: selected item indices
}

// activeViewCursor returns the cursor position for the currently active view.
func activeViewCursor(m Model) int {
	switch m.activeView {
	case viewLibrary:
		return m.library.cursor
	case viewRecentlyAdded:
		return m.recentlyAdded.cursor
	case viewHistory:
		return m.history.cursor
	case viewPlaylists:
		return m.playlists.cursor
	case viewQueue:
		return m.queue.cursor
	case viewArtists:
		return m.artists.cursor
	case viewAlbums:
		return m.albums.cursor
	case viewFavorites:
		return m.favorites.cursor
	case viewGenres:
		return m.genres.cursor
	case viewTags:
		return m.tagsView.cursor
	}
	return 0
}

// enterVisualMode snapshots the current selection, sets the anchor to the
// cursor, marks visual mode active, and selects the item at the cursor.
// For hierarchical views it is a no-op when not at track level.
func enterVisualMode(m Model) Model {
	// Level guards: only enter at track/applicable level.
	switch m.activeView {
	case viewArtists:
		if m.artists.level != arttui.TrackLevel {
			return m
		}
	case viewAlbums:
		if m.albums.level != albtui.TrackLevel {
			return m
		}
	case viewGenres:
		if m.genres.level != gentui.TrackLevel {
			return m
		}
	case viewFavorites:
		if m.favorites.level == favtui.CategoryLevel {
			return m
		}
	case viewPlaylists:
		if m.playlists.level != pltui.TrackLevel {
			return m
		}
	case viewTags:
		if m.tagsView.level != tagtui.TrackLevel {
			return m
		}
	}

	cursor := activeViewCursor(m)
	m.visual = visualState{
		active:   true,
		view:     m.activeView,
		anchor:   cursor,
		snapshot: takeSnapshot(m),
	}
	// Select the item at the cursor to start the range.
	m = selectRange(m, cursor, cursor)
	return m
}

// exitVisualMode clears visual mode state. Selections made during visual mode
// are preserved.
func exitVisualMode(m Model) Model {
	m.visual = visualState{}
	return m
}

// updateVisualSelection is called after every cursor move while visual mode is
// active. It restores the pre-visual snapshot, then selects all items in
// [min(anchor, cursor), max(anchor, cursor)].
func updateVisualSelection(m Model) Model {
	if !m.visual.active {
		return m
	}
	// Clamp anchor in case the list shrank (filter change, watch event).
	if n := visualListLen(m); n > 0 && m.visual.anchor >= n {
		m.visual.anchor = n - 1
	}
	cursor := activeViewCursor(m)
	m = restoreSnapshot(m)
	lo, hi := m.visual.anchor, cursor
	if lo > hi {
		lo, hi = hi, lo
	}
	m = selectRange(m, lo, hi)
	return m
}

// visualListLen returns the number of display-visible items for the view that
// visual mode is tracking. Used to clamp the anchor when the list shrinks.
func visualListLen(m Model) int {
	switch m.visual.view {
	case viewLibrary:
		return m.library.trackCount()
	case viewRecentlyAdded:
		return m.recentlyAdded.trackCount()
	case viewHistory:
		return m.history.trackCount()
	case viewQueue:
		return len(m.queue.filteredEntries())
	case viewPlaylists:
		return len(m.playlists.tracks)
	case viewArtists:
		return len(arttui.FilterTracks(m.artists.tracks, m.artists.filter.Text))
	case viewAlbums:
		return len(arttui.FilterTracks(m.albums.tracks, m.albums.filter.Text))
	case viewGenres:
		return len(arttui.FilterTracks(m.genres.tracks, m.genres.filter.Text))
	case viewFavorites:
		switch m.favorites.level {
		case favtui.TrackLevel:
			return len(arttui.FilterTracks(m.favorites.drilldownTracks, m.favorites.filter.Text))
		case favtui.ListLevel:
			switch m.favorites.category {
			case favtui.CategoryArtists:
				return len(arttui.FilterStrings(m.favorites.artists, m.favorites.filter.Text))
			case favtui.CategoryAlbums:
				return len(arttui.FilterStrings(m.favorites.albums, m.favorites.filter.Text))
			case favtui.CategoryTracks:
				return len(arttui.FilterTracks(m.favorites.tracks, m.favorites.filter.Text))
			}
		}
	case viewTags:
		if m.tagsView.level == tagtui.TrackLevel {
			return len(m.tagsView.displayTracks())
		}
	}
	return 0
}

// takeSnapshot clones the current selection state for the active view.
func takeSnapshot(m Model) selectionSnapshot {
	var snap selectionSnapshot
	switch m.activeView {
	case viewLibrary:
		snap.hashes = cloneStringSet(m.library.selectedHashes)
	case viewRecentlyAdded:
		snap.hashes = cloneStringSet(m.recentlyAdded.selectedHashes)
	case viewHistory:
		snap.hashes = cloneStringSet(m.history.selectedHashes)
	case viewQueue:
		for i, e := range m.queue.entries {
			if e.Selected {
				if snap.queueSel == nil {
					snap.queueSel = make(map[int]bool)
				}
				snap.queueSel[i] = true
			}
		}
	case viewPlaylists:
		snap.trackIDs = cloneInt64Set(m.playlists.selectedTrackIDs)
	case viewArtists:
		snap.hashes = cloneStringSet(m.artists.selectedHashes)
	case viewAlbums:
		snap.hashes = cloneStringSet(m.albums.selectedHashes)
	case viewGenres:
		snap.hashes = cloneStringSet(m.genres.selectedHashes)
	case viewFavorites:
		snap.favIdxSet = cloneIntSet(m.favorites.selectedItems)
	case viewTags:
		snap.hashes = cloneStringSet(m.tagsView.selectedHashes)
	}
	return snap
}

// restoreSnapshot resets the active view's selection to the snapshot state.
func restoreSnapshot(m Model) Model {
	switch m.visual.view {
	case viewLibrary:
		m.library.selectedHashes = cloneStringSet(m.visual.snapshot.hashes)
	case viewRecentlyAdded:
		m.recentlyAdded.selectedHashes = cloneStringSet(m.visual.snapshot.hashes)
	case viewHistory:
		m.history.selectedHashes = cloneStringSet(m.visual.snapshot.hashes)
	case viewQueue:
		for i := range m.queue.entries {
			m.queue.entries[i].Selected = m.visual.snapshot.queueSel[i]
		}
	case viewPlaylists:
		m.playlists.selectedTrackIDs = cloneInt64Set(m.visual.snapshot.trackIDs)
	case viewArtists:
		m.artists.selectedHashes = cloneStringSet(m.visual.snapshot.hashes)
	case viewAlbums:
		m.albums.selectedHashes = cloneStringSet(m.visual.snapshot.hashes)
	case viewGenres:
		m.genres.selectedHashes = cloneStringSet(m.visual.snapshot.hashes)
	case viewFavorites:
		m.favorites.selectedItems = cloneIntSet(m.visual.snapshot.favIdxSet)
	case viewTags:
		m.tagsView.selectedHashes = cloneStringSet(m.visual.snapshot.hashes)
	}
	return m
}

// selectRange selects all items in the display range [lo, hi] for the active view.
func selectRange(m Model, lo, hi int) Model {
	switch m.visual.view {
	case viewLibrary:
		m.library = libSelectRange(m.library, lo, hi)
	case viewRecentlyAdded:
		m.recentlyAdded = libSelectRange(m.recentlyAdded, lo, hi)
	case viewHistory:
		m.history = libSelectRange(m.history, lo, hi)
	case viewQueue:
		filtered := m.queue.filteredEntries()
		for i := lo; i <= hi; i++ {
			if i < 0 || i >= len(filtered) {
				continue
			}
			idx := filtered[i].origIdx
			if idx >= 0 && idx < len(m.queue.entries) {
				m.queue.entries[idx].Selected = true
			}
		}
	case viewPlaylists:
		if m.playlists.level == pltui.TrackLevel {
			for i := lo; i <= hi; i++ {
				realIdx := m.playlists.trackIndex(i)
				if realIdx < 0 || realIdx >= len(m.playlists.tracks) {
					continue
				}
				if m.playlists.selectedTrackIDs == nil {
					m.playlists.selectedTrackIDs = make(map[int64]struct{})
				}
				m.playlists.selectedTrackIDs[m.playlists.tracks[realIdx].ID] = struct{}{}
			}
		}
	case viewArtists:
		if m.artists.level == arttui.TrackLevel {
			m.artists.selectedHashes = hashSelectRange(m.artists.selectedHashes, m.artists.tracks, m.artists.filter.Text, lo, hi)
		}
	case viewAlbums:
		if m.albums.level == albtui.TrackLevel {
			m.albums.selectedHashes = hashSelectRange(m.albums.selectedHashes, m.albums.tracks, m.albums.filter.Text, lo, hi)
		}
	case viewGenres:
		if m.genres.level == gentui.TrackLevel {
			m.genres.selectedHashes = hashSelectRange(m.genres.selectedHashes, m.genres.tracks, m.genres.filter.Text, lo, hi)
		}
	case viewFavorites:
		if m.favorites.level != favtui.CategoryLevel {
			for i := lo; i <= hi; i++ {
				if m.favorites.selectedItems == nil {
					m.favorites.selectedItems = make(map[int]struct{})
				}
				m.favorites.selectedItems[i] = struct{}{}
			}
		}
	case viewTags:
		if m.tagsView.level == tagtui.TrackLevel {
			tracks := m.tagsView.displayTracks()
			for i := lo; i <= hi; i++ {
				if i < 0 || i >= len(tracks) {
					continue
				}
				hash := tracks[i].Hash
				if hash == "" {
					continue
				}
				if m.tagsView.selectedHashes == nil {
					m.tagsView.selectedHashes = make(map[string]struct{})
				}
				m.tagsView.selectedHashes[hash] = struct{}{}
			}
		}
	}
	return m
}

// libSelectRange selects tracks at display indices [lo, hi] in a LibraryView.
func libSelectRange(v LibraryView, lo, hi int) LibraryView {
	tracks := v.sortedTracks
	if tracks == nil {
		tracks = v.baseTracks()
	}
	for i := lo; i <= hi; i++ {
		var hash string
		if v.filter.Text != "" && v.filteredMatches != nil {
			if i < 0 || i >= len(v.filteredMatches) {
				continue
			}
			m := v.filteredMatches[i]
			if m.TrackIndex < 0 || m.TrackIndex >= len(tracks) {
				continue
			}
			hash = tracks[m.TrackIndex].Hash
		} else {
			if i < 0 || i >= len(tracks) {
				continue
			}
			hash = tracks[i].Hash
		}
		if hash == "" {
			continue
		}
		if v.selectedHashes == nil {
			v.selectedHashes = make(map[string]struct{})
		}
		v.selectedHashes[hash] = struct{}{}
	}
	return v
}

// hashSelectRange selects track hashes at display indices [lo, hi] from
// filtered tracks, returning the updated selection map.
func hashSelectRange(sel map[string]struct{}, tracks []library.Track, filterText string, lo, hi int) map[string]struct{} {
	filtered := arttui.FilterTracks(tracks, filterText)
	for i := lo; i <= hi; i++ {
		if i < 0 || i >= len(filtered) {
			continue
		}
		hash := filtered[i].Hash
		if hash == "" {
			continue
		}
		if sel == nil {
			sel = make(map[string]struct{})
		}
		sel[hash] = struct{}{}
	}
	return sel
}

// cloneStringSet returns a deep copy of a map[string]struct{}, or nil if empty.
func cloneStringSet(s map[string]struct{}) map[string]struct{} {
	if len(s) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(s))
	for k := range s {
		out[k] = struct{}{}
	}
	return out
}

// cloneInt64Set returns a deep copy of a map[int64]struct{}, or nil if empty.
func cloneInt64Set(s map[int64]struct{}) map[int64]struct{} {
	if len(s) == 0 {
		return nil
	}
	out := make(map[int64]struct{}, len(s))
	for k := range s {
		out[k] = struct{}{}
	}
	return out
}

// cloneIntSet returns a deep copy of a map[int]struct{}, or nil if empty.
func cloneIntSet(s map[int]struct{}) map[int]struct{} {
	if len(s) == 0 {
		return nil
	}
	out := make(map[int]struct{}, len(s))
	for k := range s {
		out[k] = struct{}{}
	}
	return out
}
