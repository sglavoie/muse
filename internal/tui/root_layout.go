package tui

import (
	"log/slog"

	"github.com/sglavoie/muse/internal/tui/browse"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/layout"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/sorting"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// activeViewContext maps the current activeView to its bindingContext so that
// the registry can check view-local bindings before falling back to global.
// For viewBrowse, the real sub-context (contextArtists/Albums/Genres) is resolved
// based on the active browse mode so that sub-view keybindings work correctly.
func (m Model) activeViewContext() keys.Context {
	if m.ActiveView == viewBrowse {
		switch m.browse.Mode {
		case browse.BrowseModeArtists:
			return keys.ContextArtists
		case browse.BrowseModeAlbums:
			return keys.ContextAlbums
		case browse.BrowseModeGenres:
			return keys.ContextGenres
		}
	}
	ctx := viewToContext(m.ActiveView)
	if ctx == keys.ContextGlobal {
		slog.Warn("activeViewContext: unhandled viewID, falling back to global bindings",
			"viewID", int(m.ActiveView))
	}
	return ctx
}

// resizable is implemented by every primary view that tracks its own
// terminal dimensions.
type resizable interface {
	SetSize(w, h int)
}

// setViewDimensions updates the width and height on all primary views.
// height is reduced by 4 to leave room for the status bar, separator, and
// header rows. When the footer hint strip is enabled it steals one more row.
func (m *Model) setViewDimensions(width, height int) {
	reserve := 4
	if m.showFooterHints {
		reserve = 5
	}
	viewHeight := max(height-reserve, 0)
	for _, view := range []resizable{&m.library, &m.playlists, &m.queue, &m.browse, &m.favorites, &m.tagsView, &m.mixes, &m.insights} {
		view.SetSize(width, viewHeight)
	}
}

// clampAllScrollOffsets adjusts the scroll offset for every view so that the
// cursor remains visible after a terminal resize.
func (m *Model) clampAllScrollOffsets() {
	libReserve := libtui.VisibleReserve(m.library)
	libMaxRows := layout.VisibleRows(m.library.Height, libReserve)
	m.library.ScrollOffset = sorting.AdjustScroll(m.library.Cursor, m.library.ScrollOffset, libMaxRows)

	queueMaxRows := m.queue.VisibleRows()
	m.queue.ScrollOffset = sorting.AdjustScroll(m.queue.Cursor, m.queue.ScrollOffset, queueMaxRows)

	plReserve := playlistsVisibleReserve(m.playlists)
	plMaxRows := layout.VisibleRows(m.playlists.Height, plReserve)
	m.playlists.ScrollOffset = sorting.AdjustScroll(m.playlists.Cursor, m.playlists.ScrollOffset, plMaxRows)

	artReserve := artistsVisibleReserve(m.browse.Artists)
	artMaxRows := layout.VisibleRows(m.browse.Artists.Height, artReserve)
	m.browse.Artists.ScrollOffset = sorting.AdjustScroll(m.browse.Artists.Cursor, m.browse.Artists.ScrollOffset, artMaxRows)

	albReserve := albumsVisibleReserve(m.browse.Albums)
	albMaxRows := layout.VisibleRows(m.browse.Albums.Height, albReserve)
	m.browse.Albums.ScrollOffset = sorting.AdjustScroll(m.browse.Albums.Cursor, m.browse.Albums.ScrollOffset, albMaxRows)

	favReserve := favoritesVisibleReserve(m.favorites)
	favMaxRows := layout.VisibleRows(m.favorites.Height, favReserve)
	m.favorites.ScrollOffset = sorting.AdjustScroll(m.favorites.Cursor, m.favorites.ScrollOffset, favMaxRows)

	genReserve := genresVisibleReserve(m.browse.Genres)
	genMaxRows := layout.VisibleRows(m.browse.Genres.Height, genReserve)
	m.browse.Genres.ScrollOffset = sorting.AdjustScroll(m.browse.Genres.Cursor, m.browse.Genres.ScrollOffset, genMaxRows)

	tagReserve := tagsVisibleReserve(m.tagsView)
	tagMaxRows := layout.VisibleRows(m.tagsView.Height, tagReserve)
	m.tagsView.ScrollOffset = sorting.AdjustScroll(m.tagsView.Cursor, m.tagsView.ScrollOffset, tagMaxRows)

	if m.mixes.Level == 1 {
		mixReserve := libtui.VisibleReserve(m.mixes.TrackView)
		mixMaxRows := layout.VisibleRows(m.mixes.Height, mixReserve)
		m.mixes.TrackView.ScrollOffset = sorting.AdjustScroll(
			m.mixes.TrackView.Cursor, m.mixes.TrackView.ScrollOffset, mixMaxRows)
	}

	if m.insights.AtTracks() {
		insReserve := libtui.VisibleReserve(m.insights.TrackView)
		insMaxRows := layout.VisibleRows(m.insights.Height, insReserve)
		m.insights.TrackView.ScrollOffset = sorting.AdjustScroll(
			m.insights.TrackView.Cursor, m.insights.TrackView.ScrollOffset, insMaxRows)
	} else {
		m.insights.ClampCursor()
	}
}

// tagsVisibleReserve returns the number of header rows to reserve in the tags view.
func tagsVisibleReserve(v tagtui.TagsView) int {
	return drilldownVisibleReserve(
		v.Level == tagtui.TagLevel,
		v.Filter.Active || v.Filter.Text != "",
		v.SortState.Field != sorting.SortDefault,
	)
}
