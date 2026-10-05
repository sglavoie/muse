package tui

import (
	"github.com/sglavoie/muse/internal/tui/browse"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

func (m *Model) sortPreferenceTarget() (*sorting.SortState, *int, *int) {
	switch m.ActiveView {
	case viewLibrary:
		return &m.library.SortState, &m.library.Cursor, &m.library.ScrollOffset
	case viewQueue:
		return &m.queue.SortState, &m.queue.Cursor, &m.queue.ScrollOffset
	case viewPlaylists:
		return &m.playlists.SortState, &m.playlists.Cursor, &m.playlists.ScrollOffset
	case viewBrowse:
		return m.browseSortPreferenceTarget()
	case viewFavorites:
		return &m.favorites.SortState, &m.favorites.Cursor, &m.favorites.ScrollOffset
	case viewTags:
		return &m.tagsView.SortState, &m.tagsView.Cursor, &m.tagsView.ScrollOffset
	case viewMixes:
		return &m.mixes.TrackView.SortState, &m.mixes.TrackView.Cursor, &m.mixes.TrackView.ScrollOffset
	case viewInsights:
		return &m.insights.TrackView.SortState, &m.insights.TrackView.Cursor, &m.insights.TrackView.ScrollOffset
	}
	return nil, nil, nil
}

func (m *Model) browseSortPreferenceTarget() (*sorting.SortState, *int, *int) {
	switch m.browse.Mode {
	case browse.BrowseModeArtists:
		return &m.browse.Artists.SortState, &m.browse.Artists.Cursor, &m.browse.Artists.ScrollOffset
	case browse.BrowseModeAlbums:
		return &m.browse.Albums.SortState, &m.browse.Albums.Cursor, &m.browse.Albums.ScrollOffset
	case browse.BrowseModeGenres:
		return &m.browse.Genres.SortState, &m.browse.Genres.Cursor, &m.browse.Genres.ScrollOffset
	}
	return nil, nil, nil
}

// Rebuild display caches only: restoring a Queue sort never changes playback
// order, nor does restoring a playlist sort write its track positions.
func (m Model) recomputePreferredSort() Model {
	switch m.ActiveView {
	case viewLibrary:
		m.library = m.library.RefreshView()
	case viewQueue:
		m.queue = m.queue.RecomputeDisplayOrder()
	case viewPlaylists:
		m.playlists = m.playlists.RecomputePlaylistSort()
	case viewBrowse:
		m = m.recomputePreferredBrowseSort()
	case viewFavorites:
		if m.favorites.SortState.Field == sorting.SortDefault {
			m = m.rederiveFavoritesData()
		}
		m.favorites = m.favorites.RecomputeFavoritesSort()
	case viewTags:
		m.tagsView = m.tagsView.RecomputeTagsSort()
	case viewMixes:
		m.mixes.TrackView = m.mixes.TrackView.RefreshView()
	case viewInsights:
		m.insights.TrackView = m.insights.TrackView.RefreshView()
	}
	return m
}

func (m Model) recomputePreferredBrowseSort() Model {
	switch m.browse.Mode {
	case browse.BrowseModeArtists:
		if m.browse.Artists.SortState.Field == sorting.SortDefault {
			m = m.rederiveArtistsData()
		}
		m.browse.Artists = m.browse.Artists.RecomputeArtistsSort()
	case browse.BrowseModeAlbums:
		if m.browse.Albums.SortState.Field == sorting.SortDefault {
			m = m.rederiveAlbumsData()
		}
		m.browse.Albums = m.browse.Albums.RecomputeAlbumsSort()
	case browse.BrowseModeGenres:
		if m.browse.Genres.SortState.Field == sorting.SortDefault {
			m = m.rederiveGenresData()
		}
		m.browse.Genres = m.browse.Genres.RecomputeGenresSort()
	}
	return m
}
