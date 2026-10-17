package tui

import (
	"maps"

	"github.com/sglavoie/muse/internal/tui/browse"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

// Parent contexts belong to the root, where both synchronous navigation and
// asynchronous collection loads converge. Store row identities, never just an
// index into a filtered list. A changed/deleted parent is clamped on return.
type navigationParent struct {
	filter   filter.State
	sort     sorting.SortState
	position filter.Position
}

type navigationLocation struct {
	view viewID
	mode browse.BrowseMode
}

var navigationLocations = []navigationLocation{
	{viewBrowse, browse.BrowseModeArtists},
	{viewBrowse, browse.BrowseModeAlbums},
	{viewBrowse, browse.BrowseModeGenres},
	{viewFavorites, 0}, {viewTags, 0}, {viewPlaylists, 0},
}

// Inspect each collection, including loads finishing while another view is
// active. Merely switching primary views or Browse modes never saves/restores
// a parent. The temporary active view is only used by shared view accessors.
func (m Model) restoreNavigationParents(before Model) Model {
	active, mode := m.ActiveView, m.browse.Mode
	for _, location := range navigationLocations {
		previous := before
		previous.ActiveView, previous.browse.Mode = location.view, location.mode
		m.ActiveView, m.browse.Mode = location.view, location.mode
		oldLevel, newLevel := previous.navigationLevel(), m.navigationLevel()
		if oldLevel == newLevel {
			continue
		}
		m.navigationParents = maps.Clone(m.navigationParents)
		if m.navigationParents == nil {
			m.navigationParents = make(map[string]navigationParent)
		}
		if newLevel > oldLevel {
			state, cursor, scroll := previous.sortPreferenceTarget()
			m.navigationParents[previous.navigationKey()] = navigationParent{
				filter: *previous.navigationFilter(), sort: *state,
				position: filter.CapturePosition(previous.filterRowKeys(), *cursor, *scroll),
			}
		} else if parent, ok := m.navigationParents[m.navigationKey()]; ok {
			delete(m.navigationParents, m.navigationKey())
			*m.navigationFilter() = parent.filter
			state, _, _ := m.sortPreferenceTarget()
			*state = parent.sort
			m = m.recomputePreferredSort()
			_, cursor, scroll := m.sortPreferenceTarget()
			*cursor, *scroll = parent.position.Restore(m.filterRowKeys(), parent.position.Key)
			m.clampAllScrollOffsets()
		}
	}
	m.ActiveView, m.browse.Mode = active, mode
	return m
}

func (m Model) navigationKey() string {
	if m.ActiveView == viewFavorites && m.favorites.Level == favtui.CategoryLevel {
		return "favorites.categories"
	}
	return m.sortPreferenceKey()
}

func (m Model) navigationLevel() int {
	switch m.ActiveView {
	case viewBrowse:
		switch m.browse.Mode {
		case browse.BrowseModeArtists:
			return int(m.browse.Artists.Level)
		case browse.BrowseModeAlbums:
			return int(m.browse.Albums.Level)
		default:
			return int(m.browse.Genres.Level)
		}
	case viewFavorites:
		return int(m.favorites.Level)
	case viewTags:
		return int(m.tagsView.Level)
	default:
		return int(m.playlists.Level)
	}
}

func (m *Model) navigationFilter() *filter.State {
	switch m.ActiveView {
	case viewBrowse:
		switch m.browse.Mode {
		case browse.BrowseModeArtists:
			return &m.browse.Artists.Filter
		case browse.BrowseModeAlbums:
			return &m.browse.Albums.Filter
		default:
			return &m.browse.Genres.Filter
		}
	case viewFavorites:
		return &m.favorites.Filter
	case viewTags:
		return &m.tagsView.Filter
	default:
		return &m.playlists.Filter
	}
}
