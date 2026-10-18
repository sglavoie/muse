package tui

import (
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/browse"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// visualState holds the state for Vim-like visual line selection mode.
type visualState struct {
	active     bool
	view       viewID
	browseMode browse.BrowseMode // only meaningful when view == viewBrowse
	anchor     int
	snapshot   selectionSnapshot
}

// selectionSnapshot captures the selection state before visual mode was entered
// so it can be restored on each cursor move to compute the new visual range.
type selectionSnapshot struct {
	hashes   map[string]struct{} // Library, RecentlyAdded, Artists, Albums, Genres, Insights
	queueSel map[int]bool        // Queue: entries index → was selected
	trackIDs map[int64]struct{}  // Playlists: track row IDs
	favKeys  map[string]struct{} // Favorites: stable item keys
}

// activeViewCursor returns the cursor position for the currently active view.
func activeViewCursor(m Model) int {
	switch m.ActiveView {
	case viewLibrary:
		return m.library.Cursor
	case viewPlaylists:
		return m.playlists.Cursor
	case viewQueue:
		return m.queue.Cursor
	case viewBrowse:
		switch m.browse.Mode {
		case browse.BrowseModeArtists:
			return m.browse.Artists.Cursor
		case browse.BrowseModeAlbums:
			return m.browse.Albums.Cursor
		case browse.BrowseModeGenres:
			return m.browse.Genres.Cursor
		}
	case viewFavorites:
		return m.favorites.Cursor
	case viewTags:
		return m.tagsView.Cursor
	case viewMixes:
		return m.mixes.TrackView.Cursor
	case viewInsights:
		return m.insights.TrackView.Cursor
	}
	return 0
}

// enterVisualMode snapshots the current selection, sets the anchor to the
// cursor, marks visual mode active, and selects the item at the cursor.
// For hierarchical views it is a no-op when not at track level.
func enterVisualMode(m Model) Model {
	// Level guards: only enter at track/applicable level.
	switch m.ActiveView {
	case viewBrowse:
		switch m.browse.Mode {
		case browse.BrowseModeArtists:
			if m.browse.Artists.Level != arttui.TrackLevel {
				return m
			}
		case browse.BrowseModeAlbums:
			if m.browse.Albums.Level != albtui.TrackLevel {
				return m
			}
		case browse.BrowseModeGenres:
			if m.browse.Genres.Level != gentui.TrackLevel {
				return m
			}
		}
	case viewFavorites:
		if m.favorites.Level == favtui.CategoryLevel {
			return m
		}
	case viewPlaylists:
		if m.playlists.Level != pltui.TrackLevel {
			return m
		}
	case viewTags:
		if m.tagsView.Level != tagtui.TrackLevel {
			return m
		}
	case viewInsights:
		// Only the embedded library.View is a range that can be grown. The
		// briefing and the session list are neither, and the view advertises v,
		// so say what is missing rather than swallowing the key.
		if !m.insights.AtTracks() {
			m.statusBar.Notify("Visual selection needs a track list — press enter to drill in")
			return m
		}
	case viewMixes:
		if m.mixes.Level != 1 {
			return m
		}
	}

	cursor := activeViewCursor(m)
	m.visual = visualState{
		active:     true,
		view:       m.ActiveView,
		browseMode: m.browse.Mode,
		anchor:     cursor,
		snapshot:   takeSnapshot(m),
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
		return m.library.TrackCount()
	case viewQueue:
		return len(m.queue.FilteredEntries())
	case viewPlaylists:
		return len(m.playlists.DisplayTracks())
	case viewBrowse:
		switch m.visual.browseMode {
		case browse.BrowseModeArtists:
			return len(arttui.FilterTracks(m.browse.Artists.Tracks, m.browse.Artists.Filter.Text))
		case browse.BrowseModeAlbums:
			return len(arttui.FilterTracks(m.browse.Albums.Tracks, m.browse.Albums.Filter.Text))
		case browse.BrowseModeGenres:
			return len(arttui.FilterTracks(m.browse.Genres.Tracks, m.browse.Genres.Filter.Text))
		}
		return 0
	case viewFavorites:
		switch m.favorites.Level {
		case favtui.TrackLevel:
			return len(arttui.FilterTracks(m.favorites.DrilldownTracks, m.favorites.Filter.Text))
		case favtui.ListLevel:
			switch m.favorites.Category {
			case favtui.CategoryArtists:
				return len(arttui.FilterStrings(m.favorites.Artists, m.favorites.Filter.Text))
			case favtui.CategoryAlbums:
				return len(arttui.FilterStrings(m.favorites.Albums, m.favorites.Filter.Text))
			case favtui.CategoryTracks:
				return len(arttui.FilterTracks(m.favorites.Tracks, m.favorites.Filter.Text))
			}
		}
	case viewTags:
		if m.tagsView.Level == tagtui.TrackLevel {
			return len(m.tagsView.DisplayTracks())
		}
	case viewMixes:
		if m.mixes.Level == 1 {
			return m.mixes.TrackView.TrackCount()
		}
	case viewInsights:
		if m.insights.AtTracks() {
			return m.insights.TrackView.TrackCount()
		}
	}
	return 0
}

// takeSnapshot clones the current selection state for the active view.
func takeSnapshot(m Model) selectionSnapshot {
	var snap selectionSnapshot
	switch m.ActiveView {
	case viewLibrary:
		snap.hashes = cloneStringSet(m.library.SelectedHashes)
	case viewQueue:
		for i, e := range m.queue.Entries {
			if e.Selected {
				if snap.queueSel == nil {
					snap.queueSel = make(map[int]bool)
				}
				snap.queueSel[i] = true
			}
		}
	case viewPlaylists:
		snap.trackIDs = cloneInt64Set(m.playlists.SelectedTrackIDs)
	case viewBrowse:
		switch m.browse.Mode {
		case browse.BrowseModeArtists:
			snap.hashes = cloneStringSet(m.browse.Artists.SelectedHashes)
		case browse.BrowseModeAlbums:
			snap.hashes = cloneStringSet(m.browse.Albums.SelectedHashes)
		case browse.BrowseModeGenres:
			snap.hashes = cloneStringSet(m.browse.Genres.SelectedHashes)
		}
	case viewFavorites:
		snap.favKeys = cloneStringSet(m.favorites.SelectedItems)
	case viewTags:
		snap.hashes = cloneStringSet(m.tagsView.SelectedHashes)
	case viewMixes:
		snap.hashes = cloneStringSet(m.mixes.TrackView.SelectedHashes)
	case viewInsights:
		snap.hashes = cloneStringSet(m.insights.TrackView.SelectedHashes)
	}
	return snap
}

// restoreSnapshot resets the active view's selection to the snapshot state.
func restoreSnapshot(m Model) Model {
	switch m.visual.view {
	case viewLibrary:
		m.library.SelectedHashes = cloneStringSet(m.visual.snapshot.hashes)
	case viewQueue:
		for i := range m.queue.Entries {
			m.queue.Entries[i].Selected = m.visual.snapshot.queueSel[i]
		}
	case viewPlaylists:
		m.playlists.SelectedTrackIDs = cloneInt64Set(m.visual.snapshot.trackIDs)
	case viewBrowse:
		switch m.visual.browseMode {
		case browse.BrowseModeArtists:
			m.browse.Artists.SelectedHashes = cloneStringSet(m.visual.snapshot.hashes)
		case browse.BrowseModeAlbums:
			m.browse.Albums.SelectedHashes = cloneStringSet(m.visual.snapshot.hashes)
		case browse.BrowseModeGenres:
			m.browse.Genres.SelectedHashes = cloneStringSet(m.visual.snapshot.hashes)
		}
	case viewFavorites:
		m.favorites.SelectedItems = cloneStringSet(m.visual.snapshot.favKeys)
	case viewTags:
		m.tagsView.SelectedHashes = cloneStringSet(m.visual.snapshot.hashes)
	case viewMixes:
		m.mixes.TrackView.SelectedHashes = cloneStringSet(m.visual.snapshot.hashes)
	case viewInsights:
		m.insights.TrackView.SelectedHashes = cloneStringSet(m.visual.snapshot.hashes)
	}
	return m
}

// selectRange selects all items in the display range [lo, hi] for the active view.
func selectRange(m Model, lo, hi int) Model {
	switch m.visual.view {
	case viewLibrary:
		m.library = libSelectRange(m.library, lo, hi)
	case viewQueue:
		filtered := m.queue.FilteredEntries()
		for i := lo; i <= hi; i++ {
			if i < 0 || i >= len(filtered) {
				continue
			}
			idx := filtered[i].OrigIdx
			if idx >= 0 && idx < len(m.queue.Entries) {
				m.queue.Entries[idx].Selected = true
			}
		}
	case viewPlaylists:
		if m.playlists.Level == pltui.TrackLevel {
			rows := m.playlists.DisplayTracks()
			for i := lo; i <= hi; i++ {
				if i < 0 || i >= len(rows) {
					continue
				}
				if m.playlists.SelectedTrackIDs == nil {
					m.playlists.SelectedTrackIDs = make(map[int64]struct{})
				}
				m.playlists.SelectedTrackIDs[rows[i].ID] = struct{}{}
			}
		}
	case viewBrowse:
		switch m.visual.browseMode {
		case browse.BrowseModeArtists:
			if m.browse.Artists.Level == arttui.TrackLevel {
				m.browse.Artists.SelectedHashes = hashSelectRange(m.browse.Artists.SelectedHashes, m.browse.Artists.Tracks, m.browse.Artists.Filter.Text, lo, hi)
			}
		case browse.BrowseModeAlbums:
			if m.browse.Albums.Level == albtui.TrackLevel {
				m.browse.Albums.SelectedHashes = hashSelectRange(m.browse.Albums.SelectedHashes, m.browse.Albums.Tracks, m.browse.Albums.Filter.Text, lo, hi)
			}
		case browse.BrowseModeGenres:
			if m.browse.Genres.Level == gentui.TrackLevel {
				m.browse.Genres.SelectedHashes = hashSelectRange(m.browse.Genres.SelectedHashes, m.browse.Genres.Tracks, m.browse.Genres.Filter.Text, lo, hi)
			}
		}
	case viewFavorites:
		rows := m.favorites.SelectionKeys()
		m.favorites.SelectedItems = cloneStringSet(m.favorites.SelectedItems)
		for i := max(0, lo); i <= hi && i < len(rows); i++ {
			if rows[i] == "" {
				continue
			}
			if m.favorites.SelectedItems == nil {
				m.favorites.SelectedItems = make(map[string]struct{})
			}
			m.favorites.SelectedItems[rows[i]] = struct{}{}
		}
	case viewMixes:
		if m.mixes.Level == 1 {
			m.mixes.TrackView = libSelectRange(m.mixes.TrackView, lo, hi)
		}
	case viewInsights:
		if m.insights.AtTracks() {
			m.insights.TrackView = libSelectRange(m.insights.TrackView, lo, hi)
		}
	case viewTags:
		if m.tagsView.Level == tagtui.TrackLevel {
			tracks := m.tagsView.DisplayTracks()
			for i := lo; i <= hi; i++ {
				if i < 0 || i >= len(tracks) {
					continue
				}
				hash := tracks[i].Hash
				if hash == "" {
					continue
				}
				if m.tagsView.SelectedHashes == nil {
					m.tagsView.SelectedHashes = make(map[string]struct{})
				}
				m.tagsView.SelectedHashes[hash] = struct{}{}
			}
		}
	}
	return m
}

// libSelectRange selects tracks at display indices [lo, hi] in a library.View.
func libSelectRange(v libtui.View, lo, hi int) libtui.View {
	tracks := v.SortedTracks
	if tracks == nil {
		tracks = v.BaseTracks()
	}
	for i := lo; i <= hi; i++ {
		var hash string
		if v.Filter.Text != "" && v.FilteredMatches != nil {
			if i < 0 || i >= len(v.FilteredMatches) {
				continue
			}
			m := v.FilteredMatches[i]
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
		if v.SelectedHashes == nil {
			v.SelectedHashes = make(map[string]struct{})
		}
		v.SelectedHashes[hash] = struct{}{}
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
