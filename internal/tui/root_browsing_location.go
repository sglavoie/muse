package tui

import (
	"fmt"

	"github.com/sglavoie/muse/internal/tui/browse"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	"github.com/sglavoie/muse/internal/tui/filter"
	instui "github.com/sglavoie/muse/internal/tui/insights"
)

func (m Model) browsingLevel() string {
	if m.ActiveView == viewInsights {
		return fmt.Sprintf("insights.%d", m.insights.Level)
	}
	return m.navigationKey()
}

func (m *Model) browsingFilter() *filter.State {
	switch m.ActiveView {
	case viewLibrary:
		return &m.library.Filter
	case viewQueue:
		return &m.queue.Filter
	case viewMixes:
		if m.mixes.Level == 1 {
			return &m.mixes.TrackView.Filter
		}
		return nil
	case viewInsights:
		if m.insights.AtTracks() {
			return &m.insights.TrackView.Filter
		}
		return nil
	default:
		return m.navigationFilter()
	}
}

func (m Model) browsingKeys() []string {
	if m.ActiveView == viewMixes && m.mixes.Level == 0 {
		keys := make([]string, len(m.mixes.Mixes))
		for i, row := range m.mixes.Mixes {
			keys[i] = fmt.Sprintf("mix:%d", row.ID)
		}
		return keys
	}
	if m.ActiveView == viewInsights && !m.insights.AtTracks() {
		return m.insights.NavigationKeys()
	}
	return m.filterRowKeys()
}

func (m Model) browsingPosition() filter.Position {
	if m.ActiveView == viewMixes && m.mixes.Level == 0 {
		return filter.CapturePosition(m.browsingKeys(), m.mixes.SelectedMix, 0)
	}
	if m.ActiveView == viewInsights && !m.insights.AtTracks() {
		return m.insights.NavigationPosition()
	}
	_, cursor, scroll := m.sortPreferenceTarget()
	return filter.CapturePosition(m.browsingKeys(), *cursor, *scroll)
}

func (m *Model) restoreBrowsingPosition(p filter.Position) {
	cursor, scroll := p.Restore(m.browsingKeys(), p.Key)
	switch {
	case m.ActiveView == viewMixes && m.mixes.Level == 0:
		m.mixes.SelectedMix = cursor
	case m.ActiveView == viewInsights && !m.insights.AtTracks():
		m.insights.RestoreNavigationPosition(p)
	default:
		_, c, s := m.sortPreferenceTarget()
		*c, *s = cursor, scroll
		if m.termHeight > 0 {
			m.clampAllScrollOffsets()
		}
	}
}

// Walk back on a value copy to capture the same parent filter, sort and focus
// that Esc would restore, without mutating the live navigation stack.
func (m Model) browsingBack() (Model, bool) {
	if !m.hasBrowsingParent() {
		return m, false
	}
	before := m
	switch m.ActiveView {
	case viewBrowse:
		m = m.browseBack()
	case viewFavorites:
		m.favorites = m.favorites.Back()
	case viewPlaylists:
		m.playlists = m.playlists.BackToList()
	case viewTags:
		m.tagsView = m.tagsView.Back()
	case viewMixes:
		m.mixes.Back()
	case viewInsights:
		m.insights.Back()
	}
	return m.restoreNavigationParents(before), true
}

func (m Model) hasBrowsingParent() bool {
	switch m.ActiveView {
	case viewBrowse:
		return m.navigationLevel() != 0
	case viewFavorites:
		return m.favorites.Level != favtui.CategoryLevel
	case viewPlaylists:
		return !m.playlists.AtListLevel()
	case viewTags:
		return m.tagsView.Level != 0
	case viewMixes:
		return m.mixes.Level != 0
	case viewInsights:
		return m.insights.Level != instui.LevelBriefing
	default:
		return false
	}
}

func (m Model) browseBack() Model {
	switch m.browse.Mode {
	case browse.BrowseModeArtists:
		m.browse.Artists = m.browse.Artists.Back()
	case browse.BrowseModeAlbums:
		m.browse.Albums = m.browse.Albums.Back()
	case browse.BrowseModeGenres:
		m.browse.Genres = m.browse.Genres.Back()
	}
	return m
}
