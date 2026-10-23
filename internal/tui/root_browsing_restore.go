package tui

import (
	"slices"

	tea "charm.land/bubbletea/v2"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/browse"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	"github.com/sglavoie/muse/internal/tui/filter"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

// Wrap only navigation loads. A late reply is discarded after user input so it
// cannot reopen a collection or move focus after the user has taken control.
type browsingResumeMsg struct{ Loaded tea.Msg }

func (m *Model) cancelBrowsingRestore() {
	if m.browsingRestore == nil {
		return
	}
	if m.browsingRestore.Started {
		switch m.browsingRestore.Session.View {
		case viewInsights:
			if m.insights.Loading {
				m.insightsInvalidate()
			}
		case viewPlaylists:
			if m.playlists.AtListLevel() {
				m.playlists.PendingLoadID = 0
				m.playlists.SmartQuery = nil
			}
		}
	}
	m.browsingRestore = nil
}

func browsingLoad(cmd tea.Cmd) tea.Cmd {
	return func() tea.Msg {
		if cmd == nil {
			return browsingResumeMsg{}
		}
		return browsingResumeMsg{Loaded: cmd()}
	}
}

func (m Model) resumeBrowsing(msg browsingResumeMsg) (tea.Model, tea.Cmd) {
	if m.browsingRestore == nil {
		return m, nil
	}
	// Copy on write: the startup Model and queued commands retain their snapshot.
	state := *m.browsingRestore
	m.browsingRestore = &state
	var extra tea.Cmd
	if msg.Loaded != nil {
		next, cmd := m.Update(msg.Loaded)
		m, extra = next.(Model), cmd
	}
	if !state.Started {
		state.Started = true
		var cmd tea.Cmd
		m, cmd = m.beginBrowsingRestore(state.Session)
		if cmd != nil {
			return m, browsingLoad(cmd)
		}
	}
	next, cmd := m.advanceBrowsingRestore()
	return next, tea.Batch(extra, cmd)
}

func (m Model) advanceBrowsingRestore() (Model, tea.Cmd) {
	for m.browsingRestore != nil {
		state := *m.browsingRestore
		m.browsingRestore = &state
		step := state.Session.Steps[state.Next]
		if step.Level != m.browsingLevel() {
			return m.browsingRestoreFallback(), nil
		}
		m = m.applyBrowsingStep(step)
		if state.Next == len(state.Session.Steps)-1 {
			m.browsingRestore = nil
			return m, nil
		}
		// Never open a neighboring collection when the saved target was deleted.
		if step.Position.Key == "" || !slices.Contains(m.browsingKeys(), step.Position.Key) {
			return m.browsingRestoreFallback(), nil
		}
		before := m
		next, cmd, ok := m.openBrowsingCollection()
		if !ok {
			return m.browsingRestoreFallback(), nil
		}
		m = next.restoreNavigationParents(before).restoreSortPreference()
		state.Next++
		m.browsingRestore = &state
		if cmd != nil {
			return m, browsingLoad(cmd)
		}
	}
	return m, nil
}

func (m Model) browsingRestoreFallback() Model {
	m.browsingRestore = nil
	m.statusBar.Notify("Saved collection unavailable — restored its parent")
	return m
}

func (m Model) applyBrowsingStep(step browsingStep) Model {
	if f := m.browsingFilter(); f != nil {
		*f = filter.State{Text: step.Query, Origin: step.Origin}
	}
	fields, _ := m.activeSortFields()
	if slices.Contains(fields, step.Sort.Field) && (step.Sort.Dir == sorting.SortAsc || step.Sort.Dir == sorting.SortDesc) {
		if state, _, _ := m.sortPreferenceTarget(); state != nil {
			*state = step.Sort
		}
	}
	m = m.recomputePreferredSort()
	if top := slices.Index(m.browsingKeys(), step.TopKey); top >= 0 {
		step.Position.Scroll = top
	}
	m.restoreBrowsingPosition(step.Position)
	return m
}

// Explicit collection guards prevent a malformed/stale session from ever
// replaying Enter on a track, which would enqueue or start music.
func (m Model) openBrowsingCollection() (Model, tea.Cmd, bool) {
	if m.browsingAtTracks() {
		return m, nil, false
	}
	switch m.ActiveView {
	case viewBrowse:
		return m.openBrowsingSubcollection()
	case viewPlaylists:
		return m.handlePlaylistOpen()
	case viewFavorites:
		return m.handleFavoritesOpen()
	case viewTags:
		next, cmd := m.handleTagOpen()
		return next, cmd, true
	case viewMixes:
		next, cmd := m.handleMixOpen()
		return next, cmd, true
	case viewInsights:
		next, cmd := m.handleInsightsOpen()
		return next, cmd, true
	}
	return m, nil, false
}

// Library and Queue already display tracks; Enter must never be replayed there.
func (m Model) browsingAtTracks() bool {
	switch m.ActiveView {
	case viewBrowse:
		return m.browseAtTracks()
	case viewPlaylists:
		return !m.playlists.AtListLevel()
	case viewFavorites:
		return m.favorites.Level == favtui.TrackLevel || (m.favorites.Level == favtui.ListLevel && m.favorites.Category == favtui.CategoryTracks)
	case viewTags:
		return m.tagsView.Level != 0
	case viewMixes:
		return m.mixes.Level != 0
	case viewInsights:
		return m.insights.AtTracks()
	default:
		return true
	}
}

func (m Model) browseAtTracks() bool {
	switch m.browse.Mode {
	case browse.BrowseModeArtists:
		return m.browse.Artists.Level == arttui.TrackLevel
	case browse.BrowseModeAlbums:
		return m.browse.Albums.Level == albtui.TrackLevel
	case browse.BrowseModeGenres:
		return m.browse.Genres.Level == gentui.TrackLevel
	default:
		return true
	}
}

func (m Model) openBrowsingSubcollection() (Model, tea.Cmd, bool) {
	var next tea.Model
	var cmd tea.Cmd
	var ok bool
	switch m.browse.Mode {
	case browse.BrowseModeArtists:
		if m.browse.Artists.Level == arttui.TrackLevel {
			return m, nil, false
		}
		next, cmd, ok = m.handleArtistOpenAction()
	case browse.BrowseModeAlbums:
		if m.browse.Albums.Level == albtui.TrackLevel {
			return m, nil, false
		}
		next, cmd, ok = m.handleAlbumOpenAction()
	case browse.BrowseModeGenres:
		if m.browse.Genres.Level == gentui.TrackLevel {
			return m, nil, false
		}
		next, cmd, ok = m.handleGenreOpenAction()
	default:
		return m, nil, false
	}
	return next.(Model), cmd, ok
}

func (m Model) beginBrowsingRestore(saved browsingSession) (Model, tea.Cmd) {
	m.library = m.library.SwitchMode(saved.LibraryMode)
	m.browse.Mode = saved.BrowseMode
	m.insights.RestorePanel(saved.Panel, saved.Window, saved.Resolution)
	if id := saved.TagFilter; id != nil {
		if _, exists := m.tagNames[*id]; exists {
			m.activeTagFilter = id
			m = m.applyTagFilterToViews()
		} else {
			m.statusBar.Notify("Saved tag filter no longer exists")
		}
	}
	m = m.switchToView(saved.View)
	var cmd tea.Cmd
	switch m.ActiveView {
	case viewMixes:
		cmd = m.loadMixCountsCmd()
	case viewInsights:
		cmd = m.insightsEnterCmd()
	}
	return m, cmd
}
